package randomid

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"testing"
)

func TestUnusedIDDrawsAgainWhenTheIdentifierIsInUse(t *testing.T) {
	first := bytes.Repeat([]byte{0x11}, idBytes)
	second := bytes.Repeat([]byte{0x22}, idBytes)
	random := bytes.NewReader(append(append([]byte(nil), first...), second...))
	taken := hex.EncodeToString(first)

	id, err := UnusedID(random, func(id string) (bool, error) { return id == taken, nil })
	if err != nil {
		t.Fatalf("UnusedID = %v", err)
	}
	if want := hex.EncodeToString(second); id != want {
		t.Fatalf("UnusedID = %q, want %q", id, want)
	}
}

func TestUnusedIDReportsExhaustedWhenTheRandomSourceKeepsRepeating(t *testing.T) {
	random := bytes.NewReader(bytes.Repeat([]byte{0x42}, idBytes*unusedIDAttempts))
	lookups := 0
	_, err := UnusedID(random, func(string) (bool, error) {
		lookups++
		return true, nil
	})
	if !errors.Is(err, ErrExhausted) {
		t.Fatalf("UnusedID = %v, want ErrExhausted", err)
	}
	if lookups != unusedIDAttempts {
		t.Fatalf("inUse was asked %d times, want %d", lookups, unusedIDAttempts)
	}
}

func TestUnusedIDReturnsTheLookupFailure(t *testing.T) {
	want := errors.New("store failed")
	random := bytes.NewReader(bytes.Repeat([]byte{0x42}, idBytes))
	if _, err := UnusedID(random, func(string) (bool, error) { return false, want }); !errors.Is(err, want) {
		t.Fatalf("UnusedID = %v, want %v", err, want)
	}
}

func TestUnusedIDReturnsTheRandomSourceFailure(t *testing.T) {
	random := io.MultiReader(bytes.NewReader([]byte{1, 2, 3}), errorOnlyReader{err: io.ErrUnexpectedEOF})
	if _, err := UnusedID(random, func(string) (bool, error) { return false, nil }); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("UnusedID = %v, want the read error", err)
	}
}

func TestTokenEncodes32RandomBytesAsUnpaddedBase64URL(t *testing.T) {
	raw := bytes.Repeat([]byte{0xfb}, tokenBytes)
	got, err := Token(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("Token = %v", err)
	}
	if want := base64.RawURLEncoding.EncodeToString(raw); got != want {
		t.Fatalf("Token = %q, want %q", got, want)
	}
}

func TestIsTokenAcceptsOnlyWhatTokenProduces(t *testing.T) {
	minted, err := Token(bytes.NewReader(bytes.Repeat([]byte{0x7e}, tokenBytes)))
	if err != nil {
		t.Fatalf("Token = %v", err)
	}
	for _, test := range []struct {
		name  string
		value string
		want  bool
	}{
		{name: "minted", value: minted, want: true},
		{name: "one byte short", value: base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x7e}, tokenBytes-1))},
		{name: "padded", value: base64.URLEncoding.EncodeToString(bytes.Repeat([]byte{0x7e}, tokenBytes))},
		{name: "standard alphabet", value: minted[:len(minted)-1] + "+"},
		{name: "empty"},
	} {
		if got := IsToken(test.value); got != test.want {
			t.Errorf("%s: IsToken(%q) = %v, want %v", test.name, test.value, got, test.want)
		}
	}
}

func TestTokenClearsRawRandomBytesAfterEncoding(t *testing.T) {
	var rawAlias []byte
	_, err := token(bytes.NewReader(bytes.Repeat([]byte{0x5a}, tokenBytes)), func(raw []byte) string {
		rawAlias = raw
		return "encoded"
	})
	if err != nil {
		t.Fatalf("token = %v", err)
	}
	for index, value := range rawAlias {
		if value != 0 {
			t.Fatalf("raw[%d] = %d after return, want zero", index, value)
		}
	}
}

func TestTokenClearsPartiallyFilledRawBufferWhenTheReadFails(t *testing.T) {
	reader := &capturingErrorReader{err: io.ErrUnexpectedEOF}
	if _, err := token(reader, func([]byte) string { return "unreachable" }); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("token = %v, want read error", err)
	}
	for index, value := range reader.destination {
		if value != 0 {
			t.Fatalf("partial raw[%d] = %d after return, want zero", index, value)
		}
	}
}

type errorOnlyReader struct{ err error }

func (reader errorOnlyReader) Read([]byte) (int, error) { return 0, reader.err }

// capturingErrorReader は、途中まで書いた出力先を覚えてから失敗する乱数源である。
type capturingErrorReader struct {
	destination []byte
	err         error
}

func (reader *capturingErrorReader) Read(destination []byte) (int, error) {
	reader.destination = destination
	copy(destination, []byte{1, 2, 3})
	return 3, reader.err
}
