// Package sshclient は、解決済みの ssh_config 設定を使ってプロセス内で SSH 接続を実行する。
// 外部プログラムは、利用者が明示したProxyCommandとそのPATHの取得に使う。
package sshclient

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"sshc/internal/effective"
	"sshc/internal/sshmatch"
	"sshc/internal/terminal"
	"sshc/internal/textencoding"
)

// 接続を組み立てられない理由。
var (
	// ErrJumpDepth は、深すぎる ProxyJump の連鎖を断る。
	ErrJumpDepth = errors.New("the ProxyJump chain is too deep")
	// ErrNoHostName は、接続先が決まらない設定を断る。
	ErrNoHostName = errors.New("this alias resolves to no host name")
)

// Notice は、接続を続行しつつ適用しなかった設定を報告する。
type Notice struct {
	Keyword string
	Detail  string
}

// unhonoured は、読みはするが従わないキーワードである。
//
// ここに無いものは暗黙に無視される。OpenSSH の全キーワードを列挙するのでは
// なく、「利用者が書いたのに効かない」と気づける必要があるものだけを挙げる。
//
// 各メッセージには適用しない理由を含める。
var unhonoured = map[string]string{
	// 無い。
	"forwardx11":      "sshc has no X server behind it; a browser terminal cannot show an X window",
	"controlmaster":   "connection sharing has no meaning inside this process; sshc reuses the connection it already holds",
	"controlpath":     "connection sharing has no meaning inside this process; sshc reuses the connection it already holds",
	"localcommand":    "sshc runs nothing after connecting; the only command it starts for a connection is ProxyCommand, because that one is the connection",
	"certificatefile": "sshc does not read host or user certificates; an organisation that hands out certificates hands out ssh with them",
	"sendenv":         "the value would come from this application's environment, not from your shell, so sshc sends nothing rather than the wrong thing",
	"knownhostscommand": "sshc does not run KnownHostsCommand; host keys are checked against the known_hosts files only, " +
		"and an unknown host is asked about even with StrictHostKeyChecking no or accept-new",
}

// EnvVar は、チャンネルへ送る環境変数ひとつである。
type EnvVar struct {
	Name  string
	Value string
}

// Methods は、どの認証方式を許すかである。
type Methods struct {
	// Preferred は PreferredAuthentications の並び。空なら OpenSSH の既定順。
	Preferred []string
	PublicKey bool
	Password  bool
	Keyboard  bool
}

// DefaultMethods は、何も書かれていない設定での認証方式である。
func DefaultMethods() Methods {
	return Methods{PublicKey: true, Password: true, Keyboard: true}
}

// Order は、実際に試す方式の並びを返す。
func (m Methods) Order() []string {
	allowed := map[string]bool{
		"publickey": m.PublicKey, "password": m.Password, "keyboard-interactive": m.Keyboard,
	}
	order := m.Preferred
	if len(order) == 0 {
		// OpenSSH の既定順。gssapi と hostbased はこのクライアントに無い。
		order = []string{"publickey", "keyboard-interactive", "password"}
	}
	kept := make([]string, 0, len(order))
	seen := map[string]bool{}
	for _, method := range order {
		method = strings.ToLower(strings.TrimSpace(method))
		if seen[method] || !allowed[method] {
			continue
		}
		seen[method] = true
		kept = append(kept, method)
	}
	return kept
}

