package application

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"sshc/internal/storage"
	"sshc/internal/terminal"
	"sshc/internal/textencoding"
)

const (
	// MetadataSchemaVersion はこのビルドが書き込むバージョンである。
	MetadataSchemaVersion = 3
	MetadataFileName      = "metadata.json"
	DefaultGroupsFile     = "groups.sshc.conf"
)

var (
	ErrMetadataVersion  = errors.New("metadata schema version is not supported")
	ErrMetadataSecret   = errors.New("metadata may not contain key material")
	ErrMetadataPath     = errors.New("metadata host path must be relative to the ssh directory")
	ErrMetadataGroup    = errors.New("metadata group definition is invalid")
	ErrMetadataTerminal = errors.New("metadata terminal settings are invalid")
	ErrMetadataEncoding = errors.New("metadata terminal encoding is invalid")
)

var secretMarkers = []string{"-----BEGIN", "PRIVATE KEY", "ssh-rsa ", "ssh-ed25519 ", "ecdsa-sha2-"}

type HostIdentity struct {
	Path  string `json:"path"`
	Alias string `json:"alias"`
}

func (identity HostIdentity) IsZero() bool { return identity.Path == "" || identity.Alias == "" }

// Setting は、グループがメンバーに提供する 1 個のディレクティブである。
type Setting struct {
	Keyword string   `json:"keyword"`
	Values  []string `json:"values"`
}

// HostMetadata は 1 個のホストに付随する、UI 専用の情報である。
type HostMetadata struct {
	Identity HostIdentity `json:"identity"`
	Tags     []string     `json:"tags,omitempty"`
	Colour   string       `json:"colour,omitempty"`
	Note     string       `json:"note,omitempty"`
	Order    int          `json:"order,omitempty"`
	Orphan   bool         `json:"orphan,omitempty"`
	// Appearance は、この接続を開いたときの端末の見た目である。
	Appearance *TerminalAppearance `json:"appearance,omitempty"`
	// Encoding は、接続先との間で使う文字コード。空はUTF-8である。
	Encoding string `json:"encoding,omitempty"`
}

// EngineSettings は、engine そのものの設定である。
type EngineSettings struct {
	Port int `json:"port,omitempty"`
}

// TerminalAppearance は、端末の見た目の選択である。
type TerminalAppearance struct {
	Palette string `json:"palette,omitempty"`
	Font    string `json:"font,omitempty"`
	// Background は、置いてある画像の名前である。中身は運ばない。
	Background string `json:"background,omitempty"`
	// BackgroundTint は、画像の上にかぶせる濃さ（0〜100）である。
	BackgroundTint *int `json:"backgroundTint,omitempty"`
}

// Empty は、何も選ばれていないことを返す。
func (appearance TerminalAppearance) Empty() bool { return appearance == TerminalAppearance{} }

func (host HostMetadata) Alias() string { return host.Identity.Alias }

// GroupMetadata は 1 個のグループ名に付随する見た目（presentation）である。
type GroupMetadata struct {
	Name     string    `json:"name"`
	Colour   string    `json:"colour,omitempty"`
	Note     string    `json:"note,omitempty"`
	Order    int       `json:"order,omitempty"`
	Hidden   bool      `json:"hidden,omitempty"`
	Settings []Setting `json:"settings,omitempty"`
}

// EmbeddedTerminal は、埋め込みターミナルの設定である。
type EmbeddedTerminal struct {
	MaxSessions     int `json:"maxSessions,omitempty"`
	ScrollbackBytes int `json:"scrollbackBytes,omitempty"`
	// FontSize は画面が字を描く大きさである。この engine は使わない。
	FontSize  int `json:"fontSize,omitempty"`
	Verbosity int `json:"verbosity,omitempty"`
	// Reconnect は、輸送が落ちたときに繋ぎ直しを試みる回数である。
	Reconnect       *int  `json:"reconnect,omitempty"`
	CopyOnSelect    *bool `json:"copyOnSelect,omitempty"`
	RightClickPaste *bool `json:"rightClickPaste,omitempty"`
	// StartDirectory は、ローカルシェルが始まる場所である。
	StartDirectory string `json:"startDirectory,omitempty"`
	// Appearance は、どの接続にも選ばれていないときの見た目である。
	Appearance *TerminalAppearance `json:"appearance,omitempty"`
}

// Metadata は~/.ssh/sshc/metadata.json の全体である。
type Metadata struct {
	SchemaVersion    int               `json:"schemaVersion"`
	GroupsFile       string            `json:"groupsFile,omitempty"`
	EmbeddedTerminal *EmbeddedTerminal `json:"embeddedTerminal,omitempty"`
	// Engine は engine そのものの設定である。端末のものではない。
	Engine *EngineSettings `json:"engine,omitempty"`
	Groups []GroupMetadata `json:"groups,omitempty"`
	Hosts  []HostMetadata  `json:"hosts,omitempty"`
}

