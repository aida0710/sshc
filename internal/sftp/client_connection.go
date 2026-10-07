package sftp

import (
	"io"

	pkgsftp "github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// NewSSHClient preserves server attribute presence while using pkg/sftp for all operations.
func NewSSHClient(connection *ssh.Client, options ...pkgsftp.ClientOption) (*pkgsftp.Client, error) {
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
	client, err := pkgsftp.NewClientPipe(&attributeReader{source: reader}, writer, options...)
	if err != nil {
		_ = session.Close()
		return nil, err
	}
	go func() { _ = client.Wait(); _ = session.Close() }()
	return client, nil
}
