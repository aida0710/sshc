package sftp_test

import (
	"context"
	"errors"
	"io/fs"
	"strconv"
	"strings"
	"testing"

	"sshc/internal/sftp"
)

type permissionRemote struct {
	*fakeRemote
	changed      []string
	beforeChange func(string) error
}

func (remote *permissionRemote) ChmodNoFollow(candidate string, mode fs.FileMode) error {
	if remote.beforeChange != nil {
		if err := remote.beforeChange(candidate); err != nil {
			return err
		}
	}
	if err := remote.fakeRemote.ChmodNoFollow(candidate, mode); err != nil {
		return err
	}
	remote.changed = append(remote.changed, candidate)
	return nil
}

func permissionService(remote *permissionRemote) sftp.Service {
	return sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) { return remote, nil }}
}

func chmodSelection(t *testing.T, service sftp.Service, paths []string) []sftp.ChmodEntry {
	t.Helper()
	entries := make([]sftp.ChmodEntry, len(paths))
	for index, candidate := range paths {
		entry, err := service.Stat(t.Context(), "edge", candidate)
		if err != nil {
			t.Fatal(err)
		}
		entries[index] = sftp.ChmodEntry{Path: candidate, ExpectedRevision: entry.Revision}
	}
	return entries
}

func TestChmodSelectionChangesTypeSpecificModesOnceAndLeavesLinksUntouched(t *testing.T) {
	remote := &permissionRemote{fakeRemote: remoteWith(map[string]node{
		"/single":         file("single", "single", 0o600),
		"/tree":           directory("tree"),
		"/tree/a":         file("a", "alpha", 0o600),
		"/tree/sub":       directory("sub"),
		"/tree/sub/b":     file("b", "beta", 0o600),
		"/tree/link":      {name: "link", content: []byte("/outside"), mode: fs.ModeSymlink | 0o777, modTime: testTime},
		"/outside":        directory("outside"),
		"/outside/secret": file("secret", "secret", 0o600),
	})}
	service := permissionService(remote)
	request := sftp.ChmodRequest{Alias: "edge", Entries: chmodSelection(t, service, []string{"/tree/sub/b", "/tree", "/single"}),
		Options: sftp.ChmodOptions{FileMode: 0o644, DirectoryMode: 0o750, Recursive: true}}
	plan, err := service.PrepareChmod(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	defer plan.Close()
	if plan.Files != 3 || plan.Directories != 2 || plan.SkippedSymlinks != 1 {
		t.Fatalf("plan counts = files %d, directories %d, links %d", plan.Files, plan.Directories, plan.SkippedSymlinks)
	}
	result, err := plan.Apply(t.Context(), plan.Revision)
	if err != nil || result.Applied != 5 || result.Items != 5 {
		t.Fatalf("Apply() = %+v, %v", result, err)
	}
	for _, candidate := range remote.changed {
		want := fs.FileMode(0o644)
		if remote.nodes[candidate].IsDir() {
			want = 0o750
		}
		if remote.nodes[candidate].Mode().Perm() != want {
			t.Errorf("mode(%s) = %o, want %o", candidate, remote.nodes[candidate].Mode().Perm(), want)
		}
	}
	if remote.nodes["/outside/secret"].mode != 0o600 || remote.nodes["/tree/link"].mode != fs.ModeSymlink|0o777 {
		t.Fatal("permission change followed or changed a symlink")
	}
	if _, err := plan.Apply(t.Context(), plan.Revision); !errors.Is(err, sftp.ErrUnavailable) || len(remote.changed) != 5 {
		t.Fatalf("a plan was replayed: %v, changed %v", err, remote.changed)
	}
}

func TestChmodPreflightRefusesTheEntireSelectionWhenAnyConfirmedTargetChanges(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		change func(*permissionRemote)
	}{
		{"late selection", func(remote *permissionRemote) { remote.nodes["/z"] = file("z", "changed", 0o600) }},
		{"nested file", func(remote *permissionRemote) { remote.nodes["/tree/a"] = file("a", "changed", 0o600) }},
		{"added child", func(remote *permissionRemote) { remote.nodes["/tree/new"] = file("new", "new", 0o600) }},
		{"removed child", func(remote *permissionRemote) { delete(remote.nodes, "/tree/a") }},
		{"replaced with link", func(remote *permissionRemote) {
			remote.nodes["/tree/a"] = node{name: "a", mode: fs.ModeSymlink | 0o777, content: []byte("/z"), modTime: testTime}
		}},
		{"replaced ancestor", func(remote *permissionRemote) {
			remote.nodes["/tree"] = node{name: "tree", mode: fs.ModeSymlink | 0o777, content: []byte("/elsewhere"), modTime: testTime}
		}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			remote := &permissionRemote{fakeRemote: remoteWith(map[string]node{
				"/tree": directory("tree"), "/tree/a": file("a", "alpha", 0o600), "/z": file("z", "last", 0o600),
			})}
			service := permissionService(remote)
			request := sftp.ChmodRequest{Alias: "edge", Entries: chmodSelection(t, service, []string{"/tree", "/z"}),
				Options: sftp.ChmodOptions{FileMode: 0o644, DirectoryMode: 0o755, Recursive: true}}
			plan, err := service.PrepareChmod(t.Context(), request)
			if err != nil {
				t.Fatal(err)
			}
			defer plan.Close()
			scenario.change(remote)
			result, err := plan.Apply(t.Context(), plan.Revision)
			if err == nil || result.Applied != 0 || len(remote.changed) != 0 {
				t.Fatalf("preflight changed entries: %+v, %v, %v", result, err, remote.changed)
			}
		})
	}
}

