package remotesync_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"sshc/internal/remotesync"
)

func TestExistingLargeBackgroundCanBePulled(t *testing.T) {
	bucket := &fakeBucket{}
	asset := "sshc/backgrounds/large.png"
	writer := newInstallation(t, bucket, map[string]string{asset: strings.Repeat("x", 2<<20)})
	if _, err := writer.service.PushUsing(context.Background(), keyOf(syncPassphrase), ""); err != nil {
		t.Fatalf("push: %v", err)
	}
	receiver := newInstallation(t, bucket, map[string]string{asset: strings.Repeat("x", 2<<20)})
	if _, err := receiver.service.Pull(context.Background(), syncPassphrase, remotesync.ResolveNone); err != nil && !errors.Is(err, remotesync.ErrNothingToApply) {
		t.Fatalf("pull identical 2 MiB image: %v", err)
	}
}

func TestLargeBackgroundCanBeAppliedAndReplacedBySync(t *testing.T) {
	bucket := &fakeBucket{}
	const asset = "sshc/backgrounds/large.png"
	writer := newInstallation(t, bucket, nil)
	receiver := newInstallation(t, bucket, nil)
	for _, contents := range []string{strings.Repeat("x", 2<<20), strings.Repeat("y", 2<<20)} {
		writer.write(t, asset, contents)
		if _, err := writer.service.PushUsing(t.Context(), keyOf(syncPassphrase), ""); err != nil {
			t.Fatal(err)
		}
		preview, err := receiver.service.Pull(t.Context(), syncPassphrase, remotesync.ResolveNone)
		if err != nil {
			t.Fatal(err)
		}
		if err := applyPreview(receiver.service, remotesync.ResolveNone, "", preview); err != nil {
			t.Fatalf("apply large background: %v", err)
		}
		if actual := receiver.read(t, asset); actual != contents {
			t.Fatal("synced image does not match the published contents")
		}
	}
}
