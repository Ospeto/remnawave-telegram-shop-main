package config

import "testing"

func TestDefaultLanguageFallback(t *testing.T) {
	old := conf.defaultLanguage
	t.Cleanup(func() {
		conf.defaultLanguage = old
	})

	conf.defaultLanguage = ""
	if got := DefaultLanguage(); got != "en" {
		t.Fatalf("DefaultLanguage() = %q, want %q", got, "en")
	}
}

func TestResolveVisionProvidersOpenRouterPrimaryWithoutGemini(t *testing.T) {
	primary, fallback, err := resolveVisionProviders("", "openrouter-key", "", "")
	if err != nil {
		t.Fatalf("resolveVisionProviders() error = %v", err)
	}
	if primary != "openrouter" {
		t.Fatalf("primary = %q, want %q", primary, "openrouter")
	}
	if fallback != "" {
		t.Fatalf("fallback = %q, want empty", fallback)
	}
}

func TestResolveVisionProvidersSupportsExplicitGeminiFallback(t *testing.T) {
	primary, fallback, err := resolveVisionProviders("gemini-key", "openrouter-key", "", "gemini")
	if err != nil {
		t.Fatalf("resolveVisionProviders() error = %v", err)
	}
	if primary != "openrouter" {
		t.Fatalf("primary = %q, want %q", primary, "openrouter")
	}
	if fallback != "gemini" {
		t.Fatalf("fallback = %q, want %q", fallback, "gemini")
	}
}

func TestResolveVisionProvidersUsesOpenRouterFallbackModelWithoutGemini(t *testing.T) {
	primary, fallback, err := resolveVisionProviders("", "openrouter-key", "google/gemini-3.1-flash-lite-preview", "openrouter")
	if err != nil {
		t.Fatalf("resolveVisionProviders() error = %v", err)
	}
	if primary != "openrouter" {
		t.Fatalf("primary = %q, want %q", primary, "openrouter")
	}
	if fallback != "openrouter" {
		t.Fatalf("fallback = %q, want %q", fallback, "openrouter")
	}
}

func TestResolveVisionProvidersRequiresAtLeastOneConfiguredProvider(t *testing.T) {
	_, _, err := resolveVisionProviders("", "", "", "")
	if err == nil {
		t.Fatal("resolveVisionProviders() error = nil, want error")
	}
}

func TestResolveVisionProvidersRejectsMissingFallbackProviderCredentials(t *testing.T) {
	_, _, err := resolveVisionProviders("", "openrouter-key", "", "gemini")
	if err == nil {
		t.Fatal("resolveVisionProviders() error = nil, want error")
	}
}

func TestResolveVisionProvidersRejectsOpenRouterFallbackWithoutFallbackModel(t *testing.T) {
	_, _, err := resolveVisionProviders("", "openrouter-key", "", "openrouter")
	if err == nil {
		t.Fatal("resolveVisionProviders() error = nil, want error")
	}
}