// Target は、ひとつの接続に要る値の全体である。
type Target struct {
	Alias    string
	HostName string
	Port     string
	User     string
	// authenticationBindingOverride は、非対話処理がhost key方針だけを安全側へ
	// 強制した場合に、利用者が確認した元の接続経路を保持する。接続先や経路を
	// 変えたときに設定してはならない。
	authenticationBindingOverride string
	// Encoding is applied only to terminal and command payload bytes after the
	// SSH protocol has been decoded. Empty and UTF8 both mean UTF-8.
	Encoding textencoding.Name

	// Identities は鍵のファイルの絶対パス。~ とトークンは、この行き先の値で
	// NewTarget が展開した。展開できなかった値は notice にして、ここには入れない。
	Identities     []string
	IdentitiesOnly bool

	// Jump は ProxyJump の連鎖である。手前から順に繋ぐ。
	Jump []Target

	// VPN は、この接続先へ届くために通る VPN プロファイルの名前である。
	//
	// 空なら VPN を通らない。ssh_config には書かれない。OpenSSH が解釈する
	// 語ではなく、sshc が metadata に持つ紐付けだからである。
	VPN string
	// VPNProfileID は、VPN のプロファイルの識別子である。名前と違い、改名しても
	// 変わらず、削除して同じ名前で作り直したプロファイルでは別の値になる。
	// AuthenticationBinding は名前ではなくこれを使う。
	VPNProfileID string

	// ProxyCommand は、この接続先へ届くために起動するプログラムの表記である。
	//
	// トークンは展開済みである。解決器は生のまま返す。`ssh -G` がそう
	// するからで、OpenSSH が展開するのは繋ぐ瞬間である。空なら普通に TCP で繋ぐ。
	ProxyCommand string

	// Notices は、この接続について言っておくべきことである。
	//
	// 別の戻り値にせず Target に持たせるのは、値と一緒に運べば、捨てるには捨てると
	// 書かなければならないからである。「読むが従わない」キーワードの知らせが、
	// 呼び出し元の `_` で誰にも届かなくなることを防ぐ。
	Notices []Notice

	// Forwards は、この接続の上に開く転送である。
	//
	// bind するのはループバックだけである。設定がそれ以外を求めていたら
	// 束ねて notice を出す。転送の設定ひとつで繋がらなくなる方が困る。
	Forwards []ForwardSpec
	// AgentForward は、こちらの agent をリモートへ貸すかである。
	AgentForward bool

	// HostKeyAlgorithms は、交渉で名乗るホスト鍵アルゴリズムの順である。
	// 空なら、すでに known_hosts に持っている鍵の種類が順を決める。
	HostKeyAlgorithms []string
	// KnownHosts は、ホスト鍵を照合する known_hosts のファイルである。
	KnownHosts KnownHostsFiles
	// HostKeyAlias は、known_hosts でこのホストを探して書く名前である。空なら
	// HostName と Port から作る。
	HostKeyAlias string

	SetEnv        []EnvVar
	KeepAlive     time.Duration
	KeepAliveMax  int
	RemoteCommand string
	RequestTTY    string
	Timeout       time.Duration
	Strict        string
	Methods       Methods
}

// Address は、dial する宛先である。
func (t Target) Address() string { return net.JoinHostPort(t.HostName, t.Port) }

// connectTimeout は、このホップの接続と鍵交換に掛ける上限である。
// ConnectTimeout が書かれていなければ DefaultTimeout を使う。
func (t Target) connectTimeout() time.Duration {
	if t.Timeout <= 0 {
		return DefaultTimeout
	}
	return t.Timeout
}

// JumpRoute returns every ProxyJump hop in the order the TCP chain must open.
// A hop may resolve another ProxyJump of its own, so the innermost prerequisite
// comes before the hop which names it. Dialing and diagnostics share this route
// to prevent the described path from diverging from the path actually used.
func (t Target) JumpRoute() []Target {
	var route []Target
	for _, hop := range t.Jump {
		route = appendJumpRoute(route, hop)
	}
	return route
}

func appendJumpRoute(route []Target, hop Target) []Target {
	for _, prerequisite := range hop.Jump {
		route = appendJumpRoute(route, prerequisite)
	}
	return append(route, hop)
}

