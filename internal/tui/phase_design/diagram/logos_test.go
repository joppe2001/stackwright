package diagram

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoadLogoEmptyURL(t *testing.T) {
	if got := loadLogo(""); got != nil {
		t.Error("expected nil for empty URL")
	}
}

func TestLoadLogoMissingServer(t *testing.T) {
	// A bogus port that nothing will be listening on — exercises the
	// network-failure silent-fallback path.
	logoCacheMu.Lock()
	logoCache = map[string]*image.RGBA{}
	logoTried = map[string]bool{}
	logoCacheMu.Unlock()

	url := "http://127.0.0.1:1/nonexistent.png"
	if got := loadLogo(url); got != nil {
		t.Error("expected nil for unreachable URL")
	}
	// Negative cache should prevent a second request.
	logoCacheMu.RLock()
	tried := logoTried[url]
	logoCacheMu.RUnlock()
	if !tried {
		t.Error("expected URL to be marked in negative cache")
	}
}

func TestLoadLogo404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	logoCacheMu.Lock()
	logoCache = map[string]*image.RGBA{}
	logoTried = map[string]bool{}
	logoCacheMu.Unlock()

	url := srv.URL + "/missing.png"
	if got := loadLogo(url); got != nil {
		t.Error("expected nil for 404")
	}
}

func TestLoadLogoHappyPath(t *testing.T) {
	// Serve a real (tiny) PNG so the fetch + decode + resize path runs end-to-end.
	png32 := makeTestPNG(t, 16, 16, color.RGBA{R: 255, G: 128, B: 0, A: 255})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png32)
	}))
	defer srv.Close()

	// Redirect the logo cache dir into t.TempDir so we don't pollute the
	// user's real XDG cache.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	logoCacheMu.Lock()
	logoCache = map[string]*image.RGBA{}
	logoTried = map[string]bool{}
	logoCacheMu.Unlock()

	url := srv.URL + "/good.png"
	got := loadLogo(url)
	if got == nil {
		t.Fatal("expected a decoded logo")
	}
	if got.Bounds().Dx() != logoPixelSize || got.Bounds().Dy() != logoPixelSize {
		t.Errorf("unexpected resized dims: %v", got.Bounds())
	}
	// Second call should hit the in-memory cache — same pointer.
	got2 := loadLogo(url)
	if got2 != got {
		t.Error("expected second call to return the cached pointer")
	}
}

// makeTestPNG returns a freshly encoded PNG of the given solid color.
// Used by the happy-path test to avoid ecosystem deps on test fixtures.
func makeTestPNG(t *testing.T, w, h int, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return buf.Bytes()
}

func TestLoadLogoNonPNG(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("not a png"))
	}))
	defer srv.Close()

	logoCacheMu.Lock()
	logoCache = map[string]*image.RGBA{}
	logoTried = map[string]bool{}
	logoCacheMu.Unlock()

	url := srv.URL + "/plain.txt"
	if got := loadLogo(url); got != nil {
		t.Error("expected nil for non-PNG response")
	}
}
