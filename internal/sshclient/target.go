// Package sshclient は、解決済みの ssh_config 設定を使ってプロセス内で SSH 接続を実行する。
// 外部プログラムを起動するのは、利用者が明示した ProxyCommand だけである。
package sshclient

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"sshc/internal/effective"
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
	"remoteforward":   "sshc does not ask the remote to listen; that inverts the direction of trust and depends on the server's AllowTcpForwarding",
	"forwardx11":      "sshc has no X server behind it; a browser terminal cannot show an X window",
	"controlmaster":   "connection sharing has no meaning inside this process; sshc reuses the connection it already holds",
	"controlpath":     "connection sharing has no meaning inside this process; sshc reuses the connection it already holds",
	"localcommand":    "sshc runs nothing after connecting; the only command it starts for a connection is ProxyCommand, because that one is the connection",
	"certificatefile": "sshc does not read host or user certificates; an organisation that hands out certificates hands out ssh with them",
	"sendenv":         "the value would come from this application's environment, not from your shell, so sshc sends nothing rather than the wrong thing",
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

	// Identities は解決済みの絶対パス。~ とトークンの展開は
	// internal/effective が済ませている。同じ展開を二度書かない。
	Identities     []string
	IdentitiesOnly bool

	// Jump は ProxyJump の連鎖である。手前から順に繋ぐ。
	Jump []Target

	// ProxyCommand は、この接続先へ届くために起動するプログラムの表記である。
	//
	// トークンは展開済みである。解決器は生のまま返す。`ssh -G` がそう
	// するからで、OpenSSH が展開するのは繋ぐ瞬間である。空なら普通に TCP で繋ぐ。
	ProxyCommand string

	// Notices は、この接続について言っておくべきことである。
	//
	// Target が持っている。以前は NewTarget が別の戻り値として返しており、
	// 唯一の呼び出し元がそれを `_` で捨てていた。「読むが従わない」と書いた
	// 7 つのキーワードは、誰にも届いていなかった。値と一緒に運べば、捨てるには
	// 捨てると書かなければならない。
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
// the route used to authenticate it. Saved account passwords are released only
// when this digest still matches the value recorded when the assignment was made.
// Alias is deliberately absent: renaming an alias does not change its peer.
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
	}
	bound := destination{
		HostName: t.HostName, Port: t.Port, User: t.User,
		ProxyCommand: t.ProxyCommand, AgentForward: t.AgentForward, Strict: t.Strict,
		HostKeyAlgorithms: slices.Clone(t.HostKeyAlgorithms), Methods: t.Methods.Order(),
	}
	for _, hop := range t.Jump {
		bound.Jump = append(bound.Jump, hop.AuthenticationBinding())
	}
	encoded, _ := json.Marshal(bound)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

// Resolver は alias ひとつ分の解決である。
//
// ProxyJump のホップはそれ自身が ~/.ssh/config に書かれた alias でありうるので、
// 連鎖を組むには解決をもう一度呼ぶ必要がある。関数で受け取るのは、このパッケージが
// 設定を読まないためである。
type Resolver func(alias string) (effective.Values, error)

// NewTarget は、解決済みの値から接続ひとつ分を組み立てる。
func NewTarget(alias string, resolve Resolver, home string) (Target, error) {
	return newTarget(alias, resolve, home, effective.MaxJumpDepth, nil)
}

