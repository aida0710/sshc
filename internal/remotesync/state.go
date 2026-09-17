package remotesync

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"path/filepath"

	"sshc/internal/storage"
)

// StatePath は、このマシンが最後に何を同期したかを記録する場所。ワークスペース
// ルートからの相対である。他のすべてのファイルと同じくトランザクションマネージャを
// 通して書かれるので、同期の記録が書きかけで残ることはない。
const StatePath = "sshc/sync-state.json"

const stateSchemaVersion = 1

// state は、最後に成功した同期についての、このマシンの記録。
type state struct {
	SchemaVersion int `json:"schemaVersion"`
	// ETag は、このマシンが最後に push または pull したスナップショットを識別する。
	// 次の条件付き書き込みが比較される世代である。
	ETag string `json:"etag"`
	// Key は、その ETag が属するオブジェクト。世代はひとつのオブジェクトについての
	// 事実なので、設定を別のオブジェクトへ向けること、新しいパスや、オブジェクトに
	// 正直な名前を与えた改名、は、保存された世代を無意味にする。これがないと、次の
	// push は存在しないオブジェクトの世代を要求し、「別のマシンが push した」として
	// 拒否され、そこで勧められた pull は、pull すべきものを何ひとつ見つけられな
	// かった。
	Key string `json:"key"`
	// Target は Endpoint、Bucket、Region、Key から作る同期先の識別子。同じ Key を
	// 使う別バケットへ設定を変えたとき、以前の ETag と Base を流用しないために持つ。
	// 資格情報と direction は同期先そのものではないため含めない。
	Target string `json:"target,omitempty"`
	// Base は、そのスナップショットのマニフェスト。あとの pull に「別のマシンで削除
	// された」と「前回の同期以降ここで作られた」の違いを教えるのが、これで
	// ある。
	Base *Manifest `json:"base"`
	// Origin は、このインストールの不透明な ID。一度だけ生成され、マシンに関する何から
	// も導出されない。
	Origin        string         `json:"origin"`
	LastOperation *SyncOperation `json:"lastOperation"`
}

func (s *Service) statePath() string {
	return filepath.Join(s.workspace.Root(), filepath.FromSlash(StatePath))
}

func (s *Service) readState() (state, error) {
	body, err := s.workspace.FileSystem().ReadFile(s.statePath())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return state{}, nil
		}
		return state{}, err
	}
	var parsed state
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&parsed); err != nil || parsed.SchemaVersion != stateSchemaVersion ||
		parsed.ETag == "" || parsed.Key == "" || parsed.Base == nil || parsed.LastOperation == nil {
		// 壊れた state ファイルは回復可能である。次の pull はこのマシンを、一度も
		// 同期していないマシンとして扱う。それは保守的な扱いだ、何も削除せず、
		// 推測する代わりに衝突として報告する。
		return state{}, nil
	}
	migratedBase, err := migrateSnapshotManifest(*parsed.Base)
	if err != nil {
		return state{}, nil
	}
	parsed.Base = &migratedBase
	return parsed, nil
}

func (s *Service) writeState(next state) error {
	change, err := s.stateChange(next)
	if err != nil {
		return err
	}
	_, err = s.transactions.Commit(storage.Request{
		Operation:   "sync.state",
		Directories: changeDirectories(s.workspace.Root(), []storage.Change{change}),
		Changes:     []storage.Change{change},
	})
	return err
}

func (s *Service) stateChange(next state) (storage.Change, error) {
	next.SchemaVersion = stateSchemaVersion
	body, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return storage.Change{}, err
	}
	body = append(body, '\n')
	precondition := storage.Precondition{}
	if current, err := s.workspace.FileSystem().ReadFile(s.statePath()); err == nil {
		precondition = storage.Precondition{Exists: true, Digest: storage.Digest(current)}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return storage.Change{}, err
	}
	return storage.Change{
		Path:         s.statePath(),
		Contents:     body,
		Precondition: precondition,
		// state は秘密を何も指定しないが、このアプリケーション自身のファイルで
		// あり、同期のたびにその世代が増えるのは、バックアップディレクトリの中の
		// 雑音でしかない。
		SkipBackup: true,
	}, nil
}
