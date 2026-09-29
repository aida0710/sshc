package vpn

import (
	"bytes"
	"net/netip"
	"slices"
	"strconv"
	"strings"
)

// WireGuard の設定ファイル（wg-quick の形。[Interface] と [Peer]）を読み、sshc で
// 使えるかを確かめる。
//
// 読み方は、コンテナの wg と同じ wireguard-tools（v1.0.20210914）の config.c に合わせる。
//
//   - 行の中の最初の # から先は注釈である。
//   - 行の中の空白は、キーと値の中のものも含めてすべて取り除く。
//   - 節の名前とキーは、大文字と小文字を区別しない。
//
// wg setconf が読むのは PrivateKey、ListenPort、FwMark と [Peer] の項目だけである。
// Address、DNS、MTU、Table、PreUp などは wg-quick だけが読む。sshc は wg-quick を
// 使わないので、Address、DNS、MTU は sshc が自分で扱い、コマンドを実行するものと経路を
// 変えるものは断る。画面の web/src/vpn/wireGuardConfig.ts は同じ規則の写しで、
// testdata/wireguard-config-cases.json に対するテストで揃える。

// WireGuard の設定ファイルを受け取れない理由の語である。どの backend でも使う語は
// config_refusal.go にある。画面と CLI が翻訳する。
const (
	// ReasonUnknownDirective は、WireGuard の設定ファイルに無い項目であることを表す。
	ReasonUnknownDirective Reason = "unknown_directive"
	// ReasonMisplacedDirective は、項目を書く節が違うことを表す（[Peer] の Endpoint を
	// [Interface] に書いた、など）。
	ReasonMisplacedDirective Reason = "misplaced_directive"
	// ReasonMissingDirective は、要る項目や節が無いことを表す。
	ReasonMissingDirective Reason = "missing_directive"
	// ReasonDuplicate は、ひとつしか書けない項目や節が重なっていることを表す。
	ReasonDuplicate Reason = "duplicate"
	// ReasonNoEndpoint は、どの [Peer] にも Endpoint が無く、繋ぎに行く先が無いことを表す。
	ReasonNoEndpoint Reason = "no_endpoint"
	// ReasonNotInAllowedIPs は、DNS サーバーが、どの [Peer] の AllowedIPs にも含まれない
	// ことを表す。WireGuard はその DNS サーバーへの問い合わせをどこへも送らない。
	ReasonNotInAllowedIPs Reason = "not_in_allowed_ips"
	// ReasonKeepaliveTooLong は、PersistentKeepalive が長すぎることを表す。Limit に上限が入る。
	ReasonKeepaliveTooLong Reason = "keepalive_too_long"
	// ReasonMTUOutOfRange は、MTU が範囲の外にあることを表す。
	ReasonMTUOutOfRange Reason = "mtu_out_of_range"
)

// WireGuard の設定ファイルの節の名前である。
const (
	wireGuardInterfaceSection = "[Interface]"
	wireGuardPeerSection      = "[Peer]"
)

const (
	// MaxWireGuardConfigLength は、設定ファイルの長さの上限（バイト数）である。API の
	// VPNSecrets.wireguardConfig と同じ値にする。[Peer] を上限まで並べても収まる。
	MaxWireGuardConfigLength = 16 << 10
	// maxWireGuardPeers は、[Peer] の数の上限である。API の WireGuardProfile.servers の
	// 上限と同じ値にする。
	maxWireGuardPeers = 64
	// maxWireGuardKeepaliveSeconds は、PersistentKeepalive の上限である。sshc は、最後の
	// ハンドシェイクが 180 秒より古くなったらトンネルが切れたとみなす（container/
	// backend-wireguard.sh）。keepalive がそれより長いと、使っていないだけのトンネルを
	// 切れたと誤る。
	maxWireGuardKeepaliveSeconds = 120
	// defaultWireGuardKeepaliveSeconds は、Endpoint のある [Peer] に PersistentKeepalive が
	// 無いときに使う間隔である。sshc は、ハンドシェイクが済んだことで鍵とサーバーが正しいと
	// 判断する。keepalive が無いと、送るものが無いあいだハンドシェイクが起きない。NAT の
	// 内側からでも経路を保てる長さにする。
	defaultWireGuardKeepaliveSeconds = 25
	// minWireGuardMTU と maxWireGuardMTU は、MTU として受け付ける範囲である。576 は IPv4 の
	// どのホストも受け取れる長さ、65535 は Linux の TUN の上限である。
	minWireGuardMTU = 576
	maxWireGuardMTU = 65535
	// maxPortNumber は、ListenPort の上限である。
	maxPortNumber = 65535
)

