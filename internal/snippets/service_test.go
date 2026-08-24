package snippets

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type memoryRepository struct {
	mutex   sync.Mutex
	library Library
	err     error
}

func (r *memoryRepository) Load() (Library, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return cloneLibrary(r.library), r.err
}

func (r *memoryRepository) Save(library Library) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r.err != nil {
		return r.err
	}
	r.library = cloneLibrary(library)
	return nil
}

func testService(repository Repository, run func(context.Context, string, string) (CommandOutput, error)) *Service {
	return NewService(Options{
		Repository: repository,
		Resolve: func(alias string) (Resolution, error) {
			if alias == "missing" {
				return Resolution{}, errors.New("not found")
			}
			resolution := Resolution{Target: Target{Alias: alias, HostName: alias + ".example", User: "aida", Port: "22"}}
			if run != nil {
				resolution.Run = func(ctx context.Context, command string) (CommandOutput, error) {
					return run(ctx, alias, command)
				}
			}
			return resolution, nil
		},
		Now:    func() time.Time { return time.Date(2026, 8, 24, 8, 0, 0, 0, time.UTC) },
		Random: strings.NewReader(strings.Repeat("abcdefghijklmnopqrstuvwxyz0123456789", 20)),
	})
}

func createSnippet(t *testing.T, service *Service, draft Draft) Snippet {
	t.Helper()
	snippet, err := service.Create(draft)
	if err != nil {
		t.Fatal(err)
	}
	return snippet
}

