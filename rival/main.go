package main

import (
	"os"
	"strings"

	"github.com/1F47E/rival/cmd"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Set via ldflags at build time.
var version = "dev"

func main() {
	// Load .env silently (no error if missing).
	_ = godotenv.Load()

	// Zerolog to stderr, JSON output.
	log.Logger = zerolog.New(os.Stderr).With().
		Timestamp().
		Str("app", "rival").
		Logger()

	// git describe / release tags carry a leading "v" (e.g. "v3.10.0"); the CLI
	// prints versions as "v%s", so strip it here to avoid a doubled "vv".
	cmd.Version = strings.TrimPrefix(version, "v")
	cmd.Execute()
}
