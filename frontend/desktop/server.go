package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	tokenCookie   = "scheck_desktop"
	backendCookie = "scheck_session" // session cookie of the S-Check backend
)

type asset struct {
	body  []byte
	ctype string
	etag  string
}

// app is the local web server inside the desktop app. It serves the embedded
// user interface and forwards /api to the S-Check backend the user signed in to.
type app struct {
	host    string // 127.0.0.1:port, the only accepted Host header
	token   string
	version string
	cfg     *configStore
	assets  map[string]asset
	proxy   atomic.Pointer[httputil.ReverseProxy]
	client  *http.Client
	tr      *http.Transport
}

func newTransport() *http.Transport {
	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          64,
		MaxIdleConnsPerHost:   32,
		IdleConnTimeout:       120 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
		// The browser negotiates compression with the backend itself; the
		// proxy passes bodies through unchanged.
		DisableCompression: true,
		WriteBufferSize:    64 << 10,
		ReadBufferSize:     64 << 10,
	}
}

func newApp(host, token, version string, cfg *configStore, web fs.FS) (*app, error) {
	a := &app{host: host, token: token, version: version, cfg: cfg, tr: newTransport()}
	a.client = &http.Client{Transport: a.tr, Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if err := a.loadAssets(web); err != nil {
		return nil, err
	}
	if s := cfg.get().Server; s != "" {
		a.useServer(s)
	}
	return a, nil
}

// loadAssets reads the built interface into memory once; every request after
// that is a map lookup.
func (a *app) loadAssets(web fs.FS) error {
	a.assets = map[string]asset{}
	meta := fmt.Sprintf(`<meta name="scheck-desktop" content="%s">`, a.version)
	return fs.WalkDir(web, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(web, p)
		if err != nil {
			return err
		}
		if p == "index.html" {
			b = bytes.Replace(b, []byte("<head>"), []byte("<head>\n    "+meta), 1)
		}
		ct := mime.TypeByExtension(path.Ext(p))
		if ct == "" {
			ct = http.DetectContentType(b)
		}
		sum := sha256.Sum256(b)
		a.assets["/"+p] = asset{body: b, ctype: ct, etag: `"` + hex.EncodeToString(sum[:8]) + `"`}
		return nil
	})
}

func (a *app) useServer(server string) {
	target, err := url.Parse(server)
	if err != nil {
		return
	}
	rp := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.Host = target.Host
			pr.Out.Header.Del("Origin")
			pr.Out.Header.Del("Referer")
			stripCookie(pr.Out, tokenCookie)
		},
		Transport:     a.tr,
		FlushInterval: -1, // stream uploads, downloads and long responses without buffering
		BufferPool:    bufPool{},
		ModifyResponse: func(res *http.Response) error {
			// The backend sets its cookies for its own host; here they belong to
			// the local origin of the app window.
			if sc := res.Header.Values("Set-Cookie"); len(sc) > 0 {
				res.Header.Del("Set-Cookie")
				for _, line := range sc {
					c, err := http.ParseSetCookie(line)
					if err != nil {
						continue
					}
					c.Domain, c.Secure = "", false
					res.Header.Add("Set-Cookie", c.String())
				}
			}
			res.Header.Del("Strict-Transport-Security")
			res.Header.Del("Alt-Svc")
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			if errors.Is(err, context.Canceled) {
				return
			}
			log.Printf("proxy %s %s: %v", r.Method, r.URL.Path, err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": explainNetErr(err, target)})
		},
	}
	a.proxy.Store(rp)
	// open the connection early so the first click does not wait for DNS and TLS
	go func() {
		if res, err := a.client.Get(server + "/api/health"); err == nil {
			_, _ = io.Copy(io.Discard, res.Body)
			res.Body.Close()
		}
	}()
}

type bufPool struct{}

var pool = make(chan []byte, 32)

func (bufPool) Get() []byte {
	select {
	case b := <-pool:
		return b
	default:
		return make([]byte, 256<<10)
	}
}
func (bufPool) Put(b []byte) {
	select {
	case pool <- b:
	default:
	}
}

func stripCookie(r *http.Request, name string) {
	cs := r.Cookies()
	r.Header.Del("Cookie")
	for _, c := range cs {
		if c.Name != name {
			r.AddCookie(c)
		}
	}
}

