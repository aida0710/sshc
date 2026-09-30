package main

import (
	"context"
	"errors"
	"io"
	"os"
	"unicode/utf8"
)

// visibleInputWait は、見える入力の 1 バイトを読む前に、読めるようになるか ctx が
// 取り消されるまで待つ。cooked の端末では Ctrl-C が 0x03 ではなく SIGINT になるので、
// 読み取りを待つ側が取り消しに気づけないと Enter を押すまで戻らない。
type visibleInputWait interface {
	waitReadable() error
	// waitForLateCancel は、読み取りが何も読まずに EOF で戻ったあと、その EOF が取り消しに
	// よるものなら ctx が取り消されるまで、短い上限つきで待つ。
	waitForLateCancel()
	close()
}

// readBoundedVisibleLine reads exactly through one newline without buffering
// bytes from the next hidden prompt. The fixed capacity also prevents an old
// heap buffer from retaining abandoned input after append growth.
func readBoundedVisibleLine(ctx context.Context, input *os.File) ([]byte, error) {
	wait, err := startVisibleInputWait(ctx, input)
	if err != nil {
		return nil, err
	}
	defer wait.close()
	line := make([]byte, 0, maxSyncSetupLine)
	for {
		if err := wait.waitReadable(); err != nil {
			clear(line)
			return nil, err
		}
		var one [1]byte
		count, err := input.Read(one[:])
		if count > 0 {
			switch one[0] {
			case '\n':
				// Ctrl-C と Enter が続いたときに、取り消しを空欄の回答（既定値）として扱わない。
				if err := ctx.Err(); err != nil {
					clear(line)
					return nil, err
				}
				if !utf8.Valid(line) {
					clear(line)
					return nil, errSyncSetupInput
				}
				return line, nil
			case '\r':
				// Canonical Unix terminals normally deliver Enter as LF, while
				// Windows consoles can deliver CRLF. Ignore CR so its following
				// LF is consumed by this prompt instead of the next one.
			case 0x03:
				clear(line)
				return nil, context.Canceled
			default:
				if len(line) == maxSyncSetupLine {
					clear(line)
					return nil, errSyncSetupInput
				}
				line = append(line, one[0])
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) && len(line) == 0 {
				wait.waitForLateCancel()
			}
			if ctx.Err() != nil {
				clear(line)
				return nil, ctx.Err()
			}
			if errors.Is(err, io.EOF) && len(line) > 0 && utf8.Valid(line) {
				return line, nil
			}
			clear(line)
			return nil, err
		}
		if count == 0 {
			clear(line)
			return nil, io.ErrNoProgress
		}
	}
}
