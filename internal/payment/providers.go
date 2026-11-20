package payment

import (
	"fmt"
	"html"
	"strings"
	"sync"
	"unicode"
)

// PaymentProvider describes one configured mobile banking receiver.
type PaymentProvider struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Phone       string `json:"phone,omitempty"`
	AccountName string `json:"account_name,omitempty"`
}

// Per-provider payment configuration. Values are loaded from app_config on
// startup and can be updated at runtime by admin commands.
//
// The exported variables below are mutable shared state. All runtime reads and
// writes MUST go through the accessor functions in this file (SetProviderPhone,
// SetProviderAccountName, allPaymentProviders, Get/SetReferralBonusAmount) so
// that the configMu read-write lock keeps concurrent HTTP handlers and Telegram
// admin commands from racing on them.
var (
	configMu sync.RWMutex

	PhoneKPay       string
	PhoneWavePay    string
	PhoneAyaPay     string
	AccountNameKPay string
	AccountNameWave string
	AccountNameAya  string
)

// SetProviderPhone updates the receiving phone number for a provider key.
// An empty phone disables the provider. Returns false for unknown providers.
func SetProviderPhone(providerKey, phone string) bool {
	configMu.Lock()
	defer configMu.Unlock()

	switch NormalizeProviderKey(providerKey) {
	case "kpay":
		PhoneKPay = phone
	case "wavepay":
		PhoneWavePay = phone
	case "ayapay":
		PhoneAyaPay = phone
	default:
		return false
	}
	return true
}

// SetProviderAccountName updates the recipient account name for a provider key.
// An empty name clears it. Returns false for unknown providers.
func SetProviderAccountName(providerKey, name string) bool {
	configMu.Lock()
	defer configMu.Unlock()

	switch NormalizeProviderKey(providerKey) {
	case "kpay":
		AccountNameKPay = name
	case "wavepay":
		AccountNameWave = name
	case "ayapay":
		AccountNameAya = name
	default:
		return false
	}
	return true
}

// GetReferralBonusAmount returns the current referral bonus (MMK) under the lock.
func GetReferralBonusAmount() float64 {
	configMu.RLock()
	defer configMu.RUnlock()
	return ReferralBonusAmount
}

// SetReferralBonusAmount stores the new referral bonus (MMK) under the lock.
func SetReferralBonusAmount(amount float64) {
	configMu.Lock()
	defer configMu.Unlock()
	ReferralBonusAmount = amount
}

func allPaymentProviders() []PaymentProvider {
	configMu.RLock()
	defer configMu.RUnlock()

	return []PaymentProvider{
		{Key: "kpay", Label: "KPay", Phone: PhoneKPay, AccountName: AccountNameKPay},
		{Key: "wavepay", Label: "WavePay", Phone: PhoneWavePay, AccountName: AccountNameWave},
		{Key: "ayapay", Label: "AYA Pay", Phone: PhoneAyaPay, AccountName: AccountNameAya},
	}
}

// NormalizeProviderKey maps provider aliases to the canonical keys used in the
// rest of the payment flow.
func NormalizeProviderKey(provider string) string {
	normalized := strings.ToLower(strings.TrimSpace(provider))
	normalized = strings.ReplaceAll(normalized, " ", "")
	switch normalized {
	case "kpay", "kbz", "kbzpay":
		return "kpay"
	case "wave", "wavepay", "wavemoney":
		return "wavepay"
	case "aya", "ayapay":
		return "ayapay"
	default:
		return normalized
	}
}

// GetEnabledPaymentProviders returns checkout-visible providers. A provider is
// considered enabled only when it has a receiving phone number configured.
func GetEnabledPaymentProviders() []PaymentProvider {
	providers := make([]PaymentProvider, 0, 3)
	for _, provider := range allPaymentProviders() {
		if strings.TrimSpace(provider.Phone) == "" {
			continue
		}
		providers = append(providers, provider)
	}
	return providers
}

