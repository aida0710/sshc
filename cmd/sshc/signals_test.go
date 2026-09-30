package main

import (
	"context"
	"testing"
)

func TestInteractiveStopExitCodeFollowsTheStopCause(t *testing.T) {
	running := context.Background()
	if code, stopped := interactiveStopExitCode(running); stopped {
		t.Errorf("a running context reported stopped with %d", code)
	}
	for _, test := range []struct {
		name  string
		cause error
		want  int
	}{
		{name: "Ctrl-C", cause: errInterrupted, want: 130},
		{name: "a supervisor's SIGTERM or a closed terminal", cause: errTerminated, want: 0},
		{name: "a caller that cancelled without a cause", cause: nil, want: 130},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, stop := context.WithCancelCause(running)
			stop(test.cause)
			if code, stopped := interactiveStopExitCode(ctx); !stopped || code != test.want {
				t.Errorf("interactiveStopExitCode = %d, %v, want %d, true", code, stopped, test.want)
			}
		})
	}
}
