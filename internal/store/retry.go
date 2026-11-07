package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// A local database can go away for a few seconds in the middle of a long run:
// on a small laptop Docker may be stopped under memory pressure and started
// again. Reads are safe to repeat, so instead of losing an hour of work to a
// 20 second outage, we wait and try again.
const (
	readAttempts = 12
	readPause    = 5 * time.Second // 12 x 5 s covers a Docker restart
)

// retryRead calls fn until it works, it fails for a reason waiting will not fix,
// or the attempts run out. It returns the last error in that case.
func retryRead[T any](ctx context.Context, pause time.Duration, fn func() (T, error)) (T, error) {
	var zero T
	var err error

	for attempt := 1; attempt <= readAttempts; attempt++ {
		var result T
		if result, err = fn(); err == nil || !isConnectionProblem(err) {
			return result, err
		}

		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(pause):
		}
	}
	return zero, err
}

// isConnectionProblem tells a database that is unreachable from a query that is
// wrong. Only the first is worth waiting for.
func isConnectionProblem(err error) bool {
	var serverError *pgconn.PgError
	if errors.As(err, &serverError) {
		return false // the server answered, so repeating the same query changes nothing
	}
	return !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
}
