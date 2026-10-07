package sftp

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	noFollowSetstatExtension = "lsetstat@openssh.com"
	sftpSetstatPacket        = 9
	sftpExtendedPacket       = 200
	// SFTP frames carry a four-byte length followed by a one-byte message type.
	sftpFrameHeaderBytes = 5
)

// pkg/sftp has no lsetstat API. Its serialized sendPacket writes a request
// header followed by a separate attribute/file payload. Translate owner
// SETSTAT headers and track the payload so file bytes are never parsed.
type noFollowOwnershipWriter struct {
	destination  io.WriteCloser
	payloadBytes uint32
}

func (writer *noFollowOwnershipWriter) Close() error { return writer.destination.Close() }

func (writer *noFollowOwnershipWriter) Write(packet []byte) (int, error) {
	if len(packet) == 0 {
		return 0, nil
	}
	if writer.payloadBytes != 0 {
		if uint64(len(packet)) > uint64(writer.payloadBytes) {
			return 0, fmt.Errorf("invalid SFTP payload length")
		}
		written, err := writer.destination.Write(packet)
		writer.payloadBytes -= uint32(written)
		return written, err
	}
	if len(packet) < sftpFrameHeaderBytes {
		return 0, io.ErrUnexpectedEOF
	}
	length := uint64(binary.BigEndian.Uint32(packet[:4])) + 4
	if length < uint64(len(packet)) || length > maxSFTPPacketBytes+4 {
		return 0, fmt.Errorf("invalid SFTP request length")
	}
	writer.payloadBytes = uint32(length - uint64(len(packet)))
	if packet[4] != sftpSetstatPacket {
		return writer.destination.Write(packet)
	}
	translated, err := noFollowOwnershipPacket(packet, writer.payloadBytes)
	if err != nil {
		return 0, err
	}
	written, err := writer.destination.Write(translated)
	if err == nil && written != len(translated) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return 0, err
	}
	return len(packet), nil
}

func noFollowOwnershipPacket(packet []byte, payloadBytes uint32) ([]byte, error) {
	// SETSTAT contains the request ID, a length-prefixed path, then attributes.
	const pathLengthOffset = sftpFrameHeaderBytes + 4
	if len(packet) < pathLengthOffset+4 {
		return nil, io.ErrUnexpectedEOF
	}
	pathBytes := uint64(binary.BigEndian.Uint32(packet[pathLengthOffset:]))
	flagsOffset := uint64(pathLengthOffset+4) + pathBytes
	if flagsOffset+4 > uint64(len(packet)) {
		return nil, io.ErrUnexpectedEOF
	}
	flags := binary.BigEndian.Uint32(packet[flagsOffset:])
	if flags&sftpOwnerAttributes == 0 {
		return packet, nil
	}
	// Chown sends only two uint32 owner IDs. Refuse a changed library encoding
	// rather than silently applying any other attributes through this boundary.
	if flags != sftpOwnerAttributes || flagsOffset+12 != uint64(len(packet))+uint64(payloadBytes) {
		return nil, fmt.Errorf("invalid SFTP ownership attributes")
	}
	extensionBytes := 4 + len(noFollowSetstatExtension)
	translated := make([]byte, len(packet)+extensionBytes)
	copy(translated, packet[:pathLengthOffset])
	translated[4] = sftpExtendedPacket
	binary.BigEndian.PutUint32(translated[:4], uint32(len(translated)-4)+payloadBytes)
	binary.BigEndian.PutUint32(translated[pathLengthOffset:], uint32(len(noFollowSetstatExtension)))
	copy(translated[pathLengthOffset+4:], noFollowSetstatExtension)
	copy(translated[pathLengthOffset+extensionBytes:], packet[pathLengthOffset:])
	return translated, nil
}
