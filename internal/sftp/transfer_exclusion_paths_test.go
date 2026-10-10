package sftp_test

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"sshc/internal/sftp"
)

func exclusionSource() *fakeRemote {
	return remoteWith(map[string]node{
		"/data": directory("data"), "/data/.git": directory(".git"), "/data/.git/config": file("config", "private", 0600),
		"/data/src": directory("src"), "/data/src/main.go": file("main.go", "keep", 0600),
		"/data/src/debug.log":    file("debug.log", "skip", 0600),
		"/data/src/node_modules": {name: "node_modules", mode: fs.ModeSymlink | 0777, content: []byte("/outside")},
		"/data/build":            directory("build"), "/data/build/cache": directory("cache"), "/data/build/cache/blob": file("blob", "skip", 0600),
	})
}

var exclusionPatterns = []string{".git", "*.log", "node_modules", "build/cache"}

func TestRemoteFolderCopySkipsExcludedDirectoriesBeforeOpeningThem(t *testing.T) {
	source, target := exclusionSource(), remoteWith(map[string]node{"/inbox": directory("inbox")})
	service := twoHostService(source, target)
	request := sftp.RemoteTransferRequest{SourceAlias: "source", SourcePath: "/data", TargetAlias: "target", TargetPath: "/inbox/data", Operation: sftp.RemoteCopy, ExcludePatterns: exclusionPatterns}
	plan, err := service.PlanRemoteTransfer(t.Context(), request)
	if err != nil || plan.TotalBytes != 4 {
		t.Fatalf("plan = %+v, %v", plan, err)
	}
	if err := service.CopyRemote(t.Context(), request, nil); err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{".git", "src/debug.log", "src/node_modules", "build/cache"} {
		if _, err := target.Lstat("/inbox/data/" + relative); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("excluded %s = %v", relative, err)
		}
	}
	if string(target.nodes["/inbox/data/src/main.go"].content) != "keep" {
		t.Fatal("included file not copied")
	}
}

func TestExclusionsDoNotApplyToExplicitFilesOrMoves(t *testing.T) {
	source, target := remoteWith(map[string]node{"/debug.log": file("debug.log", "keep", 0600)}), remoteWith(map[string]node{"/inbox": directory("inbox")})
	service := twoHostService(source, target)
	request := sftp.RemoteTransferRequest{SourceAlias: "source", SourcePath: "/debug.log", TargetAlias: "target", TargetPath: "/inbox/debug.log", Operation: sftp.RemoteMove, ExcludePatterns: []string{"*.log"}}
	copyRequest := request
	copyRequest.Operation, copyRequest.TargetPath = sftp.RemoteCopy, "/inbox/copied.log"
	if err := service.CopyRemote(t.Context(), copyRequest, nil); err != nil {
		t.Fatal(err)
	}
	if string(target.nodes["/inbox/copied.log"].content) != "keep" {
		t.Fatal("explicitly selected file was excluded")
	}
	plan, err := service.PlanRemoteTransfer(t.Context(), request)
	if err != nil || plan.TotalBytes != 4 {
		t.Fatalf("plan = %+v, %v", plan, err)
	}
	if err := service.CopyRemote(t.Context(), request, nil); err != nil {
		t.Fatal(err)
	}
	if string(target.nodes["/inbox/debug.log"].content) != "keep" {
		t.Fatal("moved file was excluded")
	}
	if _, err := source.Lstat("/debug.log"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("move did not remove its source")
	}
}