func (metadata Metadata) TerminalStartDirectory() string {
	if metadata.EmbeddedTerminal == nil {
		return ""
	}
	return metadata.EmbeddedTerminal.StartDirectory
}

// TerminalLimits は、保存された設定を埋め込みターミナルの用語へ移す。
func (metadata Metadata) TerminalLimits() terminal.Limits {
	if metadata.EmbeddedTerminal == nil {
		return terminal.DefaultLimits()
	}
	return terminal.Limits{
		MaxSessions: metadata.EmbeddedTerminal.MaxSessions,
		Scrollback:  metadata.EmbeddedTerminal.ScrollbackBytes,
	}.Normalise()
}

func NewMetadata() Metadata {
	return Metadata{SchemaVersion: MetadataSchemaVersion, GroupsFile: DefaultGroupsFile}
}

// GroupsPath は設定されたグループファイルを返し、無ければデフォルトに fallback する。
func (metadata Metadata) GroupsPath() string {
	if metadata.GroupsFile == "" {
		return DefaultGroupsFile
	}
	return metadata.GroupsFile
}

func DecodeMetadata(contents []byte) (Metadata, error) {
	if len(strings.TrimSpace(string(contents))) == 0 {
		return NewMetadata(), nil
	}
	var metadata Metadata
	if err := json.Unmarshal(contents, &metadata); err != nil {
		return Metadata{}, err
	}
	if metadata.SchemaVersion != MetadataSchemaVersion {
		return Metadata{}, ErrMetadataVersion
	}
	if metadata.GroupsFile == "" {
		metadata.GroupsFile = DefaultGroupsFile
	}
	return metadata, nil
}

