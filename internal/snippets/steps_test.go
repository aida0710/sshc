package snippets

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"sshc/internal/platform/windowsacl/acltest"
)

// enableSteps is a snippet that enters a network device's privileged mode: it waits for each prompt before it types.
func enableSteps() Draft {
	return Draft{
		Name: "Privileged mode",
		Steps: []Step{
			{Wait: "router>", Send: "enable", Enter: true},
			{Wait: "Password:", Send: "{{enable_password}}", Enter: true},
			{Wait: "Last login", Send: "q", Optional: true},
			{Wait: "router#", Send: "terminal length {{lines}}", Enter: true},
		},
		WaitSeconds: 20,
		Variables: []Variable{
			{Name: "enable_password", Type: VariableSecret, Required: true},
			{Name: "lines", Type: VariableInteger, Default: stringPointer("0")},
		},
	}
}

func stringPointer(value string) *string { return &value }

func TestAConnectionTypesTheStepsOfItsStartupSnippetWithTheirVariablesPutIn(t *testing.T) {
	service := testService(&memoryRepository{}, nil)
	snippet := createSnippet(t, service, enableSteps())
	if err := service.SetStartup("bastion", snippet.ID, map[string]string{"enable_password": "s3cret"}); err != nil {
		t.Fatal(err)
	}

	prepared, err := service.PrepareStartup("bastion")
	if err != nil {
		t.Fatal(err)
	}
	want := PreparedStartup{
		Steps: []PreparedStep{
			{Wait: "router>", Send: "enable", Enter: true},
			{Wait: "Password:", Send: "s3cret", Enter: true},
			{Wait: "Last login", Send: "q", Optional: true},
			{Wait: "router#", Send: "terminal length 0", Enter: true},
		},
		Wait:    20 * time.Second,
		Secrets: []string{"s3cret"},
	}
	if !reflect.DeepEqual(prepared, want) {
		t.Fatalf("PrepareStartup = %#v, want %#v", prepared, want)
	}
}

func TestStepsWaitTenSecondsWhenTheSnippetSaysNoTime(t *testing.T) {
	service := testService(&memoryRepository{}, nil)
	draft := enableSteps()
	draft.WaitSeconds = 0
	snippet := createSnippet(t, service, draft)
	if err := service.SetStartup("bastion", snippet.ID, map[string]string{"enable_password": "s3cret"}); err != nil {
		t.Fatal(err)
	}
	prepared, err := service.PrepareStartup("bastion")
	if err != nil || prepared.Wait != 10*time.Second {
		t.Fatalf("PrepareStartup = %#v, %v; want a wait of 10 seconds", prepared, err)
	}
}

func TestAStartupCommandIsOneStepThatTypesItWithEnter(t *testing.T) {
	service := testService(&memoryRepository{}, nil)
	snippet := createSnippet(t, service, Draft{Name: "Uptime", Command: "uptime"})
	if err := service.SetStartup("bastion", snippet.ID, nil); err != nil {
		t.Fatal(err)
	}
	prepared, err := service.PrepareStartup("bastion")
	if err != nil || !reflect.DeepEqual(prepared.Steps, []PreparedStep{{Send: "uptime", Enter: true}}) {
		t.Fatalf("PrepareStartup = %#v, %v", prepared, err)
	}
}

func TestStepsAreRefusedWhenTheyCannotBeTyped(t *testing.T) {
	steps := func(change func(*Draft)) Draft {
		draft := Draft{Name: "Steps", Steps: []Step{{Wait: "$ ", Send: "uptime", Enter: true}}}
		change(&draft)
		return draft
	}
	for name, draft := range map[string]Draft{
		"a command as well":           steps(func(draft *Draft) { draft.Command = "uptime" }),
		"nothing to type":             steps(func(draft *Draft) { draft.Steps[0] = Step{Wait: "$ "} }),
		"a wait of only spaces":       steps(func(draft *Draft) { draft.Steps[0].Wait = "   " }),
		"a control character":         steps(func(draft *Draft) { draft.Steps[0].Wait = "$\x1b" }),
		"too long a wait":             steps(func(draft *Draft) { draft.Steps[0].Wait = strings.Repeat("x", MaxWaitBytes+1) }),
		"too much to type at once":    steps(func(draft *Draft) { draft.Steps[0].Send = strings.Repeat("x", MaxStepSendBytes+1) }),
		"too many steps":              steps(func(draft *Draft) { draft.Steps = make([]Step, MaxSteps+1); draft.Steps[0].Send = "x" }),
		"too long a time":             steps(func(draft *Draft) { draft.WaitSeconds = MaxWaitSeconds + 1 }),
		"a time for a command":        {Name: "Command", Command: "uptime", WaitSeconds: 5},
		"an unused variable":          steps(func(draft *Draft) { draft.Variables = []Variable{{Name: "unused", Type: VariableString}} }),
		"a placeholder it can't fill": steps(func(draft *Draft) { draft.Steps[0].Send = "echo {{missing}}" }),
	} {
		service := testService(&memoryRepository{}, nil)
		if _, err := service.Create(draft); err == nil {
			t.Errorf("Create with %s succeeded", name)
		}
	}
}

func TestStepsAreTypedOnlyWhenAConnectionStarts(t *testing.T) {
	service := testService(&memoryRepository{}, nil)
	snippet := createSnippet(t, service, Draft{Name: "Steps", Steps: []Step{{Wait: "$ ", Send: "uptime", Enter: true}}})
	if _, err := service.PrepareTerminalCommand(CommandRequest{SnippetID: snippet.ID}); !errors.Is(err, ErrStepsOnlyAtStartup) {
		t.Fatalf("PrepareTerminalCommand = %v, want ErrStepsOnlyAtStartup", err)
	}
	if _, err := service.Preview(PreviewRequest{SnippetID: snippet.ID, Aliases: []string{"bastion"}}); !errors.Is(err, ErrStepsOnlyAtStartup) {
		t.Fatalf("Preview = %v, want ErrStepsOnlyAtStartup", err)
	}
}

// A document written before snippets had steps is one without them, and is read as it is.
func TestADocumentFromBeforeStepsIsRead(t *testing.T) {
	store, _ := newFileStore(t)
	body := `{"schemaVersion":2,"snippets":[{"id":"0123456789abcdef0123456789abcdef","name":"Uptime","description":"","command":"uptime","variables":[],"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z"}]}`
	sealed, err := store.protect.Seal([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	acltest.WritePrivateFile(t, store.Path(), sealed)
	library, err := store.Load()
	if err != nil || len(library.Snippets) != 1 || library.Snippets[0].Command != "uptime" {
		t.Fatalf("Load = %#v, %v", library, err)
	}
}
