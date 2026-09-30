package keys

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"sshc/internal/storage"
)

// ConfirmationSubject は、ワンタイムの確認が対象とする操作の種類を表す。これは
// このパッケージ自身の用語であり、HTTP 層が session パッケージのアクション種別を
// これに対応付ける。そのためユースケース層は、セッションがどう認証されるかに依存
// しなくてよい。
type ConfirmationSubject string

const (
	ConfirmRevealKey  ConfirmationSubject = "reveal_key"
	ConfirmPurgeEntry ConfirmationSubject = "purge_entry"
)

// ErrUnknownConfirmation は、このアプリケーションがトークンを発行しない確認対象を
// 報告する。
var ErrUnknownConfirmation = errors.New("unknown confirmation subject")

// ConfirmationEvidence は、ある操作について確認ダイアログが表示するであろう内容を
// そのままダイジェストにする。トークンをそれに結び付けられるようにするためである。
//
// ダイジェストはトークンが使われるときに再計算される。その間に鍵やごみ箱のエントリ
// が変わっていればダイジェストは食い違い、確認は拒否される。ユーザーが同意したのは
// 見せられたものであって、それを置き換えた何かではないからだ。生成されるのは
// ダイジェストだけである。鍵素材も、パスも、この関数から出ていくことは
// 決してない。
func (service *Service) ConfirmationEvidence(subject ConfirmationSubject, target string) (string, error) {
	switch subject {
	case ConfirmRevealKey:
		return service.revealEvidence(target)
	case ConfirmPurgeEntry:
		return service.purgeEvidence(target)
	default:
		return "", ErrUnknownConfirmation
	}
}

func (service *Service) revealEvidence(keyID string) (string, error) {
	_, item, err := service.privateKey(keyID)
	if err != nil {
		return "", err
	}
	contents, err := service.workspace.FileSystem().ReadFile(service.absolutePath(item.RelativePath))
	if err != nil {
		return "", err
	}
	// ファイルのダイジェストを取り、バッファは直ちに消去する。evidence が
	// バイト列そのものを保持することは決してない。
	contentsDigest := storage.Digest(contents)
	clear(contents)

	return digestFields(string(ConfirmRevealKey), item.RelativePath, item.Fingerprint, item.Permission, contentsDigest), nil
}

func (service *Service) purgeEvidence(entryID string) (string, error) {
	manifest, err := service.readManifest(entryID)
	if err != nil {
		return "", err
	}
	fields := []string{string(ConfirmPurgeEntry), manifest.EntryID, manifest.DeletedAt}
	for _, file := range manifest.Files {
		fields = append(fields, file.OriginalPath, file.TrashPath, file.Kind, file.Fingerprint, file.Permission)
		// その後に消えたファイルは、ダイアログが列挙する内容を変える。したがって
		// その存在も、ユーザーが確認している内容の一部である。
		if _, statErr := service.workspace.FileSystem().Lstat(service.absolutePath(file.TrashPath)); statErr == nil {
			fields = append(fields, "present")
			continue
		}
		fields = append(fields, "missing")
	}
	return digestFields(fields...), nil
}

// digestFields は、曖昧さのない区切りでフィールドの並びをハッシュする。異なる
// 二つのフィールド並びが、連結によって同じダイジェストになることはありえない。
func digestFields(fields ...string) string {
	hash := sha256.New()
	for _, field := range fields {
		hash.Write([]byte(field))
		hash.Write([]byte("\x00"))
	}
	return hex.EncodeToString(hash.Sum(nil))
}
