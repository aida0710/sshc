package httpserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"sshc/internal/application"
	"sshc/internal/platform/windowsacl/acltest"
)

func readSettings(t *testing.T, harness *testHarness) application.SettingsDocument {
	t.Helper()
	response := harness.call(t, http.MethodGet, "/api/v1/settings", nil, true, true)
	if response.Code != http.StatusOK {
		t.Fatalf("settings = %d, body %s", response.Code, response.Body.String())
	}
	var document application.SettingsDocument
	if err := json.Unmarshal(response.Body.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	return document
}

func TestTheSettingsDocumentListsThisMachineAndTheStandardPreset(t *testing.T) {
	harness := newConfigHarness(t)

	document := readSettings(t, harness)

	if document.MachineID == "" || document.PresetInUse != application.DefaultPresetID || len(document.Presets) != 1 {
		t.Fatalf("document = %#v", document)
	}
	if len(document.Machines) != 1 || document.Machines[0].ID != document.MachineID || document.Machines[0].Settings.Name != "desk" {
		t.Fatalf("machines = %#v", document.Machines)
	}
}

func TestSettingsSavesReturnTheirWireSaveResult(t *testing.T) {
	harness := newConfigHarness(t)
	document := readSettings(t, harness)
	standard := document.Presets[0]
	for name, request := range map[string]struct {
		target string
		body   any
	}{
		"global":    {"/api/v1/settings/global", map[string]any{"base": map[string]any{}, "settings": map[string]any{"language": "ja"}}},
		"presets":   {"/api/v1/settings/presets", map[string]any{"base": document.Presets, "presets": []any{standard, map[string]any{"id": "phone", "name": "スマホ"}}}},
		"machine":   {"/api/v1/settings/machines/" + document.MachineID, map[string]any{"base": document.Machines[0].Settings, "settings": map[string]any{"name": "desk", "port": 43124}}},
		"shortcuts": {"/api/v1/metadata/shortcuts", map[string]any{"base": []any{}, "presets": []any{}}},
	} {
		t.Run(name, func(t *testing.T) {
			response := harness.call(t, http.MethodPut, request.target, request.body, true, true)
			if response.Code != http.StatusOK {
				t.Fatalf("save = %d, body %s", response.Code, response.Body.String())
			}
			var raw map[string]json.RawMessage
			if err := json.Unmarshal(response.Body.Bytes(), &raw); err != nil {
				t.Fatal(err)
			}
			if _, exists := raw["preview"]; exists || raw["transactionId"] == nil || raw["written"] == nil {
				t.Fatalf("settings save result = %s", response.Body.String())
			}
		})
	}
}

func TestAPresetIsEditedAndAStaleEditIsRefused(t *testing.T) {
	harness := newConfigHarness(t)
	standard := readSettings(t, harness).Presets[0]
	edited := map[string]any{"id": "default", "appearance": map[string]any{"accent": "blue", "theme": "light"}}

	saved := harness.call(t, http.MethodPut, "/api/v1/settings/presets/default", map[string]any{"base": standard, "preset": edited}, true, true)
	if saved.Code != http.StatusOK {
		t.Fatalf("save = %d, body %s", saved.Code, saved.Body.String())
	}
	if appearance := readSettings(t, harness).Presets[0].Appearance; appearance == nil || appearance.Accent != "blue" || appearance.Theme != "light" {
		t.Fatalf("appearance = %#v", appearance)
	}

	stale := harness.call(t, http.MethodPut, "/api/v1/settings/presets/default", map[string]any{"base": standard, "preset": edited}, true, true)
	if stale.Code != http.StatusConflict || !strings.Contains(stale.Body.String(), "settings_changed") {
		t.Fatalf("stale save = %d, body %s", stale.Code, stale.Body.String())
	}
}

func TestAPresetOutOfRangeIsABadRequest(t *testing.T) {
	harness := newConfigHarness(t)
	standard := readSettings(t, harness).Presets[0]
	for name, preset := range map[string]any{
		"accent":     map[string]any{"id": "default", "appearance": map[string]any{"accent": "orange"}},
		"auto lock":  map[string]any{"id": "default", "vaultAutoLock": map[string]any{"mode": "idle", "value": 1000, "unit": "hours"}},
		"sessions":   map[string]any{"id": "default", "terminal": map[string]any{"maxSessions": 9999}},
		"touch mode": map[string]any{"id": "default", "interface": map[string]any{"touchMode": "auto"}},
	} {
		t.Run(name, func(t *testing.T) {
			response := harness.call(t, http.MethodPut, "/api/v1/settings/presets/default", map[string]any{"base": standard, "preset": preset}, true, true)
			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid_settings") {
				t.Fatalf("response = %d, body %s", response.Code, response.Body.String())
			}
		})
	}
}

// 保存は metadata 全体を検査する。変えていない接続先の値が通らないときは、500 ではなく、どこが通らないかを
// 添えて断る。
func TestAPresetSaveRefusedByAnotherPartOfTheMetadataNamesThatPart(t *testing.T) {
	harness := newConfigHarness(t)
	contents := fmt.Sprintf(`{"schemaVersion":%d,"presets":[{"id":"default"}],"hosts":[{"identity":{"path":"config","alias":"edge"},"detectedOS":"beos"}]}`, application.MetadataSchemaVersion)
	acltest.WritePrivateFile(t, filepath.Join(harness.workspace.StateDir(), application.MetadataFileName), []byte(contents))
	standard := readSettings(t, harness).Presets[0]

	response := harness.call(t, http.MethodPut, "/api/v1/settings/presets/default",
		map[string]any{"base": standard, "preset": map[string]any{"id": "default", "interface": map[string]any{"keepScreenAwake": true}}}, true, true)

	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "operating system") || !strings.Contains(response.Body.String(), "edge") {
		t.Fatalf("response = %d, body %s", response.Code, response.Body.String())
	}
}

