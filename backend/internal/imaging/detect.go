package imaging

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"mime/multipart"
	"net/http"
	"sort"
	"strings"
	"time"

	"scheck/internal/llm"
)

// Detector finds body parts in an image.
type Detector interface {
	Name() string
	Detect(ctx context.Context, img *image.RGBA) ([]Detection, error)
}

// NewDetector creates the configured detector (nil if disabled).
func NewDetector(s llm.VisionSettings) Detector {
	switch s.Provider {
	case "openai":
		return &VisionLLM{S: s}
	case "http":
		return &HTTPDetector{S: s}
	case "heuristic":
		return &SkinHeuristic{}
	}
	return nil
}

var defaultLabels = []string{"face", "head", "hand", "finger", "arm", "leg", "foot", "torso", "person"}

func labelsOf(s llm.VisionSettings) []string {
	if len(s.Labels) > 0 {
		return s.Labels
	}
	return defaultLabels
}

// ---------------------------------------------------------------- Vision LLM

// VisionLLM uses an OpenAI-compatible vision model with grounding
// capabilities (e.g. Qwen2.5-VL / Qwen3-VL, Gemma 3, InternVL, GPT-4o).
type VisionLLM struct{ S llm.VisionSettings }

func (v *VisionLLM) Name() string { return "vision-llm:" + v.S.Endpoint.Model }

func (v *VisionLLM) Detect(ctx context.Context, img *image.RGBA) ([]Detection, error) {
	maxSide := v.S.MaxSide
	if maxSide <= 0 {
		maxSide = 1024
	}
	small, scale := Downscale(img, maxSide)
	sw, sh := small.Bounds().Dx(), small.Bounds().Dy()
	coord := map[string]string{
		"norm1000": "integers normalized to the range 0-1000 relative to image width and height",
		"norm1":    "floats normalized to the range 0-1 relative to image width and height",
		"pixel":    fmt.Sprintf("absolute pixel coordinates; the image is %d pixels wide and %d pixels high", sw, sh),
		"gemini":   "integers normalized to 0-1000 in the order [ymin, xmin, ymax, xmax]",
	}
	cf := v.S.CoordFormat
	if _, ok := coord[cf]; !ok {
		cf = "norm1000"
	}
	order := "[x1, y1, x2, y2]"
	if cf == "gemini" {
		order = "[ymin, xmin, ymax, xmax]"
	}
	prompt := fmt.Sprintf(`You inspect illustrations and photos for technical documentation (S1000D).
Find EVERY visible human body part, including drawn, stylized or partially visible ones (line art counts).
Allowed labels: %s.
Prefer the most specific label (a hand holding a tool is "hand", not "person").
Return ONLY JSON in this form:
{"detections":[{"label":"hand","box":%s,"score":0.9}]}
Coordinates are %s.
If there are no human body parts, return {"detections":[]}.`, strings.Join(labelsOf(v.S), ", "), order, coord[cf])

	ep := v.S.Endpoint
	ep.JSONMode = false // many vision servers do not support response_format with images
	if ep.MaxTokens == 0 {
		ep.MaxTokens = 1024
	}
	answer, err := ep.Chat(ctx, []llm.Message{{Role: "user", Content: []llm.Part{
		llm.ImagePart("image/png", PNGBytes(small)),
		{Type: "text", Text: prompt},
	}}})
	if err != nil {
		return nil, err
	}
	raw, err := parseDetections(answer)
	if err != nil {
		return nil, fmt.Errorf("Antwort des Vision-Modells nicht lesbar: %w", err)
	}
	W, H := img.Bounds().Dx(), img.Bounds().Dy()
	var out []Detection
	for _, r := range raw {
		b := r.box
		var x1, y1, x2, y2 float64
		switch cf {
		case "norm1000":
			x1, y1, x2, y2 = b[0]/1000*float64(W), b[1]/1000*float64(H), b[2]/1000*float64(W), b[3]/1000*float64(H)
		case "gemini":
			y1, x1, y2, x2 = b[0]/1000*float64(H), b[1]/1000*float64(W), b[2]/1000*float64(H), b[3]/1000*float64(W)
		case "norm1":
			x1, y1, x2, y2 = b[0]*float64(W), b[1]*float64(H), b[2]*float64(W), b[3]*float64(H)
		case "pixel":
			x1, y1, x2, y2 = b[0]*scale, b[1]*scale, b[2]*scale, b[3]*scale
		}
		box := ClampBox([4]int{int(x1), int(y1), int(x2 + 0.5), int(y2 + 0.5)}, W, H)
		if box[2]-box[0] < 3 || box[3]-box[1] < 3 {
			continue
		}
		score := r.score
		if score == 0 {
			score = 0.5
		}
		if score < v.S.MinScore {
			continue
		}
		out = append(out, Detection{Label: NormalizeLabel(r.label), Box: box, Score: score, Source: "auto"})
	}
	return out, nil
}

