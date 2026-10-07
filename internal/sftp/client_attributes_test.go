package sftp

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"
)

func TestAttributeReaderPreservesFragmentedFramesAndLeavesFileDataUnchanged(t *testing.T) {
	packet := []byte{103, 0, 0, 0, 1, 'x', 'y'}
	frame := new(bytes.Buffer)
	_ = binary.Write(frame, binary.BigEndian, uint32(len(packet)))
	frame.Write(packet)
	original := append([]byte(nil), frame.Bytes()...)
	reader := &attributeReader{source: frame}
	var actual bytes.Buffer
	buffer := make([]byte, 1)
	for {
		count, err := reader.Read(buffer)
		actual.Write(buffer[:count])
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Equal(actual.Bytes(), original) {
		t.Fatalf("data packet changed: %x", actual.Bytes())
	}
	for _, packet := range [][]byte{{sftpAttributesPacket}, {sftpNamesPacket, 0, 0, 0, 1, 255, 255, 255, 255}} {
		if _, err := preserveMetadataFlags(packet); err == nil {
			t.Fatalf("malformed packet accepted: %x", packet)
		}
	}
}
