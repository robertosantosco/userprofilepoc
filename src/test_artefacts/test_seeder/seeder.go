package test_seeder

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type TestSeeder struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) TestSeeder {
	return TestSeeder{pool: pool}
}

func (ts TestSeeder) TruncateTables(ctx context.Context) {
	tables := []string{
		"temporal_properties",
		"edges",
		"entities",
	}

	for _, table := range tables {
		_, err := ts.pool.Exec(ctx, fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE", table))
		if err != nil {
			panic(fmt.Sprintf("Failed to truncate %s: %v", table, err))
		}
	}
}
