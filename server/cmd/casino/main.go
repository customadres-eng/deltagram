// Command casino runs the DeltaGram casino Mini App service: a Telegram Mini
// App bot that hosts crash / mines / upgrade / slots games, with star and NFT
// deposits and withdrawals. It shares the telesrv Postgres and moves stars
// through the main server's Admin API (the same gateway DeltaMent uses).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"embed"

	"telesrv/cmd/casino/internal/config"
	"telesrv/cmd/casino/internal/httpapi"
	"telesrv/cmd/casino/internal/network"
	"telesrv/cmd/casino/internal/service"
	"telesrv/cmd/casino/internal/store/postgres"
)

//go:embed web
var webAssets embed.FS

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("casino stopped", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := postgres.Open(ctx, cfg.PostgresDSN)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		return err
	}

	gw, err := network.New(cfg.AdminAPIURL, cfg.AdminAPIToken, cfg.Actor)
	if err != nil {
		return err
	}

	wallet := service.NewWallet(db, gw, cfg.ChipsPerStar, cfg.Actor)
	rounds := service.NewRoundService(db, cfg.ChipsPerStar)
	bank := service.NewNftBank(db)
	rounds.SetNftBank(bank)

	api := httpapi.New(cfg, wallet, rounds, bank)
	api.SetAssets(webAssets)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           api.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Info("casino serving", "addr", cfg.Addr, "stars_enabled", gw.Enabled())
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
