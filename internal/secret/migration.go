package secret

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Migration は、復号したvault文書へ適用した連続したschema更新を表す。
// zero valueは変換なしであり、秘密の値を一切含まない。
type Migration struct {
	From int
	To   int
}

// Applied は、少なくとも一段のmigrationを適用したかを報告する。
func (m Migration) Applied() bool { return m.From > 0 && m.To > m.From }

var ErrMigrationFailed = errors.New("vault schema migration failed")

// MigrationError は、どの一段が失敗したかを安全な診断情報として運ぶ。
// CauseはHTTP応答へ直接出さず、呼び出し側がerrors.Is/Asで分類するためだけに残す。
type MigrationError struct {
	From  int
	To    int
	Cause error
}

func (e *MigrationError) Error() string {
	return fmt.Sprintf("vault schema migration from %d to %d failed", e.From, e.To)
}

func (e *MigrationError) Unwrap() error { return e.Cause }

func (e *MigrationError) Is(target error) bool { return target == ErrMigrationFailed }

// documentMigrationは、fromからfrom+1への一段だけを変換する。
// schemaVersion自体はrunnerが成功後に書き換えるため、各stepが飛び級したり
// version更新だけを先に公開したりすることはできない。
type documentMigration func(map[string]json.RawMessage) error

type migrationRegistry map[int]documentMigration

// migrationBaseVersionは、段階migrationを保証する最初のschemaである。
// 対応期間を意図して短くするときだけ明示的に進める。SchemaVersionだけを上げて
// stepを追加し忘れるとcontract testが失敗する。
const migrationBaseVersion = 4

// registeredDocumentMigrationsが本番で許可する唯一の経路である。SchemaVersionを
// 上げる変更は、直前versionをkeyとするstepと旧版fixtureを同じcommitで追加する。
// 現在はschema 4を出発点とするため、過去形式を暗黙に復活させるstepは持たない。
var registeredDocumentMigrations = migrationRegistry{}

func migrateDocument(plaintext []byte, migrations migrationRegistry) ([]byte, Migration, error) {
	fields, version, err := migrationFields(plaintext)
	if err != nil {
		return nil, Migration{}, err
	}
	if version > SchemaVersion {
		return nil, Migration{}, &SchemaVersionError{Found: version, Supported: SchemaVersion}
	}
	if version == SchemaVersion {
		return plaintext, Migration{}, nil
	}

	initial := version
	for version < SchemaVersion {
		step, ok := migrations[version]
		if !ok {
			return nil, Migration{}, &SchemaVersionError{Found: initial, Supported: SchemaVersion}
		}
		if err := step(fields); err != nil {
			return nil, Migration{}, &MigrationError{From: version, To: version + 1, Cause: err}
		}
		version++
		encodedVersion, err := json.Marshal(version)
		if err != nil {
			return nil, Migration{}, &MigrationError{From: version - 1, To: version, Cause: err}
		}
		fields["schemaVersion"] = encodedVersion
	}
	migrated, err := json.Marshal(fields)
	if err != nil {
		return nil, Migration{}, &MigrationError{From: initial, To: version, Cause: err}
	}
	return migrated, Migration{From: initial, To: version}, nil
}

func migrationFields(plaintext []byte) (map[string]json.RawMessage, int, error) {
	var fields map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(plaintext))
	if err := decoder.Decode(&fields); err != nil || fields == nil {
		return nil, 0, ErrWrongPassphrase
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, 0, ErrWrongPassphrase
	}
	encodedVersion, ok := fields["schemaVersion"]
	if !ok {
		return nil, 0, ErrWrongPassphrase
	}
	var version int
	if err := json.Unmarshal(encodedVersion, &version); err != nil || version < 1 {
		return nil, 0, ErrWrongPassphrase
	}
	return fields, version, nil
}
