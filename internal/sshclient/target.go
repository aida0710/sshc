// Package sshclient は、このプロセスの中で SSH を話す。
//
// 外部の ssh を起こさない。設定を読むのは internal/effective であり、この
// パッケージが受け取るのはその答えだけである——~/.ssh/config をここで読むと、
// 「この alias に接続すると何が使われるか」に答えるものがまた二つになる。
package sshclient

import (
	"errors"
	"net"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"sshc/internal/effective"
)

// 接続を組み立てられない理由。
var (
	// ErrProxyCommand は、接続のためにプログラムを起こす設定を断る。
	//
	// B1 が Match exec について決めたことと同じ理由である。**このアプリケーションは
	// 接続のために何も実行しない。** ~/.ssh/config は正本のまま残るので、その人は
	// 端末から ssh で繋げる——逃げ道は構造上ある。
	ErrProxyCommand = errors.New("ProxyCommand starts a program, which this client does not do")
	// ErrJumpDepth は、深すぎる ProxyJump の連鎖を断る。
	ErrJumpDepth = errors.New("the ProxyJump chain is too deep")
	// ErrNoHostName は、接続先が決まらない設定を断る。
	ErrNoHostName = errors.New("this alias resolves to no host name")
)

// Notice は、接続はするが honour しなかった設定ひとつである。
//
// Refusal と違って接続は続く。転送が無いことを理由に接続そのものを断ると、
// 転送を使っていない日にも繋がらなくなる。
type Notice struct {
	Keyword string
	Detail  string
}

// unhonoured は、読みはするが従わないキーワードである。
//
// **ここに無いものは黙って無視される。** OpenSSH の全キーワードを列挙するのでは
// なく、「利用者が書いたのに効かない」と気づける必要があるものだけを挙げる。
//
// **「まだ無い」と「無い」を区別して書く。** 永久に無いものを、来週来るかの
// ように言わない。落とすと決めた理由はそれぞれの文言に入れてある。
var unhonoured = map[string]string{
	// 無い。
	"remoteforward":   "sshc does not ask the remote to listen; that inverts the direction of trust and depends on the server's AllowTcpForwarding",
	"forwardx11":      "sshc has no X server behind it; a browser terminal cannot show an X window",
	"controlmaster":   "connection sharing has no meaning inside this process; sshc reuses the connection it already holds",
	"controlpath":     "connection sharing has no meaning inside this process; sshc reuses the connection it already holds",
	"localcommand":    "sshc starts no program to connect",
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

	// Identities は解決済みの絶対パス。~ とトークンの展開は
	// internal/effective が済ませている。同じ展開を二度書かない。
	Identities     []string
	IdentitiesOnly bool

	// Jump は ProxyJump の連鎖である。手前から順に繋ぐ。
	Jump []Target

	// Forwards は、この接続の上に開く転送である。
	//
	// **bind するのはループバックだけである。** 設定がそれ以外を求めていたら
	// 束ねて notice を出す——転送の設定ひとつで繋がらなくなる方が困る。
	Forwards []ForwardSpec
	// AgentForward は、こちらの agent をリモートへ貸すかである。
	AgentForward bool

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

// Resolver は alias ひとつ分の解決である。
//
// ProxyJump のホップはそれ自身が ~/.ssh/config に書かれた alias でありうるので、
// 連鎖を組むには解決をもう一度呼ぶ必要がある。関数で受け取るのは、このパッケージが
// 設定を読まないためである。
type Resolver func(alias string) (effective.Values, error)

// NewTarget は、解決済みの値から接続ひとつ分を組み立てる。
func NewTarget(alias string, resolve Resolver, home string) (Target, []Notice, error) {
	return newTarget(alias, resolve, home, effective.MaxJumpDepth)
}

func newTarget(alias string, resolve Resolver, home string, depth int) (Target, []Notice, error) {
	if depth <= 0 {
		return Target{}, nil, ErrJumpDepth
	}
	values, err := resolve(alias)
	if err != nil {
		return Target{}, nil, err
	}
	if values.First("proxycommand") != "" && !strings.EqualFold(values.First("proxycommand"), "none") {
		return Target{}, nil, ErrProxyCommand
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
	}
	if target.HostName == "" {
		return Target{}, nil, ErrNoHostName
	}

	forwards, forwardNotices := parseForwards(values)
	target.Forwards = forwards
	target.AgentForward = yes(values.First("forwardagent"))

	notices := append(noticesFor(values), forwardNotices...)
	chain, err := effective.ParseChain(values.First("proxyjump"))
	if err != nil {
		return Target{}, nil, err
	}
	if !chain.Disabled {
		for _, hop := range chain.Hops {
			// リストに書かれた user と port は、そのホップ自身の設定に勝つ。
			// OpenSSH がそう決めている。
			stage, hopNotices, err := newTarget(hop.Host, resolve, home, depth-1)
			if err != nil {
				return Target{}, nil, err
			}
			if hop.UserExplicit {
				stage.User = hop.User
			}
			if hop.PortExplicit {
				stage.Port = hop.Port
			}
			target.Jump = append(target.Jump, stage)
			notices = append(notices, hopNotices...)
		}
	}
	return target, notices, nil
}

// parseForwards は、設定に書かれた転送を読む。
//
// **読めない値は notice を出して飛ばす。** 転送の書式ひとつで接続できなく
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
	// map の走査は順序を持たない。開く順を固定する——同じ設定が毎回同じ
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
		// no/none と書いてあるものは、無いことが望みなので黙っている。
		switch strings.ToLower(values.First(keyword)) {
		case "no", "none":
			continue
		}
		notices = append(notices, Notice{Keyword: keyword, Detail: detail})
	}
	return notices
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

// parseEnv は SetEnv の値を読む。一行に複数の代入が並ぶ——`SetEnv ONE=1 TWO=2`。
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
// ~ の展開は internal/effective が済ませていない——あちらは ssh -G と同じ答えを
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
