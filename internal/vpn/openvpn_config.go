package vpn

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
)

// OpenVPN の設定ファイル（.ovpn）を、コンテナへ渡す前に確かめる。
//
// 設定ファイルは Vault に置くシークレットである。プロバイダや組織が配るものは、鍵を
// インラインのブロック（<key>…</key> など）で含むことが多い。ここでは中身を
// OpenVPN と同じ読み方で読み、次のものを断る。
//
//   - コマンドやプログラムを実行する指示（up、plugin など）。コンテナの中で任意の
//     コマンドが動き、fail closed の約束を破りうる。
//   - 経路や DNS を変える指示（route、redirect-gateway など）。経路は sshc が接続先ごとに
//     /32 で足す。
//   - sshc が決める指示（dev、daemon、log、management など）。
//   - コンテナの中に無いファイルを指す指示（ca のファイル名など）。インラインのブロックは
//     受け付ける。
//   - VPN サーバー用の指示（server、mode など）。
//
// 読み方は OpenVPN 2.6 の src/openvpn/options.c（parse_line、read_config_file、
// check_inline_file、add_option の setenv opt）に合わせる。読み方がずれると、ここで
// 見えない指示が OpenVPN には見える。画面の web/src/vpn/openVPNConfig.ts は同じ規則の
// 写しで、testdata/openvpn-config-cases.json に対するテストで揃える。

// 設定ファイルを受け取れない理由の語である。画面と CLI が翻訳する。
const (
	// ReasonRunsCommand は、コマンドやプログラムを実行する指示であることを表す。
	ReasonRunsCommand Reason = "runs_command"
	// ReasonChangesRoutes は、経路や DNS を変える指示であることを表す。
	ReasonChangesRoutes Reason = "changes_routes"
	// ReasonDecidedBySshc は、sshc が決める指示であることを表す。
	ReasonDecidedBySshc Reason = "decided_by_sshc"
	// ReasonFileReference は、コンテナの中に無いファイルを指す指示であることを表す。
	ReasonFileReference Reason = "file_reference"
	// ReasonServerMode は、VPN サーバー用の指示であることを表す。
	ReasonServerMode Reason = "server_mode"
	// ReasonUnsupportedInline は、インラインにできない指示をブロックで書いたことを表す。
	ReasonUnsupportedInline Reason = "unsupported_inline"
	// ReasonUnclosedInline は、インラインのブロックに終わりの行が無いことを表す。
	ReasonUnclosedInline Reason = "unclosed_inline"
	// ReasonNotClient は、client も tls-client も無く、クライアントの設定ではないことを表す。
	ReasonNotClient Reason = "not_client"
	// ReasonNoRemote は、繋ぐ先（remote）が無いことを表す。
	ReasonNoRemote Reason = "no_remote"
	// ReasonConfigMismatch は、プロファイルに書いたサーバーが、設定ファイルの remote と
	// 合わないことを表す。
	ReasonConfigMismatch Reason = "config_mismatch"
	// ReasonRequiredByConfig は、設定ファイルがユーザー名とパスワードを求めているのに、
	// プロファイルにユーザー名が無いことを表す。
	ReasonRequiredByConfig Reason = "required_by_config"
)

const (
	// maxOpenVPNLineLength は、1行の長さの上限（改行を除くバイト数）である。OpenVPN は
	// 256 バイトの領域（OPTION_LINE_SIZE、改行を含む）で1行を読み、それより長い行は
	// 途中で切って続きを次の行として読む。切れ目から始まる指示はここから見えないので、
	// 切れない長さだけを受け付ける。インラインのブロックの中も同じ読み方をする。
	maxOpenVPNLineLength = 254
	// maxOpenVPNParameters は、1行から読む語の数の上限である（MAX_PARMS）。
	maxOpenVPNParameters = 16
	// maxOpenVPNRemotes は、remote の数の上限である。OpenVPN が持てる接続先の数
	// （CONNECTION_LIST_SIZE）と同じにする。
	maxOpenVPNRemotes = 64
)

// openVPNConfigField は、設定ファイルの項目の JSON パスである。
const openVPNConfigField = "secrets." + SecretKeyOpenVPNConfig

