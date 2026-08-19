package config

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestSnapshotRefusesAnInlinedGraphLargerThanTheCLIResponseLimit(t *testing.T) {
	files := map[string]string{
		testConfig: "Include conf.d/*.conf\n",
	}
	chunk := strings.Repeat("# padding\n", (1<<20)/len("# padding\n")-1)
	for index := range 5 {
		files[filepath.Join(testRoot, "conf.d", fmt.Sprintf("%d.conf", index))] = chunk
	}
	graph, err := resolverFor(files).Resolve(testConfig)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Snapshot(graph); !errors.Is(err, ErrSnapshotIncomplete) {
		t.Fatalf("Snapshot = %v, want ErrSnapshotIncomplete", err)
	}
}

func TestSnapshotInlinesEveryResolvedIncludeInOpenSSHOrder(t *testing.T) {
	graph, err := resolverFor(map[string]string{
		testConfig: "Include conf.d/*.conf\nHost direct\n",
		filepath.Join(testRoot, "conf.d", "20-b.conf"):           "Host bravo\n\tUser b\n",
		filepath.Join(testRoot, "conf.d", "10-a.conf"):           "Include conf.d/nested/user.conf\n",
		filepath.Join(testRoot, "conf.d", "nested", "user.conf"): "Host alpha\n\tUser a\n",
	}).Resolve(testConfig)
	if err != nil {
		t.Fatal(err)
	}

	got, err := Snapshot(graph)
	if err != nil {
		t.Fatalf("Snapshot = %v", err)
	}
	want := "Host alpha\n\tUser a\nHost bravo\n\tUser b\nHost direct\n"
	if string(got) != want {
		t.Fatalf("snapshot = %q, want %q", got, want)
	}
}

func TestSnapshotRefusesAnIncludeTheEngineCouldNotResolve(t *testing.T) {
	graph, err := resolverFor(map[string]string{
		testConfig: "Include %h/config\nHost direct\n",
	}).Resolve(testConfig)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := Snapshot(graph); !errors.Is(err, ErrSnapshotIncomplete) {
		t.Fatalf("Snapshot = %v, want ErrSnapshotIncomplete", err)
	}
}

func TestSnapshotKeepsFileBoundariesWhenAnIncludedFileHasNoFinalNewline(t *testing.T) {
	graph, err := resolverFor(map[string]string{
		testConfig:                            "Include child.conf\nHost after\n",
		filepath.Join(testRoot, "child.conf"): "Host child",
	}).Resolve(testConfig)
	if err != nil {
		t.Fatal(err)
	}

	got, err := Snapshot(graph)
	if err != nil {
		t.Fatal(err)
	}
	if want := "Host child\nHost after\n"; string(got) != want {
		t.Fatalf("snapshot = %q, want %q", got, want)
	}
}

func TestSnapshotRefusesAConditionalIncludeInsteadOfChangingItsMeaning(t *testing.T) {
	graph, err := resolverFor(map[string]string{
		testConfig:                            "Host ignored\n\tInclude child.conf\nHost bastion\n\tHostName good.example\n",
		filepath.Join(testRoot, "child.conf"): "Host bastion\n\tHostName wrong.example\n",
	}).Resolve(testConfig)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := Snapshot(graph); !errors.Is(err, ErrSnapshotIncomplete) {
		t.Fatalf("Snapshot = %v, want ErrSnapshotIncomplete", err)
	}
}

func TestSnapshotRefusesAnEnvironmentExpandedIncludeInsteadOfDroppingIt(t *testing.T) {
	graph, err := resolverFor(map[string]string{
		testConfig: "Include ${HOME}/.ssh/hosts.conf\nHost bastion\n",
	}).Resolve(testConfig)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := Snapshot(graph); !errors.Is(err, ErrSnapshotIncomplete) {
		t.Fatalf("Snapshot = %v, want ErrSnapshotIncomplete", err)
	}
}
