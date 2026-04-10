// Package main is the entry point for the Matrix Adapter migration tool.
//
// This tool runs one-time migrations against the Matrix homeserver.
// It is intentionally separate from the adapter service so that:
//   - Migrations run under operator control, not on every adapter restart
//   - Failures don't affect the running adapter
//   - Event storms from bulk operations don't flow through the event loop
//   - Migration logic can be iterated without redeploying the adapter
//
// Available migrations (enable via flags):
//
//	--redact-canonical-aliases  Clear m.room.canonical_alias from non-space rooms
//	--leave-conversations       Remove bot from conversation rooms (direct/group only)
//	--fix-power-levels          Set users_default=50 on rooms that have users_default=0
//	--dry-run                   Log what would be done without making changes
//
// Usage:
//
//	go run cmd/migrate/main.go --redact-canonical-aliases --dry-run
//	go run cmd/migrate/main.go --leave-conversations
//
// Prerequisites:
//   - Same env vars as the adapter (MATRIX_HOMESERVER_URL, MATRIX_AS_TOKEN, etc.)
//   - Bot must be a Synapse server admin
//   - Adapter should be scaled to 0 during migration
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/alkem-io/matrix-adapter-go/internal/config"
	"github.com/alkem-io/matrix-adapter-go/internal/infrastructure/logger"
	"github.com/alkem-io/matrix-adapter-go/internal/infrastructure/matrix"
)

func main() {
	// Flags
	redactAliases := flag.Bool("redact-canonical-aliases", false, "Clear canonical aliases from non-space rooms")
	leaveConversations := flag.Bool("leave-conversations", false, "Remove bot from conversation rooms (rooms with other joined members)")
	fixPowerLevels := flag.Bool("fix-power-levels", false, "Set users_default=50 on rooms with users_default=0")
	dryRun := flag.Bool("dry-run", false, "Log what would be done without making changes")
	flag.Parse()

	if !*redactAliases && !*leaveConversations && !*fixPowerLevels {
		fmt.Fprintln(os.Stderr, "No migration selected. Use --help to see available options.")
		os.Exit(1)
	}

	// Load config (same env vars as the adapter)
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	log, err := logger.NewZapLogger(cfg.App.LogLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to init logger: %v\n", err)
		os.Exit(1)
	}

	log.Info("Matrix migration tool starting",
		"dry_run", *dryRun,
		"redact_aliases", *redactAliases,
		"leave_conversations", *leaveConversations,
		"fix_power_levels", *fixPowerLevels,
	)

	// Initialize matrix adapter (no HTTP listener, no event loop)
	adapter, err := matrix.NewMautrixAdapter(cfg, log)
	if err != nil {
		log.Error("Failed to create matrix adapter", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()

	// Connect (bot admin + Whoami only — no listener, no events)
	if err := adapter.Connect(ctx); err != nil {
		log.Error("Failed to connect", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := adapter.Disconnect(); err != nil {
			log.Error("Failed to disconnect", "error", err)
		}
	}()

	_ = dryRun // TODO: pass to each migration

	// --- Migrations ---

	if *redactAliases {
		log.Info("=== Migration: redact canonical aliases ===")
		// TODO: move redactCanonicalAliasesFromRooms logic here
		// - Use ghost intent fallback when admin-join fails
		// - Skip rooms with no state
		log.Warn("redact-canonical-aliases: not yet implemented in migration tool")
	}

	if *leaveConversations {
		log.Info("=== Migration: leave conversation rooms ===")
		// TODO: move leaveBotFromNonSpaceRooms logic here, but ONLY for rooms
		// where other members are joined (conversation rooms).
		// Do NOT leave rooms where bot is the only member (callout, updates, post rooms).
		log.Warn("leave-conversations: not yet implemented in migration tool")
	}

	if *fixPowerLevels {
		log.Info("=== Migration: fix power levels ===")
		// TODO: for each room where users_default < 50, send a new
		// m.room.power_levels state event with users_default=50.
		// Use getIntentForRoom to find an intent that can send it.
		log.Warn("fix-power-levels: not yet implemented in migration tool")
	}

	log.Info("Migration tool finished")
}