// ConfigLineError は、設定ファイルの中の、受け取れない行である。項目の誤りに、何行目の
// どの指示かを添える。
type ConfigLineError struct {
	FieldError
	// Line は、1から数えた行番号である。ファイル全体についての誤りなら 0。
	Line int
	// Directive は、断った指示の名前である。下の表にある名前だけを入れる。設定ファイルは
	// シークレットなので、表に無い語は応答に載せない。
	Directive string
}

func (failure *ConfigLineError) Error() string {
	return fmt.Sprintf("%s: line %d: %s", failure.FieldError.Error(), failure.Line, failure.Directive)
}

func (failure *ConfigLineError) Unwrap() error { return &failure.FieldError }

func configLineError(reason Reason, line int, directive string) *ConfigLineError {
	return &ConfigLineError{
		FieldError: FieldError{Kind: ErrSecrets, Field: openVPNConfigField, Reason: reason},
		Line:       line, Directive: directive,
	}
}

// refusedOpenVPNDirectives は、引数に関係なく断る指示と、その理由である。
//
// OpenVPN 2.6 の man（script-options、generic-options、client-options、
// vpn-network-options、log-options、management-options、tls-options、
// inline-files）で確かめた。
var refusedOpenVPNDirectives = map[string]Reason{
	// 外部のコマンドを実行する。
	"up": ReasonRunsCommand, "down": ReasonRunsCommand, "route-up": ReasonRunsCommand,
	"route-pre-down": ReasonRunsCommand, "ipchange": ReasonRunsCommand, "tls-verify": ReasonRunsCommand,
	"client-connect": ReasonRunsCommand, "client-disconnect": ReasonRunsCommand,
	"client-crresponse": ReasonRunsCommand, "learn-address": ReasonRunsCommand,
	"auth-user-pass-verify": ReasonRunsCommand, "tls-crypt-v2-verify": ReasonRunsCommand,
	// ip の代わりに動かすコマンドを指定する。
	"iproute": ReasonRunsCommand,
	// 共有ライブラリを読み込む。
	"plugin": ReasonRunsCommand, "engine": ReasonRunsCommand, "pkcs11-providers": ReasonRunsCommand,
	// sshc が自分の up のためだけに 2 にする。
	"script-security": ReasonRunsCommand,

	"route": ReasonChangesRoutes, "route-ipv6": ReasonChangesRoutes,
	"redirect-gateway": ReasonChangesRoutes, "redirect-private": ReasonChangesRoutes,
	// 送る・受けるパケットのアドレスを書き換える。
	"client-nat": ReasonChangesRoutes,

	// トンネルの interface は sshc が決める。dev と dev-type は、sshc と同じ tun なら
	// 受け付ける（下の openVPNDirectiveReason）。
	"dev-node": ReasonDecidedBySshc, "lladdr": ReasonDecidedBySshc,
	"mktun": ReasonDecidedBySshc, "rmtun": ReasonDecidedBySshc,
	// ログは agent が読んで接続できたかを判断する。別の場所へ出されると読めない。
	"daemon": ReasonDecidedBySshc, "log": ReasonDecidedBySshc, "log-append": ReasonDecidedBySshc,
	"syslog": ReasonDecidedBySshc,
	// コンテナの中にファイルを書く。
	"status": ReasonDecidedBySshc, "writepid": ReasonDecidedBySshc, "tmp-dir": ReasonDecidedBySshc,
	// 実行する環境を変える。
	"chroot": ReasonDecidedBySshc, "cd": ReasonDecidedBySshc, "user": ReasonDecidedBySshc,
	"group": ReasonDecidedBySshc, "setcon": ReasonDecidedBySshc,
	// パスワードやパスフレーズをコンソールから読む。コンテナにはコンソールが無い。
	"askpass": ReasonDecidedBySshc,

	// コンテナの中に無いファイルを読む、またはファイルを書く。
	"config": ReasonFileReference, "capath": ReasonFileReference, "tls-export-cert": ReasonFileReference,
	"replay-persist": ReasonFileReference, "genkey": ReasonFileReference,

	"mode": ReasonServerMode, "server": ReasonServerMode, "server-ipv6": ReasonServerMode,
	"server-bridge": ReasonServerMode, "tls-server": ReasonServerMode,
}

