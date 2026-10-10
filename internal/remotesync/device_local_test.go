package remotesync_test

import (
	"errors"
	"path"
	"testing"

	"sshc/internal/browserauth"
	"sshc/internal/handoff"
	"sshc/internal/keys"
	"sshc/internal/recent"
	"sshc/internal/remotesync"
	"sshc/internal/secret"
	"sshc/internal/selfupdate"
	"sshc/internal/storage"
	terminalworkspace "sshc/internal/workspace"
)

// ownedDeviceLocalPaths は、このマシンだけのものとして持ち主のパッケージが決めた
// パスを、持ち主の定数から集める。app が持つパスは app のテストが照合する。
func ownedDeviceLocalPaths() []string {
	return append([]string{
		path.Join(storage.StateDirectoryName, selfupdate.StateFileName),
		path.Join(storage.StateDirectoryName, selfupdate.StateFileName+selfupdate.StateLockSuffix),
		path.Join(storage.StateDirectoryName, selfupdate.StateFileName+selfupdate.RestartLockSuffix),
		secret.LocalKeyPath,
		secret.SettingsPath,
		path.Join(storage.StateDirectoryName, handoff.FileName),
		path.Join(storage.StateDirectoryName, handoff.MutationLockName),
		keys.TrashPathRelative,
		recent.PathRelative,
		browserauth.PathRelative,
		terminalworkspace.PathRelative,
	}, storage.DeviceLocalPaths()...)
}

// ownedDeviceLocalFiles は、持ち主のパスそのものと、そのパスがディレクトリだった
// ときの中身の両方を、置くファイルの例として返す。
func ownedDeviceLocalFiles() []string {
	var files []string
	for _, owned := range ownedDeviceLocalPaths() {
		files = append(files, owned, path.Join(owned, "entry"))
	}
	return files
}

func TestCollectLeavesOutEveryOwnersDeviceLocalPath(t *testing.T) {
	for _, name := range ownedDeviceLocalFiles() {
		t.Run(name, func(t *testing.T) {
			installation := newInstallation(t, &fakeBucket{}, map[string]string{
				"config": "Host bastion\n",
				name:     "this machine only",
			})
			manifest, contents, err := installation.service.Collect()
			if err != nil {
				t.Fatalf("Collect = %v", err)
			}
			for _, entry := range manifest.Files {
				if entry.Path == name {
					t.Fatalf("the snapshot carries %s", name)
				}
			}
			if _, carried := contents[name]; carried {
				t.Fatalf("%s is in the archive even though the manifest omits it", name)
			}
		})
	}
}

func TestReadRefusesEveryOwnersDeviceLocalPath(t *testing.T) {
	for _, name := range ownedDeviceLocalFiles() {
		t.Run(name, func(t *testing.T) {
			archive := handBuilt(t, map[string]string{name: "attacker controlled"}, remotesync.Manifest{
				SchemaVersion: remotesync.SchemaVersion,
				CreatedAt:     "2026-08-30T00:00:00Z", Origin: "attacker", Message: "Reserved path",
				Files: []remotesync.Entry{entry(name, "attacker controlled")},
			})
			if _, _, err := remotesync.Read(archive); !errors.Is(err, remotesync.ErrUnsafePath) {
				t.Fatalf("Read = %v, want ErrUnsafePath", err)
			}
		})
	}
}
