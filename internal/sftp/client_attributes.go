package sftp

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strconv"
)

// pkg/sftp discards attribute presence bits, turning absent UID/GID into 0/0
// and absent mode into a regular file. Retain the flags before decoding.
const metadataFlagsAttribute = "sshc.metadata-flags"
const (
	sftpAttributesPacket      = 105
	sftpNamesPacket           = 104
	sftpOwnerAttributes       = 2
	sftpPermissionsAttributes = 4
	sftpExtendedAttributes    = uint32(1 << 31)
	// Bound every response allocation while allowing large directory metadata replies.
	maxSFTPPacketBytes = 16 << 20
)

type attributeReader struct {
	source  io.Reader
	pending *bytes.Reader
}

func (reader *attributeReader) Read(destination []byte) (int, error) {
	if len(destination) == 0 {
		return 0, nil
	}
	if reader.pending == nil || reader.pending.Len() == 0 {
		var header [4]byte
		if _, err := io.ReadFull(reader.source, header[:]); err != nil {
			return 0, err
		}
		length := binary.BigEndian.Uint32(header[:])
		if length == 0 || length > maxSFTPPacketBytes {
			return 0, fmt.Errorf("invalid SFTP packet length: %d", length)
		}
		frame := make([]byte, len(header)+int(length))
		copy(frame, header[:])
		if _, err := io.ReadFull(reader.source, frame[len(header):]); err != nil {
			return 0, err
		}
		packet := frame[len(header):]
		if packet[0] == sftpAttributesPacket || packet[0] == sftpNamesPacket {
			preserved, err := preserveMetadataFlags(packet)
			if err != nil {
				return 0, err
			}
			binary.BigEndian.PutUint32(header[:], uint32(len(preserved)))
			frame = append(header[:], preserved...)
		}
		reader.pending = bytes.NewReader(frame)
	}
	return reader.pending.Read(destination)
}

func preserveMetadataFlags(packet []byte) ([]byte, error) {
	if packet[0] != sftpAttributesPacket && packet[0] != sftpNamesPacket {
		return packet, nil
	}
	input := bytes.NewReader(packet[1:])
	output := new(bytes.Buffer)
	output.WriteByte(packet[0])
	if err := copyAttributeBytes(output, input, 4); err != nil {
		return nil, err
	} // request ID
	count := uint32(1)
	if packet[0] == sftpNamesPacket {
		if err := binary.Read(input, binary.BigEndian, &count); err != nil {
			return nil, err
		}
		_ = binary.Write(output, binary.BigEndian, count)
	}
	for index := uint32(0); index < count; index++ {
		if packet[0] == sftpNamesPacket {
			for range 2 {
				if err := copyAttributeString(output, input); err != nil {
					return nil, err
				}
			}
		}
		if err := preserveAttributeFlags(output, input); err != nil {
			return nil, err
		}
	}
	if input.Len() != 0 {
		return nil, fmt.Errorf("unexpected trailing SFTP metadata")
	}
	return output.Bytes(), nil
}

func preserveAttributeFlags(output *bytes.Buffer, input *bytes.Reader) error {
	var flags uint32
	if err := binary.Read(input, binary.BigEndian, &flags); err != nil {
		return err
	}
	if flags & ^uint32(1|2|4|8|sftpExtendedAttributes) != 0 {
		return fmt.Errorf("unknown SFTP attribute flags")
	}
	_ = binary.Write(output, binary.BigEndian, flags|sftpExtendedAttributes)
	for _, field := range []struct {
		flag uint32
		size int
	}{{1, 8}, {2, 8}, {4, 4}, {8, 8}} {
		if flags&field.flag != 0 {
			if err := copyAttributeBytes(output, input, field.size); err != nil {
				return err
			}
		}
	}
	var extendedCount uint32
	if flags&sftpExtendedAttributes != 0 {
		if err := binary.Read(input, binary.BigEndian, &extendedCount); err != nil {
			return err
		}
	}
	// Every extension needs at least two string lengths. Bound the loop and addition.
	if uint64(extendedCount)*8 > uint64(input.Len()) {
		return io.ErrUnexpectedEOF
	}
	_ = binary.Write(output, binary.BigEndian, extendedCount+1)
	for index := uint32(0); index < extendedCount; index++ {
		for range 2 {
			if err := copyAttributeString(output, input); err != nil {
				return err
			}
		}
	}
	writeAttributeString(output, metadataFlagsAttribute)
	writeAttributeString(output, strconv.FormatUint(uint64(flags), 10))
	return nil
}

func copyAttributeBytes(output *bytes.Buffer, input *bytes.Reader, size int) error {
	if size > input.Len() {
		return io.ErrUnexpectedEOF
	}
	_, err := io.CopyN(output, input, int64(size))
	return err
}

func copyAttributeString(output *bytes.Buffer, input *bytes.Reader) error {
	var length uint32
	if err := binary.Read(input, binary.BigEndian, &length); err != nil {
		return err
	}
	if uint64(length) > uint64(input.Len()) {
		return io.ErrUnexpectedEOF
	}
	_ = binary.Write(output, binary.BigEndian, length)
	return copyAttributeBytes(output, input, int(length))
}

func writeAttributeString(output *bytes.Buffer, value string) {
	_ = binary.Write(output, binary.BigEndian, uint32(len(value)))
	output.WriteString(value)
}
