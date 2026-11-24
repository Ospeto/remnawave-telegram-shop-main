package api

import (
	"log/slog"
	"net/http"
	"time"
)

const (
	// uploadBodyReadDeadline is how long a client gets to finish sending a
	// receipt photo once it has been authorised.
	//
	// A12: the server-level ReadTimeout is 10 s, which was fine for JSON calls
	// but cut off a 10 MB mobile photo on a slow connection mid-upload, so the
	// customer saw a generic failure after their data had already been spent.
	uploadBodyReadDeadline = 3 * time.Minute

	// handlerBodyReadBudget is the deadline re-armed after an upload so the
	// connection returns to the server's normal per-request budget instead of
	// staying open indefinitely.
	handlerBodyReadBudget = 10 * time.Second
)

// withExtendedReadDeadline raises the read deadline for the current request and
// returns a function that puts it back.
//
// It uses http.ResponseController, so the extension is per request and per
// connection: no other endpoint inherits the longer budget, and the server-wide
// ReadTimeout stays where it is for everything that does not stream a body.
//
// A writer that cannot set deadlines (an httptest recorder, or an HTTP/2
// implementation without the optional interface) is not an error for the
// caller: the upload simply keeps the server's default deadline.
func withExtendedReadDeadline(w http.ResponseWriter, budget time.Duration) func() {
	controller := http.NewResponseController(w)

	if err := controller.SetReadDeadline(time.Now().Add(budget)); err != nil {
		slog.Debug("Per-request read deadline is not supported by this connection; keeping the server default",
			"error", err,
			"requested_budget", budget,
		)
		return func() {}
	}

	return func() {
		if err := controller.SetReadDeadline(time.Now().Add(handlerBodyReadBudget)); err != nil {
			slog.Debug("Could not restore the default read deadline", "error", err)
		}
	}
}
