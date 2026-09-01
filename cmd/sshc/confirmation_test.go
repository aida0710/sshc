package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestReadActionConfirmationAcceptsOnlyAnExplicitYes(t *testing.T) {
	for _, test := range []struct {
		answer string
		want   bool
	}{
		{answer: "y\n", want: true},
		{answer: "YES\n", want: true},
		{answer: "\n", want: false},
		{answer: "no\n", want: false},
	} {
		var output bytes.Buffer
		got, err := readActionConfirmation(context.Background(), strings.NewReader(test.answer), &output, "Continue? [y/N] ")
		if err != nil || got != test.want {
			t.Errorf("answer %q = %v, %v; want %v", test.answer, got, err, test.want)
		}
		if output.String() != "Continue? [y/N] " {
			t.Errorf("prompt = %q", output.String())
		}
	}
}

func TestConfirmActionRequiresYesOutsideAnInteractiveTerminal(t *testing.T) {
	var stderr bytes.Buffer
	confirmed, code := confirmAction(context.Background(), false, "Continue? [y/N] ", func(context.Context, string) (bool, error) {
		return false, errConfirmationUnavailable
	}, &stderr)
	if confirmed || code != 1 || !strings.Contains(stderr.String(), "rerun with --yes") {
		t.Fatalf("confirmed=%v code=%d stderr=%q", confirmed, code, stderr.String())
	}
}

func TestConfirmActionYesBypassesThePrompt(t *testing.T) {
	called := false
	confirmed, code := confirmAction(context.Background(), true, "Continue? [y/N] ", func(context.Context, string) (bool, error) {
		called = true
		return false, errors.New("should not be called")
	}, &bytes.Buffer{})
	if !confirmed || code != 0 || called {
		t.Fatalf("confirmed=%v code=%d called=%v", confirmed, code, called)
	}
}
