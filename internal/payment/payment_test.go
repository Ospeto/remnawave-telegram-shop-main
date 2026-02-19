package payment

import (
	"testing"
	"time"
)

func TestNormalizePhone(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"international with plus", "+959123456789", "9123456789"},
		{"international without plus", "959123456789", "9123456789"},
		{"local format", "09123456789", "9123456789"},
		{"local without country code", "9123456789", "9123456789"},
		{"with spaces", "+959 123 456 789", "9123456789"},
		{"with dashes", "+959-123-456-789", "9123456789"},
		{"with parens", "+959(123)456789", "9123456789"},
		{"with asterisks (masked)", "+959*****6789", "9596789"},
		{"already clean", "9123456789", "9123456789"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizePhone(tt.input)
			if got != tt.want {
				t.Errorf("normalizePhone(%q) = %q; want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestPhoneMatchesSuffix(t *testing.T) {
	tests := []struct {
		name     string
		expected string
		actual   string
		n        int
		want     bool
	}{
		{"exact match", "9123456789", "9123456789", 4, true},
		{"suffix match last 4", "9123456789", "6789", 4, true},
		{"suffix mismatch", "9123456789", "1234", 4, false},
		{"empty actual", "9123456789", "", 4, false},
		{"actual shorter than n but matches suffix", "9123456789", "89", 4, false},
		{"actual equals expected exact", "9876", "9876", 4, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := phoneMatchesSuffix(tt.expected, tt.actual, tt.n)
			if got != tt.want {
				t.Errorf("phoneMatchesSuffix(%q, %q, %d) = %v; want %v",
					tt.expected, tt.actual, tt.n, got, tt.want)
			}
		})
	}
}

// --- GetTestTransactionID ---

func TestGetTestTransactionID(t *testing.T) {
	svc := &PaymentService{}
	got := svc.GetTestTransactionID()
	if got == "" {
		t.Fatal("GetTestTransactionID() returned empty string")
	}
	if got != testTransactionID {
		t.Errorf("GetTestTransactionID() = %q, want %q", got, testTransactionID)
	}
}

// --- SetTestMode / IsTestMode concurrency ---

func TestSetTestMode_Concurrent(t *testing.T) {
	svc := &PaymentService{}
	done := make(chan struct{}, 50)

	for i := 0; i < 50; i++ {
		go func(v bool) {
			svc.SetTestMode(v)
		}(i%2 == 0)
	}
	for i := 0; i < 50; i++ {
		go func() {
			_ = svc.IsTestMode()
			done <- struct{}{}
		}()
	}
	for i := 0; i < 50; i++ {
		<-done
	}
}

// --- syncCacheEntry TTL logic ---

func TestSyncCacheEntry_TTL(t *testing.T) {
	fresh := syncCacheEntry{
		keys:      []KeyStats{{ID: 1}},
		expiresAt: time.Now().Add(syncCacheTTL),
	}
	if !time.Now().Before(fresh.expiresAt) {
		t.Error("fresh entry should not be expired yet")
	}

	expired := syncCacheEntry{
		keys:      []KeyStats{{ID: 2}},
		expiresAt: time.Now().Add(-time.Minute),
	}
	if time.Now().Before(expired.expiresAt) {
		t.Error("stale entry should be expired")
	}
}
