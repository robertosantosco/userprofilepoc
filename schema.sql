-- =========================================================
-- Minimal graph + temporal model, managed by pg_partman
-- PostgreSQL 16
-- =========================================================

-- 1) Extensions
CREATE EXTENSION IF NOT EXISTS btree_gist;         
CREATE EXTENSION IF NOT EXISTS pg_partman;

-- 2) Core tables (non-partitioned)

-- Tabela de "Nós" do grafo. 
-- Representa qualquer entidade.
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

-- Tabela de "Arestas" do grafo. 
-- Define os relacionamentos entre as entidades.
CREATE TABLE IF NOT EXISTS edges (
  id                   int GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  left_entity_id       int NOT NULL REFERENCES entities(id),
  right_entity_id      int NOT NULL REFERENCES entities(id),
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

-- 3) Single temporal properties table with bi-weekly partitioning

-- Tabela PAI
-- Armazena atributos que mudam com o tempo e cujo histórico é importante.
CREATE TABLE IF NOT EXISTS temporal_properties (
  entity_id    int NOT NULL REFERENCES entities(id),
  key          text NOT NULL,
  value        jsonb NOT NULL,
  period       tstzrange NOT NULL,
  granularity  text NOT NULL CHECK (granularity IN ('instant', 'day', 'week', 'month', 'quarter', 'year')),
  start_ts     timestamp NOT NULL,
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now(),
  
  CONSTRAINT tp_period_not_empty CHECK (NOT isempty(period))
) PARTITION BY RANGE (start_ts);

-- adicionado indexes na tabela pai
-- Indexes for temporal table
CREATE INDEX IF NOT EXISTS tp_period_gist ON temporal_properties USING GIST (period);
CREATE INDEX IF NOT EXISTS tp_lookup_btree ON temporal_properties (entity_id, key, start_ts);
CREATE INDEX IF NOT EXISTS tp_value_gin ON temporal_properties USING GIN (value);

-- Template for partitions with exclusion constraint
-- Essa tabela serve como modelo para as partições criadas pelo pg_partman
CREATE TABLE IF NOT EXISTS temporal_properties_template
(LIKE temporal_properties INCLUDING ALL);

-- Exclusion constraint to prevent overlapping periods for the same entity and key
ALTER TABLE temporal_properties_template
  ADD CONSTRAINT tp_no_overlap_per_partition
  EXCLUDE USING gist (
    entity_id   WITH =,
    key         WITH =,
    period      WITH &&
  );

-- Setup pg_partman for bi-weekly (14-day) partitions
SELECT public.create_parent(
  p_parent_table      := 'public.temporal_properties',
  p_control           := 'start_ts',
  p_type              := 'range',
  p_interval          := '1 month',
  p_premake               := 2,
  p_start_partition   := to_char(date_trunc('week', now() - '2 years'::interval), 'YYYY-MM-DD HH24:MI:SS'),
  p_template_table    := 'public.temporal_properties_template'
);

-- Configure retention: 2 years for all data
UPDATE public.part_config
  SET retention = '2 years', infinite_time_partitions = TRUE
WHERE parent_table = 'public.temporal_properties';

-- 3) Initialize partitions
SELECT public.run_maintenance(p_parent_table := 'public.temporal_properties');

-- Grant necessary permissions
GRANT ALL ON ALL TABLES IN SCHEMA public TO postgres;
GRANT ALL ON ALL SEQUENCES IN SCHEMA public TO postgres;
GRANT ALL ON ALL FUNCTIONS IN SCHEMA public TO postgres;