package sftp_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"sshc/internal/sftp"
)

const (
	// remoteJobSettleTimeout bounds how long a test waits for a fake-backed
	// remote job; the fakes answer at once, so this only guards against a
	// stalled worker.
	remoteJobSettleTimeout = 3 * time.Second
	// remoteJobPollInterval is how often a test looks at the queue while it
	// waits. The fakes settle within microseconds, so a short interval keeps
	// the tests fast without spinning.
	remoteJobPollInterval = 5 * time.Millisecond
)

// fakeConnection stands for one SFTP connection to a shared fake. Workers that
// overlap each open and close their own connection, which must not race on
// the fake's single closed flag.
type fakeConnection struct{ *fakeRemote }

func (fakeConnection) Close() error { return nil }

func newCopyJobManager(t *testing.T, source, target *fakeRemote) *sftp.TransferManager {
	t.Helper()
	service := &sftp.Service{Open: func(_ context.Context, alias string) (sftp.Remote, error) {
		switch alias {
		case "source":
			return fakeConnection{source}, nil
		case "target":
			return fakeConnection{target}, nil
		default:
			return nil, errors.New("unexpected alias")
		}
	}}
	return newTestTransferManager(t, service)
}

// createRemoteJob queues a copy or move of /source.bin on "source" to
// /copy.bin on "target".
func createRemoteJob(t *testing.T, manager *sftp.TransferManager, id string, operation sftp.RemoteTransferOperation) {
	t.Helper()
	if _, err := manager.CreateJob(sftp.CreateTransferJob{
		ID: id, BatchID: "batch_" + id, Alias: "target", RemotePath: "/copy.bin",
		SourceAlias: "source", SourcePath: "/source.bin", Operation: operation,
		Direction: sftp.TransferRemote, Kind: sftp.TransferFile, Name: "source.bin", TotalBytes: -1,
	}); err != nil {
		t.Fatal(err)
	}
}

func waitForJob(t *testing.T, manager *sftp.TransferManager, id string, settled func(sftp.TransferJob) bool) sftp.TransferJob {
	t.Helper()
	deadline := time.Now().Add(remoteJobSettleTimeout)
	var current sftp.TransferJob
	for time.Now().Before(deadline) {
		for _, job := range listJobs(t, manager) {
			if job.ID == id {
				current = job
			}
		}
		if settled(current) {
			return current
		}
		time.Sleep(remoteJobPollInterval)
	}
	t.Fatalf("job %s did not settle: %+v", id, current)
	return current
}

func hasStatus(status sftp.TransferJobStatus) func(sftp.TransferJob) bool {
	return func(job sftp.TransferJob) bool { return job.Status == status }
}

func TestRemoteJobPausedDuringItsOperationCanBeResumed(t *testing.T) {
	source := remoteWith(map[string]node{"/source.bin": file("source.bin", "payload", 0o644)})
	target := remoteWith(nil)
	operationStarted := make(chan struct{})
	releaseOperation := make(chan struct{})
	var firstCreate sync.Once
	target.createHook = func() {
		firstCreate.Do(func() {
			close(operationStarted)
			<-releaseOperation
		})
	}
	manager := newCopyJobManager(t, source, target)
	defer manager.Close()
	var release sync.Once
	releaseStalledOperation := func() { release.Do(func() { close(releaseOperation) }) }
	// Runs before Close, which waits for the stalled worker, when the test fails early.
	defer releaseStalledOperation()
	const id = "transfer_pause_operation"
	createRemoteJob(t, manager, id, sftp.RemoteCopy)
	<-operationStarted

	paused, err := manager.UpdateJobFromClient(id, sftp.UpdateTransferJob{Action: sftp.TransferPauseAction})
	if err != nil {
		t.Fatal(err)
	}
	// Until the worker reports how the operation ended, the engine must not offer
	// a resume it would refuse.
	if actions := sftp.AllowedTransferActions(paused); !slices.Equal(actions, []sftp.TransferControlAction{sftp.TransferCancelControl}) {
		t.Fatalf("actions while the operation is in flight = %v", actions)
	}
	if _, err := manager.UpdateJobFromClient(id, sftp.UpdateTransferJob{Action: sftp.TransferResumeAction}); !errors.Is(err, sftp.ErrTransferState) {
		t.Fatalf("resume while the operation is in flight = %v", err)
	}
	releaseStalledOperation()

	interrupted := waitForJob(t, manager, id, func(job sftp.TransferJob) bool {
		return job.Status == sftp.TransferPaused && job.Problem == ""
	})
	if !slices.Contains(sftp.AllowedTransferActions(interrupted), sftp.TransferResumeControl) {
		t.Fatalf("interrupted job offers %v", sftp.AllowedTransferActions(interrupted))
	}
	if _, err := manager.UpdateJobFromClient(id, sftp.UpdateTransferJob{Action: sftp.TransferResumeAction}); err != nil {
		t.Fatalf("resume after the interruption = %v", err)
	}
	waitForJob(t, manager, id, hasStatus(sftp.TransferCompleted))
	if got := string(target.nodes["/copy.bin"].content); got != "payload" {
		t.Fatalf("copied content = %q", got)
	}
}

