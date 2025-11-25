// Package main is the entry point for the Matrix Adapter service.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/alkemio/matrix-adapter-go/internal/app"
	"github.com/alkemio/matrix-adapter-go/internal/config"
)

func main() {
	// 1. Load Config
	cfg, err := config.Load()
	if err != nil {
		panic("Failed to load config: " + err.Error())
	}

	// 2. Initialize Application
	application, err := app.NewApp(cfg)
	if err != nil {
		panic("Failed to initialize application: " + err.Error())
	}

	// 3. Start Application
	ctx := context.Background()
	if err := application.Start(ctx); err != nil {
		panic("Failed to start application: " + err.Error())
	}

	// 4. Wait for Shutdown Signal
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	// 5. Graceful Shutdown
	application.Stop(ctx)
}
