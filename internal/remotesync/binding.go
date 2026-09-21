package remotesync

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"sshc/internal/objectstore"
)

// ConfigureIfUnconfigured restores a persisted binding without overwriting a
// binding explicitly configured while the persisted settings were being read.
// The check and publication share operationMu with Reconfigure.
func (s *Service) ConfigureIfUnconfigured(config Config, credentials objectstore.Credentials, client *objectstore.Client) (bool, error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	if s.configured() {
		return false, nil
	}
	config = normalizeConfig(config)
	if err := s.validateRecoveryTarget(config); err != nil {
		return false, err
	}
	s.configure(config, credentials, client)
	return true, nil
}

// Reconfigure persists credentials and swaps the in-memory binding inside the
// same operation boundary. No key rotation can observe new secret settings with
// the previous remote client, or the reverse.
func (s *Service) Reconfigure(config Config, credentials objectstore.Credentials, client *objectstore.Client, persist func() error) error {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	config = normalizeConfig(config)
	if persist == nil {
		return errors.New("remote sync settings persistence is not configured")
	}
	if err := s.validateRecoveryTarget(config); err != nil {
		return err
	}
	if err := persist(); err != nil {
		return err
	}
	s.configure(config, credentials, client)
	return nil
}

// configure applies one complete remote binding while operationMu is held.
// Configure must be wholly before or wholly after every stateful operation so
// a successful settings response never leaves an older operation running.
func (s *Service) configure(config Config, credentials objectstore.Credentials, client *objectstore.Client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// 末尾のスラッシュがリクエストへ届いたことはない、クライアントはパス全体を
	// 置き換えるが、スナップショットの行き先を表示するすべての画面には
	// "https://host//bucket" として届いていた。設定を保存する場所だけでなくここで
	// 切り詰めることで、これができる前に保存されたものもきれいになる。
	config = normalizeConfig(config)
	s.binding = remoteBinding{config: config, creds: credentials, client: client}
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
	s.mu.Lock()
	defer s.mu.Unlock()
	binding := s.binding
	if _, ok := ParseDirection(string(binding.config.Direction)); !ok || binding.client == nil ||
		binding.config.Bucket == "" || binding.creds.AccessKeyID == "" {
		return remoteBinding{}, 0, ErrNotConfigured
	}
	return binding, s.bindingVersion, nil
}

// Configured は、バケットと資格情報が設定されているかを報告する。
func (s *Service) Configured() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.configuredLocked()
}

func (s *Service) configured() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.configuredLocked()
}

func (s *Service) configuredLocked() bool {
	_, validDirection := ParseDirection(string(s.binding.config.Direction))
	return validDirection && s.binding.client != nil && s.binding.config.Bucket != "" && s.binding.creds.AccessKeyID != ""
}

// Direction は、このマシンがどちら向きにデータを動かしてよいかを報告する。
func (s *Service) Direction() Direction {
	s.mu.Lock()
	defer s.mu.Unlock()
	// 未設定のserviceには保存済み契約がない。status formの初期選択だけは通常の双方向を返す。
	if s.binding.config.Direction == "" {
		return DirectionBoth
	}
	return s.binding.config.Direction
}

// Check は、このサービスが保持していないクライアントに対して同じ問いを投げる。
// 設定を保存する前に試せるようにするためだ。試されていない設定を登録することが、
// 打ち間違いを「正しく見える設定」に変えてしまう。
func Check(ctx context.Context, client *objectstore.Client, key string) error {
	if _, err := client.Head(ctx, key); err != nil && !errors.Is(err, objectstore.ErrNotFound) {
		return err
	}
	return nil
}

// Target は、この実行が指しているエンドポイントとバケットを、表示のために返す。
// アクセスキーと秘密が何かによって返されることは決してない。
func (s *Service) Target() (endpoint, bucket, path, region string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.binding.config.Endpoint, s.binding.config.Bucket, s.binding.config.Path, s.binding.config.Region
}

// AccessKeySuffix returns only the final five characters needed to identify
// the configured account in setup and status output. The complete identifier
// and secret access key never leave the engine.
func (s *Service) AccessKeySuffix() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	characters := []rune(s.binding.creds.AccessKeyID)
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
