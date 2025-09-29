package test_seeder

import (
	"context"
	"userprofilepoc/src/domain/entities"
)

// SelectEntityByID retrieves an entity by its ID for verification
func (ts TestSeeder) SelectEntityByID(ctx context.Context, id int64) (entities.Entity, error) {
	query := `SELECT id, type, reference, properties, created_at, updated_at
              FROM entities WHERE id = $1`

	var entity entities.Entity
	err := ts.pool.QueryRow(ctx, query, id).Scan(
		&entity.ID,
		&entity.Type,
		&entity.Reference,
		&entity.Properties,
		&entity.CreatedAt,
		&entity.UpdatedAt,
	)

	return entity, err
}

// SelectEdgesByEntityID retrieves all edges where the entity is involved
func (ts TestSeeder) SelectEdgesByEntityID(ctx context.Context, entityID int64) ([]entities.Edge, error) {
	query := `SELECT id, left_entity_id, right_entity_id, relationship_type, metadata, created_at, updated_at
              FROM edges
              WHERE left_entity_id = $1 OR right_entity_id = $1
              ORDER BY id`

	rows, err := ts.pool.Query(ctx, query, entityID)
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

// SelectTemporalPropertiesByEntityID retrieves all temporal properties for an entity
func (ts TestSeeder) SelectTemporalPropertiesByEntityID(ctx context.Context, entityID int64) ([]entities.TemporalProperty, error) {
	query := `SELECT entity_id, key, value, idempotency_key, granularity, reference_date, created_at, updated_at
              FROM temporal_properties
              WHERE entity_id = $1
              ORDER BY reference_date DESC`

	rows, err := ts.pool.Query(ctx, query, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var properties []entities.TemporalProperty
	for rows.Next() {
		var property entities.TemporalProperty
		err := rows.Scan(
			&property.EntityID,
			&property.Key,
			&property.Value,
			&property.IdempotencyKey,
			&property.Granularity,
			&property.ReferenceDate,
			&property.CreatedAt,
			&property.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		properties = append(properties, property)
	}

	return properties, rows.Err()
}
