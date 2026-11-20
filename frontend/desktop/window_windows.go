package main

import (
	"errors"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32      = syscall.NewLazyDLL("user32.dll")
	messageBoxW = user32.NewProc("MessageBoxW")
)

const (
	mbOK            = 0x0
	mbIconError     = 0x10
	mbIconInfo      = 0x40
	mbSetForeground = 0x10000
)

func messageBox(title, text string, flags uintptr) {
	t, _ := syscall.UTF16PtrFromString(title)
	m, _ := syscall.UTF16PtrFromString(text)
	messageBoxW.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), flags|mbSetForeground)
}

func showError(title, text string) { messageBox(title, text, mbOK|mbIconError) }

// findBrowser returns Microsoft Edge (part of Windows 10/11, same engine as
// WebView2) or, if it was removed, Google Chrome.
func findBrowser() string {
	for _, exe := range []string{"msedge.exe", "chrome.exe"} {
		for _, root := range []syscall.Handle{syscall.HKEY_CURRENT_USER, syscall.HKEY_LOCAL_MACHINE} {
			if p := regString(root, `SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\`+exe, ""); p != "" && fileExists(p) {
				return p
			}
		}
		for _, env := range []string{"ProgramFiles(x86)", "ProgramFiles", "LOCALAPPDATA"} {
			base := os.Getenv(env)
			if base == "" {
				continue
			}
			var p string
			if exe == "msedge.exe" {
				p = filepath.Join(base, "Microsoft", "Edge", "Application", exe)
			} else {
				p = filepath.Join(base, "Google", "Chrome", "Application", exe)
			}
			if fileExists(p) {
				return p
			}
		}
	}
	return ""
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// openWindow opens the app window. With wait it returns once the window (and
// every further window of the app) has been closed.
func openWindow(url, profile string, wait bool) error {
	browser := findBrowser()
	if browser == "" {
		if err := exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start(); err != nil {
			return err
		}
		if wait {
			messageBox("S-Check", "Microsoft Edge wurde nicht gefunden. S-Check ist deshalb in Ihrem Standardbrowser geöffnet.\n\nSchließen Sie diese Meldung, um S-Check zu beenden.", mbOK|mbIconInfo)
		}
		return nil
	}
	firstRun := !fileExists(filepath.Join(profile, "Local State"))
	args := []string{
		"--app=" + url,
		"--user-data-dir=" + profile,
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-sync",
		"--disable-background-mode",
		"--disable-features=msEdgeSidebarV2,msUndersideButton,EdgeCollections,msShowSearchInSidebar",
	}
	if firstRun {
		args = append(args, "--window-size=1440,900", "--start-maximized")
	}
	cmd := exec.Command(browser, args...)
	t0 := time.Now()
	if err := cmd.Start(); err != nil {
		return err
	}
	log.Printf("Fenster: %s (%s)", filepath.Base(browser), time.Since(t0))
	if !wait {
		go cmd.Wait()
		return nil
	}
	_ = cmd.Wait()
	// If the profile was already open, the started process hands the window
	// over and exits at once. The profile lock tells when the last window closed.
	lock := filepath.Join(profile, "lockfile")
	for {
		err := os.Remove(lock)
		if err == nil || errors.Is(err, os.ErrNotExist) {
			return nil
		}
		time.Sleep(time.Second)
	}
}
