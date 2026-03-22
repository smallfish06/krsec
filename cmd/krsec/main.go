package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/smallfish06/krsec/internal/server"
	"github.com/smallfish06/krsec/pkg/config"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	configFile := flag.String("config", "config.yaml", "Path to configuration file")
	showVersion := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("krsec %s (commit: %s, built: %s)\n", version, commit, date)
		os.Exit(0)
	}

	cfg, err := config.Load(*configFile)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	srv := server.New(cfg)
	if err := srv.Run(); err != nil {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}
