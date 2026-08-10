package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/hakunamatata0606/unified_service_scheduler/internal/database/sqlc"
)

// Store owns the database connection pool and exposes the generated sqlc queries.
type Store struct {
	pool *pgxpool.Pool
	*db.Queries
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{
		pool:    pool,
		Queries: db.New(pool),
	}
}

// ExecTx commits when fn succeeds and rolls back when fn returns an error.
func (s *Store) ExecTx(ctx context.Context, fn func(*db.Queries) error) error {
	return s.ExecTxWithOptions(ctx, pgx.TxOptions{}, fn)
}

// ExecTxWithOptions allows booking operations to select an isolation level.
func (s *Store) ExecTxWithOptions(
	ctx context.Context,
	options pgx.TxOptions,
	fn func(*db.Queries) error,
) error {
	tx, err := s.pool.BeginTx(ctx, options)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	txQueries := s.Queries.WithTx(tx)
	if err := fn(txQueries); err != nil {
		rollbackErr := tx.Rollback(ctx)
		if rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			return errors.Join(err, fmt.Errorf("rollback transaction: %w", rollbackErr))
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}
