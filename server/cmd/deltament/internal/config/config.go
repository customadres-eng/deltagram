package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Addr         string
	DatabaseURL  string
	SecretKey    string
	AdminToken   string
	AutoApprove  bool
	WelcomeBonus int64
	CORSOrigin   string
	StaticDir    string
	ServersFile  string

	// единственный сервер, который использует маркет
	PrimaryServerSlug string

	// автопривязка своей приватки DeltaGram при первом запуске
	DefaultServerSlug    string
	DefaultServerName    string
	DefaultServerURL     string
	DefaultServerToken   string
	DefaultServerRate    float64
	DefaultServerContact string
}

// всё берём из env, дефолты под локальный запуск
func Load() (Config, error) {
	c := Config{
		Addr:                 env("DELTAMENT_ADDR", "127.0.0.1:2700"),
		DatabaseURL:          firstEnv("DELTAMENT_DATABASE_URL", "DELTAMENT_POSTGRES_DSN", "TELESRV_POSTGRES_DSN"),
		SecretKey:            firstEnv("DELTAMENT_SECRET_KEY", "DELTAMENT_AUTH_SECRET"),
		AdminToken:           os.Getenv("DELTAMENT_ADMIN_TOKEN"),
		CORSOrigin:           os.Getenv("DELTAMENT_CORS_ORIGIN"),
		StaticDir:            os.Getenv("DELTAMENT_STATIC_DIR"),
		ServersFile:          os.Getenv("DELTAMENT_SERVERS_FILE"),
		DefaultServerSlug:    env("DELTAMENT_DEFAULT_SERVER_SLUG", "deltagram"),
		DefaultServerName:    env("DELTAMENT_DEFAULT_SERVER_NAME", "DeltaGram"),
		DefaultServerURL:     env("DELTAMENT_DEFAULT_SERVER_API_URL", "http://127.0.0.1:2599"),
		DefaultServerToken:   firstEnv("DELTAMENT_DEFAULT_SERVER_TOKEN", "TELESRV_ADMIN_API_TOKEN"),
		DefaultServerContact: os.Getenv("DELTAMENT_DEFAULT_SERVER_CONTACT"),
	}
	// по умолчанию маркет жёстко привязан к DeltaGram
	c.PrimaryServerSlug = env("DELTAMENT_PRIMARY_SERVER_SLUG", c.DefaultServerSlug)
	if c.PrimaryServerSlug == "" {
		c.PrimaryServerSlug = "deltagram"
	}
	if c.DatabaseURL == "" {
		c.DatabaseURL = "postgres://deltament:deltament@localhost:5432/deltament?sslmode=disable"
	}
	var err error
	if c.AutoApprove, err = boolEnv("DELTAMENT_AUTO_APPROVE", true); err != nil {
		return c, err
	}
	if c.WelcomeBonus, err = intEnv("DELTAMENT_WELCOME_BONUS", 0); err != nil {
		return c, err
	}
	if c.DefaultServerRate, err = floatEnv("DELTAMENT_DEFAULT_SERVER_STARS_PER_GRAM", 0); err != nil {
		return c, err
	}
	if c.SecretKey == "" {
		return c, errors.New("DELTAMENT_SECRET_KEY is required, it encrypts server tokens")
	}
	if len(c.SecretKey) < 16 {
		return c, errors.New("DELTAMENT_SECRET_KEY must be at least 16 chars")
	}
	return c, nil
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return ""
}

func boolEnv(key string, def bool) (bool, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def, errors.New(key + ": want true or false")
	}
	return b, nil
}

func intEnv(key string, def int64) (int64, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		return def, errors.New(key + ": want a positive number")
	}
	return n, nil
}

func floatEnv(key string, def float64) (float64, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil || n < 0 {
		return def, errors.New(key + ": want a positive number")
	}
	return n, nil
}
