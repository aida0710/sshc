package sftp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"sshc/internal/sftp"
)

func TestTransferQueueQuarantinesCorruptSnapshotAndStartsEmpty(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "sshc", "transfers.json")
	if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
		t.Fatal(err)
	}
	corrupt := []byte(`{"schemaVersion":1,"jobs":[`)
	if err := os.WriteFile(filename, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}

	manager := sftp.NewTransferManager(nil)
	if err := manager.EnableQueuePersistence(filename); err != nil {
		t.Fatalf("EnableQueuePersistence(corrupt) = %v", err)
	}
	if jobs := listJobs(t, manager); len(jobs) != 0 {
		t.Fatalf("restored corrupt jobs = %+v", jobs)
	}

	backups, err := filepath.Glob(filepath.Join(filepath.Dir(filename), ".sshc-transfers.json.corrupt-*"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("corrupt backups = %v, %v", backups, err)
	}
	preserved, err := os.ReadFile(backups[0])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(preserved, corrupt) {
		t.Fatalf("preserved corrupt queue = %q, want %q", preserved, corrupt)
	}
	active, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	var clean struct {
		SchemaVersion int               `json:"schemaVersion"`
		Jobs          []json.RawMessage `json:"jobs"`
	}
	if err := json.Unmarshal(active, &clean); err != nil || clean.SchemaVersion != 1 || len(clean.Jobs) != 0 {
		t.Fatalf("replacement queue = %s, %v", active, err)
	}
}