// EncodeMetadata は metadata を検証し、決定的にシリアライズする。
func EncodeMetadata(metadata Metadata) ([]byte, error) {
	metadata.SchemaVersion = MetadataSchemaVersion
	if metadata.GroupsFile == "" {
		metadata.GroupsFile = DefaultGroupsFile
	}
	if err := ValidateMetadata(metadata); err != nil {
		return nil, err
	}
	sorted := metadata
	sorted.Groups = append([]GroupMetadata(nil), metadata.Groups...)
	sorted.Hosts = append([]HostMetadata(nil), metadata.Hosts...)
	sort.SliceStable(sorted.Groups, func(first, second int) bool {
		return sorted.Groups[first].Name < sorted.Groups[second].Name
	})
	sort.SliceStable(sorted.Hosts, func(first, second int) bool {
		if sorted.Hosts[first].Identity.Path != sorted.Hosts[second].Identity.Path {
			return sorted.Hosts[first].Identity.Path < sorted.Hosts[second].Identity.Path
		}
		return sorted.Hosts[first].Identity.Alias < sorted.Hosts[second].Identity.Alias
	})
	encoded, err := json.MarshalIndent(sorted, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

// ValidateMetadata は設計の不変条件を破る文書を拒否する。
func ValidateMetadata(metadata Metadata) error {
	if settings := metadata.EmbeddedTerminal; settings != nil {
		if settings.MaxSessions != 0 &&
			(settings.MaxSessions < terminal.MinMaxSessions || settings.MaxSessions > terminal.MaxMaxSessions) {
			return fmt.Errorf("%w: maxSessions %d", ErrMetadataTerminal, settings.MaxSessions)
		}
		if settings.ScrollbackBytes != 0 &&
			(settings.ScrollbackBytes < terminal.MinScrollback || settings.ScrollbackBytes > terminal.MaxScrollback) {
			return fmt.Errorf("%w: scrollbackBytes %d", ErrMetadataTerminal, settings.ScrollbackBytes)
		}
		if settings.FontSize != 0 &&
			(settings.FontSize < terminal.MinFontSize || settings.FontSize > terminal.MaxFontSize) {
			return fmt.Errorf("%w: fontSize %d", ErrMetadataTerminal, settings.FontSize)
		}
	}
	if _, err := checkRelative(metadata.GroupsPath()); err != nil {
		return err
	}
	names := make(map[string]bool, len(metadata.Groups))
	for _, group := range metadata.Groups {
		if names[strings.ToLower(group.Name)] || ValidateGroupName(group.Name) != nil {
			return ErrMetadataGroup
		}
		names[strings.ToLower(group.Name)] = true
		for _, setting := range group.Settings {
			if containsSecretMarker(setting.Keyword) {
				return ErrMetadataSecret
			}
			for _, value := range setting.Values {
				if containsSecretMarker(value) {
					return ErrMetadataSecret
				}
			}
		}
	}
	for _, host := range metadata.Hosts {
		if _, err := checkRelative(host.Identity.Path); err != nil {
			return err
		}
		if host.Identity.Alias == "" {
			return ErrMetadataPath
		}
		if host.Encoding != "" {
			canonical, err := textencoding.Parse(host.Encoding)
			if err != nil || string(canonical) != host.Encoding {
				return ErrMetadataEncoding
			}
		}
		for _, text := range append([]string{host.Note, host.Colour}, host.Tags...) {
			if containsSecretMarker(text) {
				return ErrMetadataSecret
			}
		}
	}
	return nil
}

func checkRelative(candidate string) (string, error) {
	if candidate == "" || strings.HasPrefix(candidate, "/") {
		return "", ErrMetadataPath
	}
	cleaned := filepath.ToSlash(filepath.Clean(filepath.FromSlash(candidate)))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", ErrMetadataPath
	}
	return cleaned, nil
}

func containsSecretMarker(text string) bool {
	for _, marker := range secretMarkers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

type MetadataStore struct {
	workspace *storage.Workspace
}

func NewMetadataStore(workspace *storage.Workspace) *MetadataStore {
	return &MetadataStore{workspace: workspace}
}

func (store *MetadataStore) Path() string {
	return filepath.Join(store.workspace.StateDir(), MetadataFileName)
}

func (store *MetadataStore) EnsureDirectory() error {
	return store.workspace.EnsureDirectory(store.workspace.StateDir())
}

// Load は現在の文書と、後の commit が必要とする事前条件を読む。
func (store *MetadataStore) Load() (Metadata, storage.Precondition, error) {
	contents, err := store.workspace.FileSystem().ReadFile(store.Path())
	if errors.Is(err, fs.ErrNotExist) {
		return NewMetadata(), storage.Precondition{}, nil
	}
	if err != nil {
		return Metadata{}, storage.Precondition{}, err
	}
	metadata, err := DecodeMetadata(contents)
	if err != nil {
		return Metadata{}, storage.Precondition{}, err
	}
	return metadata, storage.Precondition{Exists: true, Digest: storage.Digest(contents)}, nil
}

// Change は metadata を storage transaction 用の 1 個のファイル変更に変える。
func (store *MetadataStore) Change(metadata Metadata, precondition storage.Precondition) (storage.Change, error) {
	contents, err := EncodeMetadata(metadata)
	if err != nil {
		return storage.Change{}, err
	}
	return storage.Change{Path: store.Path(), Contents: contents, Precondition: precondition}, nil
}

func ReconcileMetadata(metadata Metadata, present []HostIdentity) (Metadata, []Notice) {
	known := make(map[HostIdentity]bool, len(present))
	for _, identity := range present {
		known[identity] = true
	}
	reconciled := metadata
	reconciled.Hosts = append([]HostMetadata(nil), metadata.Hosts...)
	var notices []Notice
	for index := range reconciled.Hosts {
		host := &reconciled.Hosts[index]
		host.Orphan = !known[host.Identity]
		if !host.Orphan {
			continue
		}
		notices = appendNotice(notices, Notice{
			Code:   NoticeOrphanMetadata,
			Path:   host.Identity.Path,
			Detail: host.Identity.Alias,
		})
	}
	return reconciled, notices
}

func saysNothing(host HostMetadata) bool {
	host.Identity = HostIdentity{}
	host.Orphan = false
	return reflect.DeepEqual(host, HostMetadata{})
}

// 内容のない entry は削除する。
func ClearHostNote(metadata Metadata, identity HostIdentity) Metadata {
	cleared := metadata
	cleared.Hosts = make([]HostMetadata, 0, len(metadata.Hosts))
	for _, host := range metadata.Hosts {
		if host.Identity != identity {
			cleared.Hosts = append(cleared.Hosts, host)
			continue
		}
		host.Note = ""
		if saysNothing(host) {
			continue
		}
		cleared.Hosts = append(cleared.Hosts, host)
	}
	return cleared
}

func RelocateHostIdentities(metadata Metadata, fromPath, toPath string) Metadata {
	relocated := metadata
	relocated.Hosts = append([]HostMetadata(nil), metadata.Hosts...)
	for index := range relocated.Hosts {
		if relocated.Hosts[index].Identity.Path != fromPath {
			continue
		}
		relocated.Hosts[index].Identity.Path = toPath
		relocated.Hosts[index].Orphan = false
	}
	return relocated
}

func RenameHostIdentity(metadata Metadata, from, to HostIdentity) Metadata {
	renamed := metadata
	renamed.Hosts = append([]HostMetadata(nil), metadata.Hosts...)
	for index := range renamed.Hosts {
		if renamed.Hosts[index].Identity != from {
			continue
		}
		renamed.Hosts[index].Identity = to
		renamed.Hosts[index].Orphan = false
	}
	return renamed
}