// LookupPaymentProvider returns the canonical provider definition regardless of
// whether it is enabled.
func LookupPaymentProvider(provider string) (PaymentProvider, bool) {
	key := NormalizeProviderKey(provider)
	for _, candidate := range allPaymentProviders() {
		if candidate.Key == key {
			return candidate, true
		}
	}
	return PaymentProvider{}, false
}

// GetAllPaymentPhones returns a map of provider→phone for all enabled providers.
func GetAllPaymentPhones() map[string]string {
	phones := make(map[string]string)
	for _, provider := range GetEnabledPaymentProviders() {
		phones[provider.Key] = provider.Phone
	}
	return phones
}

// GetFirstPaymentPhone returns the first non-empty phone (backward compat).
func GetFirstPaymentPhone() string {
	providers := GetEnabledPaymentProviders()
	if len(providers) == 0 {
		return ""
	}
	return providers[0].Phone
}

// GetAcceptedProviderLabels returns the human-readable labels for enabled
// providers in display order.
func GetAcceptedProviderLabels() []string {
	providers := GetEnabledPaymentProviders()
	labels := make([]string, 0, len(providers))
	for _, provider := range providers {
		labels = append(labels, provider.Label)
	}
	return labels
}

// GetAcceptedProviderText joins enabled provider labels for compact UI text.
func GetAcceptedProviderText(separator string) string {
	return strings.Join(GetAcceptedProviderLabels(), separator)
}

// BuildPaymentReceiversHTML renders enabled providers as HTML bullet lines for
// Telegram/API instruction messages.
func BuildPaymentReceiversHTML() string {
	providers := GetEnabledPaymentProviders()
	if len(providers) == 0 {
		return "<i>No payment accounts configured.</i>"
	}

	lines := make([]string, 0, len(providers))
	for _, provider := range providers {
		line := fmt.Sprintf("• <b>%s</b>: <code>%s</code>", provider.Label, html.EscapeString(provider.Phone))
		if name := strings.TrimSpace(provider.AccountName); name != "" {
			line += fmt.Sprintf(" <i>(%s)</i>", html.EscapeString(name))
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// AnyPhoneMatchesSuffix checks if actualPhone matches any enabled provider phone.
func AnyPhoneMatchesSuffix(actualPhone string, digits int) bool {
	actual := normalizePhone(actualPhone)
	for _, provider := range GetEnabledPaymentProviders() {
		if phoneMatchesSuffix(normalizePhone(provider.Phone), actual, digits) {
			return true
		}
	}
	return false
}

// MatchPaymentRecipient tries to match a receipt to an enabled provider using
// the extracted provider key, phone, and recipient name.
func MatchPaymentRecipient(providerKey, actualPhone, actualName string, digits int) (PaymentProvider, string, bool) {
	if providerKey != "" {
		if provider, ok := LookupPaymentProvider(providerKey); ok && strings.TrimSpace(provider.Phone) != "" {
			if providerPhoneMatches(provider, actualPhone, digits) {
				return provider, "phone", true
			}
			if providerNameMatches(provider, actualName) {
				return provider, "name", true
			}
		}
	}

	for _, provider := range GetEnabledPaymentProviders() {
		if providerPhoneMatches(provider, actualPhone, digits) {
			return provider, "phone", true
		}
	}

	for _, provider := range GetEnabledPaymentProviders() {
		if providerNameMatches(provider, actualName) {
			return provider, "name", true
		}
	}

	return PaymentProvider{}, "", false
}

func providerPhoneMatches(provider PaymentProvider, actualPhone string, digits int) bool {
	if strings.TrimSpace(provider.Phone) == "" {
		return false
	}
	return phoneMatchesSuffix(normalizePhone(provider.Phone), normalizePhone(actualPhone), digits)
}

func providerNameMatches(provider PaymentProvider, actualName string) bool {
	expected := normalizeRecipientName(provider.AccountName)
	actual := normalizeRecipientName(actualName)
	if expected == "" || actual == "" {
		return false
	}
	return expected == actual
}

func normalizeRecipientName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var builder strings.Builder
	for _, r := range name {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), unicode.IsMark(r):
			builder.WriteRune(r)
		case unicode.IsSpace(r):
			continue
		}
	}
	return builder.String()
}
