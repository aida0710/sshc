package sftp

import (
	"io"

	pkgsftp "github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// NewSSHClient preserves server attribute presence while using pkg/sftp for all operations.
func NewSSHClient(connection *ssh.Client, options ...pkgsftp.ClientOption) (*Client, error) {
	session, err := connection.NewSession()
	if err != nil {
		return nil, err
	}
	reader, err := session.StdoutPipe()
	if err != nil {
		_ = session.Close()
		return nil, err
	}
	writer, err := session.StdinPipe()
	if err != nil {
		_ = session.Close()
		return nil, err
	}
	session.Stderr = io.Discard
	if err := session.RequestSubsystem("sftp"); err != nil {
		_ = session.Close()
		return nil, err
	}
	client, err := newClientPipe(reader, writer, options...)
	if err != nil {
		_ = session.Close()
		return nil, err
	}
	go func() { _ = client.Wait(); _ = session.Close() }()
	return client, nil
}

func newClientPipe(reader io.Reader, writer io.WriteCloser, options ...pkgsftp.ClientOption) (*Client, error) {
	attributes := &noFollowAttributesWriter{destination: writer}
	client, err := pkgsftp.NewClientPipe(&attributeReader{source: reader}, attributes, options...)
	if err != nil {
		return nil, err
	}
	remote := NewClient(client)
	remote.noFollowAttributes = true
	version, supported := client.HasExtension(noFollowSetstatExtension)
	attributes.permissions = supported && version == "1"
	return remote, nil
}
