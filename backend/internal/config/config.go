// Package config reads the backend configuration from environment variables.
package config

import (
	"os"
	"strconv"
	"strings"

	"scheck/internal/llm"
)

type Config struct {
	ListenAddr    string
	DatabaseURL   string
	RedisAddr     string
	RedisPassword string
	RedisDB       int
	DataDir       string
	MigrationsDir string
	Workers       int
	MaxUploadMB   int64
	SessionHours  int
	CookieSecure  bool
	AllowSignup   bool
	AdminEmail    string
	AdminPassword string
	DefaultModels llm.Settings
}

func env(k, def string) string {
	if v, ok := os.LookupEnv(k); ok && v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v, err := strconv.Atoi(env(k, "")); err == nil {
		return v
	}
	return def
}

func envBool(k string, def bool) bool {
	v := strings.ToLower(env(k, ""))
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return def
}

func envFloat(k string, def float64) float64 {
	if v, err := strconv.ParseFloat(env(k, ""), 64); err == nil {
		return v
	}
	return def
}

func endpoint(prefix string, def llm.Endpoint) llm.Endpoint {
	e := def
	e.BaseURL = env(prefix+"_BASE_URL", def.BaseURL)
	e.APIKey = env(prefix+"_API_KEY", def.APIKey)
	e.Model = env(prefix+"_MODEL", def.Model)
	e.Temperature = envFloat(prefix+"_TEMPERATURE", def.Temperature)
	e.MaxTokens = envInt(prefix+"_MAX_TOKENS", def.MaxTokens)
	e.TimeoutSec = envInt(prefix+"_TIMEOUT_SEC", def.TimeoutSec)
	e.JSONMode = envBool(prefix+"_JSON_MODE", def.JSONMode)
	e.Enabled = envBool(prefix+"_ENABLED", e.BaseURL != "" && e.Model != "")
	return e
}

func Load() Config {
	c := Config{
		ListenAddr:    env("SCHECK_LISTEN", ":8080"),
		DatabaseURL:   env("DATABASE_URL", "postgres://scheck:scheck@localhost:5432/scheck?sslmode=disable"),
		RedisAddr:     env("REDIS_ADDR", "localhost:6379"),
		RedisPassword: env("REDIS_PASSWORD", ""),
		RedisDB:       envInt("REDIS_DB", 0),
		DataDir:       env("SCHECK_DATA_DIR", "./data"),
		MigrationsDir: env("SCHECK_MIGRATIONS_DIR", "../database/migrations"),
		Workers:       envInt("SCHECK_WORKERS", 2),
		MaxUploadMB:   int64(envInt("SCHECK_MAX_UPLOAD_MB", 2048)),
		SessionHours:  envInt("SCHECK_SESSION_HOURS", 168),
		CookieSecure:  envBool("SCHECK_COOKIE_SECURE", false),
		AllowSignup:   envBool("SCHECK_ALLOW_SIGNUP", true),
		AdminEmail:    env("SCHECK_ADMIN_EMAIL", ""),
		AdminPassword: env("SCHECK_ADMIN_PASSWORD", ""),
	}
	c.DefaultModels = llm.Settings{
		Chat:      endpoint("SCHECK_CHAT", llm.Endpoint{Temperature: 0.1, MaxTokens: 2048, TimeoutSec: 180, JSONMode: true}),
		Embedding: endpoint("SCHECK_EMBED", llm.Endpoint{TimeoutSec: 60}),
		Vision: llm.VisionSettings{
			Provider:    env("SCHECK_VISION_PROVIDER", "heuristic"),
			Endpoint:    endpoint("SCHECK_VISION", llm.Endpoint{Temperature: 0, MaxTokens: 1024, TimeoutSec: 180}),
			HTTPURL:     env("SCHECK_DETECTOR_URL", ""),
			HTTPAPIKey:  env("SCHECK_DETECTOR_API_KEY", ""),
			CoordFormat: env("SCHECK_VISION_COORDS", "norm1000"),
			MaxSide:     envInt("SCHECK_VISION_MAX_SIDE", 1024),
			MinScore:    envFloat("SCHECK_VISION_MIN_SCORE", 0.25),
		},
	}
	return c
}
