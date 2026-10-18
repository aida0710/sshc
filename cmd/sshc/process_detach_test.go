package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

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
	process, err := startLoggedEngine(stateDir, engineProcessCommand{
		executable:  executable,
		arguments:   []string{"-test.run=^TestDetachedEngineChild$"},
		environment: append(os.Environ(), backgroundProcessTestEnvironment+"=inherited"),
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
	if err != nil || !strings.Contains(tail, "stdout: inherited") || !strings.Contains(tail, "stderr: inherited") {
		t.Fatalf("log = %q, %v", tail, err)
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
	fmt.Fprintln(os.Stdout, "stdout:", value)
	fmt.Fprintln(os.Stderr, "stderr:", value)
	os.Exit(23)
}
