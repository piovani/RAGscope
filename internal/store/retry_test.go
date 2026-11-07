package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestRetryReadWaitsForTheDatabaseToComeBack(t *testing.T) {
	calls := 0
	got, err := retryRead(context.Background(), time.Millisecond, func() (string, error) {
		calls++
		if calls < 3 {
			return "", errors.New("connection refused") // the database is restarting
		}
		return "ok", nil
	})

	if err != nil || got != "ok" {
		t.Fatalf("got %q, %v; want ok and no error", got, err)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

func TestRetryReadDoesNotRepeatAWrongQuery(t *testing.T) {
	calls := 0
	_, err := retryRead(context.Background(), time.Millisecond, func() (int, error) {
		calls++
		// The server answered with a SQL error, wrapped like our code wraps it.
		return 0, fmt.Errorf("search chunks: %w", &pgconn.PgError{Code: "42703", Message: "column does not exist"})
	})

	if err == nil || calls != 1 {
		t.Errorf("calls = %d, err = %v; want one call and an error", calls, err)
	}
}

func TestRetryReadGivesUpAfterTheLastAttempt(t *testing.T) {
	calls := 0
	_, err := retryRead(context.Background(), time.Millisecond, func() (int, error) {
		calls++
		return 0, errors.New("connection refused")
	})

	if err == nil || calls != readAttempts {
		t.Errorf("calls = %d, err = %v; want %d calls and the last error", calls, err, readAttempts)
	}
}

func TestRetryReadStopsWhenTheContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0

	_, err := retryRead(ctx, time.Hour, func() (int, error) {
		calls++
		cancel() // the run was interrupted while waiting
		return 0, errors.New("connection refused")
	})

	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Errorf("calls = %d, err = %v; want one call and context.Canceled", calls, err)
	}
}
