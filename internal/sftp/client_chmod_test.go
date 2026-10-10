package sftp

import "testing"

func TestPermissionChangesUseNoFollowPacketsAndRefuseUnsupportedConnections(t *testing.T) {
	testNoFollowAttributeChange(t, func(client *Client) error { return client.ChmodNoFollow("/shared/file", 0o644) },
		[]uint32{sftpPermissionsAttributes, 0o644})
}
