package postgres

import "github.com/jackc/pgx/v5/pgxpool"

type ReadWriteClient struct {
	readPool  *pgxpool.Pool
	writePool *pgxpool.Pool
}

func NewReadWriteClient(
	readHost string,
	writeHost string,
	readPort string,
	writePort string,
	dbname string,
	username string,
	password string,
	maxConnections int,
) (*ReadWriteClient, error) {

	readPool, err := NewPostgresClient(readHost, readPort, dbname, username, password, maxConnections)
	if err != nil {
		return nil, err
	}

	writePool, err := NewPostgresClient(writeHost, writePort, dbname, username, password, maxConnections)
	if err != nil {
		return nil, err
	}

	return &ReadWriteClient{
		readPool:  readPool,
		writePool: writePool,
	}, nil
}

func (rwc *ReadWriteClient) GetReadPool() *pgxpool.Pool {
	return rwc.readPool
}

func (rwc *ReadWriteClient) GetWritePool() *pgxpool.Pool {
	return rwc.writePool
}
