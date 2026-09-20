package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"

	"crowdbeats/internal/app"
)

func main() {
	cfg := app.MustLoadConfig()
	application, err := app.NewApplication(cfg)
	if err != nil {
		log.Fatalf("bootstrap server: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := application.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown error: %v", err)
		}
	}()

	log.Printf("crowd beats api listening on %s", cfg.HTTPAddr)
	if err := application.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("listen: %v", err)
	}
}
