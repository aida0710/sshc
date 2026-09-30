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
		return false, exitFailure
	}
	confirmed, err := confirmer(ctx, prompt)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return false, exitInterrupted
		}
		if errors.Is(err, errConfirmationUnavailable) {
			fmt.Fprintln(stderr, "sshc: confirmation requires an interactive terminal; rerun with --yes")
		} else {
			fmt.Fprintf(stderr, "sshc: read confirmation: %v\n", err)
		}
		return false, exitFailure
	}
	return confirmed, 0
}

// changeConfirmation は、変更の計画を出したあとで、進めてよいかを尋ねる確認である。
type changeConfirmation struct {
	// yes は、尋ねずに進める。
	yes       bool
	confirmer actionConfirmer
	stdout    io.Writer
	stderr    io.Writer
}

// confirmChange は、「Continue? [y/N]」で進めてよいかを尋ねる。進めないときは、終える
// 終了コードを返す。利用者が断ったときは、何も変えていないと伝えて 0 で終える。
func confirmChange(ctx context.Context, confirmation changeConfirmation) (bool, int) {
	confirmed, code := confirmAction(ctx, confirmation.yes, "Continue? [y/N] ", confirmation.confirmer, confirmation.stderr)
	if code != 0 {
		return false, code
	}
	if !confirmed {
		fmt.Fprintln(confirmation.stdout, "sshc: canceled; nothing changed")
		return false, 0
	}
	return true, 0
}
