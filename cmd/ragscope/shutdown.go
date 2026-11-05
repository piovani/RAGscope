package main

import (
	"context"
	"time"
)

const shutdownTimeout = 10 * time.Second

// newShutdownContext returns a context for graceful shutdown. It is deliberately
// detached from the (already cancelled) command context.
func newShutdownContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), shutdownTimeout)
}
