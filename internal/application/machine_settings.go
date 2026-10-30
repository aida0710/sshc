package application

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"sshc/internal/handoff"
	"sshc/internal/storage"
	"sshc/internal/strictjson"
)

// マシンごとの設定（名前・使うプリセット・ポート）。同期するが、そのマシンにだけ効く。
//
// 1台に1ファイル（machines/<ID>.json）にして metadata.json に入れないのは、同期がファイル単位の
// 三方向で比べるからである。どのマシンも書く metadata.json に入れると、2台がそれぞれ自分の
// 設定を変えただけで衝突になる。どのマシンからも変えられるが、変えるのはまれである。
// 頻繁に変わる今の状態は別のファイル（machine_state.go）にある。
const (
	MachinesDirectoryName        = "machines"
	machineSettingsSchemaVersion = 1
	// MaxMachineNameLength は、マシンの名前の長さの上限である。api/openapi.yaml も同じ値を約束する。
	MaxMachineNameLength = 80
)

var (
	ErrMachineSettings = errors.New("machine settings are invalid")
	// ErrEnginePort は ErrMachineSettings の種類。画面に、ポートの入力を直せばよいと返すために分ける。
	ErrEnginePort = fmt.Errorf("%w: port out of range", ErrMachineSettings)
	// ErrUnknownMachine は、無いマシンを指したことを表す。
	ErrUnknownMachine = errors.New("unknown machine")
	// ErrThisMachine は、このマシン自身を一覧から外そうとしたことを表す。
	ErrThisMachine = errors.New("this machine cannot be removed from the list")
)

// MachineSettings は、1台のマシンの設定である。
type MachineSettings struct {
	// Name は一覧に出す名前である。初めは OS が答えるホスト名。
	Name string `json:"name,omitempty"`
	// PresetID は、このマシンが使うプリセットの ID である。空か、消えたプリセットなら標準のプリセット。
	PresetID string `json:"presetId,omitempty"`
	// Port は、sshc エンジンが待ち受けるポートである。0 はマシンごとの固定ポート。次の起動から効く。
	Port int `json:"port,omitempty"`
}

// Machine は、一覧に出す1台である。Facts は、そのマシンのエンジンが最後に起動したときの記録。
type Machine struct {
	ID       string          `json:"id"`
	Settings MachineSettings `json:"settings"`
	Facts    MachineFacts    `json:"facts"`
}

type machineSettingsDocument struct {
	SchemaVersion int `json:"schemaVersion"`
	MachineSettings
}

func validateMachineSettings(settings MachineSettings) error {
	if utf8.RuneCountInString(settings.Name) > MaxMachineNameLength || strings.ContainsAny(settings.Name, "\r\n\x00") {
		return fmt.Errorf("%w: name", ErrMachineSettings)
	}
	if settings.PresetID != "" && !presetIDPattern.MatchString(settings.PresetID) {
		return fmt.Errorf("%w: presetId", ErrMachineSettings)
	}
	// ポートの範囲は、ブラウザの登録も使うので handoff にある。
	if settings.Port != 0 && handoff.EnginePort(settings.Port) != nil {
		return fmt.Errorf("%w %d", ErrEnginePort, settings.Port)
	}
	return nil
}

func decodeMachineSettings(contents []byte) (MachineSettings, error) {
	var document machineSettingsDocument
	if err := strictjson.Decode(contents, &document); err != nil {
		return MachineSettings{}, fmt.Errorf("%w: %w", ErrMachineSettings, err)
	}
	if document.SchemaVersion != machineSettingsSchemaVersion {
		return MachineSettings{}, fmt.Errorf("%w: schema version %d", ErrMachineSettings, document.SchemaVersion)
	}
	if err := validateMachineSettings(document.MachineSettings); err != nil {
		return MachineSettings{}, err
	}
	return document.MachineSettings, nil
}

