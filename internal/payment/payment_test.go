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

func TestNormalizeRecipientName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"english spaces", "Aung Aung", "aungaung"},
		{"english punctuation", "Maung-Maung", "maungmaung"},
		{"burmese spacing", "အောင် အောင်", "အောင်အောင်"},
		{"mixed symbols", "  Mg. Mg / 123 ", "mgmg123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeRecipientName(tt.input)
			if got != tt.want {
				t.Errorf("normalizeRecipientName(%q) = %q; want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestMatchPaymentRecipient(t *testing.T) {
	origPhoneKPay, origPhoneWavePay, origPhoneAyaPay := PhoneKPay, PhoneWavePay, PhoneAyaPay
	origNameKPay, origNameWave, origNameAya := AccountNameKPay, AccountNameWave, AccountNameAya
	t.Cleanup(func() {
		PhoneKPay, PhoneWavePay, PhoneAyaPay = origPhoneKPay, origPhoneWavePay, origPhoneAyaPay
		AccountNameKPay, AccountNameWave, AccountNameAya = origNameKPay, origNameWave, origNameAya
	})

	PhoneKPay = "09111111111"
	PhoneWavePay = "09222222222"
	PhoneAyaPay = ""
	AccountNameKPay = "Maung Maung"
	AccountNameWave = "Aung Aung"
	AccountNameAya = "Aya Receiver"

	tests := []struct {
		name          string
		provider      string
		phone         string
		recipient     string
		wantKey       string
		wantMatched   bool
		wantMatchedBy string
	}{
		{
			name:          "match wave by provider-specific name",
			provider:      "wavepay",
			phone:         "",
			recipient:     "AungAung",
			wantKey:       "wavepay",
			wantMatched:   true,
			wantMatchedBy: "name",
		},
		{
			name:          "match kpay by phone suffix even if provider alias used",
			provider:      "kbzpay",
			phone:         "09***1111",
			recipient:     "",
			wantKey:       "kpay",
			wantMatched:   true,
			wantMatchedBy: "phone",
		},
		{
			name:          "fall back to any enabled provider",
			provider:      "",
			phone:         "09***2222",
			recipient:     "",
			wantKey:       "wavepay",
			wantMatched:   true,
			wantMatchedBy: "phone",
		},
		{
			name:          "disabled aya is ignored",
			provider:      "ayapay",
			phone:         "",
			recipient:     "Aya Receiver",
			wantKey:       "",
			wantMatched:   false,
			wantMatchedBy: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, matchedBy, matched := MatchPaymentRecipient(tt.provider, tt.phone, tt.recipient, 4)
			if matched != tt.wantMatched {
				t.Fatalf("MatchPaymentRecipient() matched = %v; want %v", matched, tt.wantMatched)
			}
			if provider.Key != tt.wantKey {
				t.Errorf("MatchPaymentRecipient() provider = %q; want %q", provider.Key, tt.wantKey)
			}
			if matchedBy != tt.wantMatchedBy {
				t.Errorf("MatchPaymentRecipient() matchedBy = %q; want %q", matchedBy, tt.wantMatchedBy)
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
