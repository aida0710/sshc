package httpserver

import (
	"encoding/json"
	"net/http"
	"testing"

	"sshc/internal/api"
)

// 1台目が［確認して保存］で生成したキーを受け取っただけで、まだ同期していなくても、
// 同じ同期先を設定する2台目は既存の同期先として扱われ、別のキーを作らない（#632）。
func TestASecondMachineIsAskedForTheKeyTheFirstMachineWasShown(t *testing.T) {
	bucket := &measuredSyncBucket{}
	first, _ := syncEngineOn(t, bucket)
	second, _ := syncEngineOn(t, bucket)

	generate := checkSyncSetup(t, first)
	generate["key"] = ""
	recorder := sendSync(t, first, http.MethodPut, syncSetupPath, syncSetupBody(t, generate))
	if recorder.Code != http.StatusOK {
		t.Fatalf("first machine setup = %d: %s", recorder.Code, recorder.Body.String())
	}
	var created api.SyncSetupResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.GeneratedKey == nil {
		t.Fatal("first machine setup did not return the generated key")
	}
	if created.Status.Synced {
		t.Error("the first machine reports a sync although it has sent none of its files")
	}

	joining := checkSyncSetup(t, second)
	if joining["expectedState"] != string(api.Existing) {
		t.Fatalf("second machine check = %v, want existing", joining)
	}
	// 2台目が確かめる前の「空」の答えで保存しようとしても、キーを作らずに断る。
	stale := syncSetupBody(t, map[string]any{"expectedState": "empty", "key": ""})
	if recorder := sendSync(t, second, http.MethodPut, syncSetupPath, stale); recorder.Code != http.StatusConflict {
		t.Fatalf("setup with a stale empty check = %d, want 409: %s", recorder.Code, recorder.Body.String())
	}
	joining["key"] = ""
	if recorder := sendSync(t, second, http.MethodPut, syncSetupPath, syncSetupBody(t, joining)); recorder.Code != http.StatusBadRequest {
		t.Fatalf("joining without a key = %d, want 400: %s", recorder.Code, recorder.Body.String())
	}

	joining["key"] = *created.GeneratedKey
	recorder = sendSync(t, second, http.MethodPut, syncSetupPath, syncSetupBody(t, joining))
	if recorder.Code != http.StatusOK {
		t.Fatalf("joining with the first machine's key = %d: %s", recorder.Code, recorder.Body.String())
	}
	var joined api.SyncSetupResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &joined); err != nil {
		t.Fatal(err)
	}
	if joined.GeneratedKey != nil {
		t.Fatal("the second machine generated a key of its own")
	}
}