func (a *app) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Only the app window may talk to this server: Host check against DNS
	// rebinding, launch token against other programs on the same machine.
	if r.Host != a.host {
		http.Error(w, "Falscher Host", http.StatusForbidden)
		return
	}
	if r.URL.Path == "/_desktop/start" {
		a.start(w, r)
		return
	}
	if r.URL.Path == "/_desktop/ping" && a.tokenOK(r.Header.Get("X-Desktop-Token")) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	c, err := r.Cookie(tokenCookie)
	if err != nil || !a.tokenOK(c.Value) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		io.WriteString(w, forbiddenPage)
		return
	}
	switch {
	case strings.HasPrefix(r.URL.Path, "/api/"):
		a.serveAPI(w, r)
	case strings.HasPrefix(r.URL.Path, "/_desktop/"):
		a.serveDesktop(w, r)
	default:
		a.serveAsset(w, r)
	}
}

func (a *app) tokenOK(v string) bool {
	return v != "" && subtle.ConstantTimeCompare([]byte(v), []byte(a.token)) == 1
}

func (a *app) start(w http.ResponseWriter, r *http.Request) {
	if !a.tokenOK(r.URL.Query().Get("t")) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		io.WriteString(w, forbiddenPage)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: tokenCookie, Value: a.token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *app) serveAPI(w http.ResponseWriter, r *http.Request) {
	rp := a.proxy.Load()
	if rp == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Es ist noch kein S-Check-Server eingerichtet. Bitte die Serveradresse bei der Anmeldung angeben."})
		return
	}
	rp.ServeHTTP(w, r)
}

func (a *app) serveAsset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Methode nicht erlaubt", http.StatusMethodNotAllowed)
		return
	}
	p := r.URL.Path
	if p == "/" {
		p = "/index.html"
	}
	as, ok := a.assets[p]
	if !ok {
		http.NotFound(w, r)
		return
	}
	h := w.Header()
	h.Set("Content-Type", as.ctype)
	h.Set("ETag", as.etag)
	h.Set("X-Content-Type-Options", "nosniff")
	if strings.HasPrefix(p, "/assets/") {
		// file names carry a content hash
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		h.Set("Cache-Control", "no-cache")
	}
	if p == "/index.html" {
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'")
		h.Set("Referrer-Policy", "no-referrer")
	}
	if r.Header.Get("If-None-Match") == as.etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	h.Set("Content-Length", fmt.Sprint(len(as.body)))
	if r.Method == http.MethodHead {
		return
	}
	w.Write(as.body)
}

// ---- desktop endpoints: server address

type desktopInfo struct {
	Server  string   `json:"server"`
	Recent  []string `json:"recent"`
	Locked  bool     `json:"locked"`
	Version string   `json:"version"`
}

func (a *app) info() desktopInfo {
	c := a.cfg.get()
	if c.Recent == nil {
		c.Recent = []string{}
	}
	return desktopInfo{Server: c.Server, Recent: c.Recent, Locked: a.cfg.policy.Locked, Version: a.version}
}

