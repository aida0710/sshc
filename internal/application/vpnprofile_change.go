package application

import (
	"errors"
	"fmt"

	"sshc/internal/storage"
	"sshc/internal/vpn"
)

// VPN プロファイルの作成・更新・改名・削除を、metadata の変更として計画する。
//
// 秘密は Vault にある。設定と秘密は名前でつながっているので、片方だけが書かれると
// もう片方が古い名前や古い値を指したまま残る。だから計画（検査と変更の組み立て）と
// 書き込みを分け、呼び手が Vault の変更と同じ storage.Request で書けるようにする。

// ErrVPNProfileExists は、その名前のプロファイルがすでにあることを表す。
var ErrVPNProfileExists = errors.New("a vpn profile with that name already exists")

// VPNProfileChange は、検査を通った metadata の変更ひとつぶんである。
// CommitVPNProfileChange へ渡すまで、何も書かれていない。
type VPNProfileChange struct {
	planned metadataCommit
}

// PlanVPNProfileCreate は、新しいプロファイルを加える変更を作る。同じ名前が
// あれば ErrVPNProfileExists を返す。
func (s *Service) PlanVPNProfileCreate(profile VPNProfile) (VPNProfileChange, error) {
	profile = profile.Normalized()
	if _, err := profile.Profile(); err != nil {
		return VPNProfileChange{}, err
	}
	stored, precondition, err := s.metadata.Load()
	if err != nil {
		return VPNProfileChange{}, err
	}
	if vpnProfileIndex(stored.VPNProfiles, profile.Name) >= 0 {
		return VPNProfileChange{}, fmt.Errorf("%w: %s", ErrVPNProfileExists, profile.Name)
	}
	stored.VPNProfiles = append(stored.VPNProfiles, profile)
	return VPNProfileChange{planned: metadataCommit{
		operation: "vpn.profile.create", metadata: stored, precondition: precondition,
	}}, nil
}

// PlanVPNProfileUpdate は、保存済みのプロファイルを置き換える変更を作る。名前が
// 無ければ ErrUnknownVPNProfile を返す。
func (s *Service) PlanVPNProfileUpdate(profile VPNProfile) (VPNProfileChange, error) {
	profile = profile.Normalized()
	if _, err := profile.Profile(); err != nil {
		return VPNProfileChange{}, err
	}
	stored, precondition, err := s.metadata.Load()
	if err != nil {
		return VPNProfileChange{}, err
	}
	index := vpnProfileIndex(stored.VPNProfiles, profile.Name)
	if index < 0 {
		return VPNProfileChange{}, fmt.Errorf("%w: %s", ErrUnknownVPNProfile, profile.Name)
	}
	stored.VPNProfiles[index] = profile
	return VPNProfileChange{planned: metadataCommit{
		operation: "vpn.profile.update", metadata: stored, precondition: precondition,
	}}, nil
}

// PlanVPNProfileRename は、プロファイルの名前を変え、それを指している接続の
// 紐付けも追従させる変更を作る。
//
// 別々に直すと、古い名前を指したままの接続が残る。その接続は繋ぐたびに断られ、
// 利用者は設定のどこを直せばよいかを探すことになる。
func (s *Service) PlanVPNProfileRename(from, to string) (VPNProfileChange, error) {
	if err := vpn.ValidateName(to); err != nil {
		return VPNProfileChange{}, fmt.Errorf("%w: %w", ErrMetadataVPN, err)
	}
	stored, precondition, err := s.metadata.Load()
	if err != nil {
		return VPNProfileChange{}, err
	}
	index := vpnProfileIndex(stored.VPNProfiles, from)
	if index < 0 {
		return VPNProfileChange{}, fmt.Errorf("%w: %s", ErrUnknownVPNProfile, from)
	}
	if from != to && vpnProfileIndex(stored.VPNProfiles, to) >= 0 {
		return VPNProfileChange{}, fmt.Errorf("%w: %s", ErrVPNProfileExists, to)
	}
	stored.VPNProfiles[index].Name = to
	for hostIndex, host := range stored.Hosts {
		if host.VPN == from {
			stored.Hosts[hostIndex].VPN = to
		}
	}
	return VPNProfileChange{planned: metadataCommit{
		operation: "vpn.profile.rename", metadata: stored, precondition: precondition,
	}}, nil
}

// PlanVPNProfileRemove は、プロファイルと、それを指している接続の紐付けを
// 同時に消す変更を作る。
//
// 別々に消すと、消えたプロファイルを指したままの接続が残る。その接続は繋ぐ
// たびに「そのプロファイルは無い」と断られる。
func (s *Service) PlanVPNProfileRemove(name string) (VPNProfileChange, error) {
	stored, precondition, err := s.metadata.Load()
	if err != nil {
		return VPNProfileChange{}, err
	}
	index := vpnProfileIndex(stored.VPNProfiles, name)
	if index < 0 {
		return VPNProfileChange{}, fmt.Errorf("%w: %s", ErrUnknownVPNProfile, name)
	}
	stored.VPNProfiles = append(stored.VPNProfiles[:index:index], stored.VPNProfiles[index+1:]...)
	for hostIndex, host := range stored.Hosts {
		if host.VPN == name {
			stored.Hosts[hostIndex].VPN = ""
		}
	}
	return VPNProfileChange{planned: metadataCommit{
		operation: "vpn.profile.remove", metadata: stored, precondition: precondition,
	}}, nil
}

// CommitVPNProfileChange は、計画した metadata の変更と Vault の変更を、ひとつの
// storage.Request で書く。vaultChange が nil なら metadata だけを書く。
//
// 計画のあとに metadata が書き換えられていれば、読んだときの前提が崩れているので
// 何も書かずに断る。
func (s *Service) CommitVPNProfileChange(change VPNProfileChange, vaultChange *storage.Change) (SaveResult, error) {
	return s.commitMetadataWith(change.planned, vaultChange)
}

// vpnProfileIndex は、名前の一致するプロファイルの位置を返す。無ければ -1。
func vpnProfileIndex(profiles []VPNProfile, name string) int {
	for index, profile := range profiles {
		if profile.Name == name {
			return index
		}
	}
	return -1
}
