package application

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"

	"sshc/internal/config"
	"sshc/internal/effective"
	"sshc/internal/keys"
	"sshc/internal/knownhosts"
)

// ホストと保存されたパスワードとの間に立ちはだかるものを表す code である。
const (
	// BlockerPasswordAuthenticationOff は、このホストに PasswordAuthentication
	// no が設定されていることを報告する。これはクライアント側の設定なので、
	// クライアントはどれほど良いパスワードでも決して提示しない。それを保存
	// することは、使えない秘密を保存することになり、両方の意味で最悪である。
	BlockerPasswordAuthenticationOff = "password_authentication_off"
	// BlockerAliasNotSimple は、ホストではなく pattern であることを報告する。パスワードは
	// 1 台のマシンの 1 個のアカウントに属するものであり、`*`にはそのようなものは存在しない。
	BlockerAliasNotSimple = "alias_not_simple"
	// BlockerIdentityFileConfigured は、具体的な Host ブロック自身が秘密鍵を
	// 指定していることを報告する。sshc は明示鍵と保存済みアカウントパスワードを
	// 排他にし、鍵が失敗した後の手入力は OpenSSH に任せる。
	BlockerIdentityFileConfigured = "identity_file_configured"
	// WarnHostKeyUnknown は、known_hosts にこのホストの情報が何もないことを
	// 報告する。したがって最初の接続は鍵を信頼するかどうかを尋ねることに
	// なり、このアプリケーションはユーザーに代わってその問いに答えることを
	// 拒否するので、接続はパスワードが使われないままそこで止まる。
	WarnHostKeyUnknown = "host_key_unknown"
	// WarnHostNameUnresolved は、engine が HostName を特定できなかったことを
	// 報告する。パスワードはいずれにせよ alias の下に保存されるので、
	// これは推測せずに言明される。
	WarnHostNameUnresolved = "hostname_unresolved"
)

// PasswordEligibility は、何も保存する前に、このアプリケーションが
// 1 個の alias のパスワード保存について知っていることである。
//
// Blocker と Warning は意図的に分けて扱われている。blocker は保存した
// パスワードを使えなくする事実であり、その場合拒否することは制限では
// なくむしろ親切である。warning はユーザーの方がよく知っているかもしれ
// ない事実である——設定されてはいるが向こう側で認可されていない鍵は
// 普通の状況である——ので、これは言明され、判断はそれが属する場所に委ねられる。
type PasswordEligibility struct {
	Alias    string   `json:"alias"`
	Storable bool     `json:"storable"`
	Blockers []Notice `json:"blockers"`
	Warnings []Notice `json:"warnings"`
	HostName string   `json:"hostName,omitempty"`
	Port     string   `json:"port,omitempty"`
}

// PasswordEligibility は設定と known_hosts を読み、この alias と保存された
// パスワードとの間に立ちはだかるものを報告する。
//
// これは読むだけである。書き込むことも、接続することも、ssh を実行する
// ことも決してない。ここでのすべては、このアプリケーションが既に
// パースしているファイルから答えられるものであり、パスワードを保存すべきか
// 判断するために接続を開くようなチェックは、ユーザーが求めていない接続になってしまう。
func (s *Service) PasswordEligibility(alias string) (PasswordEligibility, error) {
	report := PasswordEligibility{
		Alias:    alias,
		Blockers: []Notice{},
		Warnings: []Notice{},
	}
	if err := ValidateAlias(alias); err != nil {
		report.Blockers = append(report.Blockers, Notice{Code: BlockerAliasNotSimple, Detail: alias})
		return report, nil
	}

	graph, err := s.resolve()
	if err != nil {
		return PasswordEligibility{}, err
	}
	// **Resolve に訊く。** ここは以前 effective.Project を使っていた。あちらは
	// Match ブロックの値を決して採らず、complexity として脇に記録するだけで、
	// この判定はその complexity を一度も読まなかった——`Match host db` の下に
	// 書かれた設定は、まるごとこの報告から消えていた。
	resolution := effective.Resolve(graph, alias, s.localFacts())

	if source, off := passwordAuthenticationDisabled(resolution); off {
		report.Blockers = append(report.Blockers, Notice{
			Code: BlockerPasswordAuthenticationOff,
			Path: s.displayPath(source.Path), Line: source.Line,
		})
	}
	if notice, ok := directIdentityFileForAlias(graph, alias); ok {
		notice.Path = s.displayPath(notice.Path)
		report.Blockers = append(report.Blockers, notice)
	}

	host := alias
	if source, ok := acceptedDirective(resolution, "HostName"); ok && strings.TrimSpace(firstValue(source)) != "" {
		host = strings.TrimSpace(firstValue(source))
	} else {
		report.Warnings = append(report.Warnings, Notice{Code: WarnHostNameUnresolved, Detail: alias})
	}
	report.HostName = host
	if source, ok := acceptedDirective(resolution, "Port"); ok {
		if _, err := strconv.Atoi(strings.TrimSpace(firstValue(source))); err == nil {
			report.Port = strings.TrimSpace(firstValue(source))
		}
	}

	known, err := s.hostKeyIsKnown(host, report.Port)
	if err != nil {
		return PasswordEligibility{}, err
	}
	if !known {
		report.Warnings = append(report.Warnings, Notice{Code: WarnHostKeyUnknown, Detail: host})
	}

	report.Storable = len(report.Blockers) == 0
	return report, nil
}