func TestFolderGetAndPutApplyTheSameExclusionRules(t *testing.T) {
	local := t.TempDir()
	remote := exclusionSource()
	service := serviceFor(remote)
	get := sftp.RemoteTransferRequest{SourceAlias: "edge", SourcePath: "/data", TargetAlias: "edge", TargetPath: filepath.Join(local, "download"), Operation: sftp.RemoteGet, ExcludePatterns: exclusionPatterns}
	plan, err := service.PlanLocalTransfer(t.Context(), get)
	if err != nil || plan.TotalBytes != 4 {
		t.Fatalf("get plan = %+v, %v", plan, err)
	}
	if err := service.CopyLocal(t.Context(), get, nil); err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{".git", "src/debug.log", "src/node_modules", "build/cache"} {
		if _, err := os.Lstat(filepath.Join(get.TargetPath, filepath.FromSlash(relative))); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("excluded local %s = %v", relative, err)
		}
	}
	if err := os.MkdirAll(filepath.Join(get.TargetPath, "node_modules"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(get.TargetPath, "node_modules", "package.json"), []byte("skip"), 0600); err != nil {
		t.Fatal(err)
	}
	put := sftp.RemoteTransferRequest{SourceAlias: "edge", SourcePath: get.TargetPath, TargetAlias: "edge", TargetPath: "/upload", Operation: sftp.RemotePut, ExcludePatterns: exclusionPatterns}
	plan, err = service.PlanLocalTransfer(t.Context(), put)
	if err != nil || plan.TotalBytes != 4 {
		t.Fatalf("put plan = %+v, %v", plan, err)
	}
	if err := service.CopyLocal(t.Context(), put, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := remote.Lstat("/upload/node_modules"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("put included excluded directory")
	}
	if string(remote.nodes["/upload/src/main.go"].content) != "keep" {
		t.Fatal("put omitted included file")
	}
}

type queuedArchiveExpectation struct {
	name          string
	queued, later []string
	want          []string
}

func TestArchiveUsesTheQueuedExclusionsAfterTheSettingsChange(t *testing.T) {
	for _, test := range []queuedArchiveExpectation{
		{name: "rules retained", queued: exclusionPatterns, want: []string{"data/", "data/build/", "data/src/", "data/src/main.go"}},
		{name: "empty retained", later: exclusionPatterns, want: []string{"data/", "data/.git/", "data/.git/config", "data/build/", "data/build/cache/", "data/build/cache/blob", "data/src/", "data/src/debug.log", "data/src/main.go", "data/src/node_modules"}},
	} {
		t.Run(test.name, func(t *testing.T) { assertQueuedArchiveEntries(t, test) })
	}
}

func assertQueuedArchiveEntries(t *testing.T, expectation queuedArchiveExpectation) {
	t.Helper()
	remote := exclusionSource()
	service := serviceFor(remote)
	manager := newTestTransferManager(t, &service)
	settings := sftp.DefaultTransferSettings()
	settings.ExcludePatterns = expectation.queued
	if err := manager.SetTransferSettings(settings); err != nil {
		t.Fatal(err)
	}
	const id = "archive_excludes"
	job, err := manager.CreateJob(sftp.CreateTransferJob{ID: id, BatchID: id, Alias: "edge", RemotePath: "/data", Direction: sftp.TransferDownload, Kind: sftp.TransferFolder, TotalBytes: -1})
	if err != nil {
		t.Fatal(err)
	}
	settings.ExcludePatterns = expectation.later
	if err := manager.SetTransferSettings(settings); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(job.ExcludePatterns, expectation.queued) {
		t.Fatalf("job exclusions = %v", job.ExcludePatterns)
	}
	if _, err := manager.UpdateJob(id, sftp.UpdateTransferJob{Action: sftp.TransferStartAction}); err != nil {
		t.Fatal(err)
	}
	prepared, err := manager.PrepareOwnedArchive(context.Background(), id, "edge", "/data")
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	var contents bytes.Buffer
	if _, err := prepared.WriteFrom(t.Context(), 0, &contents); err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(contents.Bytes()), int64(contents.Len()))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range archive.File {
		names = append(names, entry.Name)
	}
	if !slices.Equal(names, expectation.want) {
		t.Fatalf("archive = %v", names)
	}
}

func TestQueuedFolderCopyRetainsItsExclusionsWhenWaitingSettingsChange(t *testing.T) {
	source, target := exclusionSource(), remoteWith(map[string]node{"/inbox": directory("inbox")})
	service := twoHostService(source, target)
	manager := newTestTransferManager(t, &service)
	settings := sftp.DefaultTransferSettings()
	settings.ExcludePatterns, settings.ProcessingStopped = append([]string(nil), exclusionPatterns...), true
	if err := manager.SetTransferSettings(settings); err != nil {
		t.Fatal(err)
	}
	const id = "copy_exclusions"
	created, err := manager.CreateJob(sftp.CreateTransferJob{ID: id, BatchID: id, Alias: "target", RemotePath: "/inbox/data", SourceAlias: "source", SourcePath: "/data", Operation: sftp.RemoteCopy, Direction: sftp.TransferRemote, Kind: sftp.TransferFolder, TotalBytes: -1})
	if err != nil {
		t.Fatal(err)
	}
	created.ExcludePatterns[0] = "changed_in_returned_job"
	// The setting slice and the public job view must not mutate a queued job.
	settings.ExcludePatterns[0] = "changed"
	view, err := manager.ListJobs()
	if err != nil {
		t.Fatal(err)
	}
	view[0].ExcludePatterns[0] = "also_changed"
	settings.ExcludePatterns, settings.ProcessingStopped = nil, false
	if err := manager.SetTransferSettings(settings); err != nil {
		t.Fatal(err)
	}
	job := waitForJob(t, manager, id, hasStatus(sftp.TransferCompleted))
	if job.TotalBytes != 4 || job.ExcludePatterns[0] != ".git" {
		t.Fatalf("completed job = %+v", job)
	}
	if _, err := target.Lstat("/inbox/data/.git"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("queued exclusion changed before start")
	}
}

func TestPersistedFolderJobsRestoreTheirOriginalExclusions(t *testing.T) {
	queue := filepath.Join(t.TempDir(), "transfer-queue.json")
	first := newTestTransferManager(t, nil)
	settings := sftp.DefaultTransferSettings()
	settings.ExcludePatterns = []string{".git", "*.log"}
	if err := first.SetTransferSettings(settings); err != nil {
		t.Fatal(err)
	}
	if err := first.EnableQueuePersistence(queue); err != nil {
		t.Fatal(err)
	}
	const id = "persist_exclude"
	if _, err := first.CreateJob(sftp.CreateTransferJob{ID: id, BatchID: id, Alias: "edge", RemotePath: "/data", Direction: sftp.TransferDownload, Kind: sftp.TransferFolder, TotalBytes: -1}); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	restored := newTestTransferManager(t, nil)
	if err := restored.EnableQueuePersistence(queue); err != nil {
		t.Fatal(err)
	}
	jobs, err := restored.ListJobs()
	if err != nil || len(jobs) != 1 || !slices.Equal(jobs[0].ExcludePatterns, []string{".git", "*.log"}) {
		t.Fatalf("restored = %+v, %v", jobs, err)
	}
}
