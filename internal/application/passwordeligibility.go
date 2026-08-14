package application

import (
	"crypto/sha256"
	"encoding/hex"
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
	projection := effective.Project(graph, alias)

	if source, off := passwordAuthenticationDisabled(projection); off {
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
	if source, ok := projection.Value("HostName"); ok && strings.TrimSpace(source.Value) != "" {
		host = strings.TrimSpace(source.Value)
	} else {
		report.Warnings = append(report.Warnings, Notice{Code: WarnHostNameUnresolved, Detail: alias})
	}
	report.HostName = host
	if source, ok := projection.Value("Port"); ok {
		if _, err := strconv.Atoi(strings.TrimSpace(source.Value)); err == nil {
			report.Port = strings.TrimSpace(source.Value)
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

func passwordAuthenticationDisabled(projection effective.Projection) (effective.Source, bool) {
	source, ok := projection.Value("PasswordAuthentication")
	return source, ok && strings.EqualFold(strings.TrimSpace(source.Value), "no")
}

// credentialEnvironmentUnsafe reports configuration that can execute or
// spawn another process while SSHC_ASKPASS_TOKEN is present. Environment
// variables are inherited by every child, so such configurations must use
// ordinary interactive OpenSSH rather than receive a bearer capability.
func credentialEnvironmentUnsafe(graph *config.Graph, alias string) bool {
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
// path selected directly by a host block. RelativePath is the vault subject;
// PromptPath is the spelling OpenSSH uses when asking to decrypt that key.
type DirectKeyPassphraseTarget struct {
	RelativePath string
	PromptPath   string
	// ConfigSnapshot is the resolved user configuration with Includes inlined
	// and the one eligible IdentityFile removed. The CLI supplies PromptPath
	// with -i instead. Running ssh with this file disables the unchecked system
	// configuration and prevents a later ~/.ssh symlink change from selecting
	// another private key.
	ConfigSnapshot string
	// Evidence commits the capability to the exact configuration snapshot and
	// private-key bytes observed when the connection was requested.
	Evidence string
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
	if credentialEnvironmentUnsafe(graph, alias) {
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
	relative, promptPath, ok := keys.ResolveWorkspaceKeyPath(s.workspace, values[0])
	if !ok {
		return DirectKeyPassphraseTarget{}, false, nil
	}
	// OpenSSH prints this field through %.100s. A truncated path is not a
	// unique key identity, so it is never eligible for automatic disclosure.
	if len([]byte(promptPath)) > 100 {
		return DirectKeyPassphraseTarget{}, false, nil
	}
	configurationDigest, err := config.Digest(graph)
	if err != nil {
		return DirectKeyPassphraseTarget{}, false, nil
	}
	snapshot, err := config.Snapshot(graph)
	if err != nil {
		return DirectKeyPassphraseTarget{}, false, nil
	}
	frozen, ok := freezeIdentityFile(snapshot, alias)
	if !ok {
		return DirectKeyPassphraseTarget{}, false, nil
	}
	return DirectKeyPassphraseTarget{
		RelativePath:   relative,
		PromptPath:     promptPath,
		ConfigSnapshot: string(frozen),
		Evidence:       configurationDigest,
	}, true, nil
}

// freezeIdentityFile removes the sole effective IdentityFile from the already
// inlined snapshot; the CLI supplies its resolved path through -i. The
// eligibility pass above has proved there is exactly one. Rechecking keeps this
// helper fail-closed if parsing and policy ever drift apart.
func freezeIdentityFile(snapshot []byte, alias string) ([]byte, bool) {
	file := config.Parse(snapshot)
	rewritten := 0
	for index, line := range file.Lines {
		if line.Kind != config.LineDirective || !config.EqualKeyword(line.Keyword, "IdentityFile") {
			continue
		}
		block := file.BlockAt(index)
		if block.Kind == config.BlockHost && !MatchHostLine(block.Patterns, alias) {
			continue
		}
		values := line.Values()
		if len(values) != 1 || strings.EqualFold(strings.TrimSpace(values[0]), "none") {
			return nil, false
		}
		// The CLI supplies this one key with -i. Removing the source directive
		// avoids offering the same encrypted file twice while preserving the
		// original block and every other connection option in the snapshot.
		file.Lines[index] = config.Line{Kind: config.LineBlank, Ending: line.Ending}
		rewritten++
	}
	if rewritten != 1 {
		return nil, false
	}
	return file.Render(), true
}

func executableCredentialDirective(line config.Line) bool {
	switch strings.ToLower(line.Keyword) {
	case "proxycommand", "proxyjump", "knownhostscommand", "localcommand", "remotecommand",
		"pkcs11provider", "securitykeyprovider", "xauthlocation":
		for _, value := range line.Values() {
			if value != "" && !strings.EqualFold(value, "none") {
				return true
			}
		}
	case "sendenv":
		// SendEnv patterns select variables from ssh's own environment and can
		// therefore transmit the bearer capability to a cooperating server.
		for _, pattern := range line.Values() {
			pattern = strings.TrimSpace(pattern)
			if strings.HasPrefix(pattern, "-") {
				continue
			}
			for _, name := range []string{
				"SSHC_ASKPASS_TOKEN", "SSHC_ASKPASS_URL", "SSHC_ASKPASS_ALIAS",
				"SSHC_ASKPASS_KIND", "SSHC_ASKPASS_KEY_PATH",
			} {
				if matched, err := filepath.Match(pattern, name); err == nil && matched {
					return true
				}
			}
		}
	}
	return false
}

func digestKeyTarget(configurationDigest, keyDigest string) string {
	digest := sha256.New()
	digest.Write([]byte(configurationDigest))
	digest.Write([]byte{0})
	digest.Write([]byte(keyDigest))
	return hex.EncodeToString(digest.Sum(nil))
}

// DirectKeyPassphraseTarget additionally requires the resolved path to remain
// a current encrypted private key. A stale vault entry for a replaced or plain
// file must never arm askpass.
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
			target.Evidence = digestKeyTarget(target.Evidence, item.ContentDigest)
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