// acceptedDirective は、解決が採用した keyword の指令を返す。
//
// 見るのは Accepted であって Values ではない。**既定値を採らないためである** ——
// Values の側は hostname・user・port の 3 つを書かれていなくても埋めるので、
// そこから Port を読むと、誰も書いていない 22 が「設定された値」として報告に載る。
// Accepted に居るのは、実際に設定へ書かれた行だけである。
//
// 解決が拒んだとき（Match exec のように、このプロセスが実行しないと決めたものが
// 混ざっているとき）Accepted は空なので、この関数は何も見つけない。**知らないことを
// 知っているふりをしない。** 拒否の理由そのものは ComputeEffective が notice として
// 別に報告する。
func acceptedDirective(resolution effective.Resolution, keyword string) (effective.Accepted, bool) {
	for _, entry := range resolution.Accepted {
		if config.EqualKeyword(entry.Keyword, keyword) {
			return entry, true
		}
	}
	return effective.Accepted{}, false
}

func firstValue(entry effective.Accepted) string {
	if len(entry.Values) == 0 {
		return ""
	}
	return entry.Values[0]
}

// passwordAuthenticationDisabled は、この alias で password 方式が閉じているかを答える。
func passwordAuthenticationDisabled(resolution effective.Resolution) (effective.Accepted, bool) {
	entry, ok := acceptedDirective(resolution, "PasswordAuthentication")
	return entry, ok && strings.EqualFold(strings.TrimSpace(firstValue(entry)), "no")
}

// credentialUnstaticConfiguration は、この解析器が鍵の集合を静的に決められない
// 設定を報告する。
//
// **名前は credentialEnvironmentUnsafe だった。** 環境変数で bearer capability を
// 渡していた頃のもので、「子プロセスへ環境が継がれるから、別のプログラムを起こし
// うる設定は普通の ssh を使え」という規則だった。渡す環境変数はもう無い。
//
// それでも残しているのは、理由が置き換わったからである——ここが見ているのは
// 「この alias が使う IdentityFile を、実行も DNS も伴わずに一意に決められるか」
// であって、決められないなら保存済みパスフレーズを差し出さない。Match、条件付き
// Include、CanonicalizeHostname、実行を伴うディレクティブは、いずれも鍵の集合を
// 接続時にしか確定しない。
//
// **ProxyJump がこの一覧に残っているのは、いま見ると一貫していない。** 連鎖は
// internal/sshclient がプロセス内で辿るようになっており、アカウントパスワードの
// 側は連鎖に現れる alias のぶんを渡している。パスフレーズだけを断る理由は、
// 外部プログラムが消えた時点で無くなっている。挙動を変える判断なので、ここでは
// 名前と理由の書き換えに留める。
func credentialUnstaticConfiguration(graph *config.Graph, alias string) bool {
	for _, diagnostic := range graph.Diagnostics {
		if diagnostic.Code == config.DiagnosticIncludeConditional {
			return true
		}
	}
	unsafe := false
	WalkDirectives(graph, func(visit Visit) bool {
		if visit.Block.Kind == config.BlockMatch {
			unsafe = true
			return false
		}
		if visit.Block.Kind == config.BlockHost && !MatchHostLine(visit.Block.Patterns, alias) {
			return true
		}
		if config.EqualKeyword(visit.Line.Keyword, "CanonicalizeHostname") {
			for _, value := range visit.Line.Values() {
				// Canonicalisation makes OpenSSH parse the configuration again
				// against a different host name. A Host block that does not match
				// alias here could then add another key or executable directive.
				// This static policy deliberately does not predict DNS results.
				if value != "" && !strings.EqualFold(value, "no") {
					unsafe = true
					return false
				}
			}
		}
		if executableCredentialDirective(visit.Line) {
			unsafe = true
			return false
		}
		return true
	})
	return unsafe
}

// DirectKeyPassphraseTarget is the one concrete, workspace-owned private-key
// path selected directly by a host block. RelativePath is the vault subject.
//
// **かつてはここに 3 つの項目が並んでいた。** PromptPath（OpenSSH が復号を尋ねる
// ときの綴り）、ConfigSnapshot（Include を展開し、対象の IdentityFile を抜いた
// 設定の写し）、Evidence（その写しと鍵のバイト列に capability を縛る digest）で
// ある。どれも、答えを受け取るのが OpenSSH の起こす別のプログラムだった頃の道具
// だった——綴りを合わせるのは prompt 文字列を照合するため、写しを渡すのは `-i` で
// 起こす ssh に別の鍵を選ばせないため、digest を持つのは渡した capability を後から
// 縛るためである。
//
// **いまは要求を出した sshc 自身が答えを受け取り、自分でプロトコルを話す。**
// 3 つとも計算されたあと savedPassphrase に捨てられていたので、消した。
type DirectKeyPassphraseTarget struct {
	RelativePath string
}

