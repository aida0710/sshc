// Package vpnprofile は、VPN プロファイルの作成・更新・改名・削除の手順を持つ。
//
// プロファイルは、metadata.json の設定と Vault の秘密と、動いている経路の3つに
// またがる。どれか1つだけが変わると、残りが古い名前や古い値を指したまま残る。
// 手順をここに1か所だけ置き、HTTP の handler は入力の検査と応答への変換だけにする。
package vpnprofile

import (
	"context"
	"errors"

	"sshc/internal/application"
	"sshc/internal/secret"
	"sshc/internal/storage"
	"sshc/internal/vpn"
)

// Configuration は、metadata.json の VPN プロファイルを読み、変更を計画して書く。
// *application.Service が満たす。
type Configuration interface {
	VPNProfile(name string) (vpn.Profile, error)
	PlanVPNProfileCreate(profile application.VPNProfile) (application.VPNProfileChange, error)
	PlanVPNProfileUpdate(profile application.VPNProfile) (application.VPNProfileChange, error)
	PlanVPNProfileRename(from, to string) (application.VPNProfileChange, error)
	PlanVPNProfileRemove(name string) (application.VPNProfileChange, error)
	CommitVPNProfileChange(change application.VPNProfileChange, vaultChange *storage.Change) (application.SaveResult, error)
}

// Vault は、プロファイルの秘密を持つ。*secret.Service が満たす。
type Vault interface {
	Exists() (bool, error)
	Unlocked() bool
	VPNSecrets(profile string) (string, error)
	WithVPNSecretsTransaction(
		mutation secret.VPNSecretsMutation,
		commit func(vaultChange *storage.Change) (storage.Result, error),
	) (storage.Result, error)
}

// Routes は、動いている経路を止める。*vpn.Manager が満たす。
type Routes interface {
	Stop(ctx context.Context, name string) error
}

// Dependencies は、Service が使う3つの持ち主である。
type Dependencies struct {
	Configuration Configuration
	Vault         Vault
	Routes        Routes
}

// Service は、プロファイルの手順を持つ。状態を持たないので、同じ持ち主から
// 複数作ってよい。書き込みの整合は、metadata の前提（precondition）と Vault の
// 書き手の直列化が守る。
type Service struct {
	configuration Configuration
	vault         Vault
	routes        Routes
}

// New は、Service を組む。
func New(dependencies Dependencies) *Service {
	return &Service{
		configuration: dependencies.Configuration,
		vault:         dependencies.Vault,
		routes:        dependencies.Routes,
	}
}

// Create は、新しいプロファイルを、秘密と一緒に1回の書き込みで作る。
//
// 同じ名前の秘密が Vault に残っていても、必ず送られたもので置き換える。作り直した
// プロファイルに、前のプロファイルの秘密を黙って引き継がせない。
func (s *Service) Create(profile application.VPNProfile, secrets vpn.SecretsDocument) error {
	change, err := s.configuration.PlanVPNProfileCreate(profile)
	if err != nil {
		return err
	}
	return s.writeSecrets(change, secretsToWrite{profile: profile, secrets: secrets.Secrets()})
}

// Update は、保存済みのプロファイルを置き換える。
//
// 秘密は、送られた項目だけを保存済みのものに重ねる。空の項目は保存済みの値を残す。
// 設定だけを直すときに、秘密を入れ直させないためである。方式を変えた場合は、
// 前の方式の秘密を捨てる。sent が nil なら秘密は送られていない。
func (s *Service) Update(profile application.VPNProfile, sent *vpn.SecretsDocument) error {
	change, err := s.configuration.PlanVPNProfileUpdate(profile)
	if err != nil {
		return err
	}
	stored, err := s.storedSecrets(profile.Name)
	if err != nil {
		return err
	}
	merged := stored.Document()
	if sent != nil {
		merged = overlaySecrets(merged, *sent)
	}
	return s.writeSecrets(change, secretsToWrite{profile: profile, secrets: merged.Secrets()})
}

// Rename は、プロファイルの名前を変える。設定・秘密・接続の紐付けを1回で書く。
//
// 動いている経路はコンテナの名前も変わるので、書いたあとで止める。名前の形式、
// 重複、Vault のロックの解除を確かめて断る場合は、使っている経路を落とさない。
func (s *Service) Rename(ctx context.Context, from, to string) error {
	if from == to {
		// 名前が変わらなくても、無い名前の改名は成功にしない。
		_, err := s.configuration.VPNProfile(from)
		return err
	}
	change, err := s.configuration.PlanVPNProfileRename(from, to)
	if err != nil {
		return err
	}
	if err := s.requireUnlockedVault(); err != nil {
		return err
	}
	if err := s.commitWithVault(change, secret.VPNSecretsMutation{
		Kind: secret.VPNSecretsRename, Profile: from, NewName: to,
	}); err != nil {
		return err
	}
	s.stopRoute(ctx, from)
	return nil
}