func TestAPortOutsideTheUnprivilegedRangeIsRefused(t *testing.T) {
	harness := newConfigHarness(t)
	document := readSettings(t, harness)
	for _, port := range []int{80, 65536} {
		response := harness.call(t, http.MethodPut, "/api/v1/settings/machines/"+document.MachineID,
			map[string]any{"base": document.Machines[0].Settings, "settings": map[string]any{"name": "desk", "port": port}}, true, true)
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "port_out_of_range") {
			t.Fatalf("port %d: response = %d, body %s", port, response.Code, response.Body.String())
		}
	}
}

// ポートはマシンごとのファイルに置き、全部のマシンが読む metadata.json には書かない。
func TestThePortIsKeptOutOfTheSharedMetadata(t *testing.T) {
	harness := newConfigHarness(t)
	document := readSettings(t, harness)
	if response := harness.call(t, http.MethodPut, "/api/v1/settings/machines/"+document.MachineID,
		map[string]any{"base": document.Machines[0].Settings, "settings": map[string]any{"name": "desk", "port": 43123}}, true, true); response.Code != http.StatusOK {
		t.Fatalf("save = %d, body %s", response.Code, response.Body.String())
	}

	if port := readSettings(t, harness).Machines[0].Settings.Port; port != 43123 {
		t.Fatalf("port = %d, want 43123", port)
	}
	stored, err := os.ReadFile(filepath.Join(harness.workspace.StateDir(), application.MetadataFileName))
	if err == nil && strings.Contains(string(stored), "43123") {
		t.Fatalf("metadata.json carries the port to other machines:\n%s", stored)
	}
}

func TestThisMachineCannotBeRemovedThroughTheAPI(t *testing.T) {
	harness := newConfigHarness(t)
	document := readSettings(t, harness)

	response := harness.call(t, http.MethodDelete, "/api/v1/settings/machines/"+document.MachineID, nil, true, true)

	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "this_machine") {
		t.Fatalf("response = %d, body %s", response.Code, response.Body.String())
	}
}

func TestInterfaceStateIsTakenAtOnceAndReadBack(t *testing.T) {
	harness := newConfigHarness(t)

	response := harness.call(t, http.MethodPut, "/api/v1/settings/state", map[string]any{"interface": map[string]string{"sftpQueueView": "list"}}, true, true)
	if response.Code != http.StatusNoContent {
		t.Fatalf("response = %d, body %s", response.Code, response.Body.String())
	}

	if got := readSettings(t, harness).State.Interface["sftpQueueView"]; got != "list" {
		t.Fatalf("sftpQueueView = %q, want list", got)
	}
	refused := harness.call(t, http.MethodPut, "/api/v1/settings/state", map[string]any{"interface": map[string]string{"theme": "dark"}}, true, true)
	if refused.Code != http.StatusBadRequest {
		t.Fatalf("unknown state = %d, body %s", refused.Code, refused.Body.String())
	}
	empty := harness.call(t, http.MethodPut, "/api/v1/settings/state", map[string]any{}, true, true)
	if empty.Code != http.StatusBadRequest {
		t.Fatalf("empty state = %d, body %s", empty.Code, empty.Body.String())
	}
}

// 転送の処理を止めたことは、再起動しても止まったままにするためすぐに書き、転送キューにも当てる。
// 画面はこれだけを送るので、プリセットや全体の転送の設定には触らない。
func TestStoppingTransfersIsWrittenAtOnceAndAppliedToTheQueue(t *testing.T) {
	harness := newConfigHarness(t)
	reapplied := 0
	harness.service.OnSettingsChanged(func() { reapplied++ })
	before := readSettings(t, harness)

	response := harness.call(t, http.MethodPut, "/api/v1/settings/state", map[string]any{"transfersStopped": true}, true, true)
	if response.Code != http.StatusNoContent {
		t.Fatalf("response = %d, body %s", response.Code, response.Body.String())
	}

	after := readSettings(t, harness)
	if !after.State.TransfersStopped {
		t.Fatal("transfers are not stopped")
	}
	if reapplied == 0 {
		t.Fatal("the transfer queue was not told to read its settings again")
	}
	contents, err := os.ReadFile(filepath.Join(harness.workspace.StateDir(), "machines", after.MachineID+".state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var written struct {
		TransfersStopped bool `json:"transfersStopped"`
	}
	if err := json.Unmarshal(contents, &written); err != nil || !written.TransfersStopped {
		t.Fatalf("state file = %s (%v), want transfersStopped written at once", contents, err)
	}
	if !reflect.DeepEqual(after.Presets, before.Presets) || !reflect.DeepEqual(after.Global, before.Global) {
		t.Fatalf("settings changed: presets %+v -> %+v, global %+v -> %+v", before.Presets, after.Presets, before.Global, after.Global)
	}
}