// openVPNInlineDirectives は、インラインのブロックで書ける指示である（man の
// INLINE FILE SUPPORT）。ファイル名で書いたものは断る。値が true のものは、ファイル名
// ではない引数（フィンガープリントやハッシュ）も受け付ける。
var openVPNInlineDirectives = map[string]bool{
	"ca": false, "cert": false, "dh": false, "extra-certs": false, "key": false, "pkcs12": false,
	"secret": false, "crl-verify": false, "http-proxy-user-pass": false, "tls-auth": false,
	"auth-gen-token-secret": false, "tls-crypt": false, "tls-crypt-v2": false,
	"peer-fingerprint": true, "verify-hash": true,
	// 引数の無い auth-user-pass は受け付ける（sshc がユーザー名とパスワードを渡す）。
	"auth-user-pass": false,
}

// openVPNManagementPrefix は、管理用の口（management とその仲間）の指示の頭である。
// 管理用の口は sshc が開かない。
const openVPNManagementPrefix = "management"

// tunDevice は、sshc と同じ tun を指す dev の値である（tun、または tun と番号）。
var tunDevice = regexp.MustCompile(`^tun[0-9]*$`)

// httpProxyAutoCredentials は、http-proxy の3つ目の引数のうち、ファイルを指さないものである。
var httpProxyAutoCredentials = map[string]bool{"auto": true, "auto-nct": true}

// OpenVPNConfigSummary は、設定ファイルから読み取った、シークレットでない事実である。
type OpenVPNConfigSummary struct {
	// Servers は、remote の行のサーバー（名前またはアドレス）を、書かれた順に、重複を
	// 除いて並べたものである。
	Servers []string
	// AsksCredentials は、引数の無い auth-user-pass が有効であることを表す。OpenVPN は
	// ユーザー名とパスワードをコンソールから読もうとするので、sshc が渡す必要がある。
	AsksCredentials bool
}

// openVPNConfigReader は、設定ファイルを1回読むあいだの状態である。
type openVPNConfigReader struct {
	lines   [][]byte
	summary OpenVPNConfigSummary
	// isClient は、client か tls-client があったことを表す。
	isClient bool
	remotes  int
	seen     map[string]bool
}

// InspectOpenVPNConfig は、設定ファイルを OpenVPN と同じ読み方で読み、sshc で使えるかを
// 確かめる。使えれば、シークレットでない事実を返す。
//
// 断るときは *ConfigLineError を返す。最初に断る行を返すので、画面と同じ行を指す。
func InspectOpenVPNConfig(config []byte) (OpenVPNConfigSummary, error) {
	reader := openVPNConfigReader{lines: splitConfigLines(config), seen: map[string]bool{}}
	if err := reader.checkLines(); err != nil {
		return OpenVPNConfigSummary{}, err
	}
	if err := reader.readBlock(0, len(reader.lines)); err != nil {
		return OpenVPNConfigSummary{}, err
	}
	if !reader.isClient {
		return OpenVPNConfigSummary{}, configLineError(ReasonNotClient, 0, "")
	}
	if reader.remotes == 0 {
		return OpenVPNConfigSummary{}, configLineError(ReasonNoRemote, 0, "")
	}
	return reader.summary, nil
}

