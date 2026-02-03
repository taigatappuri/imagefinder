package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"
	pgxvector "github.com/pgvector/pgvector-go/pgx"
)

func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	pgxvector.Register(config.ConnConfig.TypeMap)
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

func EnsureSchema(ctx context.Context, pool *pgxpool.Pool, embeddingDim int) error {
	schema := fmt.Sprintf(`
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS users (
	id UUID PRIMARY KEY,
	google_user_id TEXT NOT NULL UNIQUE,
	email TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS oauth_tokens (
	user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
	access_token TEXT NOT NULL,
	refresh_token TEXT,
	expires_at TIMESTAMPTZ NOT NULL,
	scopes TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS jobs (
	id UUID PRIMARY KEY,
	user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	type TEXT NOT NULL,
	status TEXT NOT NULL,
	progress INT NOT NULL,
	started_at TIMESTAMPTZ,
	finished_at TIMESTAMPTZ,
	error_message TEXT,
	created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS photos (
	id UUID PRIMARY KEY,
	user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	google_media_id TEXT NOT NULL,
	base_url TEXT NOT NULL,
	created_time TIMESTAMPTZ,
	location TEXT,
	people_count INT,
	caption TEXT,
	embedding VECTOR(%d),
	indexed_at TIMESTAMPTZ NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS photos_google_media_id_uidx ON photos (google_media_id);
CREATE INDEX IF NOT EXISTS photos_user_id_idx ON photos (user_id);
CREATE INDEX IF NOT EXISTS photos_embedding_idx ON photos USING ivfflat (embedding vector_cosine_ops);
`, embeddingDim)

	_, err := pool.Exec(ctx, schema)
	return err
}

func VectorFromSlice(values []float32) pgvector.Vector {
	return pgvector.NewVector(values)
}
