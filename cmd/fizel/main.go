package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/gmurray/fizel/internal/app"
)

func main() {
	var logsRoot string
	var port int

	flag.StringVar(&logsRoot, "logs-root", "", "directory for logs")
	flag.IntVar(&port, "port", 0, "reserved for future web observability")
	flag.Parse()

	workflowPath := "WORKFLOW.md"
	if flag.NArg() > 0 {
		workflowPath = flag.Arg(0)
	}

	workflowPath, err := filepath.Abs(workflowPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve workflow path: %v\n", err)
		os.Exit(1)
	}

	if _, err := os.Stat(workflowPath); err != nil {
		fmt.Fprintf(os.Stderr, "workflow file not found: %s\n", workflowPath)
		os.Exit(1)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	application, err := app.New(app.Options{
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
