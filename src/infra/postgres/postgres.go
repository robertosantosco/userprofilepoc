package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgtype"
	"github.com/jackc/pgx"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func NewPostgresClient(host string, port string, dbname string, username string, password string, maxConnections int) (*pgxpool.Pool, error) {
	dbConfig := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", username, password, host, port, dbname)

	config, err := pgxpool.ParseConfig(dbConfig)
	if err != nil {
		fmt.Printf("failed to parse postgres config: %s\n", err.Error())
		return nil, err
	}

	config.MaxConns = int32(maxConnections) //nolint:all
	config.MinConns = 1

	// Idle timeout - economiza recursos
	config.MaxConnIdleTime = 5 * time.Minute

	// Lifetime das conexões - evita problemas de timeout do PostgreSQL
	config.MaxConnLifetime = 30 * time.Minute

	// Health check interval
	config.HealthCheckPeriod = 1 * time.Minute

	// Configurações de performance do driver
	config.ConnConfig.RuntimeParams = map[string]string{
		"timezone":                            "UTC", // Define o fuso horário para UTC
		"statement_timeout":                   "30s", // Tempo máximo para execução de uma query
		"lock_timeout":                        "10s", // Tempo máximo para aguardar um lock
		"idle_in_transaction_session_timeout": "60s", // Tempo máximo que uma transação pode ficar ociosa
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect postgres: %w", err)
	}

	return pool, nil
}

func NewNullUUID(u *uuid.UUID) pgtype.Text {
	if u == nil {
		return pgtype.Text{Status: pgtype.Null}
	}
	return pgtype.Text{
		String: u.String(),
		Status: pgtype.Present,
	}
}

func NewNullString(s *string) pgtype.Text {
	if s == nil || len(*s) == 0 {
		return pgtype.Text{Status: pgtype.Null}
	}
	return pgtype.Text{
		String: *s,
		Status: pgtype.Present,
	}
}

func NewNullTime(t *time.Time) pgtype.Timestamptz {
	if t == nil || t.IsZero() {
		return pgtype.Timestamptz{Status: pgtype.Null}
	}
	return pgtype.Timestamptz{
		Time:   *t,
		Status: pgtype.Present,
	}
}

func NewNullInt(i *int) pgtype.Int8 {
	if i == nil || *i == 0 {
		return pgtype.Int8{Status: pgtype.Null}
	}
	return pgtype.Int8{
		Int:    int64(*i),
		Status: pgtype.Present,
	}
}

func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		// Verifica se o código é de violação de chave única
		if pgErr.Code == "23505" { // Usando a constante pgconn.ErrCodeUniqueViolation
			return true
		}
	}

	return false
}

func IsNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows)
}

// Ela constrói um payload JSON para ser usado com o operador @> do PostgreSQL.
// Gera algo como {"document": {"value": "123.456.789-00"}}
// Essa estrutura é important para usarmos o index GIN em consultas JSONB.
func BuildSearchJSON(path string, value interface{}) (string, error) {
	keys := strings.Split(path, ".")
	jsonMap := map[string]interface{}{keys[len(keys)-1]: value}

	for i := len(keys) - 2; i >= 0; i-- {
		jsonMap = map[string]interface{}{keys[i]: jsonMap}
	}

	bytes, err := json.Marshal(jsonMap)

	if err != nil {
		return "", err
	}

	return string(bytes), nil
}
