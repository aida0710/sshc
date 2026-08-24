package sftp

import (
	"context"
	"io"
	"io/fs"

	pkgsftp "github.com/pkg/sftp"
)

// Client は github.com/pkg/sftp の client を Remote 境界へ適合させる。
type Client struct {
	client *pkgsftp.Client
}

func NewClient(client *pkgsftp.Client) *Client {
	return &Client{client: client}
}

func (c *Client) Close() error { return c.client.Close() }

func (c *Client) ReadDir(ctx context.Context, path string) ([]fs.FileInfo, error) {
	return c.client.ReadDirContext(ctx, path)
}

func (c *Client) Lstat(path string) (fs.FileInfo, error) { return c.client.Lstat(path) }

func (c *Client) ReadLink(path string) (string, error) { return c.client.ReadLink(path) }

func (c *Client) Open(path string) (io.ReadCloser, error) { return c.client.Open(path) }

func (c *Client) Create(path string) (io.WriteCloser, error) { return c.client.Create(path) }

func (c *Client) Mkdir(path string) error { return c.client.Mkdir(path) }

func (c *Client) Chmod(path string, mode fs.FileMode) error { return c.client.Chmod(path, mode) }

// Replace は、OpenSSH の posix-rename 拡張が使える場合はそれを優先する。
// 拡張がないサーバーでは標準 SFTP rename を使い、置換可否はサーバー実装に従う。
func (c *Client) Replace(oldPath, newPath string) error {
	if _, ok := c.client.HasExtension("posix-rename@openssh.com"); ok {
		return c.client.PosixRename(oldPath, newPath)
	}
	return c.client.Rename(oldPath, newPath)
}

func (c *Client) Rename(oldPath, newPath string) error { return c.client.Rename(oldPath, newPath) }

func (c *Client) Remove(path string) error { return c.client.Remove(path) }

func (c *Client) RemoveDirectory(path string) error { return c.client.RemoveDirectory(path) }

var _ Remote = (*Client)(nil)
