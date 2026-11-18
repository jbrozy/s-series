package analysis

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"scheck/internal/s1000d"
)

func loadHTMLExport(t *testing.T) *Corpus {
	t.Helper()
	root := "../s1000d/testdata/htmlexport"
	c := &Corpus{ICNs: map[string]string{}, Dictionary: map[string]bool{}}
	err := filepath.WalkDir(root, func(p string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if s1000d.IsRaster(rel) {
			c.ICNs[s1000d.ICNKey(rel)] = rel
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		d, err := s1000d.Parse(rel, rel, data)
		if err != nil {
			return err
		}
		c.Docs = append(c.Docs, d)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	c.Docs = s1000d.MergeHTML(c.Docs)
	return c
}

func TestHTMLExport(t *testing.T) {
	c := loadHTMLExport(t)
	kinds := map[string]int{}
	for _, d := range c.Docs {
		kinds[d.Kind]++
	}
	if kinds[s1000d.KindDM] != 3 || kinds[s1000d.KindPM] != 1 {
		t.Fatalf("kinds = %v", kinds)
	}
	var remove *s1000d.Doc
	for _, d := range c.Docs {
		if d.Code.IC == "520" {
			remove = d
		}
	}
	if remove == nil || remove.Content != "procedure" || remove.TechName != "Hydraulikpumpe" || remove.InfoName != "Ausbauen" {
		t.Fatalf("procedure page: %+v", remove)
	}
	steps := remove.Root.FindAll("proceduralStep")
	if len(steps) != 6 {
		t.Errorf("steps = %d, want 6 (4 + 2 nested)", len(steps))
	}
	if remove.Root.Find("warning") == nil || remove.Root.Find("caution") == nil || remove.Root.Find("note") == nil {
		t.Errorf("admonitions missing:\n%s", remove.Root.PrettyXML())
	}
	if st := steps[1]; st.Child("caution") == nil || strings.Contains(st.Child("para").TextContent(), "knicken") {
		t.Errorf("caution not attached to step 2:\n%s", st.PrettyXML())
	}

	codes := map[string]Finding{}
	for _, f := range RunDeterministic(c, Options{Lists: true, Titles: true, References: true, Consistency: true}) {
		codes[f.Code] = f
		t.Logf("%s %s:%d %s", f.Code, f.File, f.Line, f.Message)
	}
	for _, want := range []string{"LIST_DUPLICATE", "REF_LINK_MISSING", "REF_INTERNAL_MISSING", "REF_DM_MISSING", "TITLE_VARIANT"} {
		if _, ok := codes[want]; !ok {
			t.Errorf("missing finding %s", want)
		}
	}
	lines := map[string]int{"LIST_DUPLICATE": 16, "TITLE_VARIANT": 18, "REF_DM_MISSING": 19, "REF_INTERNAL_MISSING": 14, "REF_LINK_MISSING": 11}
	for code, line := range lines {
		if f := codes[code]; f.Line != line {
			t.Errorf("%s line = %d, want %d", code, f.Line, line)
		}
	}
	if _, ok := codes["REF_ICN_MISSING"]; ok {
		t.Errorf("image should resolve to ICN")
	}
}
