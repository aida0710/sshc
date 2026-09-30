package sftp_test

import (
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"sshc/internal/sftp"
)

func TestRestoringStoredTransferSettingsReplacesOnlyTheOutOfRangeOnes(t *testing.T) {
	// A hand-edited metadata.json, or a later version with a narrower range,
	// can leave one stored setting out of range.
	manager := newTestTransferManager(t, nil)
	rejected := manager.RestoreTransferSettings(sftp.TransferSettings{
		MaxConcurrent: 3, ProcessingStopped: true,
		LargeFileParallelism: sftp.MaxLargeFileParallelism + 1, LargeFileChunkBytes: 64 << 20,
	})
	if !slices.Equal(rejected, []string{"largeFileParallelism"}) {
		t.Fatalf("rejected = %v, want only the out-of-range connection count", rejected)
	}
	if manager.MaxConcurrent() != 3 || !manager.ProcessingStopped() || manager.ClearCompletedAfter() != 0 ||
		manager.LargeFileThreshold() != sftp.DefaultLargeFileThreshold ||
		manager.LargeFileParallelism() != sftp.DefaultLargeFileParallelism || manager.LargeFileChunkBytes() != 64<<20 {
		t.Fatalf("restored = %d concurrent, stopped %v, clear after %v, split %d/%d/%d",
			manager.MaxConcurrent(), manager.ProcessingStopped(), manager.ClearCompletedAfter(),
			manager.LargeFileThreshold(), manager.LargeFileParallelism(), manager.LargeFileChunkBytes())
	}
}

func TestTransferSettingsThatCannotBeSavedLeaveTheEngineAsItWas(t *testing.T) {
	manager := newTestTransferManager(t, nil)
	unwritable := errors.New("metadata.json cannot be written")
	manager.EnableTransferSettingsPersistence(func(sftp.TransferSettings) error { return unwritable })
	settings := sftp.DefaultTransferSettings()
	settings.MaxConcurrent, settings.ProcessingStopped = 5, true
	if err := manager.SetTransferSettings(settings); !errors.Is(err, unwritable) {
		t.Fatalf("SetTransferSettings() = %v, want the save error", err)
	}
	if manager.MaxConcurrent() != sftp.DefaultTransferConcurrency || manager.ProcessingStopped() {
		t.Fatalf("the engine took settings that were not saved: %d concurrent, stopped %v",
			manager.MaxConcurrent(), manager.ProcessingStopped())
	}
}

// overtakeWindow is how long the test gives a second settings update to get
// past the first while the first is still being saved.
const overtakeWindow = 50 * time.Millisecond

func TestOverlappingSettingsUpdatesLeaveTheEngineOnTheLastSavedSettings(t *testing.T) {
	manager := newTestTransferManager(t, nil)
	firstSaving := make(chan struct{})
	releaseFirst := make(chan struct{})
	var savedMutex sync.Mutex
	var saved []int
	manager.EnableTransferSettingsPersistence(func(settings sftp.TransferSettings) error {
		savedMutex.Lock()
		saved = append(saved, settings.MaxConcurrent)
		first := len(saved) == 1
		savedMutex.Unlock()
		if first {
			close(firstSaving)
			<-releaseFirst
		}
		return nil
	})
	first, second := sftp.DefaultTransferSettings(), sftp.DefaultTransferSettings()
	first.MaxConcurrent, second.MaxConcurrent = 3, 5
	firstDone, secondDone := make(chan error, 1), make(chan error, 1)
	go func() { firstDone <- manager.SetTransferSettings(first) }()
	<-firstSaving
	go func() { secondDone <- manager.SetTransferSettings(second) }()
	select {
	case err := <-secondDone:
		t.Fatalf("the second update finished while the first was being saved: %v", err)
	case <-time.After(overtakeWindow):
	}
	close(releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}
	savedMutex.Lock()
	defer savedMutex.Unlock()
	if !slices.Equal(saved, []int{3, 5}) || manager.MaxConcurrent() != 5 {
		t.Fatalf("saved %v, engine runs %d concurrent, want both on the last update", saved, manager.MaxConcurrent())
	}
}
