package strictjson_test

import (
	"errors"
	"testing"

	"sshc/internal/strictjson"
)

type sample struct {
	Name string `json:"name"`
}

func TestOneDocumentFollowedOnlyByWhitespaceIsRead(t *testing.T) {
	var decoded sample
	if err := strictjson.Decode([]byte("{\"name\":\"a\"}\n \t"), &decoded); err != nil {
		t.Fatalf("Decode = %v", err)
	}
	if decoded.Name != "a" {
		t.Fatalf("decoded = %#v", decoded)
	}
}

func TestAnUnknownFieldIsRefused(t *testing.T) {
	var decoded sample
	if err := strictjson.Decode([]byte(`{"name":"a","nmae":"b"}`), &decoded); err == nil {
		t.Fatal("an unknown field was accepted")
	}
}

func TestDataAfterTheDocumentIsRefusedAsTrailingData(t *testing.T) {
	for _, document := range []string{`{"name":"a"}}`, `{"name":"a"}]`, `{"name":"a"} {"name":"b"}`, `{"name":"a"} 1`} {
		var decoded sample
		if err := strictjson.Decode([]byte(document), &decoded); !errors.Is(err, strictjson.ErrTrailingData) {
			t.Errorf("Decode(%s) = %v, want ErrTrailingData", document, err)
		}
	}
}

func TestAnEmptyOrBrokenDocumentIsRefused(t *testing.T) {
	for _, document := range []string{"", "   ", `{"name":`, `["a"]`} {
		var decoded sample
		err := strictjson.Decode([]byte(document), &decoded)
		if err == nil || errors.Is(err, strictjson.ErrTrailingData) {
			t.Errorf("Decode(%q) = %v, want a decoding error", document, err)
		}
	}
}
