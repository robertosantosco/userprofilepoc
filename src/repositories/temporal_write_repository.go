package repositories

import (
	"context"
	"fmt"
	"userprofilepoc/src/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TemporalWriteRepository struct {
	pool *pgxpool.Pool
}

func NewTemporalWriteRepository(pool *pgxpool.Pool) *TemporalWriteRepository {
	return &TemporalWriteRepository{pool: pool}
}

func (r *TemporalWriteRepository) UpsertDataPoints(ctx context.Context, syncTemporalPropertyRequest domain.SyncTemporalPropertyRequest) error {
	if len(syncTemporalPropertyRequest.DataPoints) == 0 {
		return nil
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Prepara os dados para a tabela temporária.
	rows := make([][]interface{}, len(syncTemporalPropertyRequest.DataPoints))
	for i, dp := range syncTemporalPropertyRequest.DataPoints {
		rows[i] = []interface{}{
			dp.EntityReference, dp.EntityType, dp.Key, dp.Value,
			dp.PeriodStart, dp.PeriodEnd, dp.Granularity,
		}
	}

	// Criar a tabela temporária
	tempTableQuery := `CREATE TEMP TABLE temp_datapoints (
		entity_reference TEXT, 
		entity_type TEXT, 
		key TEXT, 
		value JSONB,
		period_start TIMESTAMPTZ, 
		period_end TIMESTAMPTZ, 
		granularity TEXT
	) ON COMMIT DROP;`
	if _, err := tx.Exec(ctx, tempTableQuery); err != nil {
		return fmt.Errorf("failed to create temp datapoints table: %w", err)
	}

	//  Copia os dados para a tabela temporária.
	_, err = tx.CopyFrom(
		ctx,
		pgx.Identifier{"temp_datapoints"},
		[]string{"entity_reference", "entity_type", "key", "value", "period_start", "period_end", "granularity"},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return fmt.Errorf("failed to copy datapoints to temp table: %w", err)
	}

	query := `
		WITH
			-- CTE 1: Faz o "upsert" das entidades e retorna os IDs tanto
			entity_ids AS (
				INSERT INTO 
					entities (type, reference)
				SELECT DISTINCT 
					entity_type, entity_reference
				FROM 
					temp_datapoints
				ON CONFLICT (type, reference)
				-- "Truque": fazemos um update que não altera nada (Ñ gera escrita), apenas para ter acesso aos dados
				DO UPDATE SET
					type = excluded.type -- Atualização inócua, pois o tipo já é o mesmo.
				RETURNING
					id, reference
			),
			-- CTE 2: Prepara os dados temporais com os IDs internos
			temporal_data_to_upsert AS (
				SELECT
					ei.id AS entity_id,
					td.key,
					td.value,
					tstzrange(td.period_start, td.period_end, '[]') AS period,
					td.granularity,
					td.period_start AS start_ts
				FROM 
					temp_datapoints td
				JOIN 
					entity_ids ei ON td.entity_reference = ei.reference
			)
			-- Faz o "upsert" na tabela de propriedades temporais.
			INSERT INTO 
				temporal_properties (entity_id, key, value, period, granularity, start_ts)
			SELECT 
				entity_id, key, value, period, granularity, start_ts
			FROM 
				temporal_data_to_upsert
			-- ON CONFLICT (entity_id, key, period)
			-- WHERE (period && period) -- Esta condição é um pouco estranha, mas é como se diz "conflito de sobreposição"
			-- DO UPDATE SET
			-- 	value = excluded.value,
			-- 	updated_at = NOW();
	`

	if _, err := tx.Exec(ctx, query); err != nil {
		return fmt.Errorf("failed to execute temporal upsert query: %w", err)
	}

	return tx.Commit(ctx)
}
