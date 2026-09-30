package application

import (
	"encoding/json"
	"errors"
)

// ErrMetadataChanged は、画面が読み込んだあとに、変えようとした節がほかの書き手
// （別のタブ、CLI、同期）で変わったことを表す。画面の写しで上書きすると、その間の
// 変更が黙って消えるので断る。
var ErrMetadataChanged = errors.New("metadata changed since it was loaded; reload before saving")

// applyHostMetadataEdit は、Path と Alias が指す接続 1 件の metadata だけを差し替える。
//
// 画面が送るのはその 1 件と、読み込んだときのその 1 件（base）だけである。ほかの
// 接続、グループ、VPN プロファイル、ターミナル設定などは、いまのディスクの値を保つ。
// HostMetadata の識別子が違えば、その entry を別の接続へ付け直す（orphan の関連付け
// 直し）。HostMetadata が nil なら entry を消す。
func applyHostMetadataEdit(stored Metadata, request EditRequest) (Metadata, error) {
	identity := HostIdentity{Path: request.Path, Alias: request.Alias}
	index := hostMetadataIndex(stored.Hosts, identity)
	var current *HostMetadata
	if index >= 0 {
		current = &stored.Hosts[index]
	}
	if !sameJSON(screenOwnedHostMetadata(current, identity), screenOwnedHostMetadata(request.HostMetadataBase, identity)) {
		return Metadata{}, ErrMetadataChanged
	}

	hosts := make([]HostMetadata, 0, len(stored.Hosts)+1)
	for _, host := range stored.Hosts {
		if host.Identity != identity {
			hosts = append(hosts, host)
		}
	}
	if request.HostMetadata != nil {
		next := *request.HostMetadata
		// 付け直す先に entry があれば、画面はそれを見ていない（見ていれば断っている）。
		if next.Identity != identity && hostMetadataIndex(hosts, next.Identity) >= 0 {
			return Metadata{}, ErrMetadataChanged
		}
		// orphan の印は保存のたびに付け直し、判定した OS は engine が書く。どちらも
		// 画面が決める値ではないので、いまの entry の値を引き継ぐ。
		next.Orphan = false
		next.DetectedOS, next.DetectedOSBinding = "", ""
		if current != nil {
			next.DetectedOS, next.DetectedOSBinding = current.DetectedOS, current.DetectedOSBinding
		}
		if !saysNothing(next) {
			hosts = append(hosts, next)
		}
	}
	stored.Hosts = hosts
	return stored, nil
}

// applyGroupsEdit は、グループの設定だけを差し替える。base は画面が読み込んだときの
// グループの設定で、いまのディスクと違えば断る。
func applyGroupsEdit(stored Metadata, request EditRequest) (Metadata, error) {
	if !sameJSON(groupsOrNil(stored.Groups), groupsOrNil(request.GroupsBase)) {
		return Metadata{}, ErrMetadataChanged
	}
	stored.Groups = request.Groups
	return stored, nil
}

// screenOwnedHostMetadata は、画面が編集する項目だけを比べられる形にする。entry が
// 無いことと、識別子だけの entry は同じ意味である（HostDetail は無い entry を識別子
// だけで返す）。
func screenOwnedHostMetadata(entry *HostMetadata, identity HostIdentity) HostMetadata {
	if entry == nil {
		return HostMetadata{Identity: identity}
	}
	owned := *entry
	owned.Orphan = false
	owned.DetectedOS, owned.DetectedOSBinding = "", ""
	return owned
}

// groupsOrNil は、空の一覧を nil にして、無い一覧と同じ値にそろえる。
func groupsOrNil(groups []GroupMetadata) []GroupMetadata {
	if len(groups) == 0 {
		return nil
	}
	return groups
}

// sameJSON は、metadata.json に書いたときに同じ中身になるかで比べる。nil と空の
// 一覧は omitempty でどちらも書かれないので、同じとみなせる。
func sameJSON(first, second any) bool {
	firstEncoded, firstErr := json.Marshal(first)
	secondEncoded, secondErr := json.Marshal(second)
	return firstErr == nil && secondErr == nil && string(firstEncoded) == string(secondEncoded)
}

// applyMetadataEdit は、kind に応じて metadata の 1 つの部分だけを差し替える。
func applyMetadataEdit(stored Metadata, request EditRequest) (Metadata, error) {
	switch request.Kind {
	case EditMetadata:
		return applyHostMetadataEdit(stored, request)
	case EditGroups:
		return applyGroupsEdit(stored, request)
	default:
		return Metadata{}, ErrUnknownEditKind
	}
}
