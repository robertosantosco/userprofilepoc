package repositories

import (
	"context"
	"fmt"
	"userprofilepoc/src/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type GraphWriteRepository struct {
	pool *pgxpool.Pool
}

func NewGraphWriteRepository(pool *pgxpool.Pool) *GraphWriteRepository {
	return &GraphWriteRepository{pool: pool}
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

	tx, err := r.pool.Begin(ctx)
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
			INSERT INTO entities 
				(type, reference, properties)
			SELECT DISTINCT 
				entity_type, entity_reference, entity_properties
			FROM 
				temp_sync_data
			WHERE 
				entity_reference IS NOT NULL
			ON CONFLICT (type, reference) DO UPDATE SET
				properties = entities.properties || excluded.properties,
				updated_at = NOW()
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
		)
		-- Query Final: Insere as arestas.
		INSERT INTO 
			edges (left_entity_id, right_entity_id, relationship_type)
		SELECT 
			left_entity_id, right_entity_id, relationship_type 
		FROM 
			edges_to_create
		ON CONFLICT 
			(left_entity_id, right_entity_id, relationship_type) DO NOTHING;
	`

	if _, err := tx.Exec(ctx, query); err != nil {
		return fmt.Errorf("failed to execute unified sync mega-query: %w", err)
	}

	return tx.Commit(ctx)
}
