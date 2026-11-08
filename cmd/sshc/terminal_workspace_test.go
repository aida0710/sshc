package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"sshc/internal/api"
	"sshc/internal/terminalworkspace"
)

// terminalTestWorkspaces は、エンジンが返す既定の並び（組み込みの2つは名前が空）に、利用者のものを足したものである。
func terminalTestWorkspaces(extra ...api.TerminalWorkspace) api.TerminalWorkspaceList {
	workspaces := []api.TerminalWorkspace{
		{Id: terminalworkspace.DefaultID, BuiltIn: true},
		{Id: terminalworkspace.AgentsID, BuiltIn: true, Hidden: true},
	}
	return api.TerminalWorkspaceList{MaxWorkspaces: 32, Workspaces: append(workspaces, extra...)}
}

func workspaceListEngine(t *testing.T, listed api.TerminalWorkspaceList) *engineAPI {
	t.Helper()
	return testTerminalEngine(t, http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/terminal/workspaces" {
			t.Errorf("unexpected %s %s", request.Method, request.URL.Path)
			http.NotFound(response, request)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(listed)
	}))
}

func TestTerminalCreateFindsAWorkspaceByTheNameTheScreenShows(t *testing.T) {
	engine := workspaceListEngine(t, terminalTestWorkspaces(api.TerminalWorkspace{Id: "a1b2c3", Name: "sshc開発"}))
	for requested, want := range map[string]string{
		"default": terminalworkspace.DefaultID,
		"Main":    terminalworkspace.DefaultID,
		"メイン":     terminalworkspace.DefaultID,
		"agents":  terminalworkspace.AgentsID,
		"Agents":  terminalworkspace.AgentsID,
		"エージェント":  terminalworkspace.AgentsID,
		"sshc開発":  "a1b2c3",
		"a1b2c3":  "a1b2c3",
		"":        terminalworkspace.AgentsID,
	} {
		got, err := resolveTerminalWorkspace(context.Background(), engine, requested)
		if err != nil || got != want {
			t.Errorf("--workspace %q = %q, %v; want %q", requested, got, err, want)
		}
	}
}

func TestTerminalCreatePrefersAWorkspaceTheUserNamedOverTheBuiltInScreenName(t *testing.T) {
	engine := workspaceListEngine(t, terminalTestWorkspaces(api.TerminalWorkspace{Id: "d4e5f6", Name: "Main"}))
	if got, err := resolveTerminalWorkspace(context.Background(), engine, "Main"); err != nil || got != "d4e5f6" {
		t.Fatalf("--workspace Main = %q, %v; want the workspace the user named Main", got, err)
	}
}

func TestTerminalCreateFindsARenamedBuiltInWorkspaceOnlyByItsNewName(t *testing.T) {
	listed := terminalTestWorkspaces()
	listed.Workspaces[0].Name = "仕事"
	engine := workspaceListEngine(t, listed)
	if got, err := resolveTerminalWorkspace(context.Background(), engine, "仕事"); err != nil || got != terminalworkspace.DefaultID {
		t.Fatalf("--workspace 仕事 = %q, %v", got, err)
	}
	if _, err := resolveTerminalWorkspace(context.Background(), engine, "メイン"); !errors.Is(err, errTerminalWorkspaceNotFound) {
		t.Fatalf("--workspace メイン after the rename = %v; want not found", err)
	}
}

func TestTerminalCreateReportsAMissingWorkspaceAsNotFoundWithoutOpeningASession(t *testing.T) {
	for _, asJSON := range []bool{true, false} {
		harness, server, stateDir := newSyncCommandHarness(t, func(response http.ResponseWriter, request *http.Request) {
			if request.URL.Path != "/api/v1/terminal/workspaces" {
				t.Errorf("unexpected %s %s", request.Method, request.URL.Path)
				http.NotFound(response, request)
				return
			}
			response.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(response).Encode(terminalTestWorkspaces())
		})
		arguments := []string{"sshc", "terminal", "create", "shell", "--workspace", "no-such-workspace"}
		if asJSON {
			arguments = append(arguments, "--json")
		}
		called, err := parseInvocation(arguments)
		if err != nil {
			t.Fatal(err)
		}

		var stdout, stderr strings.Builder
		code := runTerminal(context.Background(), *called.Terminal, commandEnvironment{
			stateDir: stateDir, client: server.Client(), stdout: &stdout, stderr: &stderr,
		})
		server.Close()

		if code != exitFailure {
			t.Fatalf("json=%v: code = %d; want %d", asJSON, code, exitFailure)
		}
		if slices.Contains(harness.methods, http.MethodPost) {
			t.Fatalf("json=%v: a session was opened: %v %v", asJSON, harness.methods, harness.paths)
		}
		if asJSON {
			want := `{"schemaVersion":1,"success":false,"failure":{"kind":"terminal_workspace_not_found","retryable":false}}`
			if strings.TrimSpace(stdout.String()) != want {
				t.Fatalf("stdout = %s; want %s", stdout.String(), want)
			}
			continue
		}
		if stderr.String() != "sshc: no terminal workspace has that ID or name\n" {
			t.Fatalf("stderr = %q", stderr.String())
		}
	}
}

// workspaceNameEntry は、画面の文言のファイルから組み込みのワークスペースの名前の行を読む。
var workspaceNameEntry = regexp.MustCompile(`^\s*"workspace\.(defaultName|agentsName)":\s*(".*"),$`)

func TestBuiltInWorkspaceDisplayNamesMatchTheWebCatalogs(t *testing.T) {
	idOfKey := map[string]string{"defaultName": terminalworkspace.DefaultID, "agentsName": terminalworkspace.AgentsID}
	shown := map[string][]string{}
	for _, catalog := range []string{"en.ts", "ja.ts"} {
		contents, err := os.ReadFile("../../web/src/i18n/messages/" + catalog)
		if err != nil {
			t.Fatal(err)
		}
		for line := range strings.SplitSeq(string(contents), "\n") {
			match := workspaceNameEntry.FindStringSubmatch(line)
			if match == nil {
				continue
			}
			name, err := strconv.Unquote(match[2])
			if err != nil {
				t.Fatalf("%s: %s: %v", catalog, line, err)
			}
			shown[idOfKey[match[1]]] = append(shown[idOfKey[match[1]]], name)
		}
	}
	for id, names := range builtInWorkspaceDisplayNames {
		if !slices.Equal(shown[id], names) {
			t.Errorf("%s: the screen shows %q, the CLI accepts %q", id, shown[id], names)
		}
	}
	if len(shown) != len(builtInWorkspaceDisplayNames) {
		t.Errorf("the screen names built-in workspaces %v, the CLI knows %v", shown, builtInWorkspaceDisplayNames)
	}
}
