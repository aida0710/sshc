package main

import (
	"bytes"
	"encoding/json"
	"testing"
)

// decodedCommandEnvelope は、result の形をコマンドごとに読めるよう生のまま残した封筒。
type decodedCommandEnvelope struct {
	SchemaVersion int             `json:"schemaVersion"`
	Success       bool            `json:"success"`
	Result        json.RawMessage `json:"result"`
	Failure       *commandFailure `json:"failure"`
}

func decodeCommandEnvelope(t *testing.T, output string) decodedCommandEnvelope {
	t.Helper()
	var envelope decodedCommandEnvelope
	decoder := json.NewDecoder(bytes.NewBufferString(output))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		t.Fatalf("--json did not print one envelope: %v\n%s", err, output)
	}
	if decoder.More() {
		t.Fatalf("--json printed more than one value:\n%s", output)
	}
	if envelope.SchemaVersion != commandEnvelopeSchemaVersion {
		t.Fatalf("schemaVersion = %d:\n%s", envelope.SchemaVersion, output)
	}
	return envelope
}

// decodeCommandSuccess は、--json の出力が成功の封筒1つであることを確かめて result を読む。
func decodeCommandSuccess(t *testing.T, output string, result any) {
	t.Helper()
	envelope := decodeCommandEnvelope(t, output)
	if !envelope.Success || envelope.Failure != nil {
		t.Fatalf("envelope is not a success:\n%s", output)
	}
	if err := json.Unmarshal(envelope.Result, result); err != nil {
		t.Fatalf("result = %v\n%s", err, output)
	}
}

// decodeCommandFailure は、--json の出力が失敗の封筒1つであることを確かめて failure を返す。
func decodeCommandFailure(t *testing.T, output string) commandFailure {
	t.Helper()
	envelope := decodeCommandEnvelope(t, output)
	if envelope.Success || envelope.Failure == nil || envelope.Result != nil {
		t.Fatalf("envelope is not a failure:\n%s", output)
	}
	return *envelope.Failure
}