func (a *app) serveDesktop(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/_desktop/config":
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, a.info())
	case r.Method == http.MethodPost && r.URL.Path == "/_desktop/server":
		if r.Header.Get("X-Requested-With") != "scheck" {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "Fehlender X-Requested-With-Header"})
			return
		}
		var in struct {
			URL string `json:"url"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&in); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": errServerFormat.Error()})
			return
		}
		server, err := normalizeServer(in.URL)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		cur := a.cfg.get().Server
		if a.cfg.policy.Locked && server != cur {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "Die Serveradresse ist von Ihrer IT vorgegeben und kann hier nicht geändert werden."})
			return
		}
		if err := a.probe(r.Context(), server); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		if server != cur {
			// a session of the previous server must not travel to the new one
			http.SetCookie(w, &http.Cookie{Name: backendCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
			a.useServer(server)
		}
		if err := a.cfg.setServer(server); err != nil {
			log.Printf("config: %v", err)
		}
		log.Printf("server: %s", server)
		writeJSON(w, http.StatusOK, a.info())
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Unbekannte Anfrage"})
	}
}

// probe checks that an S-Check backend answers at the address.
func (a *app) probe(ctx context.Context, server string) error {
	u, _ := url.Parse(server)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server+"/api/health", nil)
	req.Header.Set("Accept", "application/json")
	res, err := a.client.Do(req)
	if err != nil {
		return errors.New(explainNetErr(err, u))
	}
	defer res.Body.Close()
	var h struct {
		Database *bool `json:"database"`
		Redis    *bool `json:"redis"`
	}
	if json.NewDecoder(io.LimitReader(res.Body, 64<<10)).Decode(&h) != nil || h.Database == nil {
		if res.StatusCode >= 300 && res.StatusCode < 400 {
			return fmt.Errorf("%s leitet weiter (%d). Bitte die Adresse angeben, unter der S-Check direkt erreichbar ist.", u.Host, res.StatusCode)
		}
		return fmt.Errorf("Unter %s antwortet kein S-Check-Server (HTTP %d). Adresse prüfen.", u.Host, res.StatusCode)
	}
	if !*h.Database || (h.Redis != nil && !*h.Redis) {
		return fmt.Errorf("Der S-Check-Server %s ist erreichbar, aber seine Datenbank ist gerade nicht verfügbar. Bitte den Administrator informieren.", u.Host)
	}
	return nil
}

// explainNetErr turns transport errors into messages a user can act on.
func explainNetErr(err error, u *url.URL) string {
	host := u.Hostname()
	var dnsErr *net.DNSError
	var certErr *tls.CertificateVerificationError
	var unknownCA x509.UnknownAuthorityError
	var hostErr x509.HostnameError
	var invalid x509.CertificateInvalidError
	var recErr tls.RecordHeaderError
	if strings.Contains(err.Error(), "proxyconnect") || strings.Contains(err.Error(), "Proxy Authentication") || isProxyRefusal(err) {
		return fmt.Sprintf("Der Proxy-Server dieses Rechners hat die Verbindung zu %s abgelehnt. Die IT muss die Adresse freigeben oder für den Proxy eine Ausnahme (NO_PROXY) eintragen.", u.Host)
	}
	switch {
	case errors.As(err, &dnsErr):
		return fmt.Sprintf("Der Servername „%s“ wurde nicht gefunden. Schreibweise und Netzwerkverbindung (VPN) prüfen.", host)
	case errors.As(err, &unknownCA):
		return fmt.Sprintf("Das Zertifikat von %s stammt von einer unbekannten Stelle. Die IT muss das Stammzertifikat in Windows hinterlegen.", host)
	case errors.As(err, &hostErr):
		return fmt.Sprintf("Das Zertifikat des Servers gilt nicht für %s. Adresse prüfen.", host)
	case errors.As(err, &invalid):
		return fmt.Sprintf("Das Zertifikat von %s ist ungültig oder abgelaufen.", host)
	case errors.As(err, &certErr):
		return fmt.Sprintf("Das Zertifikat von %s konnte nicht geprüft werden.", host)
	case errors.As(err, &recErr) || strings.Contains(err.Error(), "HTTP response to HTTPS client"):
		return fmt.Sprintf("%s spricht kein HTTPS. Falls der Server nur HTTP anbietet, die Adresse mit http:// beginnen.", u.Host)
	case errors.Is(err, syscall.ECONNREFUSED) || strings.Contains(err.Error(), "refused"):
		return fmt.Sprintf("%s nimmt keine Verbindung an. Läuft der S-Check-Dienst, und stimmt der Port?", u.Host)
	case errors.Is(err, context.DeadlineExceeded) || os.IsTimeout(err):
		return fmt.Sprintf("%s antwortet nicht (Zeitüberschreitung). Netzwerk oder VPN prüfen.", u.Host)
	}
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	return fmt.Sprintf("Keine Verbindung zu %s (%v).", u.Host, err)
}

// isProxyRefusal detects a CONNECT that the HTTP proxy answered with an error status.
func isProxyRefusal(err error) bool {
	msg := err.Error()
	if os.Getenv("HTTPS_PROXY") == "" && os.Getenv("https_proxy") == "" && os.Getenv("HTTP_PROXY") == "" && os.Getenv("http_proxy") == "" {
		return false
	}
	for _, s := range []string{": Forbidden", ": Bad Gateway", ": Service Unavailable", ": Proxy Authentication Required"} {
		if strings.HasSuffix(msg, s) || strings.Contains(msg, s+" (") {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

const forbiddenPage = `<!doctype html><html lang="de"><meta charset="utf-8"><title>S-Check</title>
<body style="font:15px/1.5 'Segoe UI',system-ui,sans-serif;max-width:36em;margin:15vh auto;padding:0 1em;color:#1d2330">
<h1 style="font-size:20px">S-Check bitte über das Startmenü öffnen</h1>
<p>Diese Adresse gehört zur S-Check-Anwendung auf diesem Rechner und ist nur in deren Fenster nutzbar.</p></body></html>`
