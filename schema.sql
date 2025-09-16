-- =====================================================================
--      SCRIPT COMPLETO DE GRAFO + TEMPORAL COM PG_PARTMAN NO RDS
--                  (Usando pg_cron como agendador)
-- =====================================================================

-- 1) HABILITAÇÃO DAS EXTENSÕES
-- ---------------------------------------------------------------------
-- Garante que as extensões suportadas pelo RDS estejam ativas.

CREATE EXTENSION IF NOT EXISTS btree_gist;  -- Dependência para a constraint de exclusão
CREATE EXTENSION IF NOT EXISTS pg_partman; -- Gerenciador de partições
CREATE EXTENSION IF NOT EXISTS pg_cron;    -- Agendador de tarefas

-- 2) TABELAS CENTRAIS DO GRAFO (NÃO PARTICIONADAS)
-- ---------------------------------------------------------------------

-- Tabela de "Nós" do grafo. Representa qualquer entidade.
CREATE TABLE IF NOT EXISTS entities (
  id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  type        text NOT NULL,
  reference   text NOT NULL,
  properties  jsonb,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  
  -- Garante que a combinação de tipo e referência externa seja única.
  CONSTRAINT uq_entities_ref_type_pair UNIQUE (type, reference)
);

-- Index for convenient reference lookups
CREATE INDEX IF NOT EXISTS idx_entities_reference ON entities(reference);
-- Índice GIN para permitir buscas eficientes dentro do JSONB de propriedades.
CREATE INDEX IF NOT EXISTS idx_entities_properties_gin ON entities USING GIN(properties);

-- Tabela de "Arestas" do grafo. Define os relacionamentos.
CREATE TABLE IF NOT EXISTS edges (
  id                   BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  left_entity_id       BIGINT NOT NULL REFERENCES entities(id),
  right_entity_id      BIGINT NOT NULL REFERENCES entities(id),
  relationship_type    text NOT NULL,
  metadata             jsonb,
  created_at           timestamptz NOT NULL DEFAULT now(),
  updated_at           timestamptz NOT NULL DEFAULT now(),
  
  -- Garante que um relacionamento do mesmo tipo entre duas entidades seja único.
  CONSTRAINT uq_edges_relationship UNIQUE (left_entity_id, right_entity_id, relationship_type)
);

-- Índices para otimizar a navegação no grafo.
CREATE INDEX IF NOT EXISTS idx_edges_left_id            ON edges(left_entity_id);
CREATE INDEX IF NOT EXISTS idx_edges_right_id           ON edges(right_entity_id);
CREATE INDEX IF NOT EXISTS idx_edges_relationship       ON edges(relationship_type);
CREATE INDEX IF NOT EXISTS idx_edges_metadata           ON edges USING GIN(metadata);

-- 3) TABELA TEMPORAL PARTICIONADA
-- ---------------------------------------------------------------------

-- Tabela PAI: Armazena atributos que mudam com o tempo.
CREATE TABLE IF NOT EXISTS temporal_properties (
  entity_id    BIGINT NOT NULL REFERENCES entities(id),
  key          text NOT NULL,
  value        jsonb NOT NULL,
  period       tstzrange NOT NULL,
  granularity  text NOT NULL CHECK (granularity IN ('instant', 'day', 'week', 'month', 'quarter', 'year')),
  start_ts     timestamp NOT NULL,
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now(),
  
  -- Constraint básica para garantir que o período não seja vazio
  CONSTRAINT tp_period_not_empty CHECK (NOT isempty(period))
) PARTITION BY RANGE (start_ts);

-- adicionado indexes na tabela pai
-- Indexes for temporal table
CREATE INDEX IF NOT EXISTS tp_period_gist ON temporal_properties USING GIST (period);
CREATE INDEX IF NOT EXISTS tp_lookup_btree ON temporal_properties (entity_id, key, start_ts);
CREATE INDEX IF NOT EXISTS tp_value_gin ON temporal_properties USING GIN (value);

-- Tabela TEMPLATE: Serve como modelo para as novas partições.
CREATE TABLE IF NOT EXISTS temporal_properties_template (LIKE temporal_properties INCLUDING ALL);

ALTER TABLE temporal_properties_template
  ADD CONSTRAINT tp_no_overlap_per_partition
  EXCLUDE USING gist (
    entity_id   WITH =,
    key         WITH =,
    period      WITH &&
  );

-- 4) CONFIGURAÇÃO DO PG_PARTMAN
-- ---------------------------------------------------------------------

