package httpserver

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshc/internal/application"
)

func TestHostEditsをHTTPで送るとPreviewとSaveが通る(t *testing.T) {
	for _, kind := range []application.EditKind{application.EditMoveHosts, application.EditDeleteHosts} {
		t.Run(string(kind), func(t *testing.T) {
			harness := newConfigHarness(t)
			if _, err := harness.service.Save(application.EditRequest{Kind: application.EditGroups,
				Groups: []application.GroupMetadata{{Name: "work"}}}); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(harness.root, "config")
			base, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			base = append(base, []byte("\nHost second\n  HostName second.example\n")...)
			if err := os.WriteFile(path, base, 0o600); err != nil {
				t.Fatal(err)
			}
			request := application.EditRequest{Kind: kind, Hosts: []application.HostIdentity{
				{Path: "config", Alias: "bastion"},
				{Path: "config", Alias: "second"},
			}, Bases: []application.FileBase{{Path: "config", Base: string(base)}}}
			if kind == application.EditMoveHosts {
				request.DestinationGroup = "work"
			}
			previewResponse := harness.call(t, http.MethodPost, "/api/v1/config/preview", request, true, true)
			if previewResponse.Code != http.StatusOK {
				t.Fatalf("preview = %d: %s", previewResponse.Code, previewResponse.Body)
			}
			var preview application.SavePreview
			if err := json.Unmarshal(previewResponse.Body.Bytes(), &preview); err != nil {
				t.Fatal(err)
			}
			if preview.Operation != "config."+string(kind) || len(preview.Diffs) == 0 {
				t.Fatalf("preview = %#v", preview)
			}
			if contents, err := os.ReadFile(path); err != nil || string(contents) != string(base) {
				t.Fatalf("preview changed config: %v", err)
			}
			saveResponse := harness.call(t, http.MethodPost, "/api/v1/config/save", request, true, true)
			if saveResponse.Code != http.StatusOK {
				t.Fatalf("save = %d: %s", saveResponse.Code, saveResponse.Body)
			}
			var saved application.SaveResult
			if err := json.Unmarshal(saveResponse.Body.Bytes(), &saved); err != nil || saved.TransactionID == "" {
				t.Fatalf("save result = %#v, %v", saved, err)
			}
			contents, err := os.ReadFile(path)
			if err != nil || strings.Contains(string(contents), "Host bastion") || strings.Contains(string(contents), "Host second") {
				t.Fatalf("save did not remove both source blocks: %q, %v", contents, err)
			}
			if kind == application.EditMoveHosts {
				destination, err := os.ReadFile(filepath.Join(harness.root, "connections/work/config.conf"))
				if err != nil || !strings.Contains(string(destination), "Host bastion") || !strings.Contains(string(destination), "Host second") {
					t.Fatalf("destination = %q, %v", destination, err)
				}
			}
		})
	}
}

func TestHostEditsの不正な選択をHTTPで送ると拒否される(t *testing.T) {
	host := application.HostIdentity{Path: "config", Alias: "bastion"}
	base := application.FileBase{Path: "config", Base: handlerConfig}
	selections := map[string]application.EditRequest{
		"empty":                 {Bases: []application.FileBase{base}},
		"too many":              {Hosts: make([]application.HostIdentity, application.MaxHostEdits+1), Bases: []application.FileBase{base}},
		"missing alias":         {Hosts: []application.HostIdentity{{Path: "config"}}, Bases: []application.FileBase{base}},
		"duplicate":             {Hosts: []application.HostIdentity{host, host}, Bases: []application.FileBase{base}},
		"no contents":           {Hosts: []application.HostIdentity{host}},
		"missing base path":     {Hosts: []application.HostIdentity{host}, Bases: []application.FileBase{{Base: handlerConfig}}},
		"traversal":             {Hosts: []application.HostIdentity{{Path: "../config", Alias: "bastion"}}, Bases: []application.FileBase{{Path: "../config", Base: handlerConfig}}},
		"oversized base":        {Hosts: []application.HostIdentity{host}, Bases: []application.FileBase{{Path: "config", Base: strings.Repeat("a", maxRawLength+1)}}},
		"two contents for file": {Hosts: []application.HostIdentity{host}, Bases: []application.FileBase{base, {Path: "config", Base: "different"}}},
		"contents nothing uses": {Hosts: []application.HostIdentity{host}, Bases: []application.FileBase{base, {Path: "conf.d/10-home.conf", Base: ""}}},
	}
	for name, selection := range selections {
		t.Run(name, func(t *testing.T) {
			harness := newConfigHarness(t)
			for _, kind := range []application.EditKind{application.EditMoveHosts, application.EditDeleteHosts} {
				request := application.EditRequest{Kind: kind, Hosts: selection.Hosts, Bases: selection.Bases, DestinationPath: "config", DestinationBase: handlerConfig}
				for _, endpoint := range []string{"preview", "save"} {
					response := harness.call(t, http.MethodPost, "/api/v1/config/"+endpoint, request, true, true)
					if response.Code != http.StatusBadRequest {
						t.Fatalf("%s %s = %d: %s", kind, endpoint, response.Code, response.Body)
					}
				}
			}
		})
	}
}

func TestHostEditsの古いBaseをHTTPで送ると全体のConflictが返る(t *testing.T) {
	harness := newConfigHarness(t)
	request := application.EditRequest{Kind: application.EditDeleteHosts,
		Hosts: []application.HostIdentity{{Path: "config", Alias: "bastion"}},
		Bases: []application.FileBase{{Path: "config", Base: handlerConfig + "# stale\n"}},
	}
	for _, endpoint := range []string{"preview", "save"} {
		response := harness.call(t, http.MethodPost, "/api/v1/config/"+endpoint, request, true, true)
		if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), `"path":"config"`) {
			t.Fatalf("%s = %d: %s", endpoint, response.Code, response.Body)
		}
	}
}
