package main

import (
	"encoding/json"
	"io"
)

// commandEnvelopeSchemaVersion は --json の封筒の形の版。
const commandEnvelopeSchemaVersion = 1

type commandFailure struct {
	Kind      string `json:"kind"`
	Retryable bool   `json:"retryable"`
}

// commandEnvelope は --json を付けたコマンドが標準出力へ出す1つの結果。成功も失敗も
// この形で出すので、呼び出し側は success を見てから result か failure を読めばよい。
// Serial／Telnet の非対話の自動処理の報告だけは、docs/design.md の独自の形で出す。
type commandEnvelope struct {
	SchemaVersion int             `json:"schemaVersion"`
	Success       bool            `json:"success"`
	Result        any             `json:"result,omitempty"`
	Failure       *commandFailure `json:"failure,omitempty"`
}

func writeCommandEnvelope(out io.Writer, envelope commandEnvelope) error {
	return json.NewEncoder(out).Encode(envelope)
}

func writeCommandSuccess(out io.Writer, result any) error {
	return writeCommandEnvelope(out, commandEnvelope{
		SchemaVersion: commandEnvelopeSchemaVersion, Success: true, Result: result,
	})
}

func writeCommandFailure(out io.Writer, failure commandFailure) error {
	return writeCommandEnvelope(out, commandEnvelope{
		SchemaVersion: commandEnvelopeSchemaVersion, Success: false, Failure: &failure,
	})
}