type rawDet struct {
	label string
	box   [4]float64
	score float64
}

// parseDetections accepts several common grounding output formats:
// {"detections":[{"label","box"|"bbox"|"bbox_2d"|"box_2d", "score"}]} or a bare array.
func parseDetections(answer string) ([]rawDet, error) {
	js := llm.ExtractJSON(answer)
	var items []map[string]any
	var wrapper map[string]any
	if err := json.Unmarshal([]byte(js), &wrapper); err == nil {
		for _, k := range []string{"detections", "objects", "results", "boxes"} {
			if arr, ok := wrapper[k].([]any); ok {
				for _, a := range arr {
					if m, ok := a.(map[string]any); ok {
						items = append(items, m)
					}
				}
				break
			}
		}
	} else if err := json.Unmarshal([]byte(js), &items); err != nil {
		return nil, err
	}
	var out []rawDet
	for _, m := range items {
		var r rawDet
		for _, k := range []string{"label", "name", "class", "category"} {
			if s, ok := m[k].(string); ok {
				r.label = s
				break
			}
		}
		for _, k := range []string{"box", "bbox", "bbox_2d", "box_2d", "bounding_box"} {
			if arr, ok := m[k].([]any); ok && len(arr) == 4 {
				for i := 0; i < 4; i++ {
					f, _ := arr[i].(float64)
					r.box[i] = f
				}
				break
			}
		}
		for _, k := range []string{"score", "confidence", "conf"} {
			if f, ok := m[k].(float64); ok {
				r.score = f
				break
			}
		}
		if r.label != "" && (r.box[2] != 0 || r.box[3] != 0) {
			out = append(out, r)
		}
	}
	return out, nil
}

// ---------------------------------------------------------------- HTTP

// HTTPDetector calls an own detection service (e.g. YOLO, Sapiens,
// GroundingDINO). Contract: POST multipart/form-data with field "image"
// (PNG) and "labels" (comma separated); response
// {"detections":[{"label":"hand","box":[x1,y1,x2,y2],"score":0.93}]}
// with pixel coordinates of the uploaded image.
type HTTPDetector struct{ S llm.VisionSettings }

func (h *HTTPDetector) Name() string { return "http:" + h.S.HTTPURL }