// wireGuardConfigField は、設定ファイルの項目の JSON パスである。設定ファイルは鍵を含むので、
// シークレットとして Vault に置く。
const wireGuardConfigField = "secrets." + SecretKeyWireGuardConfig

// WireGuardKey は、設定ファイルに書いた鍵ひとつである。
type WireGuardKey struct {
	// Value は、書いてあった鍵である。シークレットなので []byte で持ち、使い終わったら
	// Forget で消せるようにする。
	Value []byte
	// Line は、鍵を書いた行（1から数える）である。書いていなければ 0。
	Line int
}

// present は、鍵が書いてあるかを返す。
func (key WireGuardKey) present() bool { return key.Line > 0 }

// WireGuardPeer は、[Peer] の節ひとつである。
type WireGuardPeer struct {
	PublicKey    string
	PresharedKey WireGuardKey
	// Endpoint は、`host:port` である。空なら、相手から繋いでくるのを待つ。
	Endpoint   string
	AllowedIPs []netip.Prefix
	// PersistentKeepalive は、秒数である。0 なら書いていないか off。
	PersistentKeepalive int
	// Line は、[Peer] の行（1から数える）である。
	Line int
}

// WireGuardConfig は、読み取った設定ファイルである。
type WireGuardConfig struct {
	// Addresses は、トンネル側で名乗る IPv4 アドレスである（Address）。IPv6 のアドレスは、
	// 接続先が IPv4 だけなので使わない。
	Addresses []netip.Prefix
	// DNS は、VPN の中の DNS サーバー（DNS の IPv4 アドレス）である。
	DNS []string
	// MTU は、トンネルの MTU である。0 なら wireguard-go の既定を使う。
	MTU int
	// ListenPort と FwMark は、wg setconf へそのまま渡す値である。空なら書かない。
	ListenPort string
	FwMark     string
	PrivateKey WireGuardKey
	Peers      []WireGuardPeer
}

// Forget は、読み取った鍵を消す。CLI は設定ファイルを送り終えたらこれを呼ぶ。
func (config WireGuardConfig) Forget() {
	clear(config.PrivateKey.Value)
	for _, peer := range config.Peers {
		clear(peer.PresharedKey.Value)
	}
}

// Servers は、[Peer] の Endpoint のサーバー（`host:port` の host。IPv6 は [] で囲んだまま）を、
// 書かれた順に重複を除いて返す。プロファイルの設定として持ち、一覧に出す。
func (config WireGuardConfig) Servers() []string {
	var servers []string
	for _, peer := range config.Peers {
		if peer.Endpoint == "" {
			continue
		}
		if host := wireGuardEndpointHost(peer.Endpoint); !slices.Contains(servers, host) {
			servers = append(servers, host)
		}
	}
	return servers
}

// wireGuardEndpointHost は、Endpoint（`host:port`）の host を返す。
func wireGuardEndpointHost(endpoint string) string {
	return endpoint[:strings.LastIndexByte(endpoint, ':')]
}

// wireGuardKeyName は、キーの名前と、そのキーを書ける節である。
type wireGuardKeyName struct {
	name string
	// inPeer は、[Peer] に書くキーであることを表す。偽なら [Interface] に書く。
	inPeer bool
}

// wireGuardKeyNames は、読むキーを、小文字にした名前から引く表である。
var wireGuardKeyNames = map[string]wireGuardKeyName{}

func init() {
	for _, name := range []string{
		"PrivateKey", "ListenPort", "FwMark", "Address", "DNS", "MTU", "Table",
		"PreUp", "PostUp", "PreDown", "PostDown", "SaveConfig",
	} {
		wireGuardKeyNames[strings.ToLower(name)] = wireGuardKeyName{name: name}
	}
	for _, name := range []string{"PublicKey", "PresharedKey", "AllowedIPs", "Endpoint", "PersistentKeepalive"} {
		wireGuardKeyNames[strings.ToLower(name)] = wireGuardKeyName{name: name, inPeer: true}
	}
}

