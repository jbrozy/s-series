// Command s-check is the installable desktop version of the S-Check user
// interface. It contains the interface itself and connects to an S-Check
// backend that runs separately; no project data is stored on this computer.
//
// On start it serves the embedded interface on 127.0.0.1, forwards /api to the
// backend chosen at sign-in and opens an application window (Microsoft Edge in
// app mode with its own profile, the engine behind WebView2).
package main

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

var version = "dev"

//go:embed all:web
var webFS embed.FS

type instanceInfo struct {
	PID   int    `json:"pid"`
	Port  int    `json:"port"`
	Token string `json:"token"`
}

func main() {
	serveOnly := flag.Bool("serve-only", false, "kein Fenster öffnen, nur die Start-Adresse ausgeben (Diagnose, Tests)")
	serverFlag := flag.String("server", "", "Serveradresse vorgeben, z. B. https://s-check.firma.de")
	portFlag := flag.Int("port", 0, "lokaler Port (Standard: beim ersten Start gewählt und gemerkt)")
	dataFlag := flag.String("data", "", "eigener Ordner für Einstellungen, Fensterprofil und Protokoll")
	flag.Parse()

	cfgDir, localDir, err := dirs()
	if *dataFlag != "" {
		cfgDir, localDir, err = *dataFlag, *dataFlag, nil
	}
	if err != nil {
		fatal("Benutzerordner nicht gefunden: %v", err)
	}
	if err := os.MkdirAll(localDir, 0o700); err != nil {
		fatal("Ordner %s kann nicht angelegt werden: %v", localDir, err)
	}
	closeLog := openLog(filepath.Join(localDir, "s-check.log"), *serveOnly)
	defer closeLog()
	log.Printf("S-Check Desktop %s startet", version)

	instFile := filepath.Join(localDir, "instance.json")
	if !*serveOnly {
		if inst, ok := runningInstance(instFile); ok {
			// second start: open another window of the running app
			log.Printf("läuft bereits (Port %d), öffne weiteres Fenster", inst.Port)
			if err := openWindow(startURL(inst.Port, inst.Token), filepath.Join(localDir, "Fenster"), false); err != nil {
				fatal("Fenster konnte nicht geöffnet werden: %v", err)
			}
			return
		}
	}

	pol := readPolicy()
	cfg, err := loadConfig(cfgDir, pol)
	if err != nil {
		fatal("Einstellungen nicht lesbar: %v", err)
	}
	if *serverFlag != "" {
		if s, err := normalizeServer(*serverFlag); err == nil && (!pol.Locked || s == pol.Server) {
			_ = cfg.setServer(s)
		}
	}

	ln, port := listen(*portFlag, cfg.get().Port)
	if ln == nil {
		fatal("Kein freier lokaler Port verfügbar.")
	}
	_ = cfg.setPort(port)
	token := randomToken()
	web, _ := fs.Sub(webFS, "web")
	host := "127.0.0.1:" + strconv.Itoa(port)
	a, err := newApp(host, token, version, cfg, web)
	if err != nil {
		fatal("Oberfläche fehlt im Programm: %v", err)
	}
	srv := &http.Server{Handler: a, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 5 * time.Minute, MaxHeaderBytes: 1 << 20}
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("lokaler Server: %v", err)
		}
	}()

	if *serveOnly {
		fmt.Println(startURL(port, token))
		select {}
	}

	writeInstance(instFile, instanceInfo{PID: os.Getpid(), Port: port, Token: token})
	defer os.Remove(instFile)

	profile := filepath.Join(localDir, "Fenster")
	if err := openWindow(startURL(port, token), profile, true); err != nil {
		log.Printf("Fenster: %v", err)
		fatal("S-Check konnte kein Fenster öffnen: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	log.Printf("beendet")
}

func startURL(port int, token string) string {
	return fmt.Sprintf("http://127.0.0.1:%d/_desktop/start?t=%s", port, token)
}

func randomToken() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// listen keeps the port of the previous start where possible, because the
// window stores sort orders and filters per address.
func listen(flagPort, savedPort int) (net.Listener, int) {
	for _, p := range []int{flagPort, savedPort} {
		if p <= 0 {
			continue
		}
		if ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(p)); err == nil {
			return ln, p
		}
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, 0
	}
	return ln, ln.Addr().(*net.TCPAddr).Port
}

func runningInstance(file string) (instanceInfo, bool) {
	var inst instanceInfo
	b, err := os.ReadFile(file)
	if err != nil || json.Unmarshal(b, &inst) != nil || inst.Port == 0 {
		return inst, false
	}
	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/_desktop/ping", inst.Port), nil)
	req.Host = fmt.Sprintf("127.0.0.1:%d", inst.Port)
	req.Header.Set("X-Desktop-Token", inst.Token)
	res, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
	if err != nil {
		return inst, false
	}
	res.Body.Close()
	return inst, res.StatusCode == http.StatusNoContent
}

func writeInstance(file string, inst instanceInfo) {
	b, _ := json.Marshal(inst)
	if err := os.WriteFile(file, b, 0o600); err != nil {
		log.Printf("instance: %v", err)
	}
}

// openLog writes to a small rotating file; with -serve-only also to stderr.
func openLog(file string, stderr bool) func() {
	if st, err := os.Stat(file); err == nil && st.Size() > 1<<20 {
		_ = os.Rename(file, file+".1")
	}
	f, err := os.OpenFile(file, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return func() {}
	}
	var w io.Writer = f
	if stderr {
		w = io.MultiWriter(f, os.Stderr)
	}
	log.SetOutput(w)
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	return func() { f.Close() }
}

func fatal(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	log.Print(msg)
	showError("S-Check", msg)
	os.Exit(1)
}
