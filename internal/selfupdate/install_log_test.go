package selfupdate

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// jobWithLog stores a job in the given state and creates its empty log, as Start does.
func jobWithLog(t *testing.T, dependencies Dependencies, id, state string) *Service {
	t.Helper()
	store := Store{Path: dependencies.StatePath}
	plan := Plan{Current: "v1.0.0", Target: "v1.1.0"}
	if _, err := store.Change(func(job *Job) error {
		*job = Job{ID: id, Target: plan.Target, State: state, StartedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), OwnerPID: 101, Plan: plan}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := createLog(LogPath(dependencies.StatePath), id); err != nil {
		t.Fatal(err)
	}
	return New(dependencies)
}

const (
	firstJobID  = "0123456789abcdef0123456789abcdef"
	secondJobID = "fedcba9876543210fedcba9876543210"
)

func writeLog(t *testing.T, statePath, jobID, text string) {
	t.Helper()
	writer := OpenLog(statePath, jobID)
	defer writer.Close()
	if written, err := writer.Write([]byte(text)); err != nil || written != len(text) {
		t.Fatalf("Write = %d, %v", written, err)
	}
}

func TestTheLogKeepsInstallerOutputUpToItsCapacityAndSaysSo(t *testing.T) {
	dependencies := updateFixture(t)
	service := jobWithLog(t, dependencies, firstJobID, JobInstalling)
	writeLog(t, dependencies.StatePath, firstJobID, strings.Repeat("a", LogCapacity-2))
	// The installer is never failed by the log: a write past the capacity still succeeds.
	writeLog(t, dependencies.StatePath, firstJobID, "bcdef")
	writeLog(t, dependencies.StatePath, firstJobID, "dropped")

	log, err := service.Log(LogCapacity-4, 16)
	if err != nil {
		t.Fatal(err)
	}
	if log.Output != "aabc" || log.End != LogCapacity || log.Next != LogCapacity || !log.ReachedCapacity {
		t.Fatalf("log at the capacity = %+v", log)
	}
	short, err := service.Log(0, 1)
	if err != nil || !short.ReachedCapacity || short.Next != 1 {
		t.Fatalf("log read before the end = %+v, %v", short, err)
	}
}

func TestANewJobReplacesTheLogOfThePreviousJob(t *testing.T) {
	dependencies := updateFixture(t)
	jobWithLog(t, dependencies, firstJobID, JobFailed)
	writeLog(t, dependencies.StatePath, firstJobID, "first job output\n")
	service := jobWithLog(t, dependencies, secondJobID, JobInstalling)
	// A late line from the first job's helper does not reach the second job's log.
	writeLog(t, dependencies.StatePath, firstJobID, "late line of the first job\n")
	writeLog(t, dependencies.StatePath, secondJobID, "second job output\n")

	log, err := service.Log(0, LogCapacity)
	if err != nil {
		t.Fatal(err)
	}
	if log.JobID != secondJobID || log.Output != "second job output\n" || log.ReachedCapacity {
		t.Fatalf("log of the new job = %+v", log)
	}
}

func TestTheLogIsReadFromTheSameOffsetsAfterTheEngineRestarts(t *testing.T) {
	dependencies := updateFixture(t)
	before := jobWithLog(t, dependencies, firstJobID, JobRestarting)
	writeLog(t, dependencies.StatePath, firstJobID, "==> Upgrading\n")
	first, err := before.Log(0, LogCapacity)
	if err != nil {
		t.Fatal(err)
	}
	writeLog(t, dependencies.StatePath, firstJobID, "sshc: stopping the engine\n")

	dependencies.Current, dependencies.PID = "v1.1.0", 202
	after := New(dependencies)
	if job, err := after.Status(); err != nil || job.State != JobSucceeded {
		t.Fatalf("restarted status = %+v, %v", job, err)
	}
	rest, err := after.Log(first.Next, LogCapacity)
	if err != nil {
		t.Fatal(err)
	}
	if rest.JobID != firstJobID || rest.Start != first.Next || rest.Output != "sshc: stopping the engine\n" {
		t.Fatalf("log after the restart = %+v", rest)
	}
}

func TestTheLogRefusesACursorPastItsEnd(t *testing.T) {
	dependencies := updateFixture(t)
	service := jobWithLog(t, dependencies, firstJobID, JobInstalling)
	writeLog(t, dependencies.StatePath, firstJobID, "12345")
	if _, err := service.Log(6, 10); !errors.Is(err, ErrLogCursorAhead) {
		t.Fatalf("cursor past the end = %v", err)
	}
	log, err := service.Log(5, 10)
	if err != nil || log.Output != "" || log.Next != 5 || log.End != 5 {
		t.Fatalf("cursor at the end = %+v, %v", log, err)
	}
	log, err = service.Log(1, 2)
	if err != nil || log.Output != "23" || log.Start != 1 || log.Next != 3 {
		t.Fatalf("bounded read = %+v, %v", log, err)
	}
}

func TestTheLogIsMissingUntilTheCurrentJobHasOne(t *testing.T) {
	dependencies := updateFixture(t)
	if _, err := New(dependencies).Log(0, 10); !errors.Is(err, ErrNoLog) {
		t.Fatalf("no job = %v", err)
	}
	service := jobWithLog(t, dependencies, firstJobID, JobInstalling)
	if err := os.Remove(LogPath(dependencies.StatePath)); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Log(0, 10); !errors.Is(err, ErrNoLog) {
		t.Fatalf("no log file = %v", err)
	}
	// A log left by an earlier job is never shown as this job's output.
	if err := createLog(LogPath(dependencies.StatePath), secondJobID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Log(0, 10); !errors.Is(err, ErrNoLog) {
		t.Fatalf("log of another job = %v", err)
	}
}

func TestTheLogWaitsForTheRestOfAPartlyWrittenCharacterOrLineEnd(t *testing.T) {
	dependencies := updateFixture(t)
	service := jobWithLog(t, dependencies, firstJobID, JobInstalling)
	path := `C:\Users\相田`
	writeLog(t, dependencies.StatePath, firstJobID, path[:len(path)-1])
	partial, err := service.Log(0, LogCapacity)
	if err != nil {
		t.Fatal(err)
	}
	if partial.Output != `C:\Users\相` || partial.Next != uint64(len(`C:\Users\相`)) {
		t.Fatalf("partly written character = %+v", partial)
	}
	writeLog(t, dependencies.StatePath, firstJobID, path[len(path)-1:]+"\r")
	rest, err := service.Log(partial.Next, LogCapacity)
	if err != nil || rest.Output != "田" {
		t.Fatalf("rest of the character = %+v, %v", rest, err)
	}
	writeLog(t, dependencies.StatePath, firstJobID, "\n")
	lineEnd, err := service.Log(rest.Next, LogCapacity)
	if err != nil || lineEnd.Output != "\n" || lineEnd.Next != lineEnd.End {
		t.Fatalf("CR LF written in two parts = %+v, %v", lineEnd, err)
	}
}

func TestTheLogOfAnEndedJobIsReadToItsLastByte(t *testing.T) {
	dependencies := updateFixture(t)
	service := jobWithLog(t, dependencies, firstJobID, JobFailed)
	writeLog(t, dependencies.StatePath, firstJobID, "broken \xe7")
	log, err := service.Log(0, LogCapacity)
	if err != nil || log.Next != log.End || log.Output != "broken " {
		t.Fatalf("ended job with an incomplete character = %+v, %v", log, err)
	}
}

func TestTheLogLeavesOutTerminalControlEvenWhenTheCursorFallsInsideIt(t *testing.T) {
	dependencies := updateFixture(t)
	service := jobWithLog(t, dependencies, firstJobID, JobInstalling)
	writeLog(t, dependencies.StatePath, firstJobID, "\x1b[1;32mok\x1b[0m done\n")
	log, err := service.Log(3, LogCapacity)
	if err != nil || log.Output != "ok done\n" {
		t.Fatalf("read from inside a control sequence = %+v, %v", log, err)
	}
}

func TestTheInstallerOutputAndItsFailureReachTheLog(t *testing.T) {
	dependencies := updateFixture(t)
	dependencies.Install = func(_ context.Context, request InstallRequest) (string, error) {
		_, _ = request.Output.Write([]byte("sshc: installing sshc-linux-amd64 (v1.1.0)\n"))
		return "", errors.New("install.sh: exit status 1")
	}
	service := New(dependencies)
	plan, err := service.Prepare(context.Background(), "v1.1.0")
	if err != nil {
		t.Fatal(err)
	}
	job, err := service.Start(plan)
	if err != nil {
		t.Fatal(err)
	}
	// The log exists from the reservation, so a page polling it never reads the previous job's output.
	if log, err := service.Log(0, LogCapacity); err != nil || log.JobID != job.ID || log.End != 0 {
		t.Fatalf("log of an accepted job = %+v, %v", log, err)
	}
	service.ResponseSent(job.ID, nil)
	waitForUpdateState(t, service, JobFailed)
	log, err := service.Log(0, LogCapacity)
	if err != nil {
		t.Fatal(err)
	}
	if want := "sshc: installing sshc-linux-amd64 (v1.1.0)\nsshc: the update failed: install.sh: exit status 1\n"; log.Output != want {
		t.Fatalf("log = %q, want %q", log.Output, want)
	}
}