// wireGuardCommandKeys は、wg-quick がシェルのコマンドとして実行するキーである。
var wireGuardCommandKeys = map[string]bool{"PreUp": true, "PostUp": true, "PreDown": true, "PostDown": true}

// wireGuardSpaces は、wg が取り除く空白である（ctype.h の char_is_space）。
const wireGuardSpaces = " \t\n\v\f\r"

// wireGuardConfigReader は、設定ファイルを1回読むあいだの状態である。
type wireGuardConfigReader struct {
	config WireGuardConfig
	// section は、いま読んでいる節である（空なら節の前）。
	section string
	// interfaceLine は、[Interface] の行である。無ければ 0。
	interfaceLine int
	// addressLine は、最初の Address の行である。無ければ 0。
	addressLine int
	// dnsLines は、DNS のアドレスごとの、書いた行である。
	dnsLines   map[string]int
	publicKeys map[string]bool
}

// ParseWireGuardConfig は、設定ファイルを wg と同じ読み方で読み、sshc で使えるかを確かめる。
//
// 断るときは *ConfigLineError を返す。
func ParseWireGuardConfig(text []byte) (WireGuardConfig, error) {
	if len(text) > MaxWireGuardConfigLength {
		return WireGuardConfig{}, wireGuardRefusal(configLine{reason: ReasonTooLong, limit: MaxWireGuardConfigLength})
	}
	reader := wireGuardConfigReader{dnsLines: map[string]int{}, publicKeys: map[string]bool{}}
	for index, line := range bytes.Split(text, []byte("\n")) {
		if err := reader.readLine(line, index+1); err != nil {
			reader.config.Forget()
			return WireGuardConfig{}, err
		}
	}
	if err := reader.finish(); err != nil {
		reader.config.Forget()
		return WireGuardConfig{}, err
	}
	return reader.config, nil
}

// wireGuardRefusal は、設定ファイルの中の受け取れない行を表す。
func wireGuardRefusal(refused configLine) *ConfigLineError {
	refused.kind, refused.field = ErrSecrets, wireGuardConfigField
	return newConfigLineError(refused)
}

// cleanWireGuardLine は、注釈を除き、空白をすべて取り除く（config_read_line）。
func cleanWireGuardLine(line []byte) []byte {
	if comment := bytes.IndexByte(line, '#'); comment >= 0 {
		line = line[:comment]
	}
	cleaned := make([]byte, 0, len(line))
	for _, character := range line {
		if strings.IndexByte(wireGuardSpaces, character) < 0 {
			cleaned = append(cleaned, character)
		}
	}
	return cleaned
}

// readLine は、1行を読む。
func (reader *wireGuardConfigReader) readLine(raw []byte, lineNumber int) error {
	// wg は C の文字列として読むので、NUL から先を読まない。
	if bytes.IndexByte(raw, 0) >= 0 {
		return wireGuardRefusal(configLine{reason: ReasonFormat, line: lineNumber})
	}
	line := cleanWireGuardLine(raw)
	// 空白を除いた写しは鍵を含みうる。鍵は readWireGuardKey が写してあるので消す。
	defer clear(line)
	if len(line) == 0 {
		return nil
	}
	if line[0] == '[' {
		return reader.enterSection(string(line), lineNumber)
	}
	separator := bytes.IndexByte(line, '=')
	if separator <= 0 || separator == len(line)-1 {
		return wireGuardRefusal(configLine{reason: ReasonFormat, line: lineNumber})
	}
	known, found := wireGuardKeyNames[strings.ToLower(string(line[:separator]))]
	if !found {
		return wireGuardRefusal(configLine{reason: ReasonUnknownDirective, line: lineNumber})
	}
	value := line[separator+1:]
	if wireGuardCommandKeys[known.name] {
		return wireGuardRefusal(configLine{reason: ReasonRunsCommand, line: lineNumber, directive: known.name})
	}
	expected := wireGuardInterfaceSection
	if known.inPeer {
		expected = wireGuardPeerSection
	}
	if reader.section != expected {
		return wireGuardRefusal(configLine{
			reason: ReasonMisplacedDirective, line: lineNumber, directive: known.name,
		})
	}
	if known.inPeer {
		return reader.readPeerKey(known.name, value, lineNumber)
	}
	return reader.readInterfaceKey(known.name, value, lineNumber)
}

