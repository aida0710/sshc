package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"sshc/internal/app"
	"sshc/internal/enginelog"
)

const backgroundProcessTestEnvironment = "SSHC_TEST_BACKGROUND_PROCESS"

// Allow process creation on slow CI hosts without waiting indefinitely for a broken child.
const backgroundProcessTestTimeout = 10 * time.Second

func TestDetachedEngineInheritsEnvironmentAndWritesBothStreamsToTheLog(t *testing.T) {
	stateDir := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	process, err := startLoggedEngine(engineProcessCommand{
		executable:  executable,
		arguments:   []string{"-test.run=^TestDetachedEngineChild$"},
		environment: append(os.Environ(), backgroundProcessTestEnvironment+"=inherited", "SSHC_TEST_BACKGROUND_STATE="+stateDir),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer process.release()
	select {
	case exit := <-process.exited:
		if exit.code != 23 {
			t.Fatalf("child exit = %+v", exit)
		}
	case <-time.After(backgroundProcessTestTimeout):
		t.Fatal("child did not exit")
	}
	tail, err := enginelog.Tail(enginelog.Path(stateDir))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"stdout: inherited", "stderr: inherited", "default logger: inherited", "app logger: inherited", "engine started in the background"} {
		if !strings.Contains(tail, expected) {
			t.Fatalf("log lacks %q: %s", expected, tail)
		}
	}
}

func TestDetachedEngineChild(t *testing.T) {
	value := os.Getenv(backgroundProcessTestEnvironment)
	if value == "" {
		return
	}
	if !isDetachedTestProcess() {
		os.Exit(24)
	}
	input, err := io.ReadAll(os.Stdin)
	if err != nil || len(input) != 0 {
		os.Exit(25)
	}
	stateDir := os.Getenv("SSHC_TEST_BACKGROUND_STATE")
	code := runEngineWithDependencies(context.Background(), userPaths{home: stateDir, stateDir: stateDir}, engineOptions{Background: true}, nil, os.Stdout, os.Stderr, engineDependencies{
		acquire: func(string) (func() error, error) { return func() error { return nil }, nil },
		runApp: func(_ context.Context, dependencies app.Dependencies, _ string) error {
			fmt.Fprintln(os.Stdout, "stdout:", value)
			fmt.Fprintln(os.Stderr, "stderr:", value)
			slog.Info("default logger: inherited")
			dependencies.Logger.Info("app logger: inherited")
			return nil
		},
	})
	if code != 0 {
		os.Exit(code)
	}
	os.Exit(23)
}
