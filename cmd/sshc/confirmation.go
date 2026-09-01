package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

var errConfirmationUnavailable = errors.New("confirmation requires an interactive terminal")

type actionConfirmer func(context.Context, string) (bool, error)

func systemActionConfirmer(ctx context.Context, prompt string) (bool, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return false, errConfirmationUnavailable
	}
	return readActionConfirmation(ctx, os.Stdin, os.Stderr, prompt)
}

func readActionConfirmation(ctx context.Context, input io.Reader, output io.Writer, prompt string) (bool, error) {
	if _, err := fmt.Fprint(output, prompt); err != nil {
		return false, err
	}
	type result struct {
		line string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		reader := bufio.NewReader(io.LimitReader(input, 4097))
		line, err := reader.ReadString('\n')
		if errors.Is(err, io.EOF) && line != "" {
			err = nil
		}
		done <- result{line: line, err: err}
	}()

	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case answer := <-done:
		if answer.err != nil {
			return false, answer.err
		}
		switch strings.ToLower(strings.TrimSpace(answer.line)) {
		case "y", "yes":
			return true, nil
		default:
			return false, nil
		}
	}
}

func confirmAction(ctx context.Context, yes bool, prompt string, confirmer actionConfirmer, stderr io.Writer) (bool, int) {
	if yes {
		return true, 0
	}
	if confirmer == nil {
		fmt.Fprintln(stderr, "sshc: confirmation is unavailable; rerun with --yes")
		return false, 1
	}
	confirmed, err := confirmer(ctx, prompt)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return false, 130
		}
		if errors.Is(err, errConfirmationUnavailable) {
			fmt.Fprintln(stderr, "sshc: confirmation requires an interactive terminal; rerun with --yes")
		} else {
			fmt.Fprintf(stderr, "sshc: read confirmation: %v\n", err)
		}
		return false, 1
	}
	return confirmed, 0
}
