package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// deadlineRecorder is an http.ResponseWriter that supports the optional
// SetReadDeadline hook http.ResponseController looks for, so the test can prove
// A12's per-request extension actually reaches the connection.
type deadlineRecorder struct {
	*httptest.ResponseRecorder
	deadlines []time.Time
	failWith  error
}

func (d *deadlineRecorder) SetReadDeadline(deadline time.Time) error {
	if d.failWith != nil {
		return d.failWith
	}
	d.deadlines = append(d.deadlines, deadline)
	return nil
}

// TestUploadHandlerExtendsReadDeadlineForSlowMobilePhotos is A12: the server's
// 10 s ReadTimeout must not apply to a 10 MB receipt photo, and the extension
// must be per request so no other endpoint inherits it.
func TestUploadHandlerExtendsReadDeadlineForSlowMobilePhotos(t *testing.T) {
	recorder := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}

	start := time.Now()
	restore := withExtendedReadDeadline(recorder, uploadBodyReadDeadline)

	if len(recorder.deadlines) != 1 {
		t.Fatalf("SetReadDeadline calls = %d, want 1 extension", len(recorder.deadlines))
	}

	extended := recorder.deadlines[0]
	if budget := extended.Sub(start); budget < uploadBodyReadDeadline-time.Minute {
		t.Fatalf("extended read budget = %s, want about %s", budget, uploadBodyReadDeadline)
	}
	if extended.Sub(start) <= 10*time.Second {
		t.Fatalf("extended read budget = %s, want more than the 10s server ReadTimeout", extended.Sub(start))
	}

	restore()

	if len(recorder.deadlines) != 2 {
		t.Fatalf("SetReadDeadline calls after restore = %d, want the deadline put back", len(recorder.deadlines))
	}
	if restored := recorder.deadlines[1]; restored.Sub(time.Now()) > handlerBodyReadBudget+5*time.Second {
		t.Fatalf("restored deadline = %s, want the short default budget back", restored)
	}
}

// TestUploadReadDeadlineIsNotFatalWhenUnsupported keeps the upload working on
// connections that cannot set deadlines (httptest recorders, some HTTP/2 stacks).
func TestUploadReadDeadlineIsNotFatalWhenUnsupported(t *testing.T) {
	recorder := &deadlineRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		failWith:         http.ErrNotSupported,
	}

	restore := withExtendedReadDeadline(recorder, uploadBodyReadDeadline)
	if restore == nil {
		t.Fatal("withExtendedReadDeadline() returned a nil restore function")
	}
	restore()

	if len(recorder.deadlines) != 0 {
		t.Fatalf("deadlines recorded = %d, want 0 when the writer refuses deadlines", len(recorder.deadlines))
	}
}