func TestCRUDPreservesIdentityAndRemovesStartupBindings(t *testing.T) {
	repository := &memoryRepository{}
	service := testService(repository, nil)
	created := createSnippet(t, service, Draft{Name: "Uptime", Command: "uptime"})
	if created.ID != "6162636465666768696a6b6c6d6e6f70" || created.CreatedAt.IsZero() || created.UpdatedAt != created.CreatedAt {
		t.Fatalf("created = %#v", created)
	}
	if err := service.SetStartup("bastion", created.ID, nil); err != nil {
		t.Fatal(err)
	}
	updated, err := service.Update(created.ID, Draft{Name: "Disk", Command: "df -h"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != created.ID || updated.CreatedAt != created.CreatedAt || updated.Command != "df -h" {
		t.Fatalf("updated = %#v", updated)
	}
	listed, err := service.List()
	if err != nil || len(listed) != 1 || listed[0].Name != "Disk" {
		t.Fatalf("List = %#v, %v", listed, err)
	}
	if err := service.Delete(created.ID); err != nil {
		t.Fatal(err)
	}
	startup, err := service.Startup()
	if err != nil || len(startup) != 0 {
		t.Fatalf("Startup = %#v, %v", startup, err)
	}
	if err := service.Delete(created.ID); !errors.Is(err, ErrUnknownSnippet) {
		t.Fatalf("second Delete = %v", err)
	}
}

func TestGeneratedSnippetIDMayStartWithADigit(t *testing.T) {
	service := NewService(Options{
		Repository: &memoryRepository{},
		Now:        time.Now,
		Random:     strings.NewReader("0123456789abcdef"),
	})
	snippet := createSnippet(t, service, Draft{Name: "Check", Command: "uptime"})
	if snippet.ID != "30313233343536373839616263646566" || !snippetIDPattern.MatchString(snippet.ID) {
		t.Fatalf("generated id = %q", snippet.ID)
	}
}

func TestMissingRepositoryReturnsAnErrorInsteadOfPanicking(t *testing.T) {
	service := NewService(Options{})
	if _, err := service.List(); !errors.Is(err, ErrNoRepository) {
		t.Fatalf("List = %v, want ErrNoRepository", err)
	}
}

func TestExpansionIsTypedStrictAndRedactsSecrets(t *testing.T) {
	repository := &memoryRepository{}
	service := testService(repository, nil)
	defaultBool := "false"
	snippet := createSnippet(t, service, Draft{
		Name:    "Deploy",
		Command: "deploy --count={{count}} --force={{force}} --token={{token}} {{environment}}",
		Variables: []Variable{
			{Name: "count", Type: VariableInteger, Required: true},
			{Name: "force", Type: VariableBoolean, Default: &defaultBool},
			{Name: "token", Type: VariableSecret, Required: true},
			{Name: "environment", Type: VariableString, Required: true},
		},
	})
	preview, err := service.Preview(PreviewRequest{
		SnippetID: snippet.ID, Aliases: []string{"bastion", "database"},
		Inputs: map[string]string{"count": "0042", "token": "do-not-leak", "environment": "staging"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "deploy --count=42 --force=false --token=[secret] staging"
	if len(preview.Targets) != 2 || preview.Targets[0].Command != want || strings.Contains(preview.Evidence, "do-not-leak") {
		t.Fatalf("preview = %#v", preview)
	}

	bad := []struct {
		name   string
		inputs map[string]string
		want   error
	}{
		{"missing", map[string]string{"count": "1", "environment": "staging"}, ErrMissingVariable},
		{"wrong integer", map[string]string{"count": "1.5", "token": "x", "environment": "staging"}, ErrInvalidVariable},
		{"unknown", map[string]string{"count": "1", "token": "x", "environment": "staging", "extra": "x"}, ErrUnknownVariable},
	}
	for _, test := range bad {
		t.Run(test.name, func(t *testing.T) {
			_, err := service.Preview(PreviewRequest{SnippetID: snippet.ID, Aliases: []string{"bastion"}, Inputs: test.inputs})
			if !errors.Is(err, test.want) {
				t.Fatalf("Preview = %v, want %v", err, test.want)
			}
		})
	}
}

func TestMalformedAndUnusedPlaceholdersAreRejectedAtCreate(t *testing.T) {
	for _, draft := range []Draft{
		{Name: "Malformed", Command: "echo {{name", Variables: []Variable{{Name: "name", Type: VariableString}}},
		{Name: "Unknown", Command: "echo {{other}}", Variables: []Variable{{Name: "name", Type: VariableString}}},
		{Name: "Unused", Command: "echo ok", Variables: []Variable{{Name: "name", Type: VariableString}}},
	} {
		service := testService(&memoryRepository{}, nil)
		if _, err := service.Create(draft); err == nil {
			t.Fatalf("Create(%q) succeeded", draft.Name)
		}
	}
}

func TestStartupRejectsSecretsAndAnEditThatBreaksAnAssignment(t *testing.T) {
	service := testService(&memoryRepository{}, nil)
	secret := createSnippet(t, service, Draft{
		Name: "Secret", Command: "echo {{token}}",
		Variables: []Variable{{Name: "token", Type: VariableSecret, Required: true}},
	})
	if err := service.SetStartup("bastion", secret.ID, map[string]string{"token": "hidden"}); !errors.Is(err, ErrSecretStartup) {
		t.Fatalf("SetStartup(secret) = %v", err)
	}

	plain := createSnippet(t, service, Draft{Name: "Plain", Command: "echo ok"})
	if err := service.SetStartup("bastion", plain.ID, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Update(plain.ID, Draft{
		Name: "Now requires input", Command: "echo {{name}}",
		Variables: []Variable{{Name: "name", Type: VariableString, Required: true}},
	}); !errors.Is(err, ErrMissingVariable) {
		t.Fatalf("Update = %v, want ErrMissingVariable", err)
	}
	preview, err := service.PreviewStartup("bastion")
	if err != nil || preview.Targets[0].Command != "echo ok" {
		t.Fatalf("PreviewStartup = %#v, %v", preview, err)
	}
}

func TestAStaleStartupBindingCanBeClearedWithoutResolvingTheHost(t *testing.T) {
	repository := &memoryRepository{}
	service := testService(repository, nil)
	snippet := createSnippet(t, service, Draft{Name: "Check", Command: "uptime"})
	if err := service.SetStartup("bastion", snippet.ID, nil); err != nil {
		t.Fatal(err)
	}
	service.resolve = func(string) (Resolution, error) { return Resolution{}, errors.New("host was deleted") }
	if err := service.SetStartup("bastion", "", nil); err != nil {
		t.Fatalf("clear startup = %v", err)
	}
}

func TestPreviewEvidenceDetectsSnippetAndTargetChanges(t *testing.T) {
	repository := &memoryRepository{}
	var hostname atomic.Value
	hostname.Store("before.example")
	service := NewService(Options{
		Repository: repository,
		Resolve: func(alias string) (Resolution, error) {
			return Resolution{
				Target: Target{Alias: alias, HostName: hostname.Load().(string), User: "aida", Port: "22"},
				Run:    func(context.Context, string) (CommandOutput, error) { return CommandOutput{}, nil },
			}, nil
		},
		Now:    time.Now,
		Random: strings.NewReader(strings.Repeat("a", 256)),
	})
	snippet := createSnippet(t, service, Draft{Name: "Check", Command: "uptime"})
	preview, err := service.Preview(PreviewRequest{SnippetID: snippet.ID, Aliases: []string{"bastion"}})
	if err != nil {
		t.Fatal(err)
	}
	hostname.Store("after.example")
	_, err = service.Start(context.Background(), ExecuteRequest{
		PreviewRequest: PreviewRequest{SnippetID: snippet.ID, Aliases: []string{"bastion"}},
		Evidence:       preview.Evidence,
	})
	if !errors.Is(err, ErrPreviewChanged) {
		t.Fatalf("Start = %v, want ErrPreviewChanged", err)
	}
}

func TestAdHocCommandPreviewAndExecution(t *testing.T) {
	service := testService(&memoryRepository{}, func(ctx context.Context, alias, command string) (CommandOutput, error) {
		return CommandOutput{Stdout: []byte(alias + ":" + command)}, nil
	})
	request := PreviewRequest{
		Command: "uname -a",
		Targets: []RequestedTarget{{TargetID: "pane-a", Alias: "bastion"}},
	}
	preview, err := service.Preview(request)
	if err != nil {
		t.Fatal(err)
	}
	if preview.SnippetID != "" || len(preview.Targets) != 1 || preview.Targets[0].TargetID != "pane-a" || preview.Targets[0].Command != "uname -a" {
		t.Fatalf("preview = %#v", preview)
	}
	if preview.ActionTarget() == "" {
		t.Fatal("ad-hoc preview action target is empty")
	}
	job, err := service.Start(context.Background(), ExecuteRequest{PreviewRequest: request, Evidence: preview.Evidence})
	if err != nil {
		t.Fatal(err)
	}
	finished, err := service.Wait(context.Background(), job.ID)
	if err != nil {
		t.Fatal(err)
	}
	result := finished.Results[0]
	if result.TargetID != "pane-a" || result.Alias != "bastion" || result.Stdout != "bastion:uname -a" {
		t.Fatalf("result = %#v", result)
	}
}

func TestAdHocCommandEvidenceRejectsChangedCommand(t *testing.T) {
	service := testService(&memoryRepository{}, func(context.Context, string, string) (CommandOutput, error) {
		return CommandOutput{}, nil
	})
	request := PreviewRequest{Command: "uptime", Targets: []RequestedTarget{{TargetID: "pane-a", Alias: "bastion"}}}
	preview, err := service.Preview(request)
	if err != nil {
		t.Fatal(err)
	}
	request.Command = "reboot"
	_, err = service.Start(context.Background(), ExecuteRequest{PreviewRequest: request, Evidence: preview.Evidence})
	if !errors.Is(err, ErrPreviewChanged) {
		t.Fatalf("Start = %v, want ErrPreviewChanged", err)
	}
}

func TestAdHocCommandDoesNotInterpretSnippetPlaceholders(t *testing.T) {
	service := testService(&memoryRepository{}, nil)
	command := `printf '%s\n' '{{raw-shell-text}}'`
	preview, err := service.Preview(PreviewRequest{
		Command: command,
		Targets: []RequestedTarget{{TargetID: "pane-a", Alias: "bastion"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Targets[0].Command != command {
		t.Fatalf("command = %q", preview.Targets[0].Command)
	}
}

func TestDuplicateAliasIsAllowedWithDistinctTargetIDs(t *testing.T) {
	service := testService(&memoryRepository{}, func(context.Context, string, string) (CommandOutput, error) {
		return CommandOutput{}, nil
	})
	request := PreviewRequest{
		Command: "uptime",
		Targets: []RequestedTarget{
			{TargetID: "pane-a", Alias: "bastion"},
			{TargetID: "pane-b", Alias: "bastion"},
		},
	}
	preview, err := service.Preview(request)
	if err != nil {
		t.Fatal(err)
	}
	job, err := service.Start(context.Background(), ExecuteRequest{PreviewRequest: request, Evidence: preview.Evidence})
	if err != nil {
		t.Fatal(err)
	}
	if job.Results[0].TargetID != "pane-a" || job.Results[1].TargetID != "pane-b" {
		t.Fatalf("job = %#v", job)
	}
}

func TestExecutionRequestRejectsAmbiguousSourcesAndTargets(t *testing.T) {
	service := testService(&memoryRepository{}, nil)
	snippet := createSnippet(t, service, Draft{Name: "Check", Command: "uptime"})
	tests := []PreviewRequest{
		{SnippetID: snippet.ID, Command: "uptime", Aliases: []string{"bastion"}},
		{Aliases: []string{"bastion"}},
		{Command: "uptime", Aliases: []string{"bastion"}, Targets: []RequestedTarget{{TargetID: "pane-a", Alias: "bastion"}}},
		{Command: "uptime", Targets: []RequestedTarget{{TargetID: "pane-a", Alias: "bastion"}, {TargetID: "pane-a", Alias: "database"}}},
	}
	for _, request := range tests {
		if _, err := service.Preview(request); !errors.Is(err, ErrInvalidTarget) && !errors.Is(err, ErrInvalidSnippet) {
			t.Fatalf("Preview(%#v) = %v", request, err)
		}
	}
}

func TestMultiExecutionIsBoundedAndKeepsHostResults(t *testing.T) {
	var active atomic.Int32
	var maximum atomic.Int32
	release := make(chan struct{})
	run := func(ctx context.Context, alias, command string) (CommandOutput, error) {
		current := active.Add(1)
		for {
			old := maximum.Load()
			if current <= old || maximum.CompareAndSwap(old, current) {
				break
			}
		}
		defer active.Add(-1)
		select {
		case <-ctx.Done():
			return CommandOutput{}, ctx.Err()
		case <-release:
		}
		if alias == "host-3" {
			return CommandOutput{ExitCode: 7, Stderr: []byte("failed")}, nil
		}
		return CommandOutput{Stdout: []byte(alias + ":ok")}, nil
	}
	service := testService(&memoryRepository{}, run)
	snippet := createSnippet(t, service, Draft{Name: "Check", Command: "uptime"})
	request := PreviewRequest{SnippetID: snippet.ID, Aliases: []string{"host-1", "host-2", "host-3", "host-4"}}
	preview, err := service.Preview(request)
	if err != nil {
		t.Fatal(err)
	}
	job, err := service.Start(context.Background(), ExecuteRequest{PreviewRequest: request, Evidence: preview.Evidence, Concurrency: 2})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for maximum.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	close(release)
	finished, err := service.Wait(context.Background(), job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if maximum.Load() != 2 || finished.Status != JobCompleted || len(finished.Results) != 4 {
		t.Fatalf("max=%d job=%#v", maximum.Load(), finished)
	}
	for index, result := range finished.Results {
		wantAlias := request.Aliases[index]
		if result.Alias != wantAlias {
			t.Fatalf("result[%d] = %#v", index, result)
		}
		if wantAlias == "host-3" && (result.Status != TargetFailed || result.ExitCode != 7) {
			t.Fatalf("failed host = %#v", result)
		}
	}
}

func TestCancelStopsDispatchAndMarksEveryTarget(t *testing.T) {
	started := make(chan string, MaxTargets)
	service := testService(&memoryRepository{}, func(ctx context.Context, alias, command string) (CommandOutput, error) {
		started <- alias
		<-ctx.Done()
		return CommandOutput{}, ctx.Err()
	})
	snippet := createSnippet(t, service, Draft{Name: "Check", Command: "uptime"})
	request := PreviewRequest{SnippetID: snippet.ID, Aliases: []string{"host-1", "host-2", "host-3", "host-4", "host-5"}}
	preview, err := service.Preview(request)
	if err != nil {
		t.Fatal(err)
	}
	job, err := service.Start(context.Background(), ExecuteRequest{PreviewRequest: request, Evidence: preview.Evidence, Concurrency: 2})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	<-started
	if err := service.Cancel(job.ID); err != nil {
		t.Fatal(err)
	}
	finished, err := service.Wait(context.Background(), job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finished.Status != JobCancelled {
		t.Fatalf("status = %s", finished.Status)
	}
	for _, result := range finished.Results {
		if result.Status != TargetCancelled || result.Problem != "cancelled" {
			t.Fatalf("result = %#v", result)
		}
	}
	if count := len(started); count != 0 {
		t.Fatalf("%d additional targets started after cancellation", count)
	}
}

func TestSecretValuesAreRedactedFromRemoteOutputAndErrorsAreStable(t *testing.T) {
	service := testService(&memoryRepository{}, func(ctx context.Context, alias, command string) (CommandOutput, error) {
		if !strings.Contains(command, "top-secret") {
			t.Errorf("runner command = %q", command)
		}
		return CommandOutput{Stdout: []byte("top-secret"), Stderr: []byte("bad top-secret")}, errors.New("top-secret broke")
	})
	snippet := createSnippet(t, service, Draft{
		Name: "Secret", Command: "login {{token}}",
		Variables: []Variable{{Name: "token", Type: VariableSecret, Required: true}},
	})
	request := PreviewRequest{SnippetID: snippet.ID, Aliases: []string{"bastion"}, Inputs: map[string]string{"token": "top-secret"}}
	preview, err := service.Preview(request)
	if err != nil {
		t.Fatal(err)
	}
	job, err := service.Start(context.Background(), ExecuteRequest{PreviewRequest: request, Evidence: preview.Evidence, Concurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	finished, err := service.Wait(context.Background(), job.ID)
	if err != nil {
		t.Fatal(err)
	}
	result := finished.Results[0]
	if result.Stdout != "[secret]" || result.Stderr != "bad [secret]" || result.Problem != "run_failed" {
		t.Fatalf("result = %#v", result)
	}
	if strings.Contains(result.Stdout+result.Stderr+result.Problem, "top-secret") {
		t.Fatal("a secret escaped in the public result")
	}
}
