-- =====================================================================
--      SCRIPT COMPLETO DE GRAFO + TEMPORAL COM PG_PARTMAN NO RDS
-- =====================================================================

-- 1) HABILITAÇÃO DAS EXTENSÕES
-- ---------------------------------------------------------------------
-- Garante que as extensões suportadas pelo RDS estejam ativas.

CREATE EXTENSION IF NOT EXISTS btree_gist;  -- Dependência para a constraint de exclusão
CREATE EXTENSION IF NOT EXISTS pg_partman; -- Gerenciador de partições

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
  entity_id         BIGINT NOT NULL REFERENCES entities(id),
  key               text NOT NULL,
  value             jsonb NOT NULL,
  idempotency_key   text NOT NULL,
  granularity       text NOT NULL CHECK (granularity IN ('instant', 'day', 'week', 'month', 'quarter', 'year')),
  reference_date    timestamptz NOT NULL,
  reference_month   date  NOT NULL,
  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now(),
  
  -- Constraint básica para garantir que o reference_month seja o primeiro dia do mês.
  CONSTRAINT tp_reference_month_is_first_day CHECK (EXTRACT(DAY FROM reference_month) = 1),
  CONSTRAINT tp_uniq_idempotency_key_reference_month UNIQUE (idempotency_key, reference_month)
) PARTITION BY RANGE (reference_month);

-- adicionado indexes na tabela pai
CREATE INDEX IF NOT EXISTS tp_value_gin ON temporal_properties USING GIN (value);

-- Tabela TEMPLATE: Serve como modelo para as novas partições.
CREATE TABLE IF NOT EXISTS temporal_properties_template (LIKE temporal_properties INCLUDING ALL);

-- 4) CONFIGURAÇÃO DO PG_PARTMAN
-- ---------------------------------------------------------------------

-- Cria a configuração de particionamento para a tabela pai.
SELECT public.create_parent(
  p_parent_table      := 'public.temporal_properties',
  p_control           := 'reference_month',
  p_type              := 'range',
  p_interval          := '1 month',
  p_premake               := 2,
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

SELECT public.run_maintenance(p_parent_table := 'public.temporal_properties');

-- Concede permissões necessárias.
GRANT ALL ON ALL TABLES IN SCHEMA public TO postgres;
GRANT ALL ON ALL SEQUENCES IN SCHEMA public TO postgres;
GRANT ALL ON ALL FUNCTIONS IN SCHEMA public TO postgres;
