package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"sshc/internal/enginelog"
)

type backgroundEngineRun struct {
	paths        userPaths
	options      engineOptions
	dependencies engineDependencies
}

// runBackgroundEngine owns the output handles for the engine's entire lifetime.
// Setting the process outputs also captures library diagnostics outside the app logger.
func runBackgroundEngine(ctx context.Context, run backgroundEngineRun) int {
	paths, options := run.paths, run.options
	output, err := enginelog.Open(paths.stateDir, version)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sshc: open engine log: %v\n", err)
		return exitFailure
	}
	defer output.Close()
	restore, err := redirectEngineOutput(output)
	if err != nil {
		fmt.Fprintf(output, "sshc: redirect engine output: %v\n", err)
		return exitFailure
	}
	defer restore()
	options.Background = false
	ctx, finish := watchSessionEnd(ctx)
	defer finish()
	return runEngineWithDependencies(ctx, paths, options, nil, output, output, run.dependencies)
}

func replaceGoEngineOutput(output *os.File) func() {
	stdout, stderr, logger := os.Stdout, os.Stderr, slog.Default()
	os.Stdout, os.Stderr = output, output
	slog.SetDefault(slog.New(slog.NewTextHandler(output, nil)))
	return func() { os.Stdout, os.Stderr = stdout, stderr; slog.SetDefault(logger) }
}
