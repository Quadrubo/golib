package postgres

import (
	"database/sql"
	"errors"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
)

func IsNoRows(err error) bool { return errors.Is(err, sql.ErrNoRows) }

func IsUniqueViolation(err error) bool { return hasCode(err, pgerrcode.UniqueViolation) }

// IsUniqueViolationOn reports the violation of one index, which keeps a caller
// answering for a duplicate id from also answering for the next index the table
// gains.
func IsUniqueViolationOn(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}

	return pgErr.Code == pgerrcode.UniqueViolation && pgErr.ConstraintName == constraint
}

func IsForeignKeyViolation(err error) bool { return hasCode(err, pgerrcode.ForeignKeyViolation) }

func hasCode(err error, code string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}

	return pgErr.Code == code
}
