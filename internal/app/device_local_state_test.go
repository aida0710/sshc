package app

import (
	"path"
	"testing"

	"sshc/internal/remotesync"
	"sshc/internal/storage"
)

// app が持つこのマシンだけの状態は、remotesync からは参照できない。名前を変えたときに
// 同期の除外から外れないよう、持ち主の定数で照合する。
func TestSyncNeverCarriesTheDeviceLocalStateThatAppOwns(t *testing.T) {
	for _, relative := range []string{
		vpnStateDirectory,
		path.Join(vpnStateDirectory, "relay.sock"),
		path.Join(storage.StateDirectoryName, sftpTransferStateName),
	} {
		if !remotesync.NeverTravels(relative) {
			t.Errorf("%s would travel to other machines", relative)
		}
	}
}
