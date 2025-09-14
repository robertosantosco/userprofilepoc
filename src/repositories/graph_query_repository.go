package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"
	"userprofilepoc/src/domain"
	"userprofilepoc/src/domain/entities"
	"userprofilepoc/src/infra/postgres"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type GraphQueryRepository struct {
	pool *pgxpool.Pool
}

func NewGraphQueryRepository(pool *pgxpool.Pool) *GraphQueryRepository {
	return &GraphQueryRepository{pool: pool}
}

type FindCondition struct {
	Field    string      // O nome da coluna no banco (ex: "id", "properties").
	Operator string      // O operador a ser usado (ex: "=", "@>").
	Value    interface{} // O valor a ser usado como argumento na query.
}

func (gqr *GraphQueryRepository) QueryTree(ctx context.Context, condition FindCondition, depthLimit int, startTime time.Time) ([]domain.GraphNode, []entities.TemporalProperty, error) {
	baseGraphNodeQuery := `
		WITH RECURSIVE entity_graph (entity_id, parent_id, relationship_type, depth) AS (
			SELECT 
				id, 
				NULL::BIGINT, 
				NULL::TEXT, 
				0 
			FROM 
				entities 
			WHERE 
				%s %s $1
			
			UNION ALL
			
			SELECT 
				e.right_entity_id, 
				e.left_entity_id, 
				e.relationship_type, 
				eg.depth + 1
			FROM 
				edges e 
			JOIN 
				entity_graph eg ON e.left_entity_id = eg.entity_id
			WHERE 
				eg.depth < $2
		),
		entities_relation AS (
			SELECT
				entity_id,
				JSONB_AGG(DISTINCT jsonb_build_object('parent_id', parent_id, 'type', relationship_type)) FILTER (WHERE parent_id IS NOT NULL) AS parents_info
			FROM 
				entity_graph
			GROUP 
				BY entity_id
		)
		SELECT 
			e.id, 
			e.type, 
			e.reference, 
			e.properties, 
			e.created_at, 
			e.updated_at, 
			er.parents_info
		FROM 
			entities e 
		JOIN 
			entities_relation er ON e.id = er.entity_id;
	`
	queryValue := condition.Value
	queryField := condition.Field
	if condition.Operator == "@>" {
		valueJson, err := postgres.BuildSearchJSON(condition.Field, condition.Value)
		if err != nil {
			return nil, nil, fmt.Errorf("GraphQueryRepository.QueryTree - failed to build search JSON: %w", err)
		}

		queryField = "properties"
		queryValue = valueJson
	}

	graphNodeQuery := fmt.Sprintf(baseGraphNodeQuery, queryField, condition.Operator)

	grapthRows, err := gqr.pool.Query(ctx, graphNodeQuery, queryValue, depthLimit)
	if err != nil {
		return nil, nil, fmt.Errorf("GraphQueryRepository.QueryTree - graph query failed: %w", err)
	}

	defer grapthRows.Close()

	var allNodes []domain.GraphNode
	entityIDs := make([]int64, 0)

	for grapthRows.Next() {
		var node domain.GraphNode
		var parentsInfoRaw json.RawMessage

		if err := grapthRows.Scan(&node.ID, &node.Type, &node.Reference, &node.Properties, &node.CreatedAt, &node.UpdatedAt, &parentsInfoRaw); err != nil {
			return nil, nil, fmt.Errorf("GraphQueryRepository.QueryTree - failed to scan entity data: %w", err)
		}

		// Desserializa o JSONB para a struct ParentsInfo
		if len(parentsInfoRaw) > 0 && string(parentsInfoRaw) != "null" {
			if err := json.Unmarshal(parentsInfoRaw, &node.ParentsInfo); err != nil {
				log.Printf("GraphQueryRepository.QueryTree - [WARN] repository could not unmarshal parents_info for entity %d: %v", node.ID, err)
			}
		}

		allNodes = append(allNodes, node)
		entityIDs = append(entityIDs, node.ID)
	}

	if err := grapthRows.Err(); err != nil {
		return nil, nil, fmt.Errorf("GraphQueryRepository.QueryTree - error iterating graph rows: %w", err)
	}

	if len(allNodes) == 0 {
		return nil, nil, fmt.Errorf("GraphQueryRepository.QueryTree - entity not found: %w", domain.ErrEntityNotFound)
	}

	temporalPropertiesQuery := `
		SELECT 
			entity_id, 
			key, value, 
			period, 
			granularity, 
			start_ts, 
			created_at, 
			updated_at
		FROM 
			temporal_properties
		WHERE 
			entity_id = ANY($1) AND start_ts >= $2;
	`
	temporalRows, err := gqr.pool.Query(ctx, temporalPropertiesQuery, entityIDs, startTime)
	if err != nil {
		return nil, nil, fmt.Errorf("GraphQueryRepository.QueryTree - temporal query failed: %w", err)
	}

	defer temporalRows.Close()

	var temporalProps []entities.TemporalProperty
	for temporalRows.Next() {
		var prop entities.TemporalProperty
		var pgRange pgtype.Range[pgtype.Timestamptz]

		if err := temporalRows.Scan(&prop.EntityID, &prop.Key, &prop.Value, &pgRange, &prop.Granularity, &prop.StartTS, &prop.CreatedAt, &prop.UpdatedAt); err != nil {
			return nil, nil, fmt.Errorf("GraphQueryRepository.QueryTree - failed to scan temporal property: %w", err)
		}

		if pgRange.Lower.Valid {
			prop.PeriodStart = pgRange.Lower.Time
		}
		if pgRange.Upper.Valid {
			endDate := pgRange.Upper.Time
			prop.PeriodEnd = &endDate
		}

		temporalProps = append(temporalProps, prop)
	}

	return allNodes, temporalProps, nil
}
