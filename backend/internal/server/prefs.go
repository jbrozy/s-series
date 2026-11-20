package server

import (
	"encoding/json"
	"net/http"
)

// Preferences are personal settings. Unknown keys are dropped, values are
// checked, so the frontend can rely on the stored shape.
type Preferences struct {
	Theme       string `json:"theme"`     // system | light | dark
	Density     string `json:"density"`   // normal | compact
	StartPage   string `json:"startPage"` // projects | last
	Shortcuts   bool   `json:"shortcuts"` // single-key shortcuts (WCAG 2.1.4: can be turned off)
	NavHidden   bool   `json:"navHidden"` // side navigation collapsed
	LastProject string `json:"lastProject"`
}

func defaultPreferences() Preferences {
	return Preferences{Theme: "system", Density: "normal", StartPage: "projects", Shortcuts: true}
}

func parsePreferences(raw []byte) Preferences {
	p := defaultPreferences()
	_ = json.Unmarshal(raw, &p)
	return p
}

func (s *Server) handlePutPreferences(w http.ResponseWriter, r *http.Request, u *User) {
	cur := parsePreferences(u.Preferences)
	var in map[string]json.RawMessage
	if err := readJSON(r, &in); err != nil {
		fail(w, err)
		return
	}
	// merge: only the given keys change
	merged, _ := json.Marshal(cur)
	var m map[string]json.RawMessage
	_ = json.Unmarshal(merged, &m)
	for k, v := range in {
		m[k] = v
	}
	b, _ := json.Marshal(m)
	p := parsePreferences(b)
	bad := ""
	switch {
	case p.Theme != "system" && p.Theme != "light" && p.Theme != "dark":
		bad = "Design muss system, light oder dark sein."
	case p.Density != "normal" && p.Density != "compact":
		bad = "Dichte muss normal oder compact sein."
	case p.StartPage != "projects" && p.StartPage != "last":
		bad = "Startseite muss projects oder last sein."
	}
	if bad != "" {
		writeErr(w, 400, bad)
		return
	}
	out, _ := json.Marshal(p)
	if _, err := s.db.ExecContext(r.Context(), `UPDATE users SET preferences=$1 WHERE id=$2`, string(out), u.ID); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, p)
}
