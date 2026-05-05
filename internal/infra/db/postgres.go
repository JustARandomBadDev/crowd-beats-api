package db

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"crowdbeats/internal/domain/queue"
	"crowdbeats/internal/domain/room"
	"crowdbeats/internal/domain/session"
	"crowdbeats/internal/domain/spotify"
	"crowdbeats/internal/domain/track"
	"crowdbeats/internal/domain/vote"
	"crowdbeats/internal/infra/db/repositories"
	"crowdbeats/internal/usecase"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
	set  *repositories.Set
}

func NewPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	return pgxpool.New(ctx, databaseURL)
}

func RunMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	migrationPath, err := resolveMigrationPath("001_init.sql")
	if err != nil {
		return err
	}
	bytes, err := os.ReadFile(migrationPath)
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx, string(bytes))
	return err
}

func resolveMigrationPath(file string) (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := wd
	for {
		candidate := filepath.Join(dir, "migrations", file)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("migration file %s not found from %s", file, wd)
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{
		pool: pool,
		set:  repositories.NewSet(pool),
	}
}

func (s *Store) Pool() *pgxpool.Pool {
	return s.pool
}

func (s *Store) Rooms() room.Repository {
	return s.set.Rooms()
}

func (s *Store) Sessions() session.Repository {
	return s.set.Sessions()
}

func (s *Store) Spotify() spotify.Repository {
	return s.set.Spotify()
}

func (s *Store) Tracks() track.Repository {
	return s.set.Tracks()
}

func (s *Store) Queue() queue.Repository {
	return s.set.Queue()
}

func (s *Store) Votes() vote.Repository {
	return s.set.Votes()
}

func (s *Store) Events() usecase.EventRepository {
	return s.set.Events()
}

func (s *Store) Run(ctx context.Context, fn func(repos usecase.RepositorySet) error) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	lockedSet := repositories.NewSet(tx)
	if err := fn(lockedSet); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}