func TestChmodPlanBindsEverySelectionRevisionAndOption(t *testing.T) {
	remote := &permissionRemote{fakeRemote: remoteWith(map[string]node{
		"/tree": directory("tree"), "/tree/a": file("a", "alpha", 0o600), "/z": file("z", "last", 0o600),
	})}
	service := permissionService(remote)
	original := sftp.ChmodRequest{Alias: "edge", Entries: chmodSelection(t, service, []string{"/tree", "/z"}), Options: sftp.ChmodOptions{FileMode: 0o644, DirectoryMode: 0o755}}
	prepare := func(request sftp.ChmodRequest) string {
		t.Helper()
		plan, err := service.PrepareChmod(t.Context(), request)
		if err != nil {
			t.Fatal(err)
		}
		defer plan.Close()
		return plan.Revision
	}
	revision := prepare(original)
	for _, change := range []func(*sftp.ChmodRequest){
		func(request *sftp.ChmodRequest) { request.Alias = "another" },
		func(request *sftp.ChmodRequest) { request.Entries = request.Entries[:1] },
		func(request *sftp.ChmodRequest) { request.Options.FileMode = 0o600 },
		func(request *sftp.ChmodRequest) { request.Options.DirectoryMode = 0o700 },
		func(request *sftp.ChmodRequest) { request.Options.Recursive = true },
	} {
		changed := original
		change(&changed)
		if prepare(changed) == revision {
			t.Fatal("changed selection/options retained the confirmation revision")
		}
	}
	original.Entries[0].ExpectedRevision = "stale"
	if _, err := service.PrepareChmod(t.Context(), original); !errors.Is(err, sftp.ErrConflict) || len(remote.changed) != 0 {
		t.Fatalf("stale selected revision = %v, changes %v", err, remote.changed)
	}
}

