package test_seeder

import (
	"context"
	"fmt"
	"userprofilepoc/src/domain/entities"
)

// InsertEntity inserts an entity into the database for testing
func (ts TestSeeder) InsertEntity(ctx context.Context, entity *entities.Entity) {
	query := `
		INSERT INTO entities (type, reference, properties, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`

	err := ts.pool.QueryRow(ctx, query,
		entity.Type,
		entity.Reference,
		entity.Properties,
		entity.CreatedAt,
		entity.UpdatedAt,
	).Scan(&entity.ID)

	if err != nil {
		panic(fmt.Sprintf("Seeder.InsertEntity failed: %v", err))
	}
}

// InsertEdge inserts an edge (relationship) into the database for testing
func (ts TestSeeder) InsertEdge(ctx context.Context, edge *entities.Edge) {
	query := `
		INSERT INTO edges (left_entity_id, right_entity_id, relationship_type, metadata, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`

	err := ts.pool.QueryRow(ctx, query,
		edge.LeftEntityID,
		edge.RightEntityID,
		edge.RelationshipType,
		edge.Metadata,
		edge.CreatedAt,
		edge.UpdatedAt,
	).Scan(&edge.ID)

	if err != nil {
		panic(fmt.Sprintf("Seeder.InsertEdge failed: %v", err))
	}
}

// InsertTemporalProperty inserts a temporal property into the database for testing
func (ts TestSeeder) InsertTemporalProperty(ctx context.Context, property entities.TemporalProperty) {
	referenceMonth := property.ReferenceDate.Truncate(24 * 60 * 60 * 1000000000) // Remove horas, minutos, segundos
	referenceMonth = referenceMonth.AddDate(0, 0, -referenceMonth.Day()+1)       // Primeiro dia do mês

	query := `
		INSERT INTO temporal_properties (entity_id, key, value, idempotency_key, granularity, reference_date, reference_month, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	_, err := ts.pool.Exec(ctx, query,
		property.EntityID,
		property.Key,
		property.Value,
		property.IdempotencyKey,
		property.Granularity,
		property.ReferenceDate,
		referenceMonth,
		property.CreatedAt,
		property.UpdatedAt,
	)
	if err != nil {
		panic(fmt.Sprintf("Seeder.InsertTemporalProperty failed: %v", err))
	}
}
