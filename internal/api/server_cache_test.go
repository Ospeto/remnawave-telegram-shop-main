package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// newStaticFixtureWorkdir creates a throwaway working directory holding a minimal
// web-app/dist tree (the SPA handler resolves only relative paths), so cache-header
// behaviour can be asserted without a real frontend build.
func newStaticFixtureWorkdir(t *testing.T) {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error = %v", err)
	}
	root := t.TempDir()
	dist := filepath.Join(root, "web-app", "dist")
	assets := filepath.Join(dist, "assets")
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", assets, err)
	}
	files := map[string]string{
		filepath.Join(dist, "index.html"):           "<!doctype html><div id=\"root\"></div>",
		filepath.Join(assets, "index-BIvCmLd5.js"):  "console.log('bundle')",
		filepath.Join(assets, "index-BGwZdthz.css"): "body{}",
	}
	for path, body := range files {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", path, err)
		}
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("os.Chdir(%q) error = %v", root, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatalf("restore cwd error = %v", err)
		}
	})
}

func serveStatic(t *testing.T, urlPath string) *httptest.ResponseRecorder {
	t.Helper()

	mux := http.NewServeMux()
	RegisterHandlers(mux, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, urlPath, nil))
	return rec
}

func TestStaticAssetsAreCachedImmutably(t *testing.T) {
	newStaticFixtureWorkdir(t)

	for _, asset := range []string{"/assets/index-BIvCmLd5.js", "/assets/index-BGwZdthz.css"} {
		rec := serveStatic(t, asset)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want %d", asset, rec.Code, http.StatusOK)
		}
		if got := rec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
			t.Fatalf("%s Cache-Control = %q, want immutable content-hashed caching", asset, got)
		}
	}
}

func TestShellAndSPAFallbackStayNoStore(t *testing.T) {
	newStaticFixtureWorkdir(t)

	const noStore = "no-store, no-cache, must-revalidate"

	for _, path := range []string{"/", "/wallet", "/checkout/plan-30"} {
		rec := serveStatic(t, path)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want %d", path, rec.Code, http.StatusOK)
		}
		if got := rec.Header().Get("Cache-Control"); got != noStore {
			t.Fatalf("%s Cache-Control = %q, want %q", path, got, noStore)
		}
	}
}

func TestNonHashedStaticFileKeepsNoCachePolicy(t *testing.T) {
	newStaticFixtureWorkdir(t)

	// A root-level file that is not content-hashed must not be marked immutable.
	logoPath := filepath.Join("web-app", "dist", "logo.jpg")
	if err := os.WriteFile(logoPath, []byte("jpeg"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", logoPath, err)
	}

	rec := serveStatic(t, "/logo.jpg")
	if rec.Code != http.StatusOK {
		t.Fatalf("/logo.jpg status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Cache-Control"); got != "" {
		t.Fatalf("/logo.jpg Cache-Control = %q, want no forced immutable caching", got)
	}
}
