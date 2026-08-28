// Package textencoding converts terminal text between sshc's UTF-8 boundary
// and a legacy encoding used by a remote SSH, Serial, or Telnet endpoint.
package textencoding

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"
)

// Name is the canonical value stored in metadata and accepted by the CLI.
type Name string

const (
	UTF8      Name = "utf-8"
	ShiftJIS  Name = "shift_jis"
	EUCJP     Name = "euc-jp"
	ISO2022JP Name = "iso-2022-jp"
)

var ErrUnsupported = errors.New("unsupported terminal text encoding")

// Parse accepts canonical values plus common spellings, and always returns a
// canonical value. Empty means the UTF-8 default for persisted metadata.
func Parse(value string) (Name, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "utf8", "utf-8":
		return UTF8, nil
	case "sjis", "shift-jis", "shift_jis", "cp932", "windows-31j":
		return ShiftJIS, nil
	case "eucjp", "euc_jp", "euc-jp":
		return EUCJP, nil
	case "jis", "iso2022jp", "iso_2022_jp", "iso-2022-jp":
		return ISO2022JP, nil
	default:
		return "", ErrUnsupported
	}
}

func codec(name Name) (encoding.Encoding, error) {
	canonical, err := Parse(string(name))
	if err != nil {
		return nil, err
	}
	switch canonical {
	case UTF8:
		return nil, nil
	case ShiftJIS:
		return japanese.ShiftJIS, nil
	case EUCJP:
		return japanese.EUCJP, nil
	case ISO2022JP:
		return japanese.ISO2022JP, nil
	default:
		return nil, ErrUnsupported
	}
}

// DecodeReader exposes remote bytes as UTF-8.
func DecodeReader(source io.Reader, name Name) (io.Reader, error) {
	selected, err := codec(name)
	if err != nil {
		return nil, err
	}
	if selected == nil {
		return source, nil
	}
	return transform.NewReader(source, selected.NewDecoder()), nil
}

// EncodeReader exposes local UTF-8 as bytes in the remote encoding.
func EncodeReader(source io.Reader, name Name) (io.Reader, error) {
	selected, err := codec(name)
	if err != nil {
		return nil, err
	}
	if selected == nil {
		return source, nil
	}
	return transform.NewReader(source, selected.NewEncoder()), nil
}

// DecodeWriter accepts remote bytes and writes UTF-8.
func DecodeWriter(destination io.Writer, name Name) (io.Writer, error) {
	selected, err := codec(name)
	if err != nil {
		return nil, err
	}
	if selected == nil {
		return destination, nil
	}
	return transform.NewWriter(destination, selected.NewDecoder()), nil
}

// EncodeWriter accepts local UTF-8 and writes bytes in the remote encoding.
func EncodeWriter(destination io.Writer, name Name) (io.Writer, error) {
	selected, err := codec(name)
	if err != nil {
		return nil, err
	}
	if selected == nil {
		return destination, nil
	}
	return transform.NewWriter(destination, selected.NewEncoder()), nil
}

// Wrap places conversion around an already protocol-clean byte stream. Telnet
// callers must wrap Conn, not the raw socket, so IAC negotiation is never fed
// to a Japanese text decoder.
func Wrap(stream io.ReadWriteCloser, name Name) (io.ReadWriteCloser, error) {
	canonical, err := Parse(string(name))
	if err != nil {
		return nil, err
	}
	if canonical == UTF8 {
		return stream, nil
	}
	reader, err := DecodeReader(stream, canonical)
	if err != nil {
		return nil, err
	}
	writer, err := EncodeWriter(stream, canonical)
	if err != nil {
		return nil, err
	}
	return &convertedStream{raw: stream, reader: reader, writer: writer}, nil
}

type convertedStream struct {
	raw    io.ReadWriteCloser
	reader io.Reader
	writer io.Writer

	writeMu   sync.Mutex
	closeOnce sync.Once
	closeErr  error
}

func (stream *convertedStream) Read(buffer []byte) (int, error) {
	return stream.reader.Read(buffer)
}

func (stream *convertedStream) Write(buffer []byte) (int, error) {
	stream.writeMu.Lock()
	defer stream.writeMu.Unlock()
	return stream.writer.Write(buffer)
}

func (stream *convertedStream) Close() error {
	stream.closeOnce.Do(func() {
		var flush error
		if closer, ok := stream.writer.(io.Closer); ok {
			flush = closer.Close()
		}
		stream.closeErr = errors.Join(flush, stream.raw.Close())
	})
	return stream.closeErr
}

// DiscardPending preserves the automation capability of Serial and Telnet.
// It runs against raw bytes before any following decoded Read begins.
func (stream *convertedStream) DiscardPending(ctx context.Context) error {
	discarder, ok := stream.raw.(interface{ DiscardPending(context.Context) error })
	if !ok {
		return nil
	}
	return discarder.DiscardPending(ctx)
}

// SetWindowSize preserves Telnet NAWS through the conversion wrapper.
func (stream *convertedStream) SetWindowSize(width, height uint16) error {
	sizer, ok := stream.raw.(interface{ SetWindowSize(uint16, uint16) error })
	if !ok {
		return nil
	}
	return sizer.SetWindowSize(width, height)
}
