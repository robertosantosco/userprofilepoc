package test_seeder

import (
	"context"
	"userprofilepoc/src/domain/entities"
)

func (ts TestSeeder) SelectEntitiesByReferences(ctx context.Context, references []string) ([]entities.Entity, error) {
	query := `SELECT id, type, reference, properties, created_at, updated_at
			  FROM entities WHERE reference = ANY($1)`

	rows, err := ts.pool.Query(ctx, query, references)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entitiesList []entities.Entity
	for rows.Next() {
		var entity entities.Entity
		err := rows.Scan(
			&entity.ID,
			&entity.Type,
			&entity.Reference,
			&entity.Properties,
			&entity.CreatedAt,
			&entity.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		entitiesList = append(entitiesList, entity)
	}

	return entitiesList, rows.Err()
}

// SelectEdgesByEntityID retrieves all edges where the entity is involved
func (ts TestSeeder) SelectEdgesByEntityReferences(ctx context.Context, entityReferences []string) ([]entities.Edge, error) {
	query := `SELECT DISTINCT e.id, e.left_entity_id, e.right_entity_id, e.relationship_type, e.metadata, e.created_at, e.updated_at
			  FROM edges e
			  JOIN entities ent ON e.left_entity_id = ent.id OR e.right_entity_id = ent.id
			  WHERE ent.reference = ANY($1)
			  ORDER BY e.id`

	rows, err := ts.pool.Query(ctx, query, entityReferences)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var edges []entities.Edge
	for rows.Next() {
		var edge entities.Edge
		err := rows.Scan(
			&edge.ID,
			&edge.LeftEntityID,
			&edge.RightEntityID,
			&edge.RelationshipType,
			&edge.Metadata,
			&edge.CreatedAt,
			&edge.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		edges = append(edges, edge)
	}

	return edges, rows.Err()
}

// SelectTemporalPropertiesByEntityReferences retrieves distinct temporal properties for entities based on their references
func (ts TestSeeder) SelectTemporalPropertiesByEntityReferences(ctx context.Context, entityReferences []string) ([]entities.TemporalProperty, error) {
	query := `SELECT DISTINCT td.entity_id, td.key, td.value, td.idempotency_key, td.granularity, td.reference_date, td.created_at, td.updated_at
			  FROM temporal_properties td
			  JOIN entities ent ON td.entity_id = ent.id
			  WHERE ent.reference = ANY($1)
			  ORDER BY td.entity_id`

	rows, err := ts.pool.Query(ctx, query, entityReferences)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var temporalProperties []entities.TemporalProperty
	for rows.Next() {
		var temporalProperty entities.TemporalProperty
		err := rows.Scan(
			&temporalProperty.EntityID,
			&temporalProperty.Key,
			&temporalProperty.Value,
			&temporalProperty.IdempotencyKey,
			&temporalProperty.Granularity,
			&temporalProperty.ReferenceDate,
			&temporalProperty.CreatedAt,
			&temporalProperty.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		temporalProperties = append(temporalProperties, temporalProperty)
	}

	return temporalProperties, rows.Err()
}
