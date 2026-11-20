package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"time"

	"telesrv/cmd/deltament/internal/config"
	"telesrv/cmd/deltament/internal/domain"
	"telesrv/cmd/deltament/internal/service"
)

type fileServer struct {
	Slug         string  `json:"slug"`
	Name         string  `json:"name"`
	Kind         string  `json:"kind"`
	APIURL       string  `json:"api_url"`
	Secret       string  `json:"secret"`
	Contact      string  `json:"contact"`
	StarsPerGram float64 `json:"stars_per_gram"`
}

type serverRegistrar interface {
	BySlug(ctx context.Context, slug string) (domain.Server, error)
	Register(ctx context.Context, in service.RegisterInput) (domain.Server, string, error)
	SetStatus(ctx context.Context, id int64, status domain.ServerStatus) error
}

// самый лёгкий способ привязать свою приватку: вписать её в servers.json
func loadServersFile(ctx context.Context, path string, servers serverRegistrar, log *slog.Logger) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var list []fileServer
	if err := json.Unmarshal(raw, &list); err != nil {
		return err
	}
	for _, s := range list {
		s.Secret = os.ExpandEnv(s.Secret)
		if _, err := servers.BySlug(ctx, s.Slug); err == nil {
			continue
		} else if !errors.Is(err, domain.ErrNotFound) {
			return err
		}
		srv, key, err := servers.Register(ctx, service.RegisterInput(s))
		if err != nil {
			log.Warn("server from file skipped", "slug", s.Slug, "err", err)
			continue
		}
		// сервер из файла добавил сам владелец, сразу включаем
		if srv.Status != domain.ServerActive {
			if err := servers.SetStatus(ctx, srv.ID, domain.ServerActive); err != nil {
				return err
			}
		}
		log.Info("server bound from file, save the operator key, it is shown once", "slug", srv.Slug, "operator_key", key)
	}
	return nil
}

// Подключает локальный DeltaGram через Admin API при первом запуске.
// Работает в фоне: DeltaGram и Postgres могут подниматься позже Deltament.
func bootstrapDefaultServer(ctx context.Context, cfg config.Config, servers serverRegistrar, log *slog.Logger) {
	if cfg.DefaultServerToken == "" {
		log.Info("default DeltaGram autoconnect is off: set TELESRV_ADMIN_API_TOKEN or DELTAMENT_DEFAULT_SERVER_TOKEN")
		return
	}
	slug := cfg.DefaultServerSlug
	if slug == "" {
		slug = "deltagram"
	}
	delay := 2 * time.Second
	for attempt := 1; ; attempt++ {
		if _, err := servers.BySlug(ctx, slug); err == nil {
			log.Info("default server already connected", "slug", slug)
			return
		}
		srv, key, err := servers.Register(ctx, service.RegisterInput{
			Slug:         slug,
			Name:         cfg.DefaultServerName,
			Kind:         "gramsrv",
			APIURL:       cfg.DefaultServerURL,
			Secret:       cfg.DefaultServerToken,
			Contact:      cfg.DefaultServerContact,
			StarsPerGram: cfg.DefaultServerRate,
		})
		if err == nil {
			if srv.Status != domain.ServerActive {
				if serr := servers.SetStatus(ctx, srv.ID, domain.ServerActive); serr != nil {
					log.Error("default server activation failed", "slug", slug, "err", serr)
					return
				}
			}
			log.Info("default DeltaGram server connected; operator key is shown once",
				"slug", slug, "api_url", cfg.DefaultServerURL, "operator_key", key)
			return
		}
		if attempt == 1 || attempt%6 == 0 {
			log.Warn("default server autoconnect is waiting for DeltaGram Admin API", "slug", slug, "api_url", cfg.DefaultServerURL, "attempt", attempt, "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		if delay < 30*time.Second {
			delay += 2 * time.Second
		}
	}
}
