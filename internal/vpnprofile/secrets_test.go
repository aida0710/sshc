package vpnprofile

import (
	"reflect"
	"testing"

	"sshc/internal/vpn"
)

// secretsDocumentFilledWith は、どの項目にも prefix と項目の名前を入れた本文である。
func secretsDocumentFilledWith(prefix string) vpn.SecretsDocument {
	var document vpn.SecretsDocument
	fields := reflect.ValueOf(&document).Elem()
	for index := range fields.NumField() {
		fields.Field(index).SetString(prefix + fields.Type().Field(index).Name)
	}
	return document
}

// 送られたシークレットは、どの方式のどの項目も保存済みの値に重なる。足し忘れると、
// その項目だけが保存済みの値のまま残り、保存は成功と表示される。
func TestEverySentSecretReplacesTheStoredOne(t *testing.T) {
	stored := secretsDocumentFilledWith("stored ")
	sent := secretsDocumentFilledWith("sent ")

	if merged := overlaySecrets(stored, sent); merged != sent {
		t.Fatalf("overlaySecrets = %+v, want %+v", merged, sent)
	}
}

// 送られていない（空の）項目は、どの方式のどの項目も保存済みの値を残す。
func TestEveryUnsentSecretKeepsTheStoredOne(t *testing.T) {
	stored := secretsDocumentFilledWith("stored ")

	if merged := overlaySecrets(stored, vpn.SecretsDocument{}); merged != stored {
		t.Fatalf("overlaySecrets = %+v, want %+v", merged, stored)
	}
}
