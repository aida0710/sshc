package application

import (
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
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

// WorkspaceKeys は、この alias が使う、ワークスペースの中にある秘密鍵を返す。
//
// 返すのはワークスペース相対の綴りである。保管庫が知っているのがその形だからで、
// ~/.ssh の外にある鍵はここに現れない——**あちらの答えは保管庫に無い。**
//
// **決めるのは effective.Resolve である。** かつてここは自前で設定を歩き、Match・
// 条件付き Include・CanonicalizeHostname・実行を伴うディレクティブ、そして
// ProxyJump を見つけたら「静的に決められない」として何も返さなかった。あの規則は、
// 答えを受け取るのが OpenSSH の起こす別のプログラムだった頃のものである——環境変数で
// 渡した capability が、設定の書ける任意の子プロセスへ継がれることを恐れていた。
//
// いまは要求を出した sshc 自身が答えを受け取り、自分でプロトコルを話す。ProxyCommand
// も KnownHostsCommand も LocalCommand も、あのクライアントは実行しない——**起こさない
// プログラムへ秘密が漏れることはない。** ProxyJump に至っては、連鎖をプロセス内で
// 辿るようになっており、アカウントパスワードの側は連鎖に現れる alias のぶんを渡して
// いる。パスフレーズだけを断る理由はもう無い。
//
// 答えが出ないのは Resolve が拒むときだけである（Match exec のように、このプロセスが
// 実行しないと決めたものが混ざっているとき）。そのとき Accepted は空なので、鍵は
// 一本も返らない——**知らないことを知っているふりをしない。**
func (s *Service) WorkspaceKeys(alias string) ([]string, error) {
	if err := ValidateAlias(alias); err != nil {
		return nil, err
	}
	graph, err := s.resolve()
	if err != nil {
		return nil, err
	}
	resolution := effective.Resolve(graph, alias, s.localFacts())
	found := make([]string, 0, 2)
	for _, entry := range resolution.Accepted {
		if !config.EqualKeyword(entry.Keyword, "IdentityFile") {
			continue
		}
		for _, value := range entry.Values {
			value = strings.TrimSpace(value)
			// `IdentityFile none` は「鍵を使わない」であって、none という名の鍵では
			// ない。ここで拾うと、存在しない綴りを保管庫へ問い合わせることになる。
			if value == "" || strings.EqualFold(value, "none") {
				continue
			}
			if relative, _, ok := keys.ResolveWorkspaceKeyPath(s.workspace, value); ok &&
				!slices.Contains(found, relative) {
				found = append(found, relative)
			}
		}
	}
	return found, nil
}

// UnlockableWorkspaceKeys は、WorkspaceKeys のうち、いま実際に暗号化された秘密鍵
// であるものだけを返す。
//
// **入れ替えられた鍵や、もう暗号化されていない鍵について、保管庫に古い項目が残って
// いることがある。** それを持ち出しても開くものが無いので、渡さない。
func (s *Service) UnlockableWorkspaceKeys(
	alias string,
	inventory func() (*keys.Inventory, error),
) ([]string, error) {
	candidates, err := s.WorkspaceKeys(alias)
	if err != nil || len(candidates) == 0 || inventory == nil {
		return nil, err
	}
	current, err := inventory()
	if err != nil {
		return nil, err
	}
	usable := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		for _, item := range current.Items {
			if filepath.Clean(item.RelativePath) != filepath.Clean(filepath.FromSlash(candidate)) {
				continue
			}
			if item.Kind == keys.KindPrivateKey && item.Encrypted && item.ContentDigest != "" {
				usable = append(usable, candidate)
			}
			break
		}
	}
	return usable, nil
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
