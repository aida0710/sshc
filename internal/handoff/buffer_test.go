package handoff

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validBufferedHandoff() Handoff {
	return Handoff{
		SchemaVersion:   SchemaVersion,
		URL:             "http://127.0.0.1:52865",
		Secret:          "buffer-canary-secret",
		Owner:           OwnerEngine,
		PID:             4242,
		Version:         "test",
		ProtocolVersion: ProtocolVersion,
	}
}

func TestReadHandoffBodyAcceptsExactLimitAndRejectsOneExtraByte(t *testing.T) {
	body, err := json.Marshal(validBufferedHandoff())
	if err != nil {
		t.Fatal(err)
	}
	exact := append(body, bytes.Repeat([]byte{' '}, handoffDocumentMaxSize-len(body))...)
	if got, err := readHandoffBody(bytes.NewReader(exact)); err != nil {
		t.Fatalf("read exact limit = %v", err)
	} else if len(got) != handoffDocumentMaxSize {
		t.Fatalf("read exact limit bytes = %d, want %d", len(got), handoffDocumentMaxSize)
	}

	overflow := append(append([]byte(nil), exact...), 'x')
	got, err := readHandoffBody(bytes.NewReader(overflow))
	if !errors.Is(err, ErrDocumentTooLarge) {
		t.Fatalf("read one-byte overflow = %v, want ErrDocumentTooLarge", err)
	}
	if strings.Contains(err.Error(), "buffer-canary-secret") {
		t.Fatalf("overflow error leaked document bytes: %v", err)
	}
	if len(got) != handoffDocumentMaxSize+1 {
		t.Fatalf("overflow evidence bytes = %d, want max+1", len(got))
	}
}

func TestReadValidatedClearsBodyAndClosesFileOnEveryPath(t *testing.T) {
	tests := []struct {
		name    string
		body    []byte
		readErr error
		wantErr error
		success bool
	}{
		{name: "success", body: mustMarshalHandoff(t, validBufferedHandoff()), success: true},
		{name: "decode error", body: []byte("body-canary-not-json"), wantErr: ErrInvalid},
		{name: "read error", body: []byte("body-canary-before-read-error"), readErr: io.ErrUnexpectedEOF, wantErr: io.ErrUnexpectedEOF},
		{name: "overflow", body: bytes.Repeat([]byte{'s'}, handoffDocumentMaxSize+1), wantErr: ErrDocumentTooLarge},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), FileName)
			if err := os.WriteFile(path, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			var opened *os.File
			bodyAlias := append([]byte(nil), test.body...)
			operations := handoffFileOperations{
				open: func(path string) (*os.File, error) {
					var err error
					opened, err = os.Open(path)
					return opened, err
				},
				read: func(io.Reader) ([]byte, error) {
					if len(bodyAlias) > handoffDocumentMaxSize {
						return bodyAlias, ErrDocumentTooLarge
					}
					return bodyAlias, test.readErr
				},
			}
			document, file, err := readValidatedHandleWith(path, operations)
			if test.success {
				if err != nil || document != validBufferedHandoff() {
					t.Fatalf("readValidatedHandleWith = %#v, %v", document, err)
				}
				if file == nil {
					t.Fatal("success returned no authenticated handle")
				}
				if err := file.Close(); err != nil {
					t.Fatal(err)
				}
			} else {
				if test.name == "decode error" {
					if err == nil || strings.Contains(err.Error(), "body-canary") {
						t.Fatalf("decode error = %v, want redacted failure", err)
					}
				} else if !errors.Is(err, test.wantErr) {
					t.Fatalf("readValidatedHandleWith = %v, want %v", err, test.wantErr)
				}
				// 閉じ済みかどうかは二度目の Close で訊く。閉じた handle への
				// Stat が何を返すかは OS ごとに違うが、Close は必ず ErrClosed である。
				if closeErr := opened.Close(); !errors.Is(closeErr, os.ErrClosed) {
					t.Fatalf("failed read left file open: %v", closeErr)
				}
			}
			for index, value := range bodyAlias {
				if value != 0 {
					t.Fatalf("body[%d] = %d after return, want zero", index, value)
				}
			}
		})
	}
}

func TestWriteClearsMarshaledBodyOnFailure(t *testing.T) {
	want := errors.New("directory failed")
	body := []byte("marshaled-body-canary")
	operations := defaultWriteOperations()
	operations.marshal = func(any) ([]byte, error) { return body, nil }
	operations.ensureDirectory = func(string) error { return want }

	err := write(t.TempDir(), validBufferedHandoff(), operations)
	if !errors.Is(err, want) {
		t.Fatalf("write = %v, want %v", err, want)
	}
	for index, value := range body {
		if value != 0 {
			t.Fatalf("marshal body[%d] = %d after failure, want zero", index, value)
		}
	}
}

func mustMarshalHandoff(t *testing.T, document Handoff) []byte {
	t.Helper()
	body, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return body
}