func newTarget(alias string, resolve Resolver, home string, depth int, override *effective.Hop) (Target, error) {
	if depth <= 0 {
		return Target{}, ErrJumpDepth
	}
	values, err := resolve(alias)
	if err != nil {
		return Target{}, err
	}
	proxyCommand := noneToEmpty(values.First("proxycommand"))
	if proxyCommand != "" && values.First("proxyjump") != "" &&
		!strings.EqualFold(values.First("proxyjump"), "none") {
		return Target{}, ErrProxyCommandWithJump
	}

	target := Target{
		Alias:          alias,
		HostName:       values.First("hostname"),
		Port:           firstOr(values, "port", effective.DefaultJumpPort),
		User:           values.First("user"),
		Identities:     absolutePaths(values.All("identityfile"), home),
		IdentitiesOnly: yes(values.First("identitiesonly")),
		SetEnv:         parseEnv(values.All("setenv")),
		KeepAlive:      seconds(values.First("serveraliveinterval")),
		KeepAliveMax:   number(values.First("serveralivecountmax"), 3),
		RemoteCommand:  noneToEmpty(values.First("remotecommand")),
		RequestTTY:     values.First("requesttty"),
		Timeout:        seconds(values.First("connecttimeout")),
		Strict:         strings.ToLower(values.First("stricthostkeychecking")),
		Methods:        methodsFrom(values),

		HostKeyAlgorithms: hostKeyAlgorithmsFrom(values),
	}
	if target.HostName == "" {
		return Target{}, ErrNoHostName
	}
	// ProxyJump のリストに明記された user と port は、そのホップ自身の設定に
	// 勝つ。ProxyCommand と入れ子の ProxyJump はこの直後にトークンを展開する
	// ため、値の上書きも展開より前に行う。展開後に Target のフィールドだけを
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
	// トークンを展開するのはここである。解決器は ProxyJump を生のまま返す
	// `ssh -G` がそうするからだ。%r が指すのは、いま組み立てているこの行き先の
	// 利用者であり、手前のホップのそれではない。
	tokens := effective.TokenTarget{
		Alias: alias, HostName: target.HostName, Port: target.Port, RemoteUser: target.User,
	}
	if proxyCommand != "" {
		expanded, err := effective.ExpandProxyTokens(proxyCommand, tokens)
		if err != nil {
			return Target{}, err
		}
		target.ProxyCommand = expanded
	}
	jump, err := effective.ExpandProxyTokens(values.First("proxyjump"), tokens)
	if err != nil {
		return Target{}, err
	}
	chain, err := effective.ParseChain(jump)
	if err != nil {
		return Target{}, err
	}
	if !chain.Disabled {
		for _, hop := range chain.Hops {
			stage, err := newTarget(hop.Host, resolve, home, depth-1, &hop)
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

// parseForwards は、設定に書かれた転送を読む。
//
// 読めない値は notice を出して飛ばす。転送の書式ひとつで接続できなく
// なる理由が無い。
func parseForwards(values effective.Values) ([]ForwardSpec, []Notice) {
	var specs []ForwardSpec
	var notices []Notice

	for keyword, parse := range map[string]func(string) (ForwardSpec, error){
		"localforward":   ParseLocalForward,
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
					Detail: "sshc binds forwards to " + LoopbackHost + " only, so " + entry +
						" listens on this machine and nowhere else",
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
			if !matchesAny(removed, algorithm) {
				kept = append(kept, algorithm)
			}
		}
		return kept
	}
	return dedupe(splitList(raw))
}

func matchesAny(patterns []string, value string) bool {
	for _, pattern := range patterns {
		if effective.MatchPattern(pattern, value) {
			return true
		}
	}
	return false
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

// parseEnv は SetEnv の値を読む。一行に複数の代入が並ぶ。`SetEnv ONE=1 TWO=2`。
func parseEnv(entries []string) []EnvVar {
	var variables []EnvVar
	for _, entry := range entries {
		for _, assignment := range strings.Fields(entry) {
			name, value, found := strings.Cut(assignment, "=")
			if !found || name == "" {
				continue
			}
			variables = append(variables, EnvVar{Name: name, Value: value})
		}
	}
	return variables
}

// absolutePaths は、鍵のパスを絶対パスにする。
//
// ~ の展開は internal/effective が済ませていない。あちらは ssh -G と同じ結果を
// 返す約束であり、ssh -G は ~ を残す。接続に使うのはこちらなので、ここで解く。
func absolutePaths(entries []string, home string) []string {
	var paths []string
	for _, entry := range entries {
		entry = strings.Trim(entry, `"`)
		switch {
		case entry == "":
			continue
		case entry == "~":
			entry = home
		case strings.HasPrefix(entry, "~/"):
			entry = filepath.Join(home, entry[2:])
		case !filepath.IsAbs(entry):
			entry = filepath.Join(home, entry)
		}
		paths = append(paths, filepath.Clean(entry))
	}
	return paths
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
