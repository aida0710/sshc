package application

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"

	"sshc/internal/storage"
)

// MachineStart は、エンジンの起動のたびに PrepareMachine へ渡す、このマシンの事実である。
type MachineStart struct {
	// Hostname は、OS が答えるホスト名である。初めて起動したときのマシンの名前に使う。
	Hostname string
	Facts    MachineFacts
}

// PrepareMachine は、エンジンを起動するたびに、HTTP を受け付ける前に呼ぶ。このマシンの ID を
// 決め（無ければ作る）、古い形のファイルを一度だけ移し、起動したことを今の状態に書く。
//
// マスターパスワードのある Vault は、この時点ではまだロックされている。世代バックアップは Vault の
// 鍵で封じるので、ここで書く変更は控えを取らない。取ろうとすると封じられずに止まり、エンジンが
// 起動しない。
func (s *Service) PrepareMachine(random io.Reader, start MachineStart) error {
	id, err := s.ensureMachineID(random)
	if err != nil {
		return fmt.Errorf("machine id: %w", err)
	}
	s.machineID = id
	if err := s.migrateMetadataFile(); err != nil {
		return fmt.Errorf("move metadata.json to schema %d: %w", MetadataSchemaVersion, err)
	}
	if err := s.ensureMachineSettings(start.Hostname, start.Facts.OS); err != nil {
		return fmt.Errorf("machine settings: %w", err)
	}
	if err := s.recordMachineStart(start.Facts); err != nil {
		return fmt.Errorf("machine state: %w", err)
	}
	return nil
}

// metadataSchema10Operation は、schema 9 の metadata.json を schema 10 で書き直す変更の名前である。
const metadataSchema10Operation = "metadata.schema10"

// migrateMetadataFile は、schema 9 の metadata.json を schema 10 で書き直す。schema 10 なら何もしない。
func (s *Service) migrateMetadataFile() error {
	if err := s.rollBackUnstartedMetadataMigration(); err != nil {
		return err
	}
	path := s.metadata.Path()
	contents, exists, err := s.readFile(path)
	if err != nil || !exists {
		return err
	}
	migrated, err := migrateMetadataFromSchema9IfOld(contents)
	if err != nil {
		// schema 10 のファイルは移さない。読めないファイルとほかの schema のファイルは、今までどおり
		// 読むときに断る。
		return nil
	}
	change, err := s.metadata.Change(migrated, preconditionFor(contents, exists))
	if err != nil {
		return err
	}
	// schema 9 の控えは、取っても戻せない（このバージョンの読み込みは schema 9 を断る）。
	change.SkipBackup = true
	_, err = s.manager.Commit(storage.Request{Operation: metadataSchema10Operation, Changes: []storage.Change{change}})
	return err
}

// rollBackUnstartedMetadataMigration は、この書き直しの記録のうち、まだどのファイルも置き換えて
// いないものを巻き戻す。控えを封じられずに止まった書き直しがこれを残し、残っている間はどの変更も
// 書けない。何も置き換えていないので、巻き戻しは記録を片付けるだけで、控えも Vault も要らない。
func (s *Service) rollBackUnstartedMetadataMigration() error {
	pending, err := s.manager.Pending()
	if err != nil {
		return err
	}
	for _, record := range pending {
		if record.Operation != metadataSchema10Operation || record.Committed != 0 || !record.CanRollback {
			continue
		}
		if err := s.manager.Rollback(record.ID); err != nil {
			return err
		}
	}
	return nil
}

// ensureMachineSettings は、このマシンの設定のファイルが無ければ作る。名前はホスト名で、ポートは
// 前のバージョンの engine-settings.json から移す。移したら engine-settings.json は消す。そこに
// あった Vault の自動ロックは移さない（プリセットの設定になったので、設定し直してもらう）。
func (s *Service) ensureMachineSettings(hostname, operatingSystem string) error {
	_, precondition, err := s.machineSettings(s.machineID)
	if err != nil || precondition.Exists {
		return err
	}
	name := hostname
	if name == "" || name == "localhost" {
		name = operatingSystem
	}
	if len([]rune(name)) > MaxMachineNameLength {
		name = string([]rune(name)[:MaxMachineNameLength])
	}
	settings := MachineSettings{Name: name}
	legacyPath := filepath.Join(s.workspace.StateDir(), legacyEngineSettingsFileName)
	legacy, legacyExists, err := s.readFile(legacyPath)
	if err != nil {
		// v0.58 までと同じく、読めないファイルは何も設定していないものとして扱う。持ち主の違う
		// ファイル（管理者として動かした sshc が作ったものなど）で、エンジンが起動しなくならないように
		// するため。中身を確かめられないので、消さずに残す（マシンの設定ができたあとは読まない）。
		legacyExists = false
	}
	request := storage.Request{Operation: "machine.settings.create"}
	if legacyExists {
		settings.Port = legacyEnginePort(legacy)
		// 控えは取らない（PrepareMachine の説明）。中身のうち要るのはポートだけで、それはここで移す。
		request.Removals = append(request.Removals, storage.Removal{Path: legacyPath, Precondition: preconditionFor(legacy, legacyExists)})
	}
	contents, err := encodeMachineSettings(settings)
	if err != nil {
		return err
	}
	if err := s.workspace.EnsureDirectory(s.machinesDirectory()); err != nil {
		return err
	}
	request.Changes = append(request.Changes, storage.Change{Path: s.machineSettingsPath(s.machineID), Contents: contents})
	_, err = s.manager.Commit(request)
	return err
}

// legacyEngineSettingsFileName は、schema 9 までのこのマシンだけの設定（ポートと Vault の自動
// ロック）のファイルである。移したら消す。全部のマシンが移ったら、この読み込みごと消す。
const legacyEngineSettingsFileName = "engine-settings.json"

// legacyEnginePort は、engine-settings.json のポートを返す。読めない・範囲の外なら 0（マシンごとの
// 固定ポート）とする。
func legacyEnginePort(contents []byte) int {
	var document struct {
		Port int `json:"port"`
	}
	if err := json.Unmarshal(contents, &document); err != nil {
		return 0
	}
	if validateMachineSettings(MachineSettings{Port: document.Port}) != nil {
		return 0
	}
	return document.Port
}
