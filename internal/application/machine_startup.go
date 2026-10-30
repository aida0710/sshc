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

// migrateMetadataFile は、schema 9 の metadata.json を schema 10 で書き直す。schema 10 なら何もしない。
func (s *Service) migrateMetadataFile() error {
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
	_, err = s.commitMetadata(migrated, preconditionFor(contents, exists), "metadata.schema10")
	return err
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
		return err
	}
	request := storage.Request{Operation: "machine.settings.create"}
	if legacyExists {
		settings.Port = legacyEnginePort(legacy)
		request.Removals = append(request.Removals, storage.Removal{Path: legacyPath, Precondition: preconditionFor(legacy, legacyExists), Backup: true})
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