func TestTransferQueueRestoresAfterEngineRestart(t *testing.T) {
	t.Parallel()
	filename := filepath.Join(t.TempDir(), "sshc", "transfers.json")
	first := sftp.NewTransferManager(nil)
	if err := first.EnableQueuePersistence(filename); err != nil {
		t.Fatal(err)
	}
	created, err := first.CreateJob(sftp.CreateTransferJob{
		ID: "transfer_persist_01", BatchID: "batch_persist_01", Alias: "edge",
		Direction: sftp.TransferDownload, Kind: sftp.TransferFile, Name: "large.bin",
		RemotePath: "/large.bin", TotalBytes: 4096,
		LargeFileThresholdBytes: 50 << 20, LargeFileParallelism: 6, LargeFileChunkBytes: 512 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.UpdateJob(created.ID, sftp.UpdateTransferJob{Action: sftp.TransferPauseAction}); err != nil {
		t.Fatal(err)
	}
	second := sftp.NewTransferManager(nil)
	if err := second.EnableQueuePersistence(filename); err != nil {
		t.Fatal(err)
	}
	jobs := listJobs(t, second)
	if len(jobs) != 1 || jobs[0].ID != created.ID || jobs[0].Status != sftp.TransferPaused ||
		jobs[0].LargeFileThresholdBytes != 50<<20 || jobs[0].LargeFileParallelism != 6 || jobs[0].LargeFileChunkBytes != 512<<20 {
		t.Fatalf("restored jobs = %#v", jobs)
	}
	info, err := filepath.Glob(filepath.Join(filepath.Dir(filename), ".transfers-*.tmp"))
	if err != nil || len(info) != 0 {
		t.Fatalf("temporary queue files = %v, %v", info, err)
	}
}

func TestTransferQueueMutationFailureRollsBackMemory(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		manager, filename := persistentManager(t)
		blockQueueWrites(t, filename)
		if _, err := manager.CreateJob(downloadJob("transfer_create1", "/create")); err == nil {
			t.Fatal("CreateJob succeeded while its durable queue was unwritable")
		}
		if jobs := listJobs(t, manager); len(jobs) != 0 {
			t.Fatalf("failed create remained in memory = %+v", jobs)
		}
	})

	t.Run("state transition", func(t *testing.T) {
		manager, filename := persistentManager(t)
		created, err := manager.CreateJob(downloadJob("transfer_update1", "/update"))
		if err != nil {
			t.Fatal(err)
		}
		blockQueueWrites(t, filename)
		if _, err := manager.UpdateJob(created.ID, sftp.UpdateTransferJob{Action: sftp.TransferPauseAction}); err == nil {
			t.Fatal("UpdateJob succeeded while its durable queue was unwritable")
		}
		jobs := listJobs(t, manager)
		if len(jobs) != 1 || jobs[0].Status != sftp.TransferQueued {
			t.Fatalf("failed transition remained in memory = %+v", jobs)
		}
	})

	t.Run("reorder", func(t *testing.T) {
		manager, filename := persistentManager(t)
		for _, job := range []sftp.CreateTransferJob{
			downloadJob("transfer_order01", "/first"),
			downloadJob("transfer_order02", "/second"),
		} {
			if _, err := manager.CreateJob(job); err != nil {
				t.Fatal(err)
			}
		}
		blockQueueWrites(t, filename)
		if err := manager.MoveQueuedJob("transfer_order02", sftp.TransferMoveTop); err == nil {
			t.Fatal("MoveQueuedJob succeeded while its durable queue was unwritable")
		}
		jobs := listJobs(t, manager)
		if len(jobs) != 2 || jobs[0].ID != "transfer_order01" || jobs[1].ID != "transfer_order02" {
			t.Fatalf("failed reorder remained in memory = %+v", jobs)
		}
	})

	t.Run("clear", func(t *testing.T) {
		manager, filename := persistentManager(t)
		created, err := manager.CreateJob(downloadJob("transfer_clear01", "/clear"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := manager.UpdateJob(created.ID, sftp.UpdateTransferJob{Action: sftp.TransferCancelAction}); err != nil {
			t.Fatal(err)
		}
		blockQueueWrites(t, filename)
		if removed, err := manager.ClearFinished(); err == nil || removed != 0 {
			t.Fatalf("ClearFinished = %d, %v; want rollback error", removed, err)
		}
		jobs := listJobs(t, manager)
		if len(jobs) != 1 || jobs[0].Status != sftp.TransferCancelled {
			t.Fatalf("failed clear removed memory record = %+v", jobs)
		}
	})

	t.Run("remove", func(t *testing.T) {
		manager, filename := persistentManager(t)
		created, err := manager.CreateJob(downloadJob("transfer_remove1", "/remove"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := manager.UpdateJob(created.ID, sftp.UpdateTransferJob{Action: sftp.TransferCancelAction}); err != nil {
			t.Fatal(err)
		}
		blockQueueWrites(t, filename)
		if err := manager.RemoveJob(created.ID); err == nil {
			t.Fatal("RemoveJob succeeded while its durable queue was unwritable")
		}
		jobs := listJobs(t, manager)
		if len(jobs) != 1 || jobs[0].ID != created.ID {
			t.Fatalf("failed remove changed memory queue = %+v", jobs)
		}
	})
}

func TestTransferQueueSweepFailureRollsBackEveryJob(t *testing.T) {
	base := time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)

	t.Run("update with another stale job", func(t *testing.T) {
		now := base
		manager, filename := persistentManagerWithClock(t, &now)
		stale := downloadJob("transfer_stale01", "/stale")
		target := downloadJob("transfer_target01", "/target")
		for _, job := range []sftp.CreateTransferJob{stale, target} {
			if _, err := manager.CreateJob(job); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := manager.UpdateJob(stale.ID, sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
			t.Fatal(err)
		}
		now = base.Add(3 * time.Minute)
		blockQueueWrites(t, filename)
		if _, err := manager.UpdateJob(target.ID, sftp.UpdateTransferJob{Action: sftp.TransferPauseAction}); err == nil {
			t.Fatal("UpdateJob succeeded after the stale sweep could not be persisted")
		}

		now = base
		unblockQueueWrites(t, filename)
		jobs := listJobs(t, manager)
		if len(jobs) != 2 || jobs[0].Status != sftp.TransferRunning || jobs[1].Status != sftp.TransferQueued {
			t.Fatalf("queue after failed sweep/update = %+v", jobs)
		}
		if _, err := manager.UpdateJob(target.ID, sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err == nil {
			t.Fatal("failed sweep did not restore the active concurrency slot")
		}
	})

	t.Run("idempotent create", func(t *testing.T) {
		now := base
		manager, filename := persistentManagerWithClock(t, &now)
		stale := downloadJob("transfer_stale02", "/stale")
		existing := downloadJob("transfer_exists01", "/existing")
		for _, job := range []sftp.CreateTransferJob{stale, existing} {
			if _, err := manager.CreateJob(job); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := manager.UpdateJob(stale.ID, sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
			t.Fatal(err)
		}
		now = base.Add(3 * time.Minute)
		blockQueueWrites(t, filename)
		if _, err := manager.CreateJob(existing); err == nil {
			t.Fatal("idempotent CreateJob hid a stale-sweep persistence failure")
		}
		now = base
		unblockQueueWrites(t, filename)
		jobs := listJobs(t, manager)
		if len(jobs) != 2 || jobs[0].Status != sftp.TransferRunning || jobs[1].Status != sftp.TransferQueued {
			t.Fatalf("queue after failed sweep/idempotent create = %+v", jobs)
		}
	})

	t.Run("list", func(t *testing.T) {
		now := base
		manager, filename := persistentManagerWithClock(t, &now)
		stale := downloadJob("transfer_stale03", "/stale")
		if _, err := manager.CreateJob(stale); err != nil {
			t.Fatal(err)
		}
		if _, err := manager.UpdateJob(stale.ID, sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
			t.Fatal(err)
		}
		now = base.Add(3 * time.Minute)
		blockQueueWrites(t, filename)
		if jobs, err := manager.ListJobs(); err == nil || jobs != nil {
			t.Fatalf("ListJobs = %+v, %v; want durable sweep error", jobs, err)
		}
		now = base
		unblockQueueWrites(t, filename)
		jobs := listJobs(t, manager)
		if len(jobs) != 1 || jobs[0].Status != sftp.TransferRunning {
			t.Fatalf("failed list sweep changed memory = %+v", jobs)
		}
	})

	t.Run("expiration", func(t *testing.T) {
		now := base
		manager, filename := persistentManagerWithClock(t, &now)
		if err := manager.SetTransferSettings(
			2, sftp.MinClearCompletedAfter, false,
			sftp.DefaultLargeFileThreshold, sftp.DefaultLargeFileParallelism, sftp.DefaultLargeFileChunkBytes,
		); err != nil {
			t.Fatal(err)
		}
		finished := downloadJob("transfer_expire1", "/finished")
		if _, err := manager.CreateJob(finished); err != nil {
			t.Fatal(err)
		}
		if _, err := manager.UpdateJob(finished.ID, sftp.UpdateTransferJob{Action: sftp.TransferCancelAction}); err != nil {
			t.Fatal(err)
		}
		now = base.Add(sftp.MinClearCompletedAfter)
		blockQueueWrites(t, filename)
		if jobs, err := manager.ListJobs(); err == nil || jobs != nil {
			t.Fatalf("ListJobs expiration = %+v, %v; want durable removal error", jobs, err)
		}
		now = base
		unblockQueueWrites(t, filename)
		jobs := listJobs(t, manager)
		if len(jobs) != 1 || jobs[0].Status != sftp.TransferCancelled {
			t.Fatalf("failed expiration removed memory record = %+v", jobs)
		}
	})
}

func TestRemoteTransferCommitFailureRequiresReconciliationWithoutRepeating(t *testing.T) {
	for _, operation := range []sftp.RemoteTransferOperation{sftp.RemoteCopy, sftp.RemoteMove} {
		t.Run(string(operation), func(t *testing.T) {
			source := remoteWith(map[string]node{"/source.bin": file("source.bin", "payload", 0o644)})
			target := remoteWith(nil)
			service := &sftp.Service{Open: func(_ context.Context, alias string) (sftp.Remote, error) {
				switch alias {
				case "source":
					return source, nil
				case "target":
					return target, nil
				default:
					return nil, errors.New("unexpected alias")
				}
			}}
			manager := sftp.NewTransferManager(service)
			filename := filepath.Join(t.TempDir(), "state", "transfers.json")
			if err := manager.EnableQueuePersistence(filename); err != nil {
				t.Fatal(err)
			}
			id := "transfer_reconcile_" + string(operation)
			job := sftp.CreateTransferJob{
				ID: id, BatchID: "batch_reconcile1", Alias: "target", RemotePath: "/target.bin",
				SourceAlias: "source", SourcePath: "/source.bin", Operation: operation, Overwrite: true,
				Direction: sftp.TransferRemote, Kind: sftp.TransferFile, Name: "source.bin", TotalBytes: -1,
			}
			if _, err := manager.CreateJob(job); err != nil {
				t.Fatal(err)
			}

			type hookOutcome struct {
				snapshot []byte
				err      error
			}
			hooked := make(chan hookOutcome, 1)
			var hookOnce sync.Once
			target.replaceHook = func() {
				hookOnce.Do(func() {
					snapshot, readErr := os.ReadFile(filename)
					if readErr == nil {
						readErr = blockQueueWritesRaw(filename)
					}
					hooked <- hookOutcome{snapshot: snapshot, err: readErr}
				})
			}
			manager.ScheduleRemoteJob(id)
			outcome := <-hooked
			if outcome.err != nil {
				t.Fatal(outcome.err)
			}

			deadline := time.Now().Add(time.Second)
			var current sftp.TransferJob
			for time.Now().Before(deadline) {
				jobs, err := manager.ListJobs()
				if err != nil {
					t.Fatal(err)
				}
				if len(jobs) == 1 {
					current = jobs[0]
				}
				if current.Status == sftp.TransferReattach {
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
			if current.Status != sftp.TransferReattach || current.Problem != sftp.RemoteReconciliationProblem {
				t.Fatalf("job after external commit/local failure = %+v", current)
			}
			if actions := sftp.AllowedTransferActions(current); len(actions) != 1 || actions[0] != sftp.TransferCancelControl {
				t.Fatalf("unsafe reconciliation actions = %v", actions)
			}
			if _, err := manager.UpdateJobFromClient(id, sftp.UpdateTransferJob{Action: sftp.TransferResumeAction}); !errors.Is(err, sftp.ErrTransferState) {
				t.Fatalf("resume uncertain remote operation = %v", err)
			}
			if got := string(target.nodes["/target.bin"].content); got != "payload" {
				t.Fatalf("published target = %q", got)
			}
			_, sourceErr := source.Lstat("/source.bin")
			if operation == sftp.RemoteMove && !errors.Is(sourceErr, fs.ErrNotExist) {
				t.Fatalf("move source still present: %v", sourceErr)
			}

			var intent struct {
				Jobs []sftp.TransferJob `json:"jobs"`
			}
			if err := json.Unmarshal(outcome.snapshot, &intent); err != nil || len(intent.Jobs) != 1 ||
				intent.Jobs[0].Status != sftp.TransferRunning || intent.Jobs[0].Problem != sftp.RemoteReconciliationProblem {
				t.Fatalf("durable pre-commit intent = %s, %v", outcome.snapshot, err)
			}
			if err := manager.Close(); err != nil {
				t.Fatal(err)
			}

			restartPath := filepath.Join(t.TempDir(), "transfers.json")
			if err := os.WriteFile(restartPath, outcome.snapshot, 0o600); err != nil {
				t.Fatal(err)
			}
			restarted := sftp.NewTransferManager(nil)
			if err := restarted.EnableQueuePersistence(restartPath); err != nil {
				t.Fatal(err)
			}
			restored := listJobs(t, restarted)
			if len(restored) != 1 || restored[0].Status != sftp.TransferReattach || restored[0].Problem != sftp.RemoteReconciliationProblem {
				t.Fatalf("restored uncertain operation = %+v", restored)
			}
		})
	}
}

func persistentManager(t *testing.T) (*sftp.TransferManager, string) {
	t.Helper()
	filename := filepath.Join(t.TempDir(), "state", "transfers.json")
	manager := sftp.NewTransferManager(nil)
	if err := manager.EnableQueuePersistence(filename); err != nil {
		t.Fatal(err)
	}
	return manager, filename
}

func persistentManagerWithClock(t *testing.T, now *time.Time) (*sftp.TransferManager, string) {
	t.Helper()
	filename := filepath.Join(t.TempDir(), "state", "transfers.json")
	manager := sftp.NewTransferManager(nil)
	manager.ConfigureJobs(1, func() time.Time { return *now })
	if err := manager.EnableQueuePersistence(filename); err != nil {
		t.Fatal(err)
	}
	return manager, filename
}

func blockQueueWrites(t *testing.T, filename string) {
	t.Helper()
	if err := blockQueueWritesRaw(filename); err != nil {
		t.Fatal(err)
	}
}

func blockQueueWritesRaw(filename string) error {
	directory := filepath.Dir(filename)
	if err := os.Remove(filename); err != nil {
		return err
	}
	if err := os.Remove(directory); err != nil {
		return err
	}
	if err := os.WriteFile(directory, []byte("not a directory"), 0o600); err != nil {
		return err
	}
	return nil
}

func unblockQueueWrites(t *testing.T, filename string) {
	t.Helper()
	directory := filepath.Dir(filename)
	if err := os.Remove(directory); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
}

func downloadJob(id, remotePath string) sftp.CreateTransferJob {
	return sftp.CreateTransferJob{
		ID: id, BatchID: "batch_persist_01", Alias: "edge",
		Direction: sftp.TransferDownload, Kind: sftp.TransferFile,
		Name: id, RemotePath: remotePath, TotalBytes: 1,
	}
}

func listJobs(t *testing.T, manager *sftp.TransferManager) []sftp.TransferJob {
	t.Helper()
	jobs, err := manager.ListJobs()
	if err != nil {
		t.Fatal(err)
	}
	return jobs
}
