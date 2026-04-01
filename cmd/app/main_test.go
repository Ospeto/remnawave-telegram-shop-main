package main

import (
	"os"
	"syscall"
	"testing"
)

func TestShutdownSignals(t *testing.T) {
	signals := shutdownSignals()

	if len(signals) != 2 {
		t.Fatalf("shutdownSignals() length = %d, want 2", len(signals))
	}

	if signals[0] != os.Interrupt {
		t.Fatalf("shutdownSignals()[0] = %v, want %v", signals[0], os.Interrupt)
	}

	if signals[1] != syscall.SIGTERM {
		t.Fatalf("shutdownSignals()[1] = %v, want %v", signals[1], syscall.SIGTERM)
	}
}
