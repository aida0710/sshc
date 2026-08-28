package sshclient

import (
	"bytes"
	"io"
	"testing"

	"sshc/internal/textencoding"
)

func TestEncodeStreamsKeepsLocalBoundaryUTF8(t *testing.T) {
	input := bytes.NewBufferString("送信")
	var output bytes.Buffer
	streams, closeStreams, err := encodeStreams(Streams{In: input, Out: &output}, textencoding.ShiftJIS)
	if err != nil {
		t.Fatal(err)
	}
	wireInput, err := io.ReadAll(streams.In)
	if err != nil {
		t.Fatal(err)
	}
	wantWire := []byte{0x91, 0x97, 0x90, 0x4d}
	if !bytes.Equal(wireInput, wantWire) {
		t.Fatalf("wire input = %x, want %x", wireInput, wantWire)
	}
	if _, err := streams.Out.Write([]byte{0x8e, 0xf3, 0x90, 0x4d}); err != nil {
		t.Fatal(err)
	}
	if err := closeStreams(); err != nil {
		t.Fatal(err)
	}
	if got := output.String(); got != "受信" {
		t.Fatalf("local output = %q", got)
	}
}
