package sftp

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"reflect"
	"testing"

	pkgsftp "github.com/pkg/sftp"
)

// Ownership and permissions share the same transport and refusal scenarios.
func testNoFollowAttributeChange(t *testing.T, change func(*Client) error, expectedAttributes []uint32) {
	t.Helper()
	for _, scenario := range []struct {
		name      string
		version   string
		protected bool
		allowed   bool
	}{
		{name: "supported", version: "1", protected: true, allowed: true},
		{name: "missing extension", protected: true},
		{name: "unknown extension version", version: "2", protected: true},
		{name: "unprotected client", version: "1"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			serverConnection, clientConnection := net.Pipe()
			requests := make(chan []byte, 1)
			serverErrors := make(chan error, 1)
			go func() {
				defer serverConnection.Close()
				serverErrors <- serveNoFollowFixture(serverConnection, scenario.version, requests)
			}()
			var client *Client
			var err error
			if scenario.protected {
				client, err = newClientPipe(clientConnection, clientConnection)
			} else {
				var raw *pkgsftp.Client
				raw, err = pkgsftp.NewClientPipe(clientConnection, clientConnection)
				client = NewClient(raw)
			}
			if err != nil {
				t.Fatal(err)
			}
			err = change(client)
			if scenario.allowed && err != nil || !scenario.allowed && !errors.Is(err, ErrUnsupportedOperation) {
				t.Fatalf("metadata change = %v", err)
			}
			_ = client.Close()
			if err := <-serverErrors; err != nil {
				t.Fatal(err)
			}
			if !scenario.allowed {
				select {
				case packet := <-requests:
					t.Fatalf("refused operation sent a mutation: %x", packet)
				default:
				}
				return
			}
			packet := <-requests
			if len(packet) == 0 || packet[0] != sftpExtendedPacket {
				t.Fatalf("metadata change followed links: %x", packet)
			}
			request := bytes.NewReader(packet[5:]) // type and request ID
			for _, expected := range []string{noFollowSetstatExtension, "/shared/file"} {
				var length uint32
				if err := binary.Read(request, binary.BigEndian, &length); err != nil {
					t.Fatal(err)
				}
				value := make([]byte, length)
				if _, err := io.ReadFull(request, value); err != nil || string(value) != expected {
					t.Fatalf("request value = %q, %v", value, err)
				}
			}
			attributes := make([]uint32, len(expectedAttributes))
			if err := binary.Read(request, binary.BigEndian, &attributes); err != nil || !reflect.DeepEqual(attributes, expectedAttributes) || request.Len() != 0 {
				t.Fatalf("attributes = %v, %v", attributes, err)
			}
		})
	}
}

func serveNoFollowFixture(connection net.Conn, version string, requests chan<- []byte) error {
	if _, err := readNoFollowFixtureFrame(connection); err != nil {
		return err
	}
	hello := new(bytes.Buffer)
	hello.WriteByte(2) // VERSION
	_ = binary.Write(hello, binary.BigEndian, uint32(3))
	if version != "" {
		writeAttributeString(hello, noFollowSetstatExtension)
		writeAttributeString(hello, version)
	}
	if err := writeNoFollowFixtureFrame(connection, hello.Bytes()); err != nil {
		return err
	}
	packet, err := readNoFollowFixtureFrame(connection)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return err
	}
	requests <- packet
	status := make([]byte, 17) // STATUS, echoed request ID, OK, empty message/language
	status[0] = 101
	copy(status[1:5], packet[1:5])
	if err := writeNoFollowFixtureFrame(connection, status); err != nil {
		return err
	}
	_, err = io.Copy(io.Discard, connection)
	return err
}

func readNoFollowFixtureFrame(reader io.Reader) ([]byte, error) {
	var length uint32
	if err := binary.Read(reader, binary.BigEndian, &length); err != nil {
		return nil, err
	}
	packet := make([]byte, length)
	_, err := io.ReadFull(reader, packet)
	return packet, err
}

func writeNoFollowFixtureFrame(writer io.Writer, packet []byte) error {
	frame := new(bytes.Buffer)
	_ = binary.Write(frame, binary.BigEndian, uint32(len(packet)))
	frame.Write(packet)
	_, err := writer.Write(frame.Bytes())
	return err
}

type ownershipBuffer struct{ bytes.Buffer }

func (*ownershipBuffer) Close() error { return nil }
