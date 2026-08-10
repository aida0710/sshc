package config

import (
	"errors"
	"testing"
)

func TestSnapshotInlinesEveryResolvedIncludeInOpenSSHOrder(t *testing.T) {
	graph, err := resolverFor(map[string]string{
		"/Users/tester/.ssh/config":                  "Include conf.d/*.conf\nHost direct\n",
		"/Users/tester/.ssh/conf.d/20-b.conf":        "Host bravo\n\tUser b\n",
		"/Users/tester/.ssh/conf.d/10-a.conf":        "Include conf.d/nested/user.conf\n",
		"/Users/tester/.ssh/conf.d/nested/user.conf": "Host alpha\n\tUser a\n",
	}).Resolve("/Users/tester/.ssh/config")
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
		"/Users/tester/.ssh/config": "Include %h/config\nHost direct\n",
	}).Resolve("/Users/tester/.ssh/config")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := Snapshot(graph); !errors.Is(err, ErrSnapshotIncomplete) {
		t.Fatalf("Snapshot = %v, want ErrSnapshotIncomplete", err)
	}
}

func TestSnapshotKeepsFileBoundariesWhenAnIncludedFileHasNoFinalNewline(t *testing.T) {
	graph, err := resolverFor(map[string]string{
		"/Users/tester/.ssh/config":     "Include child.conf\nHost after\n",
		"/Users/tester/.ssh/child.conf": "Host child",
	}).Resolve("/Users/tester/.ssh/config")
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
		"/Users/tester/.ssh/config":     "Host ignored\n\tInclude child.conf\nHost bastion\n\tHostName good.example\n",
		"/Users/tester/.ssh/child.conf": "Host bastion\n\tHostName wrong.example\n",
	}).Resolve("/Users/tester/.ssh/config")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := Snapshot(graph); !errors.Is(err, ErrSnapshotIncomplete) {
		t.Fatalf("Snapshot = %v, want ErrSnapshotIncomplete", err)
	}
}

func TestSnapshotRefusesAnEnvironmentExpandedIncludeInsteadOfDroppingIt(t *testing.T) {
	graph, err := resolverFor(map[string]string{
		"/Users/tester/.ssh/config": "Include ${HOME}/.ssh/hosts.conf\nHost bastion\n",
	}).Resolve("/Users/tester/.ssh/config")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := Snapshot(graph); !errors.Is(err, ErrSnapshotIncomplete) {
		t.Fatalf("Snapshot = %v, want ErrSnapshotIncomplete", err)
	}
}
