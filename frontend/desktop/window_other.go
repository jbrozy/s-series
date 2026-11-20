//go:build !windows

package main

// Outside Windows the app is only built for development and tests: the window
// is any Chromium found via SCHECK_BROWSER or PATH, presets come from the
// environment (SCHECK_SERVER_URL, SCHECK_LOCK_SERVER=1).

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func showError(title, text string) { fmt.Fprintf(os.Stderr, "%s: %s\n", title, text) }

func readPolicy() Policy {
	var p Policy
	if s, err := normalizeServer(os.Getenv("SCHECK_SERVER_URL")); err == nil {
		p.Server = s
		p.Locked = os.Getenv("SCHECK_LOCK_SERVER") == "1"
	}
	return p
}

func findBrowser() string {
	if b := os.Getenv("SCHECK_BROWSER"); b != "" {
		return b
	}
	for _, n := range []string{"microsoft-edge", "chromium", "chromium-browser", "google-chrome"} {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	return ""
}

func openWindow(url, profile string, wait bool) error {
	b := findBrowser()
	if b == "" {
		return errors.New("kein Chromium-Browser gefunden (SCHECK_BROWSER setzen)")
	}
	cmd := exec.Command(b, "--app="+url, "--user-data-dir="+profile, "--no-first-run", "--no-default-browser-check")
	if err := cmd.Start(); err != nil {
		return err
	}
	if !wait {
		go cmd.Wait()
		return nil
	}
	_ = cmd.Wait()
	for {
		if _, err := os.Lstat(filepath.Join(profile, "SingletonLock")); err != nil {
			return nil
		}
		time.Sleep(time.Second)
	}
}