// enterSection は、節の始まりを読む。
func (reader *wireGuardConfigReader) enterSection(name string, lineNumber int) error {
	switch {
	case strings.EqualFold(name, wireGuardInterfaceSection):
		if reader.interfaceLine > 0 {
			return wireGuardRefusal(configLine{
				reason: ReasonDuplicate, line: lineNumber, directive: wireGuardInterfaceSection,
			})
		}
		reader.interfaceLine, reader.section = lineNumber, wireGuardInterfaceSection
	case strings.EqualFold(name, wireGuardPeerSection):
		if len(reader.config.Peers) == maxWireGuardPeers {
			return wireGuardRefusal(configLine{reason: ReasonTooMany, line: lineNumber, limit: maxWireGuardPeers})
		}
		reader.config.Peers = append(reader.config.Peers, WireGuardPeer{Line: lineNumber})
		reader.section = wireGuardPeerSection
	default:
		return wireGuardRefusal(configLine{reason: ReasonUnknownDirective, line: lineNumber})
	}
	return nil
}

// readInterfaceKey は、[Interface] の項目ひとつを読む。
func (reader *wireGuardConfigReader) readInterfaceKey(name string, value []byte, lineNumber int) error {
	refuse := func(reason Reason) error {
		return wireGuardRefusal(configLine{reason: reason, line: lineNumber, directive: name})
	}
	if name == "PrivateKey" {
		if reader.config.PrivateKey.present() {
			return refuse(ReasonDuplicate)
		}
		key, ok := readWireGuardKey(value, lineNumber)
		if !ok {
			return refuse(ReasonFormat)
		}
		reader.config.PrivateKey = key
		return nil
	}
	text := string(value)
	switch name {
	case "ListenPort":
		port, ok := parseDecimal(text)
		if !ok {
			return refuse(ReasonFormat)
		}
		if port > maxPortNumber {
			return refuse(ReasonOutOfRange)
		}
		reader.config.ListenPort = text
	case "FwMark":
		if !strings.EqualFold(text, "off") && !validFwMark(text) {
			return refuse(ReasonFormat)
		}
		reader.config.FwMark = text
	case "Address":
		return reader.readAddresses(text, lineNumber)
	case "DNS":
		return reader.readResolvers(text, lineNumber)
	case "MTU":
		mtu, ok := parseDecimal(text)
		if !ok {
			return refuse(ReasonFormat)
		}
		if mtu < minWireGuardMTU || mtu > maxWireGuardMTU {
			return refuse(ReasonMTUOutOfRange)
		}
		reader.config.MTU = mtu
	case "Table":
		// off は「経路を作らない」で、sshc のすることと同じである。
		if !strings.EqualFold(text, "off") {
			return refuse(ReasonChangesRoutes)
		}
	case "SaveConfig":
		switch strings.ToLower(text) {
		case "false":
		case "true":
			return refuse(ReasonDecidedBySshc)
		default:
			return refuse(ReasonFormat)
		}
	}
	return nil
}

// validFwMark は、wg が FwMark として読める数（10進か、0x で始まる16進の 32 ビット）かを
// 返す（parse_fwmark）。
func validFwMark(value string) bool {
	if hexadecimal, found := strings.CutPrefix(value, "0x"); found {
		_, err := strconv.ParseUint(hexadecimal, 16, 32)
		return err == nil && !strings.ContainsAny(hexadecimal, "+-_")
	}
	number, ok := parseDecimal(value)
	return ok && uint64(number) <= 1<<32-1
}

// parseDecimal は、符号の無い10進の数を読む。wg は符号や空白の付いた数を読まない。
func parseDecimal(text string) (int, bool) {
	if text == "" || strings.Trim(text, "0123456789") != "" {
		return 0, false
	}
	number, err := strconv.Atoi(text)
	return number, err == nil
}

// readAddresses は、Address の並びを読む。IPv4 だけを使い、IPv6 は読み飛ばす。
func (reader *wireGuardConfigReader) readAddresses(value string, lineNumber int) error {
	if reader.addressLine == 0 {
		reader.addressLine = lineNumber
	}
	for _, entry := range strings.Split(value, ",") {
		prefix, ok := parseWireGuardPrefix(entry)
		if !ok {
			return wireGuardRefusal(configLine{reason: ReasonFormat, line: lineNumber, directive: "Address"})
		}
		if !prefix.Addr().Is4() {
			continue
		}
		if unroutableIPv4(prefix.Addr()) {
			return wireGuardRefusal(configLine{reason: ReasonUnroutable, line: lineNumber, directive: "Address"})
		}
		reader.config.Addresses = append(reader.config.Addresses, prefix)
	}
	return nil
}

