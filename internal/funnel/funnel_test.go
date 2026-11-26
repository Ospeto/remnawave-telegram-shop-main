package funnel

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// captureLogs redirects the default slog logger into a buffer for one test.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return buf
}

// TestEmitRecordsTheFunnelEventName is the guard that keeps the funnel
// countable: a query filters on funnel_event, so the name must always be present
// and must match the constant.
func TestEmitRecordsTheFunnelEventName(t *testing.T) {
	for _, event := range []string{
		EventPurchaseOpened,
		EventPurchaseCreated,
		EventScreenshotUploaded,
		EventVerificationSucceeded,
	} {
		buf := captureLogs(t)
		Emit(event, SurfaceMiniApp, "purchase_id", 42)

		line := buf.String()
		if !strings.Contains(line, "funnel_event="+event) {
			t.Fatalf("log line %q does not carry funnel_event=%s", line, event)
		}
		if !strings.Contains(line, "funnel_surface="+SurfaceMiniApp) {
			t.Fatalf("log line %q does not carry the surface", line)
		}
		if !strings.Contains(line, "purchase_id=42") {
			t.Fatalf("log line %q dropped the extra attributes", line)
		}
	}
}

// TestEmitOmitsAnUnknownSurface keeps the bot and Mini App counts honest: a
// stage whose surface is not known must not be attributed to either.
func TestEmitOmitsAnUnknownSurface(t *testing.T) {
	buf := captureLogs(t)
	Emit(EventPurchaseOpened, "")

	if line := buf.String(); strings.Contains(line, "funnel_surface=") {
		t.Fatalf("log line %q invented a surface", line)
	}
}

// TestStageNamesAreDistinct prevents a copy-paste that would collapse two
// stages into one count.
func TestStageNamesAreDistinct(t *testing.T) {
	seen := map[string]bool{}
	for _, event := range []string{
		EventPurchaseOpened,
		EventPurchaseCreated,
		EventScreenshotUploaded,
		EventVerificationSucceeded,
	} {
		if seen[event] {
			t.Fatalf("duplicate funnel stage name %q", event)
		}
		seen[event] = true
	}
	if len(seen) != 4 {
		t.Fatalf("funnel stages = %d, want 4", len(seen))
	}
}
