package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sshc/internal/streamrun"
	"sync"

	"golang.org/x/term"
)

const transportEscapeByte = byte(0x1d) // Ctrl+]

type duplexStream interface {
	io.ReadWriteCloser
}

type transportWindowSizer interface {
	SetWindowSize(width, height uint16) error
}

var errLocalEscape = errors.New("local transport escape")

// attachTransport connects a local terminal to an untrusted byte stream. Ctrl+]
// is consumed locally so an interactive serial device always has a way out.
func attachTransport(ctx context.Context, stream duplexStream, stdin *os.File, stdout, stderr io.Writer) error {
	descriptor := int(stdin.Fd())
	if term.IsTerminal(descriptor) {
		state, err := term.MakeRaw(descriptor)
		if err != nil {
			return err
		}
		defer func() { _ = term.Restore(descriptor, state) }()
		if cols, rows, err := term.GetSize(descriptor); err == nil {
			if sizer, ok := stream.(transportWindowSizer); ok && cols > 0 && rows > 0 && cols <= 1000 && rows <= 1000 {
				_ = sizer.SetWindowSize(uint16(cols), uint16(rows))
			}
		}
	}
	fmt.Fprintln(stderr, "sshc: connected; press Ctrl+] to disconnect")

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var closeOnce sync.Once
	closeStream := func() { closeOnce.Do(func() { _ = stream.Close() }) }
	defer closeStream()

	inputDone := make(chan error, 1)
	outputDone := make(chan error, 1)
	go func() { inputDone <- copyTransportInput(stream, stdin) }()
	go func() {
		_, err := io.Copy(stdout, stream)
		outputDone <- err
	}()

	select {
	case err := <-inputDone:
		closeStream()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, errLocalEscape) || errors.Is(err, io.EOF) {
			return nil
		}
		return err
	case err := <-outputDone:
		closeStream()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err == nil || errors.Is(err, io.EOF) || errors.Is(err, os.ErrClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		closeStream()
		return ctx.Err()
	}
}

func copyTransportInput(destination io.Writer, source io.Reader) error {
	buffer := make([]byte, 32<<10)
	for {
		count, err := source.Read(buffer)
		if count > 0 {
			if forwardErr := forwardTransportInput(destination, buffer[:count]); forwardErr != nil {
				return forwardErr
			}
		}
		if err != nil {
			return err
		}
		if count == 0 {
			return io.ErrNoProgress
		}
	}
}

// forwardTransportInput は、読んだ入力を相手へ送る。transportEscapeByte があれば、
// その手前までを送って errLocalEscape を返す。
func forwardTransportInput(destination io.Writer, payload []byte) error {
	escapeAt := bytesIndex(payload, transportEscapeByte)
	if escapeAt < 0 {
		return streamrun.WriteAll(destination, payload)
	}
	if escapeAt > 0 {
		if err := streamrun.WriteAll(destination, payload[:escapeAt]); err != nil {
			return err
		}
	}
	return errLocalEscape
}

func bytesIndex(payload []byte, wanted byte) int {
	for index, value := range payload {
		if value == wanted {
			return index
		}
	}
	return -1
}
