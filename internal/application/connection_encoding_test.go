package application

import (
	"testing"

	"sshc/internal/textencoding"
)

func TestConnectionEncodingFollowsTheConcreteHostMetadata(t *testing.T) {
	metadata := NewMetadata()
	metadata.Hosts = []HostMetadata{{
		Identity: HostIdentity{Path: "config", Alias: "legacy"}, Encoding: string(textencoding.ShiftJIS),
	}}
	service := serviceWithConfig(t, "Host legacy\n  HostName example.test\n", metadata)

	got, err := service.ConnectionEncoding("legacy")
	if err != nil {
		t.Fatal(err)
	}
	if got != textencoding.ShiftJIS {
		t.Fatalf("encoding = %q", got)
	}
	// 接続先の解決と同じく大文字と小文字を区別する。LEGACY にはどのブロックも適用されない。
	if other, err := service.ConnectionEncoding("LEGACY"); err != nil || other != textencoding.UTF8 {
		t.Fatalf("encoding(LEGACY) = %q, %v", other, err)
	}
	defaulted, err := service.ConnectionEncoding("other")
	if err != nil {
		t.Fatal(err)
	}
	if defaulted != textencoding.UTF8 {
		t.Fatalf("default encoding = %q", defaulted)
	}
}
