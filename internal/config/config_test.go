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
