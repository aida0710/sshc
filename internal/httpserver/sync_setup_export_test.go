package httpserver

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
)

const (
	syncSetupExportPath = "/api/v1/sync/setup/export"
	syncSetupImportPath = "/api/v1/sync/setup/import"
)

// setUpFirstMachine は、空の bucket に engine を設定し、生成された同期鍵を返す。
func setUpFirstMachine(t *testing.T, engine *echo.Echo) string {
	t.Helper()
	generate := checkSyncSetup(t, engine)
	generate["key"] = ""
	recorder := sendSync(t, engine, http.MethodPut, syncSetupPath, syncSetupBody(t, generate))
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
	return *created.GeneratedKey
}

func exportSyncSetup(t *testing.T, engine *echo.Echo) api.SyncSetupExport {
	t.Helper()
	recorder := sendSync(t, engine, http.MethodPost, syncSetupExportPath, "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("export = %d: %s", recorder.Code, recorder.Body.String())
	}
	var exported api.SyncSetupExport
	if err := json.Unmarshal(recorder.Body.Bytes(), &exported); err != nil {
		t.Fatal(err)
	}
	return exported
}

func importSyncSetupBody(t *testing.T, document, key string) string {
	t.Helper()
	body, err := json.Marshal(api.SyncSetupImportRequest{Document: document, Key: key, Direction: api.SyncDirectionPull})
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func syncStatusOf(t *testing.T, engine *echo.Echo) api.SyncStatus {
	t.Helper()
	recorder := sendSync(t, engine, http.MethodGet, "/api/v1/sync", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var status api.SyncStatus
	if err := json.Unmarshal(recorder.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	return status
}

func TestASecondMachineJoinsWithTheExportedSettingsAndTheSyncKey(t *testing.T) {
	bucket := &measuredSyncBucket{}
	first, _ := syncEngineOn(t, bucket)
	second, _ := syncEngineOn(t, bucket)
	syncKey := setUpFirstMachine(t, first)

	exported := exportSyncSetup(t, first)
	if !exported.KeyGenerated {
		t.Error("the generated key is reported as chosen by the user")
	}
	if exported.QrRows == nil {
		t.Error("ordinary settings did not come with a QR code")
	}
	for _, plain := range []string{"s3.example.invalid", "AKID", syncKey} {
		if strings.Contains(exported.Document, plain) {
			t.Fatalf("the exported document shows %q in the clear", plain)
		}
	}

	// 鍵の前後の空白は、手で設定するときと同じく無視する。
	recorder := sendSync(t, second, http.MethodPost, syncSetupImportPath, importSyncSetupBody(t, exported.Document, " "+syncKey+"\n"))
	if recorder.Code != http.StatusOK {
		t.Fatalf("import = %d: %s", recorder.Code, recorder.Body.String())
	}
	var joined api.SyncSetupResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &joined); err != nil {
		t.Fatal(err)
	}
	if joined.GeneratedKey != nil {
		t.Error("the second machine generated a key of its own")
	}
	status := joined.Status
	if !status.Configured || !status.KeyConfigured || status.Endpoint != "https://s3.example.invalid" ||
		status.Bucket != "sshc" || status.Direction != api.SyncDirectionPull {
		t.Fatalf("status after import = %+v", status)
	}
	if status.AccessKeySuffix == nil || *status.AccessKeySuffix != "AKID" {
		t.Fatalf("access key suffix after import = %v, want AKID", status.AccessKeySuffix)
	}

	// 取り込んだ鍵がこのマシンの同期鍵になっているので、そのまま受信の内容を確かめられる。
	if recorder := sendSync(t, second, http.MethodPost, "/api/v1/sync/pull", `{"apply":false}`); recorder.Code != http.StatusOK {
		t.Fatalf("pull preview after import = %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestImportSavesNothingWhenItIsRefused(t *testing.T) {
	bucket := &measuredSyncBucket{}
	first, _ := syncEngineOn(t, bucket)
	syncKey := setUpFirstMachine(t, first)
	document := exportSyncSetup(t, first).Document

	refusals := map[string]struct {
		bucket   *measuredSyncBucket
		document string
		key      string
		status   int
		code     string
	}{
		"another sync key":          {bucket, document, "ZZ12-CD34-EF56-GH78-JK90-MN12", http.StatusForbidden, "wrong_passphrase"},
		"not an exported document":  {bucket, `{"endpoint":"https://s3.example.invalid"}`, syncKey, http.StatusBadRequest, "sync_settings_export_rejected"},
		"the target has been wiped": {&measuredSyncBucket{}, document, syncKey, http.StatusConflict, "sync_setup_target_empty"},
	}
	for name, refusal := range refusals {
		t.Run(name, func(t *testing.T) {
			second, _ := syncEngineOn(t, refusal.bucket)
			recorder := sendSync(t, second, http.MethodPost, syncSetupImportPath, importSyncSetupBody(t, refusal.document, refusal.key))
			if recorder.Code != refusal.status || !strings.Contains(recorder.Body.String(), refusal.code) {
				t.Fatalf("import = %d %s, want %d %s", recorder.Code, recorder.Body.String(), refusal.status, refusal.code)
			}
			if status := syncStatusOf(t, second); status.Configured {
				t.Fatalf("a refused import left the machine configured: %+v", status)
			}
			if objects := len(refusal.bucket.objects); refusal.bucket != bucket && objects != 0 {
				t.Fatalf("a refused import wrote %d objects to the empty target", objects)
			}
		})
	}
}

func TestExportIsRefusedBeforeSyncIsSetUp(t *testing.T) {
	engine, _ := syncEngine(t)
	recorder := sendSync(t, engine, http.MethodPost, syncSetupExportPath, "")
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "sync_not_configured") {
		t.Fatalf("export = %d %s, want 409 sync_not_configured", recorder.Code, recorder.Body.String())
	}
}

func TestExportWarnsAboutAKeyTheUserChose(t *testing.T) {
	engine, _ := syncEngine(t)
	chosen := checkSyncSetup(t, engine)
	chosen["key"] = "a key the user chose"
	if recorder := sendSync(t, engine, http.MethodPut, syncSetupPath, syncSetupBody(t, chosen)); recorder.Code != http.StatusOK {
		t.Fatalf("setup = %d: %s", recorder.Code, recorder.Body.String())
	}
	if exportSyncSetup(t, engine).KeyGenerated {
		t.Fatal("a key the user chose is reported as generated")
	}
}