// readResolvers は、DNS の並びを読む。wg-quick と同じく、数字と点だけのものと : を含む
// ものを DNS サーバー、それ以外を検索ドメインとして読む。使うのは IPv4 の DNS サーバー
// だけである。
func (reader *wireGuardConfigReader) readResolvers(value string, lineNumber int) error {
	refuse := func(reason Reason) error {
		refused := configLine{reason: reason, line: lineNumber, directive: "DNS"}
		if reason == ReasonTooMany {
			refused.limit = maxResolvers
		}
		return wireGuardRefusal(refused)
	}
	for _, entry := range strings.Split(value, ",") {
		if entry == "" {
			return refuse(ReasonFormat)
		}
		if strings.Contains(entry, ":") {
			if address, err := netip.ParseAddr(entry); err != nil || !address.Is6() {
				return refuse(ReasonFormat)
			}
			continue
		}
		if strings.Trim(entry, "0123456789.") != "" {
			continue
		}
		address, err := netip.ParseAddr(entry)
		if err != nil || !address.Is4() {
			return refuse(ReasonFormat)
		}
		if unroutableIPv4(address) {
			return refuse(ReasonUnroutable)
		}
		if len(reader.config.DNS) == maxResolvers {
			return refuse(ReasonTooMany)
		}
		reader.config.DNS = append(reader.config.DNS, entry)
		reader.dnsLines[entry] = lineNumber
	}
	return nil
}

// readPeerKey は、[Peer] の項目ひとつを読む。
func (reader *wireGuardConfigReader) readPeerKey(name string, value []byte, lineNumber int) error {
	peer := &reader.config.Peers[len(reader.config.Peers)-1]
	refuse := func(reason Reason) error {
		refused := configLine{reason: reason, line: lineNumber, directive: name}
		if reason == ReasonKeepaliveTooLong {
			refused.limit = maxWireGuardKeepaliveSeconds
		}
		return wireGuardRefusal(refused)
	}
	if name == "PresharedKey" {
		if peer.PresharedKey.present() {
			return refuse(ReasonDuplicate)
		}
		key, ok := readWireGuardKey(value, lineNumber)
		if !ok {
			return refuse(ReasonFormat)
		}
		peer.PresharedKey = key
		return nil
	}
	text := string(value)
	switch name {
	case "PublicKey":
		if peer.PublicKey != "" {
			return refuse(ReasonDuplicate)
		}
		if !validWireGuardKeyBytes(value) {
			return refuse(ReasonFormat)
		}
		if reader.publicKeys[text] {
			return refuse(ReasonDuplicate)
		}
		reader.publicKeys[text] = true
		peer.PublicKey = text
	case "AllowedIPs":
		for _, entry := range strings.Split(text, ",") {
			prefix, ok := parseWireGuardPrefix(entry)
			if !ok {
				return refuse(ReasonFormat)
			}
			peer.AllowedIPs = append(peer.AllowedIPs, prefix)
		}
	case "Endpoint":
		if !validWireGuardEndpoint(text) {
			return refuse(ReasonFormat)
		}
		peer.Endpoint = text
	case "PersistentKeepalive":
		return readWireGuardKeepalive(peer, text, refuse)
	}
	return nil
}

// readWireGuardKeepalive は、PersistentKeepalive を読む。
func readWireGuardKeepalive(peer *WireGuardPeer, value string, refuse func(Reason) error) error {
	if strings.EqualFold(value, "off") {
		peer.PersistentKeepalive = 0
		return nil
	}
	seconds, ok := parseDecimal(value)
	if !ok {
		return refuse(ReasonFormat)
	}
	if seconds > maxWireGuardKeepaliveSeconds {
		return refuse(ReasonKeepaliveTooLong)
	}
	peer.PersistentKeepalive = seconds
	return nil
}

// readWireGuardKey は、鍵を読む。鍵は文字列にせずに写す。
func readWireGuardKey(value []byte, lineNumber int) (WireGuardKey, bool) {
	if !validWireGuardKeyBytes(value) {
		return WireGuardKey{}, false
	}
	return WireGuardKey{Value: bytes.Clone(value), Line: lineNumber}, true
}

