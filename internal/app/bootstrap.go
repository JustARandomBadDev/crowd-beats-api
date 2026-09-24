package app

import (
	"context"
	"net/http"

	"crowdbeats/internal/infra/cache"
	"crowdbeats/internal/infra/db"
	"crowdbeats/internal/infra/scheduler"
	spotifyinfra "crowdbeats/internal/infra/spotify"
	"crowdbeats/internal/infra/ws"
	"crowdbeats/internal/platform/auth"
	"crowdbeats/internal/usecase"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Application struct {
	Config    Config
	DB        *pgxpool.Pool
	Server    *http.Server
	Scheduler *scheduler.QueueRecalculator
}

func NewApplication(cfg Config) (*Application, error) {
	ctx := context.Background()
	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	if cfg.AutoMigrate {
		if err := db.RunMigrations(ctx, pool); err != nil {
			return nil, err
		}
	}

	store := db.NewStore(pool)
	searchCache := cache.NewSearchCache(cfg.SearchCacheTTL)
	registry := ws.NewRegistry()
	spotifyClient := spotifyinfra.NewClient(spotifyinfra.Config{
		ClientID: cfg.SpotifyClientID,
		Secret:   cfg.SpotifySecret,
		TokenURL: cfg.SpotifyTokenURL,
		APIBase:  cfg.SpotifyAPIBase,
	}, store.Spotify())
	usecases := usecase.NewServices(store, store, spotifyClient, registry, searchCache, auth.NewTokenManager())

	router := NewRouter(cfg, usecases, registry, pool)
	server := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: router,
	}
	worker := scheduler.NewQueueRecalculator(usecases)
	worker.Start()

	return &Application{
		Config:    cfg,
		DB:        pool,
		Server:    server,
		Scheduler: worker,
	}, nil
}

func (a *Application) ListenAndServe() error {
	return a.Server.ListenAndServe()
}

func (a *Application) Shutdown(ctx context.Context) error {
	if a.Scheduler != nil {
		a.Scheduler.Stop()
	}
	if a.DB != nil {
		defer a.DB.Close()
	}
	return a.Server.Shutdown(ctx)
}