// splitConfigLines は、改行で行に分ける。OpenVPN はストリームの先頭の UTF-8 の BOM を
// 読み飛ばす。
func splitConfigLines(config []byte) [][]byte {
	config = bytes.TrimPrefix(config, []byte("\xEF\xBB\xBF"))
	lines := bytes.Split(config, []byte("\n"))
	// 最後の改行のあとの空の行は、行として数えない。
	if len(lines) > 0 && len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// checkLines は、OpenVPN と読み方がずれうる行を断る。
func (reader *openVPNConfigReader) checkLines() error {
	for index, line := range reader.lines {
		if len(line) > maxOpenVPNLineLength {
			failure := configLineError(ReasonTooLong, index+1, "")
			failure.Limit = maxOpenVPNLineLength
			return failure
		}
		// OpenVPN は C の文字列として読むので、NUL から先を読まない。
		if bytes.IndexByte(line, 0) >= 0 {
			return configLineError(ReasonFormat, index+1, "")
		}
	}
	return nil
}

// readBlock は、lines[start:end] を指示の並びとして読む。
func (reader *openVPNConfigReader) readBlock(start, end int) error {
	for index := start; index < end; index++ {
		lineNumber := index + 1
		words, ok := splitOpenVPNLine(reader.lines[index])
		if !ok {
			return configLineError(ReasonFormat, lineNumber, "")
		}
		if len(words) == 0 {
			continue
		}
		words[0] = withoutDoubleDash(words[0])
		tag, isInline := inlineTag(words)
		if !isInline {
			if err := reader.readDirective(words, lineNumber); err != nil {
				return err
			}
			continue
		}
		closing := reader.closingLine(tag, index+1, end)
		if closing < 0 {
			return configLineError(ReasonUnclosedInline, lineNumber, "")
		}
		if err := reader.readInline(inlineBlock{tag: tag, open: index, close: closing}); err != nil {
			return err
		}
		index = closing
	}
	return nil
}

// inlineBlock は、<tag> から </tag> までのブロックひとつである。open と close は行の
// 添字である。
type inlineBlock struct {
	tag         string
	open, close int
}

// readInline は、インラインのブロックを確かめる。<connection> の中は指示の並びとして読む。
//
// <connection> の中に <connection> を書いても、外側のブロックは最初の </connection> で
// 終わるので、内側のブロックは閉じないまま終わる（OpenVPN も同じく断る）。
func (reader *openVPNConfigReader) readInline(block inlineBlock) error {
	lineNumber := block.open + 1
	if reason, refused := refusedOpenVPNDirectives[block.tag]; refused {
		return configLineError(reason, lineNumber, block.tag)
	}
	if block.tag == "connection" {
		return reader.readBlock(block.open+1, block.close)
	}
	if _, inlineCapable := openVPNInlineDirectives[block.tag]; !inlineCapable {
		return configLineError(ReasonUnsupportedInline, lineNumber, "")
	}
	if block.tag == "auth-user-pass" {
		// ユーザー名とパスワードは設定ファイルが持っている。
		reader.summary.AsksCredentials = false
	}
	return nil
}

// closingLine は、</tag> の行の添字を返す。無ければ -1。
//
// OpenVPN は、行の頭の空白を飛ばしたあとが </tag> で始まる行を終わりとみなす
// （read_inline_file）。
func (reader *openVPNConfigReader) closingLine(tag string, from, end int) int {
	closing := []byte("</" + tag + ">")
	for index := from; index < end; index++ {
		if bytes.HasPrefix(bytes.TrimLeft(reader.lines[index], openVPNSpaces), closing) {
			return index
		}
	}
	return -1
}

// readDirective は、指示ひとつを確かめ、分かったことを summary に足す。
func (reader *openVPNConfigReader) readDirective(words []string, lineNumber int) error {
	words = withoutSetenvOpt(words)
	if len(words) == 0 {
		return nil
	}
	name, arguments := words[0], words[1:]
	if reason, refused := openVPNDirectiveReason(name, arguments); refused {
		return configLineError(reason, lineNumber, name)
	}
	switch name {
	case "client", "tls-client":
		reader.isClient = true
	case "auth-user-pass":
		reader.summary.AsksCredentials = true
	case "remote":
		return reader.addRemote(arguments, lineNumber)
	}
	return nil
}

// addRemote は、remote の行のサーバーを覚える。
func (reader *openVPNConfigReader) addRemote(arguments []string, lineNumber int) error {
	if len(arguments) == 0 || validateServerName(openVPNConfigField, arguments[0]) != nil {
		return configLineError(ReasonFormat, lineNumber, "remote")
	}
	reader.remotes++
	if reader.remotes > maxOpenVPNRemotes {
		failure := configLineError(ReasonTooMany, lineNumber, "remote")
		failure.Limit = maxOpenVPNRemotes
		return failure
	}
	server := arguments[0]
	if !reader.seen[server] {
		reader.seen[server] = true
		reader.summary.Servers = append(reader.summary.Servers, server)
	}
	return nil
}

// openVPNDirectiveReason は、指示を断るなら、その理由を返す。
func openVPNDirectiveReason(name string, arguments []string) (Reason, bool) {
	if reason, refused := refusedOpenVPNDirectives[name]; refused {
		return reason, true
	}
	if strings.HasPrefix(name, openVPNManagementPrefix) {
		return ReasonDecidedBySshc, true
	}
	switch name {
	case "dev":
		return ReasonDecidedBySshc, len(arguments) > 0 && !tunDevice.MatchString(arguments[0])
	case "dev-type":
		return ReasonDecidedBySshc, len(arguments) > 0 && arguments[0] != "tun"
	case "http-proxy":
		// http-proxy <サーバー> <ポート> [<ファイル>|auto|auto-nct] [<認証方式>]
		return ReasonFileReference, len(arguments) >= 3 && !httpProxyAutoCredentials[arguments[2]]
	case "socks-proxy":
		// socks-proxy <サーバー> [<ポート>] [<ファイル>]
		return ReasonFileReference, len(arguments) >= 3
	}
	if acceptsValue, inlineCapable := openVPNInlineDirectives[name]; inlineCapable && !acceptsValue {
		return ReasonFileReference, len(arguments) > 0
	}
	return "", false
}

// withoutSetenvOpt は、`setenv opt` の頭を外す。OpenVPN は、この頭の付いた指示を、
// 知らない指示でも断らずに、ふつうの指示として使う（add_option）。頭を外して確かめ
// ないと、`setenv opt up …` が素通りする。
func withoutSetenvOpt(words []string) []string {
	for len(words) >= 2 && words[0] == "setenv" && words[1] == "opt" {
		words = words[2:]
		if len(words) > 0 {
			words[0] = withoutDoubleDash(words[0])
		}
	}
	return words
}

// withoutDoubleDash は、指示の頭の `--` を外す（bypass_doubledash）。設定ファイルでは
// `--up` も `up` と同じ指示である。
func withoutDoubleDash(word string) string {
	if len(word) >= 3 && strings.HasPrefix(word, "--") {
		return word[2:]
	}
	return word
}

// inlineTag は、行が <tag> だけなら、その tag を返す（check_inline_file）。
func inlineTag(words []string) (string, bool) {
	if len(words) != 1 {
		return "", false
	}
	word := words[0]
	if len(word) < 2 || word[0] != '<' || word[len(word)-1] != '>' {
		return "", false
	}
	return word[1 : len(word)-1], true
}

// openVPNSpaces は、OpenVPN が空白とみなす字である（C の isspace）。
const openVPNSpaces = " \t\n\v\f\r"

func isOpenVPNSpace(character byte) bool {
	return character == 0 || strings.IndexByte(openVPNSpaces, character) >= 0
}

// openVPNWordState は、1行を語に分けるあいだの状態である（parse_line の STATE_*）。
type openVPNWordState int

const (
	betweenWords openVPNWordState = iota
	inUnquotedWord
	inDoubleQuotedWord
	inSingleQuotedWord
)

// splitOpenVPNLine は、1行を語に分ける。OpenVPN の parse_line と同じ規則で読む。
//
//   - 語は空白で区切る。二重引用符と一重引用符で空白を含められる。
//   - バックスラッシュは、一重引用符の外で、次の字（\、"、空白）をそのまま使わせる。
//     それ以外の字の前のバックスラッシュは誤りである。
//   - 語の始まりの位置にある # と ; から先は注釈である。
//
// 読めない行（閉じていない引用符など）は false を返す。OpenVPN もその行を使わない。
func splitOpenVPNLine(line []byte) ([]string, bool) {
	var words []string
	var word []byte
	state := betweenWords
	backslash := false
	// 行の終わりは NUL として扱い、読みかけの語を閉じる（C の文字列の終わり）。
	for index := 0; index <= len(line); index++ {
		var in byte
		if index < len(line) {
			in = line[index]
		}
		if !backslash && in == '\\' && state != inSingleQuotedWord {
			backslash = true
			continue
		}
		var out byte
		done := false
		switch state {
		case betweenWords:
			if !isOpenVPNSpace(in) {
				if in == ';' || in == '#' {
					return words, true
				}
				switch {
				case !backslash && in == '"':
					state = inDoubleQuotedWord
				case !backslash && in == '\'':
					state = inSingleQuotedWord
				default:
					out, state = in, inUnquotedWord
				}
			}
		case inUnquotedWord:
			if !backslash && isOpenVPNSpace(in) {
				done = true
			} else {
				out = in
			}
		case inDoubleQuotedWord:
			if !backslash && in == '"' {
				done = true
			} else {
				out = in
			}
		case inSingleQuotedWord:
			if in == '\'' {
				done = true
			} else {
				out = in
			}
		}
		if done {
			words = append(words, string(word))
			word, state = word[:0], betweenWords
		}
		if backslash && out != 0 && out != '\\' && out != '"' && !isOpenVPNSpace(out) {
			return nil, false
		}
		backslash = false
		if out != 0 {
			word = append(word, out)
		}
		if len(words) >= maxOpenVPNParameters {
			return words, true
		}
	}
	return words, state == betweenWords
}
