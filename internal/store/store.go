// Package store keeps chunks and their vectors in Postgres (pgvector).
package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"

	"github.com/piovani/RAGscope/internal/domain"
)

// Every experiment writes into its own "collection", so different chunking
// strategies and embedding models can live side by side in the same table.
//
// The vector column has no fixed size on purpose: models like bge-m3 and
// e5 produce different sizes. There is no ANN index either, so search is
// exact. That is slower on huge corpora but fine for a few thousand chunks,
// and it keeps recall numbers free of index approximation.
const createTable = `
CREATE TABLE IF NOT EXISTS chunks (
    collection  text   NOT NULL,
    id          text   NOT NULL,
    document_id text   NOT NULL,
    position    int    NOT NULL,
    content     text   NOT NULL,
    metadata    jsonb  NOT NULL,
    embedding   vector NOT NULL,
    PRIMARY KEY (collection, id)
)`

type Store struct {
	pool *pgxpool.Pool
}

// Open connects to Postgres and makes sure the table exists.
func Open(ctx context.Context, url string) (*Store, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("connect to postgres: %w", err)
	}

	s := &Store{pool: pool}
	if err := s.migrate(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() { s.pool.Close() }

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS vector"); err != nil {
		return fmt.Errorf("enable pgvector: %w", err)
	}
	if _, err := s.pool.Exec(ctx, createTable); err != nil {
		return fmt.Errorf("create chunks table: %w", err)
	}
	return nil
}

// Replace swaps everything stored in a collection for the given chunks. It runs
// in one transaction, so a failed ingest never leaves a half-empty collection.
func (s *Store) Replace(ctx context.Context, collection string, chunks []domain.Chunk, vectors [][]float32) error {
	if len(chunks) != len(vectors) {
		return fmt.Errorf("got %d chunks but %d vectors", len(chunks), len(vectors))
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) // does nothing after a successful Commit

	if _, err := tx.Exec(ctx, "DELETE FROM chunks WHERE collection = $1", collection); err != nil {
		return err
	}

	batch := &pgx.Batch{}
	for i, c := range chunks {
		metadata := c.Metadata
		if metadata == nil {
			metadata = map[string]string{}
		}
		batch.Queue(
			`INSERT INTO chunks (collection, id, document_id, position, content, metadata, embedding)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			collection, c.ID, c.DocumentID, c.Index, c.Text, metadata, pgvector.NewVector(vectors[i]),
		)
	}

	results := tx.SendBatch(ctx, batch)
	for range chunks {
		if _, err := results.Exec(); err != nil {
			results.Close()
			return fmt.Errorf("insert chunk: %w", err)
		}
	}
	if err := results.Close(); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// Search returns the k chunks closest to the vector, best first. The score is
// cosine similarity (1 means the same direction).
func (s *Store) Search(ctx context.Context, collection string, vector []float32, k int) ([]domain.ScoredChunk, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, document_id, position, content, metadata, 1 - (embedding <=> $2::vector)
		FROM chunks
		WHERE collection = $1
		ORDER BY embedding <=> $2::vector
		LIMIT $3`,
		collection, pgvector.NewVector(vector), k,
	)
	if err != nil {
		return nil, fmt.Errorf("search chunks: %w", err)
	}

	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.ScoredChunk, error) {
		var sc domain.ScoredChunk
		err := row.Scan(&sc.ID, &sc.DocumentID, &sc.Index, &sc.Text, &sc.Metadata, &sc.Score)
		return sc, err
	})
}
