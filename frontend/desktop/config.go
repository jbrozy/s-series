package main

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const maxRecent = 8

// Config is the per-user state of the desktop app. It holds no business data:
// everything the user works on lives in the S-Check backend.
type Config struct {
	Server string   `json:"server"`
	Recent []string `json:"recent"`
	Port   int      `json:"port"`
}

// Policy is what the IT department or the installer preset (registry on Windows).
type Policy struct {
	Server string // address to use when the user has not chosen one
	Locked bool   // the user may not change the address
}

type configStore struct {
	mu     sync.Mutex
	path   string
	cfg    Config
	policy Policy
}

// dirs returns the roaming config directory and the local directory for
// window profile, log and instance file.
func dirs() (cfgDir, localDir string, err error) {
	c, err := os.UserConfigDir()
	if err != nil {
		return "", "", err
	}
	l, err := os.UserCacheDir()
	if err != nil {
		return "", "", err
	}
	return filepath.Join(c, "S-Check"), filepath.Join(l, "S-Check"), nil
}

func loadConfig(dir string, pol Policy) (*configStore, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	s := &configStore{path: filepath.Join(dir, "desktop.json"), policy: pol}
	if b, err := os.ReadFile(s.path); err == nil {
		_ = json.Unmarshal(b, &s.cfg)
	}
	if n, err := normalizeServer(s.cfg.Server); err == nil {
		s.cfg.Server = n
	} else {
		s.cfg.Server = ""
	}
	if pol.Server != "" && (pol.Locked || s.cfg.Server == "") {
		s.cfg.Server = pol.Server
	}
	return s, nil
}

func (s *configStore) get() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.cfg
	c.Recent = append([]string(nil), s.cfg.Recent...)
	return c
}

func (s *configStore) setServer(server string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.Server = server
	rec := []string{server}
	for _, r := range s.cfg.Recent {
		if r != server && len(rec) < maxRecent {
			rec = append(rec, r)
		}
	}
	s.cfg.Recent = rec
	return s.saveLocked()
}

func (s *configStore) setPort(p int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cfg.Port == p {
		return nil
	}
	s.cfg.Port = p
	return s.saveLocked()
}

func (s *configStore) saveLocked() error {
	b, err := json.MarshalIndent(s.cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

var errServerFormat = errors.New("Bitte eine Serveradresse wie https://s-check.firma.de angeben.")

// normalizeServer turns user input into a base URL without trailing slash.
// Without a scheme HTTPS is assumed; plain HTTP has to be typed explicitly.
func normalizeServer(in string) (string, error) {
	s := strings.TrimSpace(in)
	if s == "" {
		return "", errServerFormat
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return "", errServerFormat
	}
	if u.User != nil || u.RawQuery != "" {
		return "", errServerFormat
	}
	// an address copied from the browser (…/#/login) is fine
	u.Fragment, u.RawFragment = "", ""
	p := strings.TrimRight(u.Path, "/")
	p = strings.TrimSuffix(p, "/api")
	u.Path, u.RawPath = strings.TrimRight(p, "/"), ""
	u.Host = strings.ToLower(u.Host)
	return u.String(), nil
}
