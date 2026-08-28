package sshclient

import (
	"errors"
	"io"

	"sshc/internal/textencoding"
)

// encodeStreams keeps every local boundary UTF-8 while translating the byte
// streams carried inside SSH. stdout and stderr receive independent decoder
// state because SSH transports them as independent streams.
func encodeStreams(streams Streams, name textencoding.Name) (Streams, func() error, error) {
	converted := streams
	var closers []io.Closer
	if streams.In != nil {
		reader, err := textencoding.EncodeReader(streams.In, name)
		if err != nil {
			return Streams{}, nil, err
		}
		converted.In = reader
	}
	if streams.Out != nil {
		writer, err := textencoding.DecodeWriter(streams.Out, name)
		if err != nil {
			return Streams{}, nil, err
		}
		converted.Out = writer
		if closer, ok := writer.(io.Closer); ok {
			closers = append(closers, closer)
		}
	}
	if streams.Err != nil {
		writer, err := textencoding.DecodeWriter(streams.Err, name)
		if err != nil {
			return Streams{}, nil, err
		}
		converted.Err = writer
		if closer, ok := writer.(io.Closer); ok {
			closers = append(closers, closer)
		}
	}
	return converted, func() error {
		var joined error
		for _, closer := range closers {
			joined = errors.Join(joined, closer.Close())
		}
		return joined
	}, nil
}
