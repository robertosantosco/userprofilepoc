package repositories

import (
	"context"
	"fmt"
	"log"
	"userprofilepoc/src/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type GraphWriteRepository struct {
	writePool             *pgxpool.Pool
	cachedGraphRepository *CachedGraphRepository
}

func NewGraphWriteRepository(writePool *pgxpool.Pool, cachedGraphRepository *CachedGraphRepository) *GraphWriteRepository {
	return &GraphWriteRepository{writePool: writePool, cachedGraphRepository: cachedGraphRepository}
}

func (r *GraphWriteRepository) SyncGraph(ctx context.Context, request domain.SyncGraphRequest) error {
	// Preparar os dados para a tabela temporária unificada.
	rows := make([][]interface{}, 0, len(request.Entities)+len(request.Relationships))

	for _, entity := range request.Entities {
		rows = append(rows, []interface{}{entity.Type, entity.Reference, entity.Properties, nil, nil, nil})
	}

	for _, rel := range request.Relationships {
		rows = append(rows, []interface{}{nil, nil, nil, rel.SourceReference, rel.TargetReference, rel.RelationshipType})
	}

	if len(rows) == 0 {
		return nil
	}

	tx, err := r.writePool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Cria a tabela temporária.
	tempTableQuery := `CREATE TEMP TABLE temp_sync_data (
		entity_type TEXT, entity_reference TEXT, entity_properties JSONB,
		source_reference TEXT, target_reference TEXT, relationship_type TEXT
	) ON COMMIT DROP;`
	_, err = tx.Exec(ctx, tempTableQuery)
	if err != nil {
		return fmt.Errorf("failed to create unified temp table: %w", err)
	}

	// copia os dados para a tabela temporaria.
	_, err = tx.CopyFrom(
		ctx,
		pgx.Identifier{"temp_sync_data"},
		[]string{"entity_type", "entity_reference", "entity_properties", "source_reference", "target_reference", "relationship_type"},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return fmt.Errorf("failed to copy unified data to temp table: %w", err)
	}

	// Executar a única query com CTEs que faz todo o trabalho.
	query := `
		WITH
		-- CTE 1: Upsert das entidades a partir dos dados na tabela temporária.
		upserted_entities AS (
			INSERT INTO 
				entities (type, reference, properties)
			SELECT DISTINCT 
				entity_type, entity_reference, entity_properties
			FROM 
				temp_sync_data
			WHERE 
				entity_reference IS NOT NULL
			ON CONFLICT (type, reference) DO UPDATE SET
				properties = COALESCE(entities.properties, '{}'::jsonb) || excluded.properties,
				updated_at = NOW()
			WHERE
				COALESCE(entities.properties, '{}'::jsonb) || excluded.properties IS DISTINCT FROM entities.properties
			RETURNING 
				id, 
				reference
		),
		-- CTE 2: Coleta todas as referências do payload (entidades e relacionamentos).
		all_references AS (
			SELECT entity_reference AS ref FROM temp_sync_data WHERE entity_reference IS NOT NULL
			UNION
			SELECT source_reference AS ref FROM temp_sync_data WHERE source_reference IS NOT NULL
			UNION
			SELECT target_reference AS ref FROM temp_sync_data WHERE target_reference IS NOT NULL
		),
		-- CTE 3: Busca os IDs de todas as entidades envolvidas, combinando os recem inseridos/atualizados.
		entity_ids AS (
			SELECT id, reference FROM upserted_entities
			UNION
			SELECT e.id, e.reference FROM entities e JOIN all_references ar ON e.reference = ar.ref
		),
		-- CTE 4: Prepara os dados das arestas, trocando as referências de negócio pelos IDs internos.
		edges_to_create AS (
			SELECT
				src_id.id AS left_entity_id,
				tgt_id.id AS right_entity_id,
				tsd.relationship_type
			FROM 
				temp_sync_data tsd
			JOIN 
				entity_ids src_id ON tsd.source_reference = src_id.reference
			JOIN 
				entity_ids tgt_id ON tsd.target_reference = tgt_id.reference
			WHERE 
				tsd.relationship_type IS NOT NULL
		),

		inserted_edges AS (
			INSERT INTO 
				edges (left_entity_id, right_entity_id, relationship_type)
			SELECT 
				left_entity_id, right_entity_id, relationship_type 
			FROM 
				edges_to_create
			ON CONFLICT (left_entity_id, right_entity_id) DO UPDATE SET
				relationship_type = excluded.relationship_type,
				updated_at = NOW()
			WHERE edges.relationship_type IS DISTINCT FROM excluded.relationship_type
		)
		
		SELECT DISTINCT id FROM entity_ids;
	`

	rowss, err := tx.Query(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to execute unified sync mega-query: %w", err)
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
