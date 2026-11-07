// Package postgres opens the PostgreSQL connection pool and classifies driver
// errors for the repository adapters of every bounded context.
package postgres

import (
	"context"
	"errors"
	"net"
	"net/url"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Config struct {
	Host     string
	Port     string
	Username string
	Password string
	DBName   string
	SSLMode  string
}

// DSN returns a postgres:// URL with every component escaped, so passwords
// containing spaces, quotes or URL-reserved characters survive intact.
func (c Config) DSN() string {
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(c.Username, c.Password),
		Host:     net.JoinHostPort(c.Host, c.Port),
		Path:     "/" + c.DBName,
		RawQuery: url.Values{"sslmode": {c.SSLMode}}.Encode(),
	}
	return u.String()
}

func Open(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	return Connect(ctx, cfg.DSN())
}

// Connect creates a pool and pings it, so bad settings fail at startup.
func Connect(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return pool, nil
}

// SQLSTATE codes, see https://www.postgresql.org/docs/current/errcodes-appendix.html.
const (
	uniqueViolation     = "23505"
	foreignKeyViolation = "23503"
)

func IsUniqueViolation(err error) bool { return hasCode(err, uniqueViolation) }

func IsForeignKeyViolation(err error) bool { return hasCode(err, foreignKeyViolation) }

func hasCode(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}
