package validate

import (
	"regexp"
	"strings"
)

// ここにあるのは、ブラウザにも同じ答えを出してほしい規則である。
//
// **パターンは文字列として持つ。** そのまま TypeScript の RegExp へ渡すからで、
// 綴りが 2 つあれば、いつか片方だけが直る。使ってよいのは Go の RE2 と JavaScript の
// 両方が同じ意味で読む書き方だけ——文字クラス、アンカー、回数指定に限る。後方参照も
// 先読みも書かない。
//
// **守る契約は「ブラウザはサーバーより厳しくしない」である。** 緩い側にずれても
// サーバーが正しく断り、利用者は理由を受け取る。厳しい側にずれると、**正しい入力が
// 画面で止められる**——そちらだけが直せない失敗である。cmd/rulegen が両方の側へ
// この表と、判定を突き合わせるためのコーパスを配る。
const (
	// GroupSegmentPattern は、グループ名の 1 区切りが従う形である。
	//
	// **先頭は英数字でなければならない。** `-` で始まる名前はディレクトリ名になり、
	// その綴りは Include のパスに現れる——alias で同じ理由から拒んでいるのと同じ
	// 危うさである。長らく Go はこれを受け入れ、画面だけが断っていた。
	GroupSegmentPattern = `^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`
	// AliasPattern は、Host alias が従う形である。
	AliasPattern = `^[A-Za-z0-9][A-Za-z0-9._-]*$`
	// HostnamePattern は DNS 名と IPv4 リテラルに使う。IPv6 は圧縮表記の先頭や末尾が
	// ':' になりうるので、この形では判定しない。
	HostnamePattern = `^[A-Za-z0-9]([A-Za-z0-9._-]*[A-Za-z0-9])?$`
)

const (
	// MaxGroupSegments は、グループがネストできる深さの上限である。
	//
	// この制限は鍵スキャナー由来である。~/.ssh から最大 8 階層を歩き、"keys" 自体が
	// そのうち 1 つを消費するので、7 段目のグループの中にある鍵は depth_exceeded と
	// して一覧から消える。**受け入れてから取りこぼすより、名前を拒む方がよい。**
	MaxGroupSegments = 6
	// MaxGroupSegmentBytes は鍵 vault のファイル名ポリシーに合わせてある。
	MaxGroupSegmentBytes = 64
)

// ReservedGroupNames は、OpenSSH とこのアプリケーションが ~/.ssh の中で既に意味を
// 与えている名前である。
//
// **比較は大小文字を区別しない。** 既定の macOS ボリュームは "Config" と "config" を
// 同じディレクトリエントリとして扱う。
//
// 並びは決めてある——生成物が呼び出しごとに変わると、verify-generated が意味の無い
// 差分を出す。
var ReservedGroupNames = []string{
	"authorized_keys",
	"authorized_keys2",
	"config",
	"connections",
	"environment",
	"keys",
	"known_hosts",
	"known_hosts2",
	"rc",
	"sshc",
}

var (
	groupSegment  = regexp.MustCompile(GroupSegmentPattern)
	reservedGroup = func() map[string]bool {
		set := make(map[string]bool, len(ReservedGroupNames))
		for _, name := range ReservedGroupNames {
			set[name] = true
		}
		return set
	}()
)

// GroupName は、この綴りを connections の下のディレクトリとして受け付けてよいかを
// 報告する。
func GroupName(name string) error {
	if name == "" || strings.ContainsRune(name, '\x00') {
		return ErrInvalidGroupName
	}
	segments := strings.Split(name, "/")
	if len(segments) > MaxGroupSegments {
		return ErrInvalidGroupName
	}
	for _, segment := range segments {
		if err := GroupSegment(segment); err != nil {
			return err
		}
	}
	return nil
}

// GroupSegment は、区切りひとつを見る。
//
// 長さはバイトで数える。パターンの {0,63} は Go では**バイト**、JavaScript では
// **UTF-16 の単位**を数えるので、そこだけは両者で意味が違う——文字集合が ASCII に
// 限られているぶん、受理される綴りでは一致する。**受理されないものについては、
// ブラウザの方が緩い側へ外れる**ので、契約は保たれる。
func GroupSegment(segment string) error {
	if len(segment) > MaxGroupSegmentBytes || !groupSegment.MatchString(segment) {
		return ErrInvalidGroupName
	}
	if reservedGroup[strings.ToLower(segment)] {
		return ErrInvalidGroupName
	}
	return nil
}
