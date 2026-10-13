// Package main is the entry point for the Matrix Adapter service.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/alkem-io/matrix-adapter/internal/app"
	"github.com/alkem-io/matrix-adapter/internal/config"
)

func main() {
	// 1. Load Config
	cfg, err := config.Load()
	if err != nil {
		if len(os.Args) > 1 && os.Args[1] == "attachments-export" {
			fmt.Fprintln(os.Stderr, "cannot load attachment export configuration")
			os.Exit(1)
		}
		panic("Failed to load config: " + err.Error())
	}
	if len(os.Args) > 1 && os.Args[1] == "attachments-export" {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		err := app.RunAttachmentsExport(ctx, cfg, os.Args[2:])
		stop()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "attachment export complete; verify coverage against the supported-room inventory before normalization")
		return
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