// AuthenticationBinding returns a stable digest of the resolved destination and
// the route used to authenticate it, including the VPN profile. Saved account
// passwords, TOTP seeds and startup snippets are released only when this digest
// still matches the value recorded when the assignment was made.
// Alias is deliberately absent: renaming an alias does not change its peer.
// So is the name of the VPN profile: renaming a profile does not change its
// route, while a profile recreated under a removed name may be another VPN.
//
// NewTarget does not fill VPN and VPNProfileID, so a caller that records or
// compares a binding must set them exactly as the connection that uses the
// binding does.
func (t Target) AuthenticationBinding() string {
	if t.authenticationBindingOverride != "" {
		return t.authenticationBindingOverride
	}
	type destination struct {
		HostName          string
		Port              string
		User              string
		Jump              []string
		ProxyCommand      string
		AgentForward      bool
		Strict            string
		HostKeyAlgorithms []string
		Methods           []string
		// HostKeyAlias は、どの known_hosts の行が相手を認めるかを変える。書かれて
		// いない接続では省き、保存済みの割り当ての digest を変えない。
		HostKeyAlias string `json:",omitempty"`
		// VPN は、どのネットワークの HostName へ繋ぐかを変える。同じアドレスが別の
		// VPN の中では別のマシンでありうる。名前ではなくプロファイルの識別子を入れる
		// （vpnProfileBinding）。VPN を付けていない接続では省き、保存済みの割り当ての
		// digest を変えない。
		VPN string `json:",omitempty"`
	}
	bound := destination{
		HostName: t.HostName, Port: t.Port, User: t.User,
		ProxyCommand: t.ProxyCommand, AgentForward: t.AgentForward, Strict: t.Strict,
		HostKeyAlgorithms: slices.Clone(t.HostKeyAlgorithms), Methods: t.Methods.Order(),
		HostKeyAlias: t.HostKeyAlias, VPN: t.vpnProfileBinding(),
	}
	for _, hop := range t.Jump {
		bound.Jump = append(bound.Jump, hop.AuthenticationBinding())
	}
	encoded, _ := json.Marshal(bound)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

// unknownVPNProfileBinding は、付けた VPN プロファイルが見つからない（識別子の無い）
// 接続が結び付けの値に入れる値である。識別子（application の VPNProfile.ID）は 16 進の
// 数字だけなので、どの識別子とも重ならない。
const unknownVPNProfileBinding = "unknown"

// vpnProfileBinding は、結び付けの値に入れる VPN の値である。VPN を付けていなければ空に
// する。付けたプロファイルが見つからなければ、VPN を付けていない接続とも、どの
// プロファイルを通る接続とも違う値にする。見つからないプロファイルでは繋げないが、
// 識別子を入れ忘れた呼び手が、VPN を付けていない接続の割り当てを VPN の先へ渡さない
// ようにする。
func (t Target) vpnProfileBinding() string {
	switch {
	case t.VPN == "":
		return ""
	case t.VPNProfileID == "":
		return unknownVPNProfileBinding
	default:
		return t.VPNProfileID
	}
}

// Resolver は alias ひとつ分の解決である。
//
// ProxyJump のホップはそれ自身が ~/.ssh/config に書かれた alias でありうるので、
// 連鎖を組むには解決をもう一度呼ぶ必要がある。関数で受け取るのは、このパッケージが
// 設定を読まないためである。
type Resolver func(alias string) (effective.Values, error)

// NewTarget は、解決済みの値から接続ひとつ分を組み立てる。
//
// facts はトークンと ~ の展開に使う。IdentityFile の %h や %r は、この行き先の
// 値で展開する。
func NewTarget(alias string, resolve Resolver, facts effective.LocalFacts) (Target, error) {
	builder := targetBuilder{resolve: resolve, facts: facts}
	return builder.build(alias, effective.MaxJumpDepth, nil)
}

// targetBuilder は、ProxyJump のホップをたどりながら Target を組み立てる。
type targetBuilder struct {
	resolve Resolver
	facts   effective.LocalFacts
}

// build は alias ひとつ分の Target を組み立てる。override は、ProxyJump のリストに
// 明記された user と port である。
func (b targetBuilder) build(alias string, depth int, override *effective.Hop) (Target, error) {
	if depth <= 0 {
		return Target{}, ErrJumpDepth
	}
	values, err := b.resolve(alias)
	if err != nil {
		return Target{}, err
	}
	// ProxyCommand と ProxyJump の両方が書かれた設定は、解決器が OpenSSH と同じく
	// 先に受理した方だけを残しているので、ここには片方しか届かない。
	proxyCommand := noneToEmpty(values.First("proxycommand"))

	target := Target{
		Alias:          alias,
		HostName:       values.First("hostname"),
		Port:           firstOr(values, "port", effective.DefaultPort),
		User:           values.First("user"),
		IdentitiesOnly: yes(values.First("identitiesonly")),
		SetEnv:         parseEnv(values.All("setenv")),
		KeepAlive:      seconds(values.First("serveraliveinterval")),
		KeepAliveMax:   number(values.First("serveralivecountmax"), 3),
		RemoteCommand:  noneToEmpty(values.First("remotecommand")),
		RequestTTY:     values.First("requesttty"),
		Timeout:        seconds(values.First("connecttimeout")),
		Strict:         strictHostKeyChecking(values.First("stricthostkeychecking")),
		Methods:        methodsFrom(values),

		HostKeyAlgorithms: hostKeyAlgorithmsFrom(values),
		HostKeyAlias:      hostKeyAliasFrom(values),
	}
	if target.HostName == "" {
		return Target{}, ErrNoHostName
	}
	// ProxyJump のリストに明記された user と port は、そのホップ自身の設定に
	// 勝つ。IdentityFile、ProxyCommand、入れ子の ProxyJump はこの直後にトークンを
	// 展開するため、値の上書きも展開より前に行う。展開後に Target のフィールドだけを
	// 変えると、認証上の宛先と ProxyCommand が実際に開く宛先が食い違う。
	if override != nil {
		if override.UserExplicit {
			target.User = override.User
		}
		if override.PortExplicit {
			target.Port = override.Port
		}
	}

	forwards, forwardNotices := parseForwards(values)
	target.Forwards = forwards
	target.AgentForward = yes(values.First("forwardagent"))

	notices := append(noticesFor(values), forwardNotices...)
	// トークンを展開するのはここである。解決器は IdentityFile と ProxyJump を
	// 生のまま返す。`ssh -G` がそうするからだ。%r が指すのは、いま組み立てている
	// この行き先の利用者であり、手前のホップのそれではない。
	tokens := effective.TokenTarget{
		Alias: alias, HostName: target.HostName, Port: target.Port, RemoteUser: target.User,
		HostKeyAlias: target.HostKeyAlias,
	}
	jump, err := effective.ExpandProxyTokens(values.First("proxyjump"), tokens)
	if err != nil {
		return Target{}, err
	}
	chain, err := effective.ParseChain(jump)
	if err != nil {
		return Target{}, err
	}
	tokens.JumpHost = jumpHostToken(chain)
	identities, identityNotices := identityPaths(values.All("identityfile"), b.facts, tokens)
	target.Identities = identities
	notices = append(notices, identityNotices...)
	target.KnownHosts = knownHostsFilesFrom(values, b.facts, tokens)
	// KnownHostsCommand が返すはずの鍵と照合できないので、未知に見えるホストを
	// 尋ねずに受け入れない。
	if noneToEmpty(values.First("knownhostscommand")) != "" && (target.Strict == "no" || target.Strict == "accept-new") {
		target.Strict = "ask"
	}
	if proxyCommand != "" {
		expanded, err := effective.ExpandProxyTokens(proxyCommand, tokens)
		if err != nil {
			return Target{}, err
		}
		target.ProxyCommand = expanded
	}
	if !chain.Disabled {
		for _, hop := range chain.Hops {
			stage, err := b.build(hop.Host, depth-1, &hop)
			if err != nil {
				return Target{}, err
			}
			target.Jump = append(target.Jump, stage)
			notices = append(notices, stage.Notices...)
		}
	}
	target.Notices = notices
	return target, nil
}

// identityPaths は、IdentityFile の値を、この行き先について OpenSSH が開く鍵の
// パスへ展開する。
//
// 展開できない値の鍵は使わず、理由を notice にする。文字どおりのファイル名として
// 探しにいくと、存在しない鍵を黙って試し、認証の失敗だけが残る。
func identityPaths(entries []string, facts effective.LocalFacts, tokens effective.TokenTarget) ([]string, []Notice) {
	var paths []string
	var notices []Notice
	for _, entry := range entries {
		path, err := effective.ExpandFilePath(entry, facts, tokens)
		if err != nil {
			notices = append(notices, Notice{
				Keyword: "identityfile",
				Detail:  "sshc does not use the key " + entry + ": " + identityPathProblem(err),
			})
			continue
		}
		paths = append(paths, path)
	}
	return paths, notices
}

func identityPathProblem(err error) string {
	switch {
	case errors.Is(err, effective.ErrRelativePath):
		return "a relative path is opened from the directory ssh starts in, which sshc does not have"
	case errors.Is(err, effective.ErrOtherUsersHome):
		return "sshc does not look up another user's home directory"
	case errors.Is(err, effective.ErrEnvironmentVariable):
		return err.Error()
	default:
		return "it uses a token sshc does not expand"
	}
}

// parseForwards は、設定に書かれた転送を読む。
//
// 読めない値は notice を出して飛ばす。転送の書式ひとつで接続できなく
// なる理由が無い。
func parseForwards(values effective.Values) ([]ForwardSpec, []Notice) {
	var specs []ForwardSpec
	var notices []Notice

	for keyword, parse := range map[string]func(string) (ForwardSpec, error){
		"localforward":   ParseLocalForward,
		"remoteforward":  ParseRemoteForward,
		"dynamicforward": ParseDynamicForward,
	} {
		for _, entry := range values.All(keyword) {
			if strings.EqualFold(strings.TrimSpace(entry), "none") {
				continue
			}
			spec, err := parse(entry)
			if err != nil {
				notices = append(notices, Notice{
					Keyword: keyword,
					Detail:  "sshc does not understand this forwarding specification: " + entry,
				})
				continue
			}
			if spec.Bound() {
				notices = append(notices, Notice{
					Keyword: keyword,
					Detail:  forwardBindNotice(spec, entry),
				})
			}
			specs = append(specs, spec)
		}
	}
	// map の走査は順序を持たない。開く順を固定する。同じ設定が毎回同じ
	// 順で報告されないと、画面の一覧が接続のたびに並び替わる。
	sort.Slice(specs, func(i, j int) bool {
		if specs[i].Kind != specs[j].Kind {
			return specs[i].Kind < specs[j].Kind
		}
		return specs[i].ListenPort < specs[j].ListenPort
	})
	return specs, notices
}

func forwardBindNotice(spec ForwardSpec, entry string) string {
	if spec.Kind == terminal.ForwardRemote {
		return "sshc requests " + LoopbackHost + " on the SSH server, so " + entry + " uses that listen address"
	}
	return "sshc binds forwards to " + LoopbackHost + " only, so " + entry +
		" listens on this machine and nowhere else"
}

func noticesFor(values effective.Values) []Notice {
	var notices []Notice
	for _, keyword := range values.Keywords {
		detail, listed := unhonoured[keyword]
		if !listed {
			continue
		}
		// no/none は機能を無効にする指定なので通知しない。
		switch strings.ToLower(values.First(keyword)) {
		case "no", "none":
			continue
		}
		notices = append(notices, Notice{Keyword: keyword, Detail: detail})
	}
	return notices
}

// hostKeyAlgorithmsFrom は HostKeyAlgorithms の指定を読む。
//
// 先頭の一文字は OpenSSH が決めている形である。+ は既定へ足し、- は既定から
// 外し、^ は既定の先頭へ移す。それ以外は既定を置き換える。外す側だけがパターンを
// 受け取る。`-ecdsa-sha2-*` のような書き方は、足す側には意味が無い。
//
// 何も書かれていなければ nil を返す。そのとき順を決めるのは known_hosts である。
func hostKeyAlgorithmsFrom(values effective.Values) []string {
	raw := strings.TrimSpace(values.First("hostkeyalgorithms"))
	if raw == "" {
		return nil
	}
	switch raw[0] {
	case '+':
		return dedupe(append(slices.Clone(defaultHostKeyAlgorithms), splitList(raw[1:])...))
	case '^':
		return dedupe(append(splitList(raw[1:]), defaultHostKeyAlgorithms...))
	case '-':
		removed := splitList(raw[1:])
		kept := make([]string, 0, len(defaultHostKeyAlgorithms))
		for _, algorithm := range defaultHostKeyAlgorithms {
			if !sshmatch.PatternList(removed, algorithm, sshmatch.CaseSensitive) {
				kept = append(kept, algorithm)
			}
		}
		return kept
	}
	return dedupe(splitList(raw))
}

// splitList は、カンマ区切りの並びを読む。空の要素は落とす。
func splitList(raw string) []string {
	var listed []string
	for _, element := range strings.Split(raw, ",") {
		if element = strings.TrimSpace(element); element != "" {
			listed = append(listed, element)
		}
	}
	return listed
}

func dedupe(listed []string) []string {
	kept := make([]string, 0, len(listed))
	seen := map[string]bool{}
	for _, element := range listed {
		if seen[element] {
			continue
		}
		seen[element] = true
		kept = append(kept, element)
	}
	return kept
}

func methodsFrom(values effective.Values) Methods {
	methods := DefaultMethods()
	if preferred := values.First("preferredauthentications"); preferred != "" {
		methods.Preferred = strings.Split(preferred, ",")
	}
	if value := values.First("pubkeyauthentication"); value != "" {
		methods.PublicKey = yes(value)
	}
	if value := values.First("passwordauthentication"); value != "" {
		methods.Password = yes(value)
	}
	if value := values.First("kbdinteractiveauthentication"); value != "" {
		methods.Keyboard = yes(value)
	}
	return methods
}

// parseEnv は SetEnv の値を読む。解決器は代入ひとつずつを別の値として返す。
// `SetEnv X="a b" ONE=1` は "X=a b" と "ONE=1" であり、値の中の空白で分けない。
func parseEnv(entries []string) []EnvVar {
	var variables []EnvVar
	for _, assignment := range entries {
		name, value, found := strings.Cut(assignment, "=")
		if !found || name == "" {
			continue
		}
		variables = append(variables, EnvVar{Name: name, Value: value})
	}
	return variables
}

// strictHostKeyChecking は、StrictHostKeyChecking の書き方を、未知のホスト鍵の
// 扱いを表す yes・no・accept-new・ask のどれかに揃える。
//
// OpenSSH の multistate_strict_hostkey と同じ対応で、true は yes、false と off は
// no である。true を ask と同じに扱うと、利用者が決めた「未知のホストは断る」が
// 黙って「尋ねる」に弱まる。書かれていなければ空のまま返し、ask として扱う。
func strictHostKeyChecking(value string) string {
	switch lowered := strings.ToLower(value); lowered {
	case "yes", "true":
		return "yes"
	case "no", "false", "off":
		return "no"
	default:
		return lowered
	}
}

func firstOr(values effective.Values, keyword, fallback string) string {
	if value := values.First(keyword); value != "" {
		return value
	}
	return fallback
}

func yes(value string) bool {
	return strings.EqualFold(value, "yes") || strings.EqualFold(value, "true")
}

func noneToEmpty(value string) string {
	if strings.EqualFold(value, "none") {
		return ""
	}
	return value
}

func seconds(value string) time.Duration {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return 0
	}
	return time.Duration(parsed) * time.Second
}

func number(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}