func encodeMachineSettings(settings MachineSettings) ([]byte, error) {
	if err := validateMachineSettings(settings); err != nil {
		return nil, err
	}
	encoded, err := json.MarshalIndent(machineSettingsDocument{
		SchemaVersion: machineSettingsSchemaVersion, MachineSettings: settings,
	}, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

func (s *Service) machinesDirectory() string {
	return filepath.Join(s.workspace.StateDir(), MachinesDirectoryName)
}

func (s *Service) machineSettingsPath(id string) string {
	return filepath.Join(s.machinesDirectory(), id+".json")
}

// machineSettings は、ID のマシンの設定と、あとの commit が要る事前条件を読む。無いファイルは
// 何も設定していないものとして読む。
func (s *Service) machineSettings(id string) (MachineSettings, storage.Precondition, error) {
	contents, exists, err := s.readFile(s.machineSettingsPath(id))
	if err != nil {
		return MachineSettings{}, storage.Precondition{}, err
	}
	if !exists {
		return MachineSettings{}, storage.Precondition{}, nil
	}
	settings, err := decodeMachineSettings(contents)
	if err != nil {
		return MachineSettings{}, storage.Precondition{}, err
	}
	return settings, preconditionFor(contents, exists), nil
}

// ThisMachineSettings は、このマシンの設定を返す。読めなければ、何も設定していないものとして扱う。
func (s *Service) ThisMachineSettings() MachineSettings {
	if s.machineID == "" {
		return MachineSettings{}
	}
	settings, _, err := s.machineSettings(s.machineID)
	if err != nil {
		return MachineSettings{}
	}
	return settings
}

// Machines は、同期でつながっているマシンの一覧を、このマシンを先頭に、名前の順で返す。
// 読めないファイルのマシンは並べない。
func (s *Service) Machines() ([]Machine, error) {
	entries, err := s.workspace.FileSystem().ReadDir(s.machinesDirectory())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var machines []Machine
	for _, entry := range entries {
		id, isSettings := strings.CutSuffix(entry.Name(), ".json")
		if !isSettings || entry.IsDir() || !machineIDPattern.MatchString(id) {
			continue
		}
		settings, _, err := s.machineSettings(id)
		if err != nil {
			continue
		}
		machines = append(machines, Machine{ID: id, Settings: settings, Facts: s.machineState(id).Facts})
	}
	sort.SliceStable(machines, func(first, second int) bool {
		if (machines[first].ID == s.machineID) != (machines[second].ID == s.machineID) {
			return machines[first].ID == s.machineID
		}
		return machines[first].Settings.Name < machines[second].Settings.Name
	})
	return machines, nil
}

// SetMachineSettings は、ID のマシンの設定を置き換える。どのマシンの設定も変えられる。base は
// 画面が読んだときの値で、そのあとにほかの画面やマシンが変えていたら断る。
func (s *Service) SetMachineSettings(id string, base, settings MachineSettings) (SaveResult, error) {
	if !machineIDPattern.MatchString(id) {
		return SaveResult{}, ErrUnknownMachine
	}
	contents, err := encodeMachineSettings(settings)
	if err != nil {
		return SaveResult{}, err
	}
	s.saveMutex.Lock()
	defer s.saveMutex.Unlock()
	stored, precondition, err := s.machineSettings(id)
	if err != nil {
		return SaveResult{}, err
	}
	if !precondition.Exists {
		return SaveResult{}, ErrUnknownMachine
	}
	if stored != base {
		return SaveResult{}, ErrSettingsChanged
	}
	result, err := s.manager.Commit(storage.Request{
		Operation: "machine.settings",
		Changes:   []storage.Change{{Path: s.machineSettingsPath(id), Contents: contents, Precondition: precondition}},
	})
	if err != nil {
		return SaveResult{}, err
	}
	return SaveResult{TransactionID: result.ID, Written: result.Written}, nil
}

// RemoveMachine は、使わなくなったマシンを一覧から外す。そのマシンのファイル（設定と今の状態）を
// 消す。このマシン自身は外せない。外したマシンのエンジンが後で起動すると、また一覧に並ぶ。
func (s *Service) RemoveMachine(id string) error {
	if id == s.machineID {
		return ErrThisMachine
	}
	if !machineIDPattern.MatchString(id) {
		return ErrUnknownMachine
	}
	s.saveMutex.Lock()
	defer s.saveMutex.Unlock()
	var removals []storage.Removal
	for _, path := range []string{s.machineSettingsPath(id), s.machineStatePath(id)} {
		contents, exists, err := s.readFile(path)
		if err != nil {
			return err
		}
		if exists {
			removals = append(removals, storage.Removal{Path: path, Precondition: preconditionFor(contents, exists), Backup: true})
		}
	}
	if len(removals) == 0 {
		return ErrUnknownMachine
	}
	_, err := s.manager.Commit(storage.Request{Operation: "machine.remove", Removals: removals})
	return err
}
