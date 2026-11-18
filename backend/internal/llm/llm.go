// Package llm is a configurable client for OpenAI-compatible model servers
// (Ollama, vLLM, LM Studio, LocalAI, llama.cpp server, OpenAI, Azure via
// gateway ...). Chat, embedding and vision models are configured
// independently.
package llm

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Endpoint configures one model.
type Endpoint struct {
	Enabled     bool              `json:"enabled"`
	BaseURL     string            `json:"baseUrl"`
	APIKey      string            `json:"apiKey,omitempty"`
	Model       string            `json:"model"`
	Temperature float64           `json:"temperature"`
	MaxTokens   int               `json:"maxTokens"`
	TimeoutSec  int               `json:"timeoutSec"`
	JSONMode    bool              `json:"jsonMode"`
	Headers     map[string]string `json:"headers,omitempty"`
}

// VisionSettings configures the body part detector.
type VisionSettings struct {
	// Provider: none | openai (vision LLM) | http (own detector service) | heuristic (skin colour)
	Provider    string   `json:"provider"`
	Endpoint    Endpoint `json:"endpoint"`
	HTTPURL     string   `json:"httpUrl"`
	HTTPAPIKey  string   `json:"httpApiKey,omitempty"`
	CoordFormat string   `json:"coordFormat"` // norm1000 | norm1 | pixel
	MaxSide     int      `json:"maxSide"`
	MinScore    float64  `json:"minScore"`
	Labels      []string `json:"labels"`
}

// Settings bundles all model settings.
type Settings struct {
	Chat      Endpoint       `json:"chat"`
	Embedding Endpoint       `json:"embedding"`
	Vision    VisionSettings `json:"vision"`
}

func (e Endpoint) Ready() bool { return e.Enabled && e.BaseURL != "" && e.Model != "" }

func (e Endpoint) client() *http.Client {
	t := e.TimeoutSec
	if t <= 0 {
		t = 120
	}
	return &http.Client{Timeout: time.Duration(t) * time.Second}
}

func (e Endpoint) url(path string) string {
	return strings.TrimRight(e.BaseURL, "/") + path
}

func (e Endpoint) do(ctx context.Context, path string, body any, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.url(path), bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if e.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.APIKey)
		req.Header.Set("api-key", e.APIKey) // Azure-style gateways
	}
	for k, v := range e.Headers {
		req.Header.Set(k, v)
	}
	resp, err := e.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("Modellserver antwortete %d: %s", resp.StatusCode, truncate(string(data), 400))
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("unerwartete Antwort des Modellservers: %w", err)
	}
	return nil
}

// Message is a chat message. Content is either a string or a []Part.
type Message struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type Part struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

type ImageURL struct {
	URL string `json:"url"`
}

// ImagePart builds an image content part from raw image bytes.
func ImagePart(mime string, data []byte) Part {
	return Part{Type: "image_url", ImageURL: &ImageURL{URL: "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)}}
}

// Chat sends a chat completion request and returns the assistant text.
func (e Endpoint) Chat(ctx context.Context, msgs []Message) (string, error) {
	if !e.Ready() {
		return "", fmt.Errorf("Modell ist nicht konfiguriert")
	}
	body := map[string]any{
		"model":       e.Model,
		"messages":    msgs,
		"temperature": e.Temperature,
		"stream":      false,
	}
	if e.MaxTokens > 0 {
		body["max_tokens"] = e.MaxTokens
	}
	if e.JSONMode {
		body["response_format"] = map[string]string{"type": "json_object"}
	}
	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := e.do(ctx, "/chat/completions", body, &resp); err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("leere Antwort des Modells")
	}
	return stripThinking(resp.Choices[0].Message.Content), nil
}

// Embed returns embeddings for the inputs.
func (e Endpoint) Embed(ctx context.Context, inputs []string) ([][]float64, error) {
	if !e.Ready() {
		return nil, fmt.Errorf("Embedding-Modell ist nicht konfiguriert")
	}
	var resp struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := e.do(ctx, "/embeddings", map[string]any{"model": e.Model, "input": inputs}, &resp); err != nil {
		return nil, err
	}
	out := make([][]float64, len(inputs))
	for i, d := range resp.Data {
		idx := d.Index
		if idx < 0 || idx >= len(out) {
			idx = i
		}
		out[idx] = d.Embedding
	}
	for i := range out {
		if out[i] == nil {
			return nil, fmt.Errorf("Embedding %d fehlt in der Antwort", i)
		}
	}
	return out, nil
}

var thinkRe = regexp.MustCompile(`(?s)<think>.*?</think>`)

func stripThinking(s string) string { return strings.TrimSpace(thinkRe.ReplaceAllString(s, "")) }

// ExtractJSON finds the first JSON object or array in a model answer
// (handles ```json fences and surrounding prose).
func ExtractJSON(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "```"); i >= 0 {
		rest := s[i+3:]
		rest = strings.TrimPrefix(rest, "json")
		if j := strings.Index(rest, "```"); j >= 0 {
			s = strings.TrimSpace(rest[:j])
		}
	}
	start := strings.IndexAny(s, "{[")
	if start < 0 {
		return s
	}
	open := s[start]
	closeCh := byte('}')
	if open == '[' {
		closeCh = ']'
	}
	depth, inStr, esc := 0, false, false
	for i := start; i < len(s); i++ {
		c := s[i]
		if inStr {
			if esc {
				esc = false
			} else if c == '\\' {
				esc = true
			} else if c == '"' {
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case open:
			depth++
		case closeCh:
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return s[start:]
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// MaskSecrets returns a copy with API keys replaced for display.
func (s Settings) MaskSecrets() Settings {
	m := s
	m.Chat.APIKey = mask(m.Chat.APIKey)
	m.Embedding.APIKey = mask(m.Embedding.APIKey)
	m.Vision.Endpoint.APIKey = mask(m.Vision.Endpoint.APIKey)
	m.Vision.HTTPAPIKey = mask(m.Vision.HTTPAPIKey)
	return m
}

const MaskedKey = "••••••••"

func mask(k string) string {
	if k == "" {
		return ""
	}
	return MaskedKey
}

// MergeSecrets keeps existing keys where the update contains the mask.
func (s Settings) MergeSecrets(old Settings) Settings {
	keep := func(n, o string) string {
		if n == MaskedKey {
			return o
		}
		return n
	}
	s.Chat.APIKey = keep(s.Chat.APIKey, old.Chat.APIKey)
	s.Embedding.APIKey = keep(s.Embedding.APIKey, old.Embedding.APIKey)
	s.Vision.Endpoint.APIKey = keep(s.Vision.Endpoint.APIKey, old.Vision.Endpoint.APIKey)
	s.Vision.HTTPAPIKey = keep(s.Vision.HTTPAPIKey, old.Vision.HTTPAPIKey)
	return s
}
