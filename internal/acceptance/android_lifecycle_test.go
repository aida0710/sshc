package acceptance_test

import (
	"strings"
	"testing"
)

// アプリの内部に保存した状態は、クラウドバックアップにも機種変更時のデバイス間転送にも
// 載せない。Android 11以前が読むbackup_rulesと、Android 12以降が読むdata_extraction_rulesの
// 両方で、すべての保存先を除外する。
func TestTheAndroidShellExcludesPrivateStateFromBackupAndDeviceTransfer(t *testing.T) {
	manifest := readRepoFile(t, "android", "app", "src", "main", "AndroidManifest.xml")
	for _, required := range []string{
		`android:allowBackup="false"`,
		`android:fullBackupContent="@xml/backup_rules"`,
		`android:dataExtractionRules="@xml/data_extraction_rules"`,
	} {
		if !strings.Contains(manifest, required) {
			t.Errorf("Android manifest lacks private-state backup boundary %q", required)
		}
	}

	legacy := readRepoFile(t, "android", "app", "src", "main", "res", "xml", "backup_rules.xml")
	modern := readRepoFile(t, "android", "app", "src", "main", "res", "xml", "data_extraction_rules.xml")
	for _, domain := range []string{
		"root", "file", "database", "sharedpref", "external",
		"device_root", "device_file", "device_database", "device_sharedpref",
	} {
		exclusion := `<exclude domain="` + domain + `" path="." />`
		if !strings.Contains(legacy, exclusion) {
			t.Errorf("legacy backup rules do not exclude %s", domain)
		}
		if count := strings.Count(modern, exclusion); count != 2 {
			t.Errorf("Android 12 rules exclude %s %d times, want cloud backup and device transfer", domain, count)
		}
	}
	for _, section := range []string{"<cloud-backup>", "<device-transfer>"} {
		if !strings.Contains(modern, section) {
			t.Errorf("Android 12 extraction rules lack %s", section)
		}
	}
}

// dataSyncのforeground serviceは、利用者がタスクを閉じたときとAndroidが時間切れを
// 通知したときに止まる。起動に失敗した画面では、serviceへの接続を保ったまま診断を出し、
// 同じ画面から再試行できる。
func TestTheAndroidDataSyncEngineHasABoundedLifetime(t *testing.T) {
	manifest := readRepoFile(t, "android", "app", "src", "main", "AndroidManifest.xml")
	if !strings.Contains(manifest, `android:foregroundServiceType="dataSync"`) ||
		!strings.Contains(manifest, `android:stopWithTask="true"`) {
		t.Fatal("the dataSync foreground engine must stop with its user task")
	}

	service := readRepoFile(t, "android", "app", "src", "main", "java", "com", "github", "aida0710", "sshc", "EngineService.java")
	for _, required := range []string{
		"public void onTimeout(int startId, int foregroundServiceType)",
		"stopServiceAndEngine(startId);",
		"shutdown.request();",
		"Executors.newSingleThreadExecutor",
		"ENGINE.execute(this::startEngine);",
		"stopForeground(STOP_FOREGROUND_REMOVE);",
		"stopSelf(startId);",
		"return START_NOT_STICKY;",
	} {
		if !strings.Contains(service, required) {
			t.Errorf("bounded foreground lifecycle lacks %q", required)
		}
	}
	if strings.Contains(service, "return START_STICKY;") {
		t.Error("the foreground engine still asks Android to recreate it as a permanent service")
	}
	if strings.Contains(service, "private void shutdown()") {
		t.Error("the Android main looper still performs the blocking Go shutdown itself")
	}

	activity := readRepoFile(t, "android", "app", "src", "main", "java", "com", "github", "aida0710", "sshc", "MainActivity.java")
	for _, required := range []string{
		"long failure = service.failure();",
		"showFailure(failure, service.failureCode(), service.failureDetail());",
		"startForegroundService(new Intent(this, EngineService.class));",
		"service.retry()",
		"unbindService(connection);",
	} {
		if !strings.Contains(activity, required) {
			t.Errorf("failed engine start cannot expose diagnostics and retry safely: lacks %q", required)
		}
	}
	if strings.Contains(activity, "releaseService();\n        showFailure") {
		t.Error("failure screen unbinds the stopped service, so its in-place retry cannot work")
	}
}
