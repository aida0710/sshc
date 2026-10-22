package browserauth

import (
	"time"

	"sshc/internal/strictjson"
)

// schemaVersionWithPrevious は、退役 token を直前の 1 世代だけ覚えていた形式の版。
// 登録を読み替えて引き継ぎ、ブラウザに `sshc open` で入り直させない。
const schemaVersionWithPrevious = 2

type registrationWithPrevious struct {
	Hash       string    `json:"hash"`
	Previous   string    `json:"previous,omitempty"`
	IssuedAt   time.Time `json:"issuedAt"`
	LastUsedAt time.Time `json:"lastUsedAt"`
}

type documentWithPrevious struct {
	SchemaVersion int                        `json:"schemaVersion"`
	Port          int                        `json:"port,omitempty"`
	Registrations []registrationWithPrevious `json:"registrations"`
}

// decodeDocumentWithPrevious は、版 2 の文書を現在の形に読み替える。直前の 1 世代は
// 退役 token の列の最初の 1 件になる。中身の検査は load が現在の形に対して行う。
func decodeDocumentWithPrevious(contents []byte) (document, error) {
	var stored documentWithPrevious
	if err := strictjson.Decode(contents, &stored); err != nil {
		return document{}, err
	}
	converted := document{SchemaVersion: SchemaVersion, Port: stored.Port, Registrations: make([]registration, 0, len(stored.Registrations))}
	for _, entry := range stored.Registrations {
		var retired []string
		if entry.Previous != "" {
			retired = []string{entry.Previous}
		}
		converted.Registrations = append(converted.Registrations, registration{
			Hash: entry.Hash, Retired: retired, IssuedAt: entry.IssuedAt, LastUsedAt: entry.LastUsedAt,
		})
	}
	return converted, nil
}
