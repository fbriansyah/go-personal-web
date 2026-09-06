package content

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// The errors a caller can act on. Everything else that comes back from this
// package means something is broken rather than something was refused.
//
// The schema is where the rules live — "its shape is enforced here rather than
// trusted to whatever writes the row" — so this package's job is not to repeat
// them in Go but to translate their enforcement into the domain's words. These
// are what ErrNotFound turned out to be the first of.
var (
	// ErrNotFound is returned when no Post or Page matches, including when one
	// exists but is not visible to the Audience that asked.
	ErrNotFound = errors.New("content: not found")

	// ErrSlugTaken means another Post or Page already answers to that Slug.
	ErrSlugTaken = errors.New("content: slug already taken")

	// ErrSlugInvalid means the Slug is not a shape a URL could carry.
	ErrSlugInvalid = errors.New("content: slug must be lowercase words joined by hyphens")

	// ErrTitleRequired means the title was blank or only whitespace.
	ErrTitleRequired = errors.New("content: title is required")
)

// PostgreSQL error codes, named so the mapping below reads as intent.
const (
	pgUniqueViolation = "23505"
	pgCheckViolation  = "23514"
)

// wrap turns the database's enforcement into this package's vocabulary and
// gives every other failure the slug or operation it happened on. Pop and
// driver errors never escape this package (ADR-0004).
func wrap(err error, format string, args ...any) error {
	what := fmt.Sprintf(format, args...)

	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%s: %w", what, ErrNotFound)
	}
	if domain := domainError(err); domain != nil {
		return fmt.Errorf("%s: %w", what, domain)
	}
	return fmt.Errorf("content: %s: %w", what, err)
}

// domainError reports which rule a constraint violation broke, or nil if the
// error is not one.
//
// Constraints are matched by suffix rather than by full name so that the same
// mapping serves posts and pages, whose constraints differ only in the table
// they are named after.
func domainError(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return nil
	}
	switch {
	case pgErr.Code == pgUniqueViolation && strings.HasSuffix(pgErr.ConstraintName, "_slug_key"):
		return ErrSlugTaken
	case pgErr.Code == pgCheckViolation && strings.HasSuffix(pgErr.ConstraintName, "_slug_format"):
		return ErrSlugInvalid
	case pgErr.Code == pgCheckViolation && strings.HasSuffix(pgErr.ConstraintName, "_title_present"):
		return ErrTitleRequired
	}
	return nil
}
