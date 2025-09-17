package repositories

import (
	"context"
	"fmt"
	"log"
	"time"
	"userprofilepoc/src/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TemporalWriteRepository struct {
	pool                  *pgxpool.Pool
	cachedGraphRepository *CachedGraphRepository
}

func NewTemporalWriteRepository(pool *pgxpool.Pool, cachedGraphRepository *CachedGraphRepository) *TemporalWriteRepository {
	return &TemporalWriteRepository{pool: pool, cachedGraphRepository: cachedGraphRepository}
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
	minReferenceDate := syncTemporalPropertyRequest.DataPoints[0].ReferenceDate
	maxReferenceDate := syncTemporalPropertyRequest.DataPoints[0].ReferenceDate
	for i, dp := range syncTemporalPropertyRequest.DataPoints {

		referenceMonth := time.Date(dp.ReferenceDate.Year(), dp.ReferenceDate.Month(), 1, 0, 0, 0, 0, dp.ReferenceDate.Location())

		rows[i] = []interface{}{
			dp.EntityReference, dp.EntityType, dp.Key, dp.Value,
			dp.Granularity, dp.ReferenceDate, referenceMonth,
		}

		if dp.ReferenceDate.Before(minReferenceDate) {
			minReferenceDate = dp.ReferenceDate
		}

		if dp.ReferenceDate.After(maxReferenceDate) {
			maxReferenceDate = dp.ReferenceDate
		}
	}

	// Criar a tabela temporária
	tempTableQuery := `CREATE TEMP TABLE temp_datapoints (
		entity_reference TEXT, 
		entity_type TEXT, 
		key TEXT, 
		value JSONB,
		granularity TEXT,
		reference_date TIMESTAMPTZ, 
		reference_month DATE
	) ON COMMIT DROP;`
	if _, err := tx.Exec(ctx, tempTableQuery); err != nil {
		return fmt.Errorf("failed to create temp datapoints table: %w", err)
	}

	//  Copia os dados para a tabela temporária.
	_, err = tx.CopyFrom(
		ctx,
		pgx.Identifier{"temp_datapoints"},
		[]string{"entity_reference", "entity_type", "key", "value", "granularity", "reference_date", "reference_month"},
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
					td.granularity,
					td.reference_date,
					td.reference_month,
					CASE 
						WHEN td.granularity = 'instant' THEN ei.id || ':' || td.key || ':' || td.granularity || ':' || to_char(td.reference_date, 'YYYY-MM-DD"T"HH24:MI:SS.MS')
						WHEN td.granularity = 'day'     THEN ei.id || ':' || td.key || ':' || td.granularity || ':' || to_char(td.reference_date, 'YYYY-MM-DD')
						WHEN td.granularity = 'week'    THEN ei.id || ':' || td.key || ':' || td.granularity || ':' || to_char(td.reference_date, 'IYYY-"W"IW')
						WHEN td.granularity = 'month'   THEN ei.id || ':' || td.key || ':' || td.granularity || ':' || to_char(td.reference_month, 'YYYY-MM')
						WHEN td.granularity = 'quarter' THEN ei.id || ':' || td.key || ':' || td.granularity || ':' || to_char(td.reference_date, 'YYYY') || '-Q' || ((extract(month from td.reference_date)-1)/3 + 1)
						WHEN td.granularity = 'year'    THEN ei.id || ':' || td.key || ':' || td.granularity || ':' || to_char(td.reference_date, 'YYYY')
						END AS idempotency_key
				FROM
					temp_datapoints td
				JOIN
					entity_ids ei ON td.entity_reference = ei.reference
			),

			inserted_temporal AS (
				INSERT INTO temporal_properties (
					entity_id,
					key,
					value,
					granularity,
					reference_date,
					reference_month,
					idempotency_key
				)
				SELECT
					entity_id,
					key,
					value,
					granularity,
					reference_date,
					reference_month,
					idempotency_key
				FROM
					temporal_data_to_upsert
				ON CONFLICT ON CONSTRAINT 
					tp_uniq_idempotency_key_reference_month
				DO UPDATE SET
					value = excluded.value,
					updated_at = NOW()
			)
			
			SELECT DISTINCT id FROM entity_ids;
	`
	rowss, err := tx.Query(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to execute temporal upsert query: %w", err)
	}

	var affectedIDs []int64
	for rowss.Next() {
		var id int64
		if err := rowss.Scan(&id); err != nil {
			return fmt.Errorf("failed to scan entity ID: %w", err)
		}
		affectedIDs = append(affectedIDs, id)
	}

	// Invalidar cache em background
	go func() {
		if invalidateErr := r.cachedGraphRepository.InvalidateByEntityIDs(context.Background(), affectedIDs); invalidateErr != nil {
			log.Printf("Failed to invalidate cache: %v", invalidateErr)
		}
	}()

	return tx.Commit(ctx)
}
