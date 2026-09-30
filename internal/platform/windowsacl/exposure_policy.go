package windowsacl

import (
	"fmt"
	"slices"
)

// accessEntryKind は、露出の判定が読み分ける ACE の種類である。
type accessEntryKind int

const (
	accessAllowed accessEntryKind = iota
	accessDenied
	// accessUnreadable は、object ACE など、sshc が読み方を持たない種類である。
	accessUnreadable
)

// accessEntry は、DACL の ACE ひとつを、露出の判定に要る形にしたものである。
type accessEntry struct {
	kind accessEntryKind
	// aceType は ACE の種類の生の値で、読み方を持たない種類を報告するときに使う。
	aceType uint8
	// grantsRead は、その ACE の権利に中身を読む権利が含まれるかである。
	grantsRead bool
	// trustee は、許可の ACE が権利を与える相手の SID（"S-1-5-..." の文字列）である。
	trustee string
}

// keySecurity は、鍵のファイルの記述子のうち、露出の判定に要るものである。
//
// SID を文字列で持つのは、判定とそのテストを Windows の型から離すためである。
// 所有者を任意の SID にしたファイルを実際に作るには SeRestorePrivilege が要り、
// 権限の無いテストでは作れない。
type keySecurity struct {
	// owner は所有者の SID である。記述子に所有者が無ければ空になる。
	owner string
	// hasDACL が偽なのは、DACL が無いことである。DACL が無ければ誰でも読める。
	// 空の DACL（ACE がひとつも無い）とは違う。あちらは誰も読めない。
	hasDACL bool
	entries []accessEntry
}

// readableByOthers は、notOthers に入っていない誰かが、その鍵を読めるかを判定する。
//
// notOthers は、この利用者本人・SYSTEM・Administrators と、このトークンのものと
// 言える所有者である。
//
// 所有者が notOthers に入っていなければ、ACE を見る前に露出とする。所有者は
// DACL に関係なく READ_CONTROL と WRITE_DAC を暗黙に持ち、自分に読み取りを
// 足せるからである。別のアカウントが所有する鍵は、そのアカウントの ACE が
// 無くても閉じていない。所有者の無い記述子も、閉じていると確かめられないので
// 同じに扱う。
//
// 拒否 (deny) は数えない。許可と拒否の効き方は ACE の並び順で決まり、
// 順序の壊れた DACL では許可が先に効く。そこで拒否を引き算すると、実際には
// 読める鍵を「安全」と報告しうる。間違える方向としてそれが最も悪い。
// 読ませない意図の deny があるのに警告が出るのは、その逆よりずっと軽い。
//
// 読み方を持たない種類の ACE に出会ったら、閉じていることを確かめられなかった
// のだから、危険の側へ倒し、その理由をエラーで返す。
func readableByOthers(security keySecurity, notOthers []string) (bool, error) {
	if !slices.Contains(notOthers, security.owner) {
		return true, nil
	}
	if !security.hasDACL {
		return true, nil
	}
	for _, entry := range security.entries {
		switch entry.kind {
		case accessDenied:
			continue
		case accessAllowed:
			if !entry.grantsRead || slices.Contains(notOthers, entry.trustee) {
				continue
			}
			return true, nil
		default:
			// object ACE などは、ファイルの DACL に現れることはまず無い。
			return true, fmt.Errorf("access entry of type %d is not one sshc reads", entry.aceType)
		}
	}
	return false, nil
}
