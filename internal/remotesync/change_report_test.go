package remotesync_test

import (
	"context"
	"slices"
	"testing"

	"sshc/internal/remotesync"
)

// push と pull が、追加・変更・削除のどれを行ったかをパスで報告することを確かめる。

func TestPushReportsTheFilesItAddedChangedAndRemoved(t *testing.T) {
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{
		"config":               "Host bastion\n",
		"keys/work/id_ed25519": "-----BEGIN OPENSSH PRIVATE KEY-----\nfirst\n",
	})

	first, err := machine.service.PushUsing(context.Background(), keyOf(syncPassphrase), "")
	if err != nil {
		t.Fatalf("Push = %v", err)
	}
	// 最初の push には親が無いので、収集したファイルはすべて追加である。
	expectPaths(t, "first push added", first.Added, []string{"config", "keys/work/id_ed25519"})
	expectPaths(t, "first push modified", first.Modified, nil)
	expectPaths(t, "first push removed", first.Removed, nil)

	machine.write(t, "config", "Host bastion\n\tPort 2222\n")
	machine.write(t, "connections/work/lon.conf", "Host lon\n")
	machine.remove(t, "keys/work/id_ed25519")

	second, err := machine.service.PushUsing(context.Background(), keyOf(syncPassphrase), "")
	if err != nil {
		t.Fatalf("Push = %v", err)
	}
	expectPaths(t, "second push added", second.Added, []string{"connections/work/lon.conf"})
	expectPaths(t, "second push modified", second.Modified, []string{"config"})
	expectPaths(t, "second push removed", second.Removed, []string{"keys/work/id_ed25519"})
}

func TestPullSeparatesNewFilesFromTheOnesItReplaces(t *testing.T) {
	bucket := &fakeBucket{}
	sender := newInstallation(t, bucket, map[string]string{
		"config":               "Host bastion\n",
		"keys/work/id_ed25519": "-----BEGIN OPENSSH PRIVATE KEY-----\nfirst\n",
	})
	if _, err := sender.service.PushUsing(context.Background(), keyOf(syncPassphrase), ""); err != nil {
		t.Fatalf("Push = %v", err)
	}

	receiver := newInstallation(t, bucket, map[string]string{})
	empty, err := receiver.service.Pull(context.Background(), syncPassphrase, remotesync.ResolveNone)
	if err != nil {
		t.Fatalf("Pull = %v", err)
	}
	// 空のワークスペースでは、書き込むファイルがそのまま追加になる。
	expectPaths(t, "first pull written", empty.Written, []string{"config", "keys/work/id_ed25519"})
	expectPaths(t, "first pull added", empty.Added, []string{"config", "keys/work/id_ed25519"})
	if err := applyPreview(receiver.service, remotesync.ResolveNone, "", empty); err != nil {
		t.Fatalf("Apply = %v", err)
	}

	sender.write(t, "config", "Host bastion\n\tPort 2222\n")
	sender.write(t, "connections/work/lon.conf", "Host lon\n")
	if _, err := sender.service.PushUsing(context.Background(), keyOf(syncPassphrase), ""); err != nil {
		t.Fatalf("Push = %v", err)
	}

	second, err := receiver.service.Pull(context.Background(), syncPassphrase, remotesync.ResolveNone)
	if err != nil {
		t.Fatalf("Pull = %v", err)
	}
	expectPaths(t, "second pull written", second.Written, []string{"config", "connections/work/lon.conf"})
	// すでにある config は置き換えなので、added はここに来ていない方だけになる。
	expectPaths(t, "second pull added", second.Added, []string{"connections/work/lon.conf"})
}

func expectPaths(t *testing.T, label string, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("%s = %v, want %v", label, got, want)
	}
}
