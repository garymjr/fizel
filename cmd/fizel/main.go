package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/gmurray/fizel/internal/app"
	"github.com/gmurray/fizel/internal/config"
)

func main() {
	var logsRoot string
	var port int
	var configPath string
	var workflowPath string

	flag.StringVar(&logsRoot, "logs-root", "", "directory for logs")
	flag.IntVar(&port, "port", 0, "reserved for future web observability")
	flag.StringVar(&configPath, "config", config.DefaultGlobalPath(), "path to global config file")
	flag.StringVar(&workflowPath, "workflow", "", "path to single workflow file")
	flag.Parse()

	if hasFlag("config") && hasFlag("workflow") {
		fmt.Fprintln(os.Stderr, "cannot use -config and -workflow together")
		os.Exit(1)
	}

	if workflowPath != "" {
		var err error
		workflowPath, err = filepath.Abs(workflowPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "resolve workflow path: %v\n", err)
			os.Exit(1)
		}
		if _, err := os.Stat(workflowPath); err != nil {
			fmt.Fprintf(os.Stderr, "workflow file not found: %s\n", workflowPath)
			os.Exit(1)
		}
	} else {
		var err error
		configPath, err = filepath.Abs(configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "resolve config path: %v\n", err)
			os.Exit(1)
		}
		if _, err := os.Stat(configPath); err != nil {
			fmt.Fprintf(os.Stderr, "config file not found: %s\n", configPath)
			os.Exit(1)
		}
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	application, err := app.New(app.Options{
		ConfigPath:   configPath,
		WorkflowPath: workflowPath,
		LogsRoot:     logsRoot,
		Port:         port,
		Logger:       logger,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "startup failed: %v\n", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := application.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "runtime failed: %v\n", err)
		os.Exit(1)
	}
}

func hasFlag(name string) bool {
	prefix := "-" + name + "="
	for _, arg := range os.Args[1:] {
		if arg == "-"+name || strings.HasPrefix(arg, prefix) {
			return true
		}
	}
	return false
}
