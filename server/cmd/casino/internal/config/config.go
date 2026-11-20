// Package config loads the casino service configuration from the environment.
// Every knob has a safe default so the service runs out of the box against the
// shared telesrv Postgres and Admin API.
package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
)

// Config is the resolved runtime configuration for the casino service.
type Config struct {
	Addr string // HTTP listen address for the mini app

	// PostgresDSN points at the shared telesrv database. The casino stores its
	// own casino_* tables there and reads peer_star_gifts for NFT escrow.
	PostgresDSN string

	// AdminAPIURL/AdminAPIToken target the main server's Admin API, used to
	// move stars in and out of player accounts (the same gateway DeltaMent
	// uses). Empty token disables star deposit/withdraw.
	AdminAPIURL   string
	AdminAPIToken string

	// RelayerUsername is the fixed service account that escrows NFTs while they
	// are deposited. Seeded by a migration; mirror of DeltaMent's relayer.
	RelayerUsername string

	// BotToken verifies the Telegram Mini App init_data (the HMAC secret the
	// client sends on every request). Empty means the API trusts a dev header.
	BotToken string

	// ChipsPerStar is how many casino chips make one star. Bets and payouts are
	// fractional, so the ledger counts integer chips and converts to whole
	// stars only at the deposit/withdraw boundary.
	ChipsPerStar int64

	// Actor is the admin-command actor label stamped on star movements.
	Actor string
}

// Load reads the configuration from the environment.
func Load() (Config, error) {
	chipsPerStar, err := intEnv("CASINO_CHIPS_PER_STAR", 100)
	if err != nil || chipsPerStar <= 0 {
		return Config{}, errors.New("CASINO_CHIPS_PER_STAR must be a positive integer")
	}
	c := Config{
		Addr:            env("CASINO_ADDR", "127.0.0.1:7100"),
		PostgresDSN:     firstEnv("CASINO_POSTGRES_DSN", "TELESRV_POSTGRES_DSN"),
		AdminAPIURL:     env("CASINO_ADMIN_API_URL", "http://10.89.2.1:2599"),
		AdminAPIToken:   firstEnv("CASINO_ADMIN_API_TOKEN", "TELESRV_ADMIN_API_TOKEN"),
		RelayerUsername: env("CASINO_RELAYER_USERNAME", "casino_relayer"),
		BotToken:        env("CASINO_BOT_TOKEN", ""),
		ChipsPerStar:    chipsPerStar,
		Actor:           env("CASINO_ACTOR", "casino"),
	}
	if c.PostgresDSN == "" {
		return Config{}, errors.New("CASINO_POSTGRES_DSN (or TELESRV_POSTGRES_DSN) is required")
	}
	return c, nil
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

func intEnv(key string, fallback int64) (int64, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	return strconv.ParseInt(raw, 10, 64)
}
