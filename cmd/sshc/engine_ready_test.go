package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"
)

// waitLongerThanAnyTest is an upper bound that a wait ending for the right reason never reaches.
const waitLongerThanAnyTest = time.Minute

func TestWaitForReadyEngineEndsAsSoonAsTheManagerSeesTheEngineExit(t *testing.T) {
	exited := errors.New("the engine exited")
	checks := 0
	started := time.Now()
	_, err := waitForReadyEngine(context.Background(), t.TempDir(), engineReadiness{
		timeout: waitLongerThanAnyTest,
		exitedEarly: func(context.Context) error {
			checks++
			if checks < 3 {
				return nil
			}
			return exited
		},
	})
	if !errors.Is(err, exited) {
		t.Fatalf("wait = %v", err)
	}
	if elapsed := time.Since(started); elapsed >= waitLongerThanAnyTest {
		t.Fatalf("waited %s for an engine that had exited", elapsed)
	}
}

func TestWaitForReadyEngineGoesOnWhileTheEngineLivesAndHasNotPublishedItsHandoff(t *testing.T) {
	client := backgroundTestClient(func(response http.ResponseWriter, _ *http.Request) {
		io.WriteString(response, validEngineStatus())
	})
	stateDir := t.TempDir()
	checks := 0
	found, err := waitForReadyEngine(context.Background(), stateDir, engineReadiness{
		client:  client,
		timeout: waitLongerThanAnyTest,
		exitedEarly: func(context.Context) error {
			checks++
			if checks == 3 {
				writeTestHandoff(t, stateDir, backgroundTestURL)
			}
			return nil
		},
	})
	if err != nil || found.PID != testHandoff(backgroundTestURL).PID {
		t.Fatalf("found = %+v, %v", found, err)
	}
}