// validWireGuardKeyBytes は、鍵が base64 の 32 バイト（`=` で終わる 44 字）かを返す。鍵を
// 文字列にせずに確かめる。
func validWireGuardKeyBytes(key []byte) bool {
	if len(key) != wireGuardKeyLength || key[len(key)-1] != '=' {
		return false
	}
	for _, character := range key[:len(key)-1] {
		if !isASCIIAlphanumeric(rune(character)) && character != '+' && character != '/' {
			return false
		}
	}
	return true
}

// parseWireGuardPrefix は、アドレスか、アドレスと長さ（CIDR）を読む。長さが無ければ、
// アドレスひとつぶん（/32、/128）とする（wg と ip の読み方）。
func parseWireGuardPrefix(entry string) (netip.Prefix, bool) {
	if !strings.Contains(entry, "/") {
		address, err := netip.ParseAddr(entry)
		if err != nil || address.Zone() != "" {
			return netip.Prefix{}, false
		}
		return netip.PrefixFrom(address, address.BitLen()), true
	}
	prefix, err := netip.ParsePrefix(entry)
	return prefix, err == nil
}

// unroutableIPv4 は、トンネルのアドレスや DNS サーバーに使えないアドレスかを返す。
func unroutableIPv4(address netip.Addr) bool {
	return address.IsUnspecified() || address.IsLoopback() || address.IsMulticast()
}

// validWireGuardEndpoint は、Endpoint が `host:port`（IPv6 は `[address]:port`）かを返す。
func validWireGuardEndpoint(endpoint string) bool {
	separator := strings.LastIndexByte(endpoint, ':')
	if separator <= 0 {
		return false
	}
	host, portText := endpoint[:separator], endpoint[separator+1:]
	port, ok := parseDecimal(portText)
	if !ok || port == 0 || port > maxPortNumber {
		return false
	}
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		address, err := netip.ParseAddr(host[1 : len(host)-1])
		return err == nil && address.Is6()
	}
	// 名前は wg が getaddrinfo で名前解決する。以前の項目の形で受け付けていた名前（VPN
	// サーバーの欄と同じ規則）は、そのまま受け付ける。
	return validateServerName(wireGuardConfigField, host) == nil && !strings.ContainsAny(host, ":[]")
}

// finish は、読み終えた設定ファイルに、要るものが揃っているかを確かめる。
func (reader *wireGuardConfigReader) finish() error {
	config := &reader.config
	missing := func(directive string, line int) error {
		return wireGuardRefusal(configLine{reason: ReasonMissingDirective, line: line, directive: directive})
	}
	if reader.interfaceLine == 0 {
		return missing(wireGuardInterfaceSection, 0)
	}
	if !config.PrivateKey.present() {
		return missing("PrivateKey", 0)
	}
	if reader.addressLine == 0 {
		return missing("Address", 0)
	}
	if len(config.Addresses) == 0 {
		return wireGuardRefusal(configLine{reason: ReasonNotIPv4, line: reader.addressLine, directive: "Address"})
	}
	if len(config.Peers) == 0 {
		return missing(wireGuardPeerSection, 0)
	}
	hasEndpoint := false
	for _, peer := range config.Peers {
		if peer.PublicKey == "" {
			return missing("PublicKey", peer.Line)
		}
		hasEndpoint = hasEndpoint || peer.Endpoint != ""
	}
	if !hasEndpoint {
		return wireGuardRefusal(configLine{reason: ReasonNoEndpoint})
	}
	for _, resolver := range config.DNS {
		if !config.allows(netip.MustParseAddr(resolver)) {
			return wireGuardRefusal(configLine{
				reason: ReasonNotInAllowedIPs, line: reader.dnsLines[resolver], directive: "DNS",
			})
		}
	}
	return nil
}

// allows は、どれかの [Peer] の AllowedIPs が address を含むかを返す。WireGuard は、
// 含まれない宛先へのパケットをどの Peer にも送らない。
func (config WireGuardConfig) allows(address netip.Addr) bool {
	for _, peer := range config.Peers {
		for _, allowed := range peer.AllowedIPs {
			if allowed.Contains(address) {
				return true
			}
		}
	}
	return false
}