-- Cria a configuração de particionamento para a tabela pai.
SELECT public.create_parent(
  p_parent_table      := 'public.temporal_properties',
  p_control           := 'start_ts',
  p_type              := 'range',
  p_interval          := '1 month',
  p_premake               := 36,  -- Cria partições para os próximos 36 meses
  p_start_partition   := to_char(date_trunc('week', now() - '2 years'::interval), 'YYYY-MM-DD HH24:MI:SS'),
  p_template_table    := 'public.temporal_properties_template'
);

-- Configure retention: 2 years for all data
UPDATE 
  public.part_config
SET 
  retention = '2 years', 
  infinite_time_partitions = TRUE
WHERE 
  parent_table = 'public.temporal_properties';


-- 5) LÓGICA DE AUTOMAÇÃO (ABORDAGEM COM FUNÇÃO WRAPPER)
-- ---------------------------------------------------------------------

-- 5.1 -- Função que busca partições sem a constraint de exclusão e a aplica.
CREATE OR REPLACE FUNCTION apply_exclusion_constraint_to_partitions()
RETURNS void LANGUAGE plpgsql AS $$
DECLARE
    partition_record record;
BEGIN
    RAISE NOTICE 'Verificando partições de temporal_properties que precisam da constraint de exclusão...';

    FOR partition_record IN
        SELECT
            child.relname AS partition_name,
            nmsp.nspname AS partition_schema
        FROM pg_inherits
        JOIN pg_class parent ON pg_inherits.inhparent = parent.oid
        JOIN pg_class child ON pg_inherits.inhrelid = child.oid
        JOIN pg_namespace nmsp ON child.relnamespace = nmsp.oid
        WHERE parent.relname = 'temporal_properties'
          AND NOT EXISTS (
              SELECT 1
              FROM pg_constraint
              WHERE conrelid = child.oid AND contype = 'x' -- 'x' é o tipo para EXCLUDE constraint
          )
    LOOP
        RAISE NOTICE 'Aplicando constraint na partição %.%', partition_record.partition_schema, partition_record.partition_name;
        
        EXECUTE format(
            'ALTER TABLE %I.%I ADD CONSTRAINT %I_no_overlap '
            'EXCLUDE USING gist (entity_id WITH =, key WITH =, period WITH &&)',
            partition_record.partition_schema,
            partition_record.partition_name,
            partition_record.partition_name
        );
    END LOOP;

    RAISE NOTICE 'Verificação de constraints concluída.';
END;
$$;

-- 5.2 -- Função "Wrapper" que executa a manutenção completa em dois passos.
-- É ESTA FUNÇÃO que será agendada pelo pg_cron.
CREATE OR REPLACE FUNCTION run_full_partition_maintenance()
RETURNS void LANGUAGE plpgsql AS $$
BEGIN
    RAISE NOTICE 'Iniciando job de manutenção completa...';

    -- PASSO 1: Executa a manutenção do pg_partman para criar/remover partições.
    RAISE NOTICE 'Passo 1: Executando public.run_maintenance()...';
    PERFORM public.run_maintenance(p_parent_table := 'public.temporal_properties');
    RAISE NOTICE 'Passo 1 concluído.';

    -- PASSO 2: Executa a função para garantir que as novas partições tenham a constraint.
    RAISE NOTICE 'Passo 2: Executando public.apply_exclusion_constraint_to_partitions()...';
    PERFORM public.apply_exclusion_constraint_to_partitions();
    RAISE NOTICE 'Passo 2 concluído.';

    RAISE NOTICE 'Job de manutenção completa finalizado com sucesso.';
END;
$$;


-- 6) AGENDAMENTO COM PG_CRON
-- ---------------------------------------------------------------------
-- Agenda a execução da nossa função wrapper para rodar todo dia às 3 da manhã.

SELECT cron.schedule(
    'full-partman-maintenance', -- Nome do job
    '0 3 * * *',                -- Expressão Cron: "Às 03:00 todos os dias"
    $$SELECT public.run_full_partition_maintenance();$$ -- Comando a ser executado
);


-- 7) INICIALIZAÇÃO E PERMISSÕES
-- ---------------------------------------------------------------------

-- Executa a manutenção completa uma vez para criar as partições iniciais
-- e já aplicar as constraints imediatamente.
SELECT public.run_full_partition_maintenance();


-- Concede permissões necessárias.
GRANT ALL ON ALL TABLES IN SCHEMA public TO postgres;
GRANT ALL ON ALL SEQUENCES IN SCHEMA public TO postgres;
GRANT ALL ON ALL FUNCTIONS IN SCHEMA public TO postgres;
