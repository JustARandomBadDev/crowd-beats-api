package usecase

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

func isUniqueConstraint(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == constraint
}
