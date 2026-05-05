package app

import (
	"log"
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddr         string
	DatabaseURL      string
	AllowedOrigins   string
	AutoMigrate      bool
	ShutdownTimeout  time.Duration
	SearchCacheTTL   time.Duration
	SpotifyClientID  string
	SpotifySecret    string
	SpotifyTokenURL  string
	SpotifyAPIBase   string
	DefaultRoomEvery time.Duration
}

func MustLoadConfig() Config {
	cfg := Config{
		HTTPAddr:         envOr("HTTP_ADDR", ":8080"),
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		AllowedOrigins:   envOr("ALLOWED_ORIGINS", "*"),
		AutoMigrate:      envBool("AUTO_MIGRATE", true),
		ShutdownTimeout:  envDuration("SHUTDOWN_TIMEOUT", 10*time.Second),
		SearchCacheTTL:   envDuration("SEARCH_CACHE_TTL", 30*time.Second),
		SpotifyClientID:  os.Getenv("SPOTIFY_CLIENT_ID"),
		SpotifySecret:    os.Getenv("SPOTIFY_CLIENT_SECRET"),
		SpotifyTokenURL:  envOr("SPOTIFY_TOKEN_URL", "https://accounts.spotify.com/api/token"),
		SpotifyAPIBase:   envOr("SPOTIFY_API_BASE", "https://api.spotify.com/v1"),
		DefaultRoomEvery: envDuration("DEFAULT_RECALC_INTERVAL", 5*time.Second),
	}
	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	return cfg
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return v
}

func envDuration(key string, fallback time.Duration) time.Duration {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	v, err := time.ParseDuration(raw)
	if err == nil {
		return v
	}
	if seconds, err := strconv.Atoi(raw); err == nil {
		return time.Duration(seconds) * time.Second
	}
	return fallback
}
