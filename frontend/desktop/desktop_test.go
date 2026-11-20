package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/fstest"
)

func TestNormalizeServer(t *testing.T) {
	cases := map[string]string{
		"s-check.firma.de":                "https://s-check.firma.de",
		" https://S-Check.Firma.de/ ":     "https://s-check.firma.de",
		"http://10.0.0.5:8080/api":        "http://10.0.0.5:8080",
		"https://firma.de/scheck/#/login": "https://firma.de/scheck",
		"https://firma.de/scheck/api/":    "https://firma.de/scheck",
	}
	for in, want := range cases {
		got, err := normalizeServer(in)
		if err != nil || got != want {
			t.Errorf("%q: got %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "ftp://x", "https://user:pw@x", "https://x/?a=1", "://"} {
		if _, err := normalizeServer(bad); err == nil {
			t.Errorf("%q: expected error", bad)
		}
	}
}

func TestConfigPrecedence(t *testing.T) {
	dir := t.TempDir()
	// preset applies while the user has chosen nothing
	s, _ := loadConfig(dir, Policy{Server: "https://preset.example"})
	if s.get().Server != "https://preset.example" {
		t.Fatalf("preset not applied: %q", s.get().Server)
	}
	_ = s.setServer("https://own.example")
	s, _ = loadConfig(dir, Policy{Server: "https://preset.example"})
	if s.get().Server != "https://own.example" {
		t.Fatalf("own choice lost: %q", s.get().Server)
	}
	// a locked policy wins
	s, _ = loadConfig(dir, Policy{Server: "https://it.example", Locked: true})
	if s.get().Server != "https://it.example" {
		t.Fatalf("lock ignored: %q", s.get().Server)
	}
	for i := 0; i < 12; i++ {
		_ = s.setServer("https://h" + strings.Repeat("x", i) + ".example")
	}
	if n := len(s.get().Recent); n != maxRecent {
		t.Fatalf("recent list has %d entries", n)
	}
	if st, err := os.Stat(dir + "/desktop.json"); err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("config file mode: %v %v", st.Mode(), err)
	}
}

func TestLocalServerGuards(t *testing.T) {
	web := fstest.MapFS{
		"index.html":      {Data: []byte("<html><head><title>x</title></head></html>")},
		"assets/app-1.js": {Data: []byte("console.log(1)")},
	}
	cfg, _ := loadConfig(t.TempDir(), Policy{})
	a, err := newApp("127.0.0.1:1234", "tok", "9.9.9", cfg, web)
	if err != nil {
		t.Fatal(err)
	}
	do := func(path, host string, cookie bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r.Host = host
		if cookie {
			r.AddCookie(&http.Cookie{Name: tokenCookie, Value: "tok"})
		}
		w := httptest.NewRecorder()
		a.ServeHTTP(w, r)
		return w
	}
	if w := do("/", "127.0.0.1:1234", false); w.Code != 403 {
		t.Errorf("no token: %d", w.Code)
	}
	if w := do("/", "evil:1234", true); w.Code != 403 {
		t.Errorf("wrong host: %d", w.Code)
	}
	w := do("/_desktop/start?t=tok", "127.0.0.1:1234", false)
	if w.Code != 303 || !strings.Contains(w.Header().Get("Set-Cookie"), "SameSite=Strict") {
		t.Errorf("start: %d %q", w.Code, w.Header().Get("Set-Cookie"))
	}
	w = do("/", "127.0.0.1:1234", true)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `name="scheck-desktop" content="9.9.9"`) {
		t.Errorf("index: %d %s", w.Code, w.Body.String())
	}
	if w = do("/assets/app-1.js", "127.0.0.1:1234", true); !strings.Contains(w.Header().Get("Cache-Control"), "immutable") {
		t.Errorf("asset cache: %q", w.Header().Get("Cache-Control"))
	}
	if w = do("/api/projects", "127.0.0.1:1234", true); w.Code != 503 {
		t.Errorf("api without server: %d", w.Code)
	}
}
