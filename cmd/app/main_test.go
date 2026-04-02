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

func TestNewVisionProviderBuildsOpenRouterClient(t *testing.T) {
	provider, err := newVisionProvider("openrouter", "", "", "openrouter-key", "openai/gpt-4.1-mini", "")
	if err != nil {
		t.Fatalf("newVisionProvider() error = %v", err)
	}
	if provider == nil {
		t.Fatal("newVisionProvider() provider = nil, want provider")
	}
	if provider.Name() != "openrouter" {
		t.Fatalf("provider.Name() = %q, want %q", provider.Name(), "openrouter")
	}
}

func TestNewVisionProviderRejectsMissingGeminiCredentials(t *testing.T) {
	_, err := newVisionProvider("gemini", "", "gemini-2.5-flash", "", "", "")
	if err == nil {
		t.Fatal("newVisionProvider() error = nil, want error")
	}
}

func TestNewVisionProviderBuildsOpenRouterFallbackClient(t *testing.T) {
	provider, err := newVisionProvider("openrouter", "", "", "openrouter-key", "openai/gpt-4.1-mini", "google/gemini-3.1-flash-lite-preview")
	if err != nil {
		t.Fatalf("newVisionProvider() error = %v", err)
	}
	if provider == nil {
		t.Fatal("newVisionProvider() provider = nil, want provider")
	}
	if provider.Name() != "openrouter-fallback" {
		t.Fatalf("provider.Name() = %q, want %q", provider.Name(), "openrouter-fallback")
	}
}
