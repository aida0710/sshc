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
	BlockerPasswordAuthenticationOff = "password_authentication_off"
	BlockerAliasNotSimple            = "alias_not_simple"
	BlockerIdentityFileConfigured    = "identity_file_configured"
	// WarnHostKeyUnknown は、known_hosts にホスト鍵が登録されていないことを示す。
	WarnHostKeyUnknown     = "host_key_unknown"
	WarnHostNameUnresolved = "hostname_unresolved"
)

type PasswordEligibility struct {
	Alias    string   `json:"alias"`
	Storable bool     `json:"storable"`
	Blockers []Notice `json:"blockers"`
	Warnings []Notice `json:"warnings"`
	HostName string   `json:"hostName,omitempty"`
	Port     string   `json:"port,omitempty"`
}

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

// passwordAuthenticationDisabled は、この alias で password 方式が閉じているかを返す。
func passwordAuthenticationDisabled(resolution effective.Resolution) (effective.Accepted, bool) {
	entry, ok := acceptedDirective(resolution, "PasswordAuthentication")
	return entry, ok && strings.EqualFold(strings.TrimSpace(firstValue(entry)), "no")
}

// WorkspaceKeys は、この alias が使う、ワークスペースの中にある秘密鍵を返す。
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