// directKeyPassphraseTarget accepts only a configuration whose complete,
// statically evaluable IdentityFile set contains one workspace key. Match,
// conditional Include, executable directives, and ProxyJump are deliberately
// refused: all can make a bearer-token environment reach another process or
// make the effective key set depend on facts this parser does not execute.
func (s *Service) directKeyPassphraseTarget(alias string) (DirectKeyPassphraseTarget, bool, error) {
	if err := ValidateAlias(alias); err != nil {
		return DirectKeyPassphraseTarget{}, false, err
	}
	graph, err := s.resolve()
	if err != nil {
		return DirectKeyPassphraseTarget{}, false, err
	}
	if credentialUnstaticConfiguration(graph, alias) {
		return DirectKeyPassphraseTarget{}, false, nil
	}
	values := make([]string, 0, 2)
	sawNone := false
	WalkDirectives(graph, func(visit Visit) bool {
		if visit.Block.Kind == config.BlockHost && !MatchHostLine(visit.Block.Patterns, alias) {
			return true
		}
		if !config.EqualKeyword(visit.Line.Keyword, "IdentityFile") {
			return true
		}
		for _, value := range visit.Line.Values() {
			value = strings.TrimSpace(value)
			if strings.EqualFold(value, "none") {
				sawNone = true
			} else if value != "" {
				values = append(values, value)
			}
		}
		return true
	})
	if sawNone || len(values) != 1 {
		return DirectKeyPassphraseTarget{}, false, nil
	}
	relative, _, ok := keys.ResolveWorkspaceKeyPath(s.workspace, values[0])
	if !ok {
		return DirectKeyPassphraseTarget{}, false, nil
	}
	return DirectKeyPassphraseTarget{RelativePath: relative}, true, nil
}

// executableCredentialDirective は、実行や別プロセスの起動を伴う指令を報告する。
func executableCredentialDirective(line config.Line) bool {
	switch strings.ToLower(line.Keyword) {
	case "proxycommand", "proxyjump", "knownhostscommand", "localcommand", "remotecommand",
		"pkcs11provider", "securitykeyprovider", "xauthlocation":
		for _, value := range line.Values() {
			if value != "" && !strings.EqualFold(value, "none") {
				return true
			}
		}
	}
	return false
}

// DirectKeyPassphraseTarget additionally requires the resolved path to remain
// a current encrypted private key. **入れ替えられた鍵や、もう暗号化されていない
// 鍵について、保管庫に古い項目が残っていることがある。** それを持ち出しても開く
// ものが無いので、渡さない。
func (s *Service) DirectKeyPassphraseTarget(
	alias string,
	inventory func() (*keys.Inventory, error),
) (DirectKeyPassphraseTarget, bool, error) {
	target, ok, err := s.directKeyPassphraseTarget(alias)
	if err != nil || !ok || inventory == nil {
		return DirectKeyPassphraseTarget{}, false, err
	}
	current, err := inventory()
	if err != nil {
		return DirectKeyPassphraseTarget{}, false, err
	}
	for _, item := range current.Items {
		if filepath.Clean(item.RelativePath) == filepath.Clean(filepath.FromSlash(target.RelativePath)) {
			if item.Kind != keys.KindPrivateKey || !item.Encrypted || item.ContentDigest == "" {
				return DirectKeyPassphraseTarget{}, false, nil
			}
			return target, true, nil
		}
	}
	return DirectKeyPassphraseTarget{}, false, nil
}

// hostKeyIsKnown は、known_hosts が既にこのホストの鍵を保持しているかを報告する。
//
// デフォルト以外のポートは known_hosts に`[host]:port`の形で書かれるので、
// 両方の形を試す。ファイルが無いことはエラーではない。それはどこにも
// まだ接続したことのないマシンの通常の状態であり、答えは単純に no である。
func (s *Service) hostKeyIsKnown(host, port string) (bool, error) {
	body, err := s.workspace.FileSystem().ReadFile(filepath.Join(s.workspace.Root(), "known_hosts"))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	candidates := []string{host}
	if port != "" && port != "22" {
		candidates = append(candidates, "["+host+"]:"+port)
	}
	for _, line := range knownhosts.ParseFile(body).Entries() {
		if line.Entry == nil {
			continue
		}
		for _, candidate := range candidates {
			if line.Entry.MatchesHost(candidate) {
				return true, nil
			}
		}
	}
	return false, nil
}
