// Package funnel emits the four server-side events that describe the purchase
// funnel.
//
// Every ranking in the UX scout's report was inference: the shop had no record of
// how many customers opened the purchase surface, how many actually created a
// purchase, how many uploaded a receipt, and how many verifications succeeded.
// These four events are that record. They are log lines only - no new table, no
// new dependency, no customer-facing change - so the funnel can be counted from
// whatever log sink is already running.
//
// The bot and the API are separate processes and separate packages, so the event
// names and their stage order live here rather than being spelled by hand at
// each call site. A stage name typo would silently break a count.
package funnel

import (
	"log/slog"
)

// Stage names. Keep these stable: dashboards and queries will reference them.
const (
	// EventPurchaseOpened is a customer reaching a purchase surface: the plans
	// list in the Mini App, or the in-chat purchase menu in the bot.
	EventPurchaseOpened = "purchase_opened"

	// EventPurchaseCreated is a purchase row actually being created. This is the
	// first stage where money is committed to a plan.
	EventPurchaseCreated = "purchase_created"

	// EventScreenshotUploaded is a receipt image being accepted for verification.
	EventScreenshotUploaded = "screenshot_uploaded"

	// EventVerificationSucceeded is a receipt being verified and the purchase
	// fulfilled. This is the stage the customer experiences as "I got my key".
	EventVerificationSucceeded = "verification_succeeded"
)

// Surface values, so a query can tell the bot's in-chat chain from the Mini App.
const (
	SurfaceMiniApp = "miniapp"
	SurfaceBot     = "bot"
)

// Emit writes one funnel event.
//
// Every field is optional: a stage whose identity is not known at the call site
// still counts, because the stage count is the measurement that was missing. The
// event name is always the first attribute so a log query can filter on it
// without depending on attribute order elsewhere.
func Emit(event, surface string, attrs ...any) {
	fields := make([]any, 0, len(attrs)+4)
	fields = append(fields, "funnel_event", event)
	if surface != "" {
		fields = append(fields, "funnel_surface", surface)
	}
	fields = append(fields, attrs...)

	slog.Info("purchase funnel", fields...)
}