// Remove は、プロファイルと、それを指している接続の紐付けと、秘密を1回で消す。
//
// Vault がロック中なら断る。設定だけを消して秘密を残すと、同じ名前で作り直した
// 経路がそれを黙って引き継ぎうる。
func (s *Service) Remove(ctx context.Context, name string) error {
	change, err := s.configuration.PlanVPNProfileRemove(name)
	if err != nil {
		return err
	}
	if err := s.requireUnlockedVault(); err != nil {
		return err
	}
	if err := s.commitWithVault(change, secret.VPNSecretsMutation{Kind: secret.VPNSecretsRemove, Profile: name}); err != nil {
		return err
	}
	s.stopRoute(ctx, name)
	return nil
}

// Route は、保存済みの設定と秘密を、経路ひとつぶんとして集める。
func (s *Service) Route(name string) (vpn.Profile, vpn.Secrets, error) {
	profile, err := s.configuration.VPNProfile(name)
	if err != nil {
		return vpn.Profile{}, vpn.Secrets{}, err
	}
	stored, err := s.vault.VPNSecrets(name)
	if err != nil {
		return vpn.Profile{}, vpn.Secrets{}, err
	}
	secrets, err := vpn.DecodeSecrets(stored)
	if err != nil {
		return vpn.Profile{}, vpn.Secrets{}, err
	}
	return profile, secrets, nil
}

// LogRedactions は、ログから伏せるための秘密を返す。プロファイルが無ければ断る。
//
// 秘密は伏せるためだけに読む。Vault がロック中でも、秘密が壊れていても、ログは
// 返せるように空の秘密を返す。伏せる相手が分からないときは、伏せられる範囲で伏せる。
func (s *Service) LogRedactions(name string) (vpn.Secrets, error) {
	if _, err := s.configuration.VPNProfile(name); err != nil {
		return vpn.Secrets{}, err
	}
	var secrets vpn.Secrets
	if stored, err := s.vault.VPNSecrets(name); err == nil {
		secrets, _ = vpn.DecodeSecrets(stored)
	}
	return secrets, nil
}

// secretsToWrite は、書き込む前に確かめる設定と秘密の組である。
type secretsToWrite struct {
	profile application.VPNProfile
	secrets vpn.Secrets
}

// writeSecrets は、方式に合う秘密だけを残して確かめ、metadata の変更と一緒に書く。
func (s *Service) writeSecrets(change application.VPNProfileChange, written secretsToWrite) error {
	profile, err := written.profile.Normalized().Profile()
	if err != nil {
		return err
	}
	own := profile.OwnSecrets(written.secrets)
	if err := profile.ValidateSecrets(own); err != nil {
		return err
	}
	document, err := vpn.EncodeSecrets(own)
	if err != nil {
		return err
	}
	return s.commitWithVault(change, secret.VPNSecretsMutation{
		Kind: secret.VPNSecretsSet, Profile: profile.Name, Document: document,
	})
}

// commitWithVault は、metadata の変更と Vault の変更を、ひとつの storage.Request で書く。
func (s *Service) commitWithVault(change application.VPNProfileChange, mutation secret.VPNSecretsMutation) error {
	_, err := s.vault.WithVPNSecretsTransaction(mutation, func(vaultChange *storage.Change) (storage.Result, error) {
		saved, err := s.configuration.CommitVPNProfileChange(change, vaultChange)
		return storage.Result{ID: saved.TransactionID, Written: saved.Written}, err
	})
	return err
}

// storedSecrets は、保存済みの秘密を返す。まだ無ければ空である。
func (s *Service) storedSecrets(name string) (vpn.Secrets, error) {
	stored, err := s.vault.VPNSecrets(name)
	if errors.Is(err, secret.ErrUnknownCredential) {
		return vpn.Secrets{}, nil
	}
	if err != nil {
		return vpn.Secrets{}, err
	}
	return vpn.DecodeSecrets(stored)
}

// requireUnlockedVault は、Vault がロック中なら ErrLocked を返す。
//
// Vault がまだ作られていなければ、動かす秘密も無いので通す。
func (s *Service) requireUnlockedVault() error {
	if s.vault.Unlocked() {
		return nil
	}
	exists, err := s.vault.Exists()
	if err != nil {
		return err
	}
	if exists {
		return secret.ErrLocked
	}
	return nil
}

// stopRoute は、設定を書き終えたあとで、古い名前の経路を止める。
//
// 止めるのは書いたあとである。先に止めると、止めてから書くまでのあいだに、
// Terminal の再接続が古い設定で経路を起こし直す。書いたあとなら、再接続は
// 「そのプロファイルは無い」で断られる。
//
// 止められなくても失敗にしない。設定はもう書いてある。Docker が止まっていれば
// コンテナも止まっており、残った経路は、無操作の停止か、次の engine の起動で片付く。
func (s *Service) stopRoute(ctx context.Context, name string) {
	_ = s.routes.Stop(ctx, name)
}
