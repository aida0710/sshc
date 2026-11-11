package remotesync_test

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"sshc/internal/envelope"
	"sshc/internal/remotesync"
)

const settingsExportTestKey = "AB12-CD34-EF56-GH78-JK90-MN12"

func exportedSettingsForTest() remotesync.SettingsExport {
	return remotesync.SettingsExport{
		Endpoint: "https://s3.example.com", Bucket: "team-bucket", Path: "team/laptop", Region: "auto",
		Credentials: remotesync.Credentials{AccessKeyID: "AKIAEXAMPLE", SecretAccessKey: "secret-access-key"},
	}
}

func TestExportedSettingsOpenWithTheSyncKeyThatSealedThem(t *testing.T) {
	want := exportedSettingsForTest()
	document, err := remotesync.SealSettingsExport(want, settingsExportTestKey)
	if err != nil {
		t.Fatalf("SealSettingsExport: %v", err)
	}
	for _, plain := range []string{want.Endpoint, want.Bucket, want.Credentials.AccessKeyID, want.Credentials.SecretAccessKey} {
		if strings.Contains(document, plain) {
			t.Fatalf("the exported document shows %q in the clear", plain)
		}
	}

	got, err := remotesync.OpenSettingsExport(document+"\n", settingsExportTestKey)
	if err != nil {
		t.Fatalf("OpenSettingsExport: %v", err)
	}
	if got != want {
		t.Fatalf("opened settings = %+v, want %+v", got, want)
	}
}

func TestExportedSettingsRefuseAnotherSyncKey(t *testing.T) {
	document, err := remotesync.SealSettingsExport(exportedSettingsForTest(), settingsExportTestKey)
	if err != nil {
		t.Fatalf("SealSettingsExport: %v", err)
	}
	_, err = remotesync.OpenSettingsExport(document, "ZZ12-CD34-EF56-GH78-JK90-MN12")
	if !errors.Is(err, remotesync.ErrWrongPassphrase) {
		t.Fatalf("error = %v, want ErrWrongPassphrase", err)
	}
}

func TestDocumentsThatAreNotExportedSettingsAreRejected(t *testing.T) {
	derivedKey, err := envelope.Derive(settingsExportTestKey)
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	sealedDocument := func(plaintext string) string {
		sealed, err := derivedKey.Seal([]byte(plaintext))
		if err != nil {
			t.Fatalf("Seal: %v", err)
		}
		document, err := json.Marshal(map[string]any{
			"sshcSyncSettings": 1, "sealed": base64.StdEncoding.EncodeToString(sealed),
		})
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		return string(document)
	}
	documents := map[string]string{
		"empty":                        "",
		"not JSON":                     "sshc",
		"another format version":       `{"sshcSyncSettings":2,"sealed":""}`,
		"an unknown outer field":       `{"sshcSyncSettings":1,"sealed":"","extra":true}`,
		"sealed is not base64":         `{"sshcSyncSettings":1,"sealed":"***"}`,
		"sealed is not an envelope":    `{"sshcSyncSettings":1,"sealed":"c3NoYw=="}`,
		"sealed content of other kind": sealedDocument(`{"kind":"snapshot","endpoint":"https://s3.example.com","bucket":"sshc","path":"","region":"auto","accessKeyId":"a","secretAccessKey":"b"}`),
		"sealed content is not JSON":   sealedDocument("archive bytes"),
		"longer than the limit":        `{"sshcSyncSettings":1,"sealed":"` + strings.Repeat("A", remotesync.MaxSettingsExportLength) + `"}`,
	}
	for name, document := range documents {
		t.Run(name, func(t *testing.T) {
			_, err := remotesync.OpenSettingsExport(document, settingsExportTestKey)
			if !errors.Is(err, remotesync.ErrSettingsExportRejected) {
				t.Fatalf("error = %v, want ErrSettingsExportRejected", err)
			}
		})
	}
}

func TestExportedSettingsFitInAQRCodeUnlessTheyAreTooLong(t *testing.T) {
	document, err := remotesync.SealSettingsExport(exportedSettingsForTest(), settingsExportTestKey)
	if err != nil {
		t.Fatalf("SealSettingsExport: %v", err)
	}
	rows, ok := remotesync.SettingsExportQRCode(document)
	if !ok {
		t.Fatalf("a %d-character document did not fit in a QR code", len(document))
	}
	for _, row := range rows {
		if len(row) != len(rows) || strings.Trim(row, "01") != "" {
			t.Fatalf("row %q is not a square row of modules", row)
		}
	}
	// 左上の位置合わせの印は、黒い 7 マスの辺で始まる。
	if !strings.HasPrefix(rows[0], "1111111") {
		t.Fatalf("first row %q does not start with a finder pattern", rows[0])
	}

	longest := exportedSettingsForTest()
	longest.Endpoint = "https://" + strings.Repeat("a", remotesync.MaxEndpointLength-len("https://"))
	longest.Credentials.SecretAccessKey = strings.Repeat("s", remotesync.MaxSecretAccessKeyLength)
	document, err = remotesync.SealSettingsExport(longest, settingsExportTestKey)
	if err != nil {
		t.Fatalf("SealSettingsExport: %v", err)
	}
	if _, ok := remotesync.SettingsExportQRCode(document); ok {
		t.Fatalf("a %d-character document was reported to fit in a QR code", len(document))
	}
	if len(document) > remotesync.MaxSettingsExportLength {
		t.Fatalf("the longest exported document is %d characters, over the import limit", len(document))
	}
}

func TestOnlyKeysInTheGeneratedShapeLookGenerated(t *testing.T) {
	generated, err := remotesync.NewKey()
	if err != nil {
		t.Fatalf("NewKey: %v", err)
	}
	if !remotesync.KeyLooksGenerated(generated) {
		t.Fatalf("generated key %q does not look generated", generated)
	}
	for _, chosen := range []string{"correct horse battery", "AB12-CD34-EF56-GH78-JK90", "AB12-CD34-EF56-GH78-JK90-MNIL", "ab12-cd34-ef56-gh78-jk90-mn12"} {
		if remotesync.KeyLooksGenerated(chosen) {
			t.Fatalf("chosen key %q looks generated", chosen)
		}
	}
}