func (h *HTTPDetector) Detect(ctx context.Context, img *image.RGBA) ([]Detection, error) {
	if h.S.HTTPURL == "" {
		return nil, fmt.Errorf("URL des Detektor-Dienstes fehlt")
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("image", "image.png")
	_, _ = fw.Write(PNGBytes(img))
	_ = mw.WriteField("labels", strings.Join(labelsOf(h.S), ","))
	_ = mw.Close()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.S.HTTPURL, &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if h.S.HTTPAPIKey != "" {
		req.Header.Set("Authorization", "Bearer "+h.S.HTTPAPIKey)
	}
	timeout := h.S.Endpoint.TimeoutSec
	if timeout <= 0 {
		timeout = 120
	}
	resp, err := (&http.Client{Timeout: time.Duration(timeout) * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Detektor antwortete %d: %s", resp.StatusCode, string(data))
	}
	raw, err := parseDetections(string(data))
	if err != nil {
		return nil, err
	}
	W, H := img.Bounds().Dx(), img.Bounds().Dy()
	var out []Detection
	for _, r := range raw {
		if r.score > 0 && r.score < h.S.MinScore {
			continue
		}
		box := ClampBox([4]int{int(r.box[0]), int(r.box[1]), int(r.box[2]), int(r.box[3])}, W, H)
		out = append(out, Detection{Label: NormalizeLabel(r.label), Box: box, Score: r.score, Source: "auto"})
	}
	return out, nil
}

// ---------------------------------------------------------------- Heuristic

// SkinHeuristic is a dependency-free fallback that proposes skin coloured
// regions in photos as candidates. It does not work for line art and
// produces false positives; candidates must be reviewed.
type SkinHeuristic struct{}

func (s *SkinHeuristic) Name() string { return "heuristic:skin" }

func (s *SkinHeuristic) Detect(_ context.Context, img *image.RGBA) ([]Detection, error) {
	small, scale := Downscale(img, 400)
	w, h := small.Bounds().Dx(), small.Bounds().Dy()
	mask := make([]bool, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			o := y*small.Stride + x*4
			r, g, b := float64(small.Pix[o]), float64(small.Pix[o+1]), float64(small.Pix[o+2])
			mx, mn := max(r, g, b), min(r, g, b)
			cb := 128 - 0.168736*r - 0.331264*g + 0.5*b
			cr := 128 + 0.5*r - 0.418688*g - 0.081312*b
			mask[y*w+x] = r > 95 && g > 40 && b > 20 && mx-mn > 15 && r-g > 15 && r > b &&
				cb >= 77 && cb <= 127 && cr >= 133 && cr <= 173
		}
	}
	mask = morph(mask, w, h, 1, false) // erode
	mask = morph(mask, w, h, 2, true)  // dilate
	comps := components(mask, w, h)
	total := w * h
	var out []Detection
	for _, c := range comps {
		if c.area < 60 || float64(c.area) < 0.004*float64(total) || float64(c.area) > 0.6*float64(total) {
			continue
		}
		bw, bh := c.x2-c.x1+1, c.y2-c.y1+1
		fill := float64(c.area) / float64(bw*bh)
		if fill < 0.2 {
			continue
		}
		W, H := img.Bounds().Dx(), img.Bounds().Dy()
		box := ClampBox([4]int{int(float64(c.x1) * scale), int(float64(c.y1) * scale), int(float64(c.x2+1) * scale), int(float64(c.y2+1) * scale)}, W, H)
		out = append(out, Detection{Label: "skin", Box: box, Score: fill, Source: "auto"})
	}
	sort.Slice(out, func(i, j int) bool {
		ai := (out[i].Box[2] - out[i].Box[0]) * (out[i].Box[3] - out[i].Box[1])
		aj := (out[j].Box[2] - out[j].Box[0]) * (out[j].Box[3] - out[j].Box[1])
		return ai > aj
	})
	if len(out) > 10 {
		out = out[:10]
	}
	return out, nil
}

type comp struct{ x1, y1, x2, y2, area int }

func components(mask []bool, w, h int) []comp {
	seen := make([]bool, len(mask))
	var out []comp
	stack := []int{}
	for i := range mask {
		if !mask[i] || seen[i] {
			continue
		}
		c := comp{x1: w, y1: h, x2: -1, y2: -1}
		stack = append(stack[:0], i)
		seen[i] = true
		for len(stack) > 0 {
			p := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			x, y := p%w, p/w
			c.area++
			c.x1, c.y1, c.x2, c.y2 = min(c.x1, x), min(c.y1, y), max(c.x2, x), max(c.y2, y)
			for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				nx, ny := x+d[0], y+d[1]
				if nx < 0 || ny < 0 || nx >= w || ny >= h {
					continue
				}
				q := ny*w + nx
				if mask[q] && !seen[q] {
					seen[q] = true
					stack = append(stack, q)
				}
			}
		}
		out = append(out, c)
	}
	return out
}
