// Package main is the entry point for the Matrix Adapter service.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alkem-io/matrix-adapter/internal/app"
	"github.com/alkem-io/matrix-adapter/internal/config"
	"github.com/alkem-io/matrix-adapter/internal/core/service"
	"github.com/alkem-io/matrix-adapter/internal/infrastructure/logger"
	"github.com/alkem-io/matrix-adapter/internal/infrastructure/matrix"
)

func main() {
	// Mode switch: `matrix-adapter sweep-devices [--idle 720h] [--dry-run]`
	// runs the nightly idle-device sweep and exits; the default mode is the
	// long-running adapter service (unchanged).
	if len(os.Args) > 1 && os.Args[1] == "sweep-devices" {
		os.Exit(runSweepDevices(os.Args[2:]))
	}

	runAdapter()
}

func runAdapter() {
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

// runSweepDevices deletes devices idle longer than --idle (default 30 days),
// printing the report as one JSON line. Exit codes: 0 on success, 1 on a
// setup failure, 2 when the sweep reported per-user failures (kept non-zero
// so the CronJob's failedJobsHistoryLimit surfaces it).
func runSweepDevices(args []string) int {
	flags := flag.NewFlagSet("sweep-devices", flag.ExitOnError)
	idle := flags.Duration("idle", 720*time.Hour, "delete devices last used longer than this ago")
	dryRun := flags.Bool("dry-run", false, "report without deleting")
	if err := flags.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, "sweep-devices: "+err.Error())
		return 1
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "sweep-devices: failed to load config: "+err.Error())
		return 1
	}
	log, err := logger.NewZapLogger(cfg.App.LogLevel)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sweep-devices: failed to initialize logger: "+err.Error())
		return 1
	}

	adapter, err := matrix.NewMautrixAdapter(cfg, log)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sweep-devices: failed to create Matrix adapter: "+err.Error())
		return 1
	}

	ctx := context.Background()
	// No HTTP listener, no event delivery — the sweep only needs the admin API.
	if err := adapter.ConnectWithoutListener(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "sweep-devices: failed to connect: "+err.Error())
		return 1
	}

	devices := service.NewDeviceService(adapter, log)
	report, err := devices.Sweep(ctx, *idle, *dryRun)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sweep-devices: sweep failed: "+err.Error())
		return 1
	}

	// One machine-readable JSON line — the CronJob's log IS the report.
	line, err := json.Marshal(report)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sweep-devices: failed to encode report: "+err.Error())
		return 1
	}
	fmt.Println(string(line))

	if report.Failed > 0 {
		return 2
	}
	return 0
}