func TestResumeDuringASlowPlanRunsANewWorkerAndIgnoresTheOldOne(t *testing.T) {
	source := remoteWith(map[string]node{"/source.bin": file("source.bin", "payload", 0o644)})
	target := remoteWith(nil)
	planStarted := make(chan struct{})
	releasePlan := make(chan struct{})
	var mutex sync.Mutex
	sourceLstats := 0
	source.lstatHook = func(string) error {
		mutex.Lock()
		sourceLstats++
		first := sourceLstats == 1
		mutex.Unlock()
		if !first {
			return nil
		}
		close(planStarted)
		<-releasePlan
		return errors.New("the stalled request finally failed")
	}
	manager := newCopyJobManager(t, source, target)
	defer manager.Close()
	var release sync.Once
	releaseStalledPlan := func() { release.Do(func() { close(releasePlan) }) }
	// Runs before Close, which waits for the stalled worker, when the test fails early.
	defer releaseStalledPlan()
	const id = "transfer_slow_plan"
	createRemoteJob(t, manager, id, sftp.RemoteCopy)
	<-planStarted

	if _, err := manager.UpdateJobFromClient(id, sftp.UpdateTransferJob{Action: sftp.TransferPauseAction}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJobFromClient(id, sftp.UpdateTransferJob{Action: sftp.TransferResumeAction}); err != nil {
		t.Fatal(err)
	}
	waitForJob(t, manager, id, hasStatus(sftp.TransferCompleted))

	releaseStalledPlan()
	// Close waits for every worker, including the one that was stuck planning.
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}
	for _, job := range listJobs(t, manager) {
		if job.ID == id && job.Status != sftp.TransferCompleted {
			t.Fatalf("the paused run's late failure replaced the result: %s (%s)", job.Status, job.Problem)
		}
	}
}

func TestPauseAndResumeWhileWaitingForASlotStillRunsTheJob(t *testing.T) {
	source := remoteWith(map[string]node{"/source.bin": file("source.bin", "payload", 0o644)})
	target := remoteWith(nil)
	manager := newCopyJobManager(t, source, target)
	manager.ConfigureJobs(1, time.Now)
	defer manager.Close()
	const slotHolder = "transfer_slot_holder"
	if _, err := manager.CreateJob(sftp.CreateTransferJob{
		ID: slotHolder, BatchID: "batch_slot_holder", Alias: "edge",
		Direction: sftp.TransferDownload, Kind: sftp.TransferFile, Name: "held.bin", RemotePath: "/held.bin", TotalBytes: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateJob(slotHolder, sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
		t.Fatal(err)
	}
	const id = "transfer_waiting_slot"
	createRemoteJob(t, manager, id, sftp.RemoteCopy)

	// Each round replaces the waiting worker; several rounds let an old worker
	// wake after its replacement started, which is the case under test.
	const pauseResumeRounds = 20
	for range pauseResumeRounds {
		if _, err := manager.UpdateJobFromClient(id, sftp.UpdateTransferJob{Action: sftp.TransferPauseAction}); err != nil {
			t.Fatal(err)
		}
		if _, err := manager.UpdateJobFromClient(id, sftp.UpdateTransferJob{Action: sftp.TransferResumeAction}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := manager.UpdateJobFromClient(slotHolder, sftp.UpdateTransferJob{Action: sftp.TransferCancelAction}); err != nil {
		t.Fatal(err)
	}
	waitForJob(t, manager, id, hasStatus(sftp.TransferCompleted))
}

func TestMoveThatRemovedTheSourceWhilePausedOffersOnlyCancel(t *testing.T) {
	source := remoteWith(map[string]node{"/source.bin": file("source.bin", "payload", 0o644)})
	target := remoteWith(nil)
	removalStarted := make(chan struct{})
	releaseRemoval := make(chan struct{})
	var firstRemoval sync.Once
	source.removeHook = func(candidate string) {
		if candidate != "/source.bin" {
			return
		}
		firstRemoval.Do(func() {
			close(removalStarted)
			<-releaseRemoval
		})
	}
	manager := newCopyJobManager(t, source, target)
	var release sync.Once
	releaseStalledRemoval := func() { release.Do(func() { close(releaseRemoval) }) }
	// Runs before Close, which waits for the stalled worker, when the test fails early.
	defer releaseStalledRemoval()
	const id = "transfer_move_unrecorded"
	createRemoteJob(t, manager, id, sftp.RemoteMove)
	<-removalStarted

	// The copy is published and the source is being removed: the move finishes
	// even though the pause ends the run, and its result can no longer be
	// recorded.
	if _, err := manager.UpdateJobFromClient(id, sftp.UpdateTransferJob{Action: sftp.TransferPauseAction}); err != nil {
		t.Fatal(err)
	}
	releaseStalledRemoval()
	if _, err := manager.UpdateJobFromClient(id, sftp.UpdateTransferJob{Action: sftp.TransferResumeAction}); !errors.Is(err, sftp.ErrTransferState) {
		t.Fatalf("resume of a move that may have finished = %v, want %v", err, sftp.ErrTransferState)
	}
	// Close waits for the worker, so what follows is the state it left.
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}
	if _, exists := source.nodes["/source.bin"]; exists {
		t.Fatal("the move did not remove the source")
	}
	for _, job := range listJobs(t, manager) {
		if job.ID != id {
			continue
		}
		if job.Status != sftp.TransferPaused || job.Problem != sftp.RemoteReconciliationProblem {
			t.Fatalf("job after the move finished = %s (%q), want paused and marked", job.Status, job.Problem)
		}
		// validateTransferUpdate refuses resume and retry by the same predicate.
		if actions := sftp.AllowedTransferActions(job); !slices.Equal(actions, []sftp.TransferControlAction{sftp.TransferCancelControl}) {
			t.Fatalf("actions after the move finished = %v, want only cancel", actions)
		}
	}
}
