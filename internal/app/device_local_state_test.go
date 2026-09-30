package app

import (
	"path"
	"testing"

	"sshc/internal/application"
	"sshc/internal/remotesync"
	"sshc/internal/storage"
)

// app と application が持つこのマシンだけの状態は、remotesync からは参照しない。名前を
// 変えたときに同期の除外から外れないよう、持ち主の定数で照合する。
func TestSyncNeverCarriesTheDeviceLocalStateThatAppAndApplicationOwn(t *testing.T) {
	for _, relative := range []string{
		vpnStateDirectory,
		path.Join(vpnStateDirectory, "relay.sock"),
		path.Join(storage.StateDirectoryName, sftpTransferStateName),
		application.EngineSettingsPathRelative,
	} {
		if !remotesync.NeverTravels(relative) {
			t.Errorf("%s would travel to other machines", relative)
		}
	}
}