func TestChmodCancellationAndPermissionFailuresReportPartialApplication(t *testing.T) {
	for _, cancelDuringChange := range []bool{false, true} {
		t.Run(map[bool]string{false: "permission refusal", true: "cancellation"}[cancelDuringChange], func(t *testing.T) {
			remote := &permissionRemote{fakeRemote: remoteWith(map[string]node{"/a": file("a", "a", 0o600), "/z": file("z", "z", 0o600)})}
			service := permissionService(remote)
			plan, err := service.PrepareChmod(t.Context(), sftp.ChmodRequest{Alias: "edge", Entries: chmodSelection(t, service, []string{"/a", "/z"}), Options: sftp.ChmodOptions{FileMode: 0o644}})
			if err != nil {
				t.Fatal(err)
			}
			defer plan.Close()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			remote.beforeChange = func(candidate string) error {
				if cancelDuringChange {
					cancel()
					return nil
				}
				if candidate == "/z" {
					return fs.ErrPermission
				}
				return nil
			}
			result, err := plan.Apply(ctx, plan.Revision)
			if err == nil || result.Applied != 1 || len(remote.changed) != 1 || remote.nodes["/z"].mode != 0o600 {
				t.Fatalf("partial change = %+v, %v, %v", result, err, remote.changed)
			}
		})
	}
}

func TestChmodRejectsInvalidSelectionsAndCancelledPlansWithoutChanges(t *testing.T) {
	remote := &permissionRemote{fakeRemote: remoteWith(map[string]node{
		"/a": file("a", "a", 0o600), "/link": {name: "link", mode: fs.ModeSymlink | 0o777, content: []byte("/a"), modTime: testTime},
	})}
	service := permissionService(remote)
	valid := sftp.ChmodRequest{Alias: "edge", Entries: chmodSelection(t, service, []string{"/a"}), Options: sftp.ChmodOptions{FileMode: 0o644, DirectoryMode: 0o755}}
	for _, entries := range [][]sftp.ChmodEntry{
		nil,
		{{Path: "/a"}},
		{valid.Entries[0], valid.Entries[0]},
		{{Path: "/", ExpectedRevision: "revision"}},
		{{Path: "/.a.sshc-upload-transfer_123456.part", ExpectedRevision: "revision"}},
		{{Path: "/" + strings.Repeat("a", 4097), ExpectedRevision: "revision"}},
		chmodSelection(t, service, []string{"/link"}),
		make([]sftp.ChmodEntry, sftp.MaxChmodSelection+1),
	} {
		request := valid
		request.Entries = entries
		if _, err := service.PrepareChmod(t.Context(), request); err == nil {
			t.Fatalf("invalid selection accepted: %v", entries)
		}
	}
	plan, err := service.PrepareChmod(t.Context(), valid)
	if err != nil {
		t.Fatal(err)
	}
	defer plan.Close()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if result, err := plan.Apply(ctx, plan.Revision); !errors.Is(err, context.Canceled) || result.Applied != 0 {
		t.Fatalf("cancelled plan = %+v, %v", result, err)
	}
	if len(remote.changed) != 0 {
		t.Fatal("invalid/cancelled plans changed permissions")
	}
}

func TestChmodSelectedTreesShareOneTraversalBudget(t *testing.T) {
	nodes := map[string]node{"/left": directory("left"), "/right": directory("right")}
	for index := range sftp.MaxSearchVisitedForTest {
		root := "/left"
		if index%2 != 0 {
			root = "/right"
		}
		name := "f" + strconv.Itoa(index)
		nodes[root+"/"+name] = file(name, "", 0o600)
	}
	remote := &permissionRemote{fakeRemote: remoteWith(nodes)}
	service := permissionService(remote)
	_, err := service.PrepareChmod(t.Context(), sftp.ChmodRequest{Alias: "edge", Entries: chmodSelection(t, service, []string{"/left", "/right"}),
		Options: sftp.ChmodOptions{FileMode: 0o644, DirectoryMode: 0o755, Recursive: true}})
	if !errors.Is(err, sftp.ErrTraversalLimit) || len(remote.changed) != 0 {
		t.Fatalf("shared traversal budget = %v, changed %v", err, remote.changed)
	}
}
