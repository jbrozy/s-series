// scheck-ci uploads an export to S-Check, waits for the analysis and exits
// with the quality gate result. Intended for CSDB export jobs and CI.
//
//	scheck-ci -url https://s-check.example -project <id> -label "Issue 004" export.zip
//
// Exit codes: 0 gate passed, 1 gate failed, 2 error or timeout.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type client struct {
	base, token string
	http        *http.Client
}

func (c *client) do(method, path string, body io.Reader, contentType string, out any) error {
	req, err := http.NewRequest(method, c.base+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		var e struct{ Error string }
		_ = json.Unmarshal(b, &e)
		if e.Error == "" {
			e.Error = strings.TrimSpace(string(b))
		}
		return fmt.Errorf("%s %s: HTTP %d: %s", method, path, res.StatusCode, e.Error)
	}
	if out != nil {
		return json.Unmarshal(b, out)
	}
	return nil
}

// upload streams all files as multipart form; folders are walked and keep
// their relative paths.
func (c *client) upload(project, label string, paths []string) (string, error) {
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		err := func() error {
			if err := mw.WriteField("label", label); err != nil {
				return err
			}
			if err := mw.WriteField("autoAnalyze", "true"); err != nil {
				return err
			}
			for _, p := range paths {
				st, err := os.Stat(p)
				if err != nil {
					return err
				}
				if !st.IsDir() {
					if err := addFile(mw, p, filepath.Base(p)); err != nil {
						return err
					}
					continue
				}
				root := filepath.Dir(filepath.Clean(p))
				err = filepath.WalkDir(p, func(fp string, d fs.DirEntry, err error) error {
					if err != nil || d.IsDir() {
						return err
					}
					rel, _ := filepath.Rel(root, fp)
					return addFile(mw, fp, filepath.ToSlash(rel))
				})
				if err != nil {
					return err
				}
			}
			return mw.Close()
		}()
		pw.CloseWithError(err)
	}()
	var snap struct {
		ID        string
		FileCount int
	}
	if err := c.do("POST", "/api/projects/"+project+"/snapshots", pr, mw.FormDataContentType(), &snap); err != nil {
		return "", err
	}
	fmt.Printf("Upload angelegt: %s (%d Dateien)\n", snap.ID, snap.FileCount)
	return snap.ID, nil
}

func addFile(mw *multipart.Writer, path, name string) error {
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="files"; filename="%s"`, strings.ReplaceAll(name, `"`, "")))
	h.Set("Content-Type", "application/octet-stream")
	w, err := mw.CreatePart(h)
	if err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(w, f)
	return err
}

type check struct {
	Label  string
	Limit  *int
	Actual int
	OK     bool
}

type release struct {
	Gate struct {
		State, Reason, RunID string
		Checks               []check
	}
	Current *struct{ Decision, UserName, Comment string }
}

func main() {
	url := flag.String("url", os.Getenv("SCHECK_URL"), "Adresse von S-Check, z. B. https://s-check.intern (oder SCHECK_URL)")
	project := flag.String("project", os.Getenv("SCHECK_PROJECT"), "Projekt-ID (oder SCHECK_PROJECT)")
	label := flag.String("label", "CI "+time.Now().Format("2006-01-02 15:04"), "Bezeichnung des Uploads")
	wait := flag.Duration("timeout", 30*time.Minute, "maximale Wartezeit auf die Analyse")
	csvOut := flag.String("csv", "", "Befundliste zusätzlich als CSV speichern")
	snapshot := flag.String("snapshot", "", "keinen Upload anlegen, sondern einen vorhandenen Upload auswerten")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Aufruf: scheck-ci [Optionen] DATEI|ORDNER …\nToken über Umgebungsvariable SCHECK_TOKEN.\nRückgabe: 0 Kriterien erfüllt, 1 nicht erfüllt, 2 Fehler.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	token := os.Getenv("SCHECK_TOKEN")
	if *url == "" || *project == "" && *snapshot == "" || token == "" || flag.NArg() == 0 && *snapshot == "" {
		flag.Usage()
		os.Exit(2)
	}
	c := &client{base: strings.TrimRight(*url, "/"), token: token, http: &http.Client{Timeout: 0}}
	code, err := run(c, *project, *label, *snapshot, *csvOut, *wait, flag.Args())
	if err != nil {
		fmt.Fprintln(os.Stderr, "Fehler:", err)
		os.Exit(2)
	}
	os.Exit(code)
}

func run(c *client, project, label, sid, csvOut string, timeout time.Duration, paths []string) (int, error) {
	var err error
	if sid == "" {
		if sid, err = c.upload(project, label, paths); err != nil {
			return 2, err
		}
	}
	deadline := time.Now().Add(timeout)
	var rel release
	last := ""
	for {
		var snap struct {
			Status, Error string
			LatestRun     *struct {
				Status, Message string
				Progress        int
			}
		}
		if err := c.do("GET", "/api/snapshots/"+sid, nil, "", &snap); err != nil {
			return 2, err
		}
		if snap.Status == "failed" {
			return 2, errors.New("Einlesen fehlgeschlagen: " + snap.Error)
		}
		if snap.LatestRun != nil {
			msg := fmt.Sprintf("Analyse: %s %d %% %s", snap.LatestRun.Status, snap.LatestRun.Progress, snap.LatestRun.Message)
			if msg != last {
				fmt.Println(msg)
				last = msg
			}
			if snap.LatestRun.Status == "failed" {
				return 2, errors.New("Analyse fehlgeschlagen")
			}
			if snap.LatestRun.Status == "done" {
				break
			}
		}
		if time.Now().After(deadline) {
			return 2, errors.New("Zeitüberschreitung beim Warten auf die Analyse")
		}
		time.Sleep(3 * time.Second)
	}
	if err := c.do("GET", "/api/snapshots/"+sid+"/release", nil, "", &rel); err != nil {
		return 2, err
	}
	if csvOut != "" && rel.Gate.RunID != "" {
		var buf bytes.Buffer
		req, _ := http.NewRequest("GET", c.base+"/api/runs/"+rel.Gate.RunID+"/findings.csv", nil)
		req.Header.Set("Authorization", "Bearer "+c.token)
		if res, err := c.http.Do(req); err == nil {
			_, _ = io.Copy(&buf, res.Body)
			res.Body.Close()
			if err := os.WriteFile(csvOut, buf.Bytes(), 0o644); err == nil {
				fmt.Println("Befundliste gespeichert:", csvOut)
			}
		}
	}
	fmt.Println("\nFreigabekriterien:")
	for _, ch := range rel.Gate.Checks {
		lim := "beliebig"
		if ch.Limit != nil {
			lim = fmt.Sprint(*ch.Limit)
		}
		mark := "ok  "
		if !ch.OK {
			mark = "NEIN"
		}
		fmt.Printf("  [%s] %-40s %5d (erlaubt: %s)\n", mark, ch.Label, ch.Actual, lim)
	}
	fmt.Printf("\nUpload: %s/#/p/%s/overview?snap=%s\n", c.base, project, sid)
	switch rel.Gate.State {
	case "passed":
		fmt.Println("Ergebnis: Kriterien erfüllt.")
		return 0, nil
	case "failed":
		fmt.Println("Ergebnis: Kriterien NICHT erfüllt.", rel.Gate.Reason)
		return 1, nil
	}
	return 2, errors.New(rel.Gate.Reason)
}
