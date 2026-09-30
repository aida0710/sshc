package remotesync

import (
	"path/filepath"
	"strings"

	"sshc/internal/objectstore"
)

// ConfigureIfUnconfigured restores a persisted binding without overwriting a
// binding explicitly configured while the persisted settings were being read.
// The check and publication share operationMutex with CompleteSetup.
func (s *Service) ConfigureIfUnconfigured(config Config, credentials objectstore.Credentials, client *objectstore.Client) (bool, error) {
	s.operationMutex.Lock()
	defer s.operationMutex.Unlock()
	if s.Configured() {
		return false, nil
	}
	config = normalizeConfig(config)
	if err := s.validateRecoveryTarget(config); err != nil {
		return false, err
	}
	s.configure(config, credentials, client)
	return true, nil
}

// configure applies one complete remote binding while operationMutex is held.
// Configure must be wholly before or wholly after every stateful operation so
// a successful settings response never leaves an older operation running.
func (s *Service) configure(config Config, credentials objectstore.Credentials, client *objectstore.Client) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	// endpoint の末尾のスラッシュはリクエストには影響しない（クライアントがパス全体を
	// 置き換える）が、スナップショットの行き先を表示する画面では "https://host//bucket"
	// になる。設定を保存する場所だけでなくここでも切り詰めるので、どこから来た設定も
	// 同じ形で保持する。
	config = normalizeConfig(config)
	s.binding = remoteBinding{config: config, credentials: credentials, client: client}
	s.bindingVersion++
}

// Forget は、同期先の接続一式（アクセスキーとシークレットを含む）を手放す。
//
// 同期の設定は Vault の中にあり、Vault が閉じているあいだは読めない。Vault が
// ロックされたらここを呼び、その平文をメモリに残さない。ロックを解除したあとは、自動同期の
// 準備と同期の画面が、未設定と見て Vault から組み直す。
//
// operationMutex は待たない。走っている操作は開始時に接続一式を写し取っており、世代を
// 進めれば、古い世代で読んだ結果を返す操作は止まる。リモートは変わっていないので、
// 止まった操作は ErrRemoteMoved ではなく ErrNotConfigured を返す（bindingChangedLocked）。
func (s *Service) Forget() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.binding = remoteBinding{}
	s.bindingVersion++
}

func normalizeConfig(config Config) Config {
	config.Endpoint = strings.TrimRight(config.Endpoint, "/")
	config.Path = strings.Trim(config.Path, "/")
	return config
}

func (s *Service) validateRecoveryTarget(config Config) error {
	journal, exists, err := s.readKeyRecovery()
	if err != nil {
		return err
	}
	if exists && (journal.Target != targetID(config) || journal.ObjectKey != ObjectKeyFor(config)) {
		return ErrRecoveryTargetChange
	}
	return nil
}

// configuredBinding は、同期処理の開始時点の接続一式をひとつの値として返す。
// Configure はこのロックの前か後のどちらかにしか現れず、一回の操作の途中で
// config と client の世代が混ざることはない。
func (s *Service) configuredBinding() (remoteBinding, error) {
	binding, _, err := s.configuredBindingVersion()
	return binding, err
}

func (s *Service) configuredBindingVersion() (remoteBinding, uint64, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if !s.configuredLocked() {
		return remoteBinding{}, 0, ErrNotConfigured
	}
	return s.binding, s.bindingVersion, nil
}

// Configured は、バケットと資格情報が設定されているかを報告する。
func (s *Service) Configured() bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.configuredLocked()
}

// bindingChangedLocked は、captured の世代で読んだ結果を返してよいかを確かめる。s.mutex を
// 持って呼ぶ。Forget で接続一式を手放したあとなら ErrNotConfigured、設定が変わった
// あとなら ErrRemoteMoved を返す。
func (s *Service) bindingChangedLocked(captured uint64) error {
	if !s.configuredLocked() {
		return ErrNotConfigured
	}
	if s.bindingVersion != captured {
		return ErrRemoteMoved
	}
	return nil
}

func (s *Service) configuredLocked() bool {
	_, validDirection := ParseDirection(string(s.binding.config.Direction))
	return validDirection && s.binding.client != nil && s.binding.config.Bucket != "" && s.binding.credentials.AccessKeyID != ""
}

// Direction は、このマシンがどちら向きにデータを動かしてよいかを報告する。
func (s *Service) Direction() Direction {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	// 未設定のserviceには保存済み契約がない。status formの初期選択だけは通常の双方向を返す。
	if s.binding.config.Direction == "" {
		return DirectionBoth
	}
	return s.binding.config.Direction
}

// Target は、この実行が指しているエンドポイントとバケットを、表示のために返す。
// アクセスキーと秘密が何かによって返されることは決してない。
func (s *Service) Target() (endpoint, bucket, path, region string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.binding.config.Endpoint, s.binding.config.Bucket, s.binding.config.Path, s.binding.config.Region
}

// AccessKeySuffix returns only the final five characters needed to identify
// the configured account in setup and status output. The complete identifier
// and secret access key never leave the engine.
func (s *Service) AccessKeySuffix() string {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	characters := []rune(s.binding.credentials.AccessKeyID)
	if len(characters) > 5 {
		characters = characters[len(characters)-5:]
	}
	return string(characters)
}

// SyncState returns a detached view so callers cannot mutate the state value
// retained by a later response.
func (s *Service) SyncState() SyncStateView {
	current, err := s.readState()
	if err != nil || current.ETag == "" || current.Base == nil {
		return SyncStateView{}
	}
	binding, err := s.configuredBinding()
	if err != nil || !stateMatchesTarget(current, binding.config) {
		return SyncStateView{}
	}
	view := SyncStateView{
		Synced: true, At: current.Base.CreatedAt, Origin: current.Base.Origin,
		Files: len(current.Base.Files),
	}
	if current.LastOperation != nil {
		operation := *current.LastOperation
		view.LastOperation = &operation
	}
	return view
}

// DisplayPath は、ワークスペースの絶対パスを、このアプリケーションの他の部分が
// 表示する相対パスへ戻す。
func (s *Service) DisplayPath(absolute string) string {
	relative, err := filepath.Rel(s.workspace.Root(), absolute)
	if err != nil {
		return filepath.Base(absolute)
	}
	return filepath.ToSlash(relative)
}
