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

	"sshc/internal/effective"
	"sshc/internal/remoteos"
	"sshc/internal/sshclient"
	"sshc/internal/storage"
	"sshc/internal/terminal"
	"sshc/internal/textencoding"
	"sshc/internal/vpn"
)

const (
	// MetadataSchemaVersion はこのビルドが書き込むバージョンである。
	MetadataSchemaVersion = 9
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
	ErrMetadataOSC52    = errors.New("metadata OSC 52 policy is invalid")
	// ErrMetadataDuplicateHost は、同じ接続の entry が 2 つあることを表す。schema 9 より
	// 前の形で、移行（DecodeMetadata）だけが 1 つにする。
	ErrMetadataDuplicateHost = errors.New("metadata has two entries for one connection")
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
	Identity          HostIdentity `json:"identity"`
	Tags              []string     `json:"tags,omitempty"`
	Colour            string       `json:"colour,omitempty"`
	Note              string       `json:"note,omitempty"`
	Order             int          `json:"order,omitempty"`
	OS                string       `json:"os,omitempty"`
	DetectedOS        string       `json:"detectedOS,omitempty"`
	DetectedOSBinding string       `json:"detectedOSBinding,omitempty"`
	Orphan            bool         `json:"orphan,omitempty"`
	// Appearance は、この接続を開いたときの端末の見た目である。
	Appearance *TerminalAppearance `json:"appearance,omitempty"`
	// Encoding は、接続先との間で使う文字コード。空はUTF-8である。
	Encoding string `json:"encoding,omitempty"`
	// OSC52 は、このSSH接続だけのclipboard policyである。空は全体設定を継ぐ。
	OSC52 string `json:"osc52,omitempty"`
	// VPN は、この接続へ届くために通るVPNプロファイルの名前である。
	//
	// 空ならVPNを通らない。ssh_configには書かない。OpenSSHが解釈する語では
	// なく、sshcだけが持つ紐付けだからである。
	VPN string `json:"vpn,omitempty"`
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

// api/openapi.yaml の TerminalAppearance・TerminalSettings と同じ上限。契約は読み取り
// 応答にも掛かるので、範囲外を書き込ませると GET /api/v1/metadata が契約違反になる。
const (
	maxAppearanceNameLength = 64
	MaxBackgroundNameLength = 128
	maxBackgroundTint       = 100
	maxStartDirectoryLength = 4096
)

func validateAppearance(appearance *TerminalAppearance) error {
	if appearance == nil {
		return nil
	}
	if len(appearance.Palette) > maxAppearanceNameLength || len(appearance.Font) > maxAppearanceNameLength {
		return fmt.Errorf("%w: appearance name", ErrMetadataTerminal)
	}
	if len(appearance.Background) > MaxBackgroundNameLength {
		return fmt.Errorf("%w: appearance background", ErrMetadataTerminal)
	}
	if tint := appearance.BackgroundTint; tint != nil && (*tint < 0 || *tint > maxBackgroundTint) {
		return fmt.Errorf("%w: backgroundTint %d", ErrMetadataTerminal, *tint)
	}
	return nil
}

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

// FileTransferSettings は、SFTP転送キューの設定である。
type FileTransferSettings struct {
	MaxConcurrent              int   `json:"maxConcurrent,omitempty"`
	ClearCompletedAfterSeconds int   `json:"clearCompletedAfterSeconds,omitempty"`
	ProcessingStopped          bool  `json:"processingStopped,omitempty"`
	LargeFileThresholdBytes    int64 `json:"largeFileThresholdBytes,omitempty"`
	LargeFileParallelism       int   `json:"largeFileParallelism,omitempty"`
	LargeFileChunkBytes        int64 `json:"largeFileChunkBytes,omitempty"`
}

// BackgroundSettings は背景画像ライブラリの保存方針である。
// 画像のバイト列は変換せず、ここでは利用者が許可する合計容量だけを持つ。
type BackgroundSettings struct {
	CapacityMiB int `json:"capacityMiB,omitempty"`
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
	// WebGL は nil なら既定の有効、false なら明示的にDOM描画を使う。
	WebGL *bool `json:"webgl,omitempty"`
	// BrowserScrollbackLines はbrowserのxtermが保持する表示行数である。
	// ScrollbackBytesはengineが再接続時に再生するbyte ringで、用途が異なる。
	BrowserScrollbackLines int `json:"browserScrollbackLines,omitempty"`
	// OSC52は全接続の既定。HostMetadata.OSC52が設定されていればそちらを優先する。
	OSC52 bool `json:"osc52,omitempty"`
	// JISYenBackslashはJIS配列の¥キーをbackslashとして端末へ送る。
	JISYenBackslash bool `json:"jisYenBackslash,omitempty"`
	// LocalShellProfileは検出済みprofileの安定IDであり、command文字列ではない。
	LocalShellProfile string `json:"localShellProfile,omitempty"`
	// StartDirectory は、ローカルシェルが始まる場所である。
	StartDirectory string `json:"startDirectory,omitempty"`
	// Appearance は、どの接続にも選ばれていないときの見た目である。
	Appearance *TerminalAppearance `json:"appearance,omitempty"`
}

// Metadata は~/.ssh/sshc/metadata.json の全体である。
type Metadata struct {
	ShortcutPresets  []ShortcutPreset  `json:"shortcutPresets,omitempty"`
	SchemaVersion    int               `json:"schemaVersion"`
	GroupsFile       string            `json:"groupsFile,omitempty"`
	EmbeddedTerminal *EmbeddedTerminal `json:"embeddedTerminal,omitempty"`
	// FileTransfers は SFTP 転送キューの設定である。
	FileTransfers *FileTransferSettings `json:"fileTransfers,omitempty"`
	// Backgrounds は端末設定の保存で巻き戻らない独立したライブラリ設定である。
	Backgrounds *BackgroundSettings `json:"backgrounds,omitempty"`
	// VPNProfiles は、接続ごとに通せるVPN経路の定義である。秘密はVaultにあり、
	// ここには無い。
	VPNProfiles []VPNProfile    `json:"vpnProfiles,omitempty"`
	Groups      []GroupMetadata `json:"groups,omitempty"`
	Hosts       []HostMetadata  `json:"hosts,omitempty"`
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
	var version struct {
		SchemaVersion int `json:"schemaVersion"`
	}
	if err := json.Unmarshal(contents, &version); err != nil {
		return Metadata{}, err
	}
	switch version.SchemaVersion {
	case MetadataSchemaVersion, 8, 7, 6, 5, 4, 3:
	default:
		return Metadata{}, ErrMetadataVersion
	}
	var metadata Metadata
	if err := json.Unmarshal(contents, &metadata); err != nil {
		return Metadata{}, err
	}
	fillWireGuardServers(&metadata)
	// v3/v4→v5は追加fieldだけのmigrationである。v5は同期するキー設定を旧バージョンで消さないための境界。
	// 旧scrollbackBytesはengineの
	// replay bufferとして意味を変えず、browser側は未設定の既定行数から始める。
	// v5→v6は、VPNプロファイルの接続先（vpnProfiles[].target）を捨てる。接続先は
	// プロファイルを付けた接続のHostNameとPortで決まる。知らない項目として読み
	// 飛ばすので、書き直すと消える。v6はtargetの無いプロファイルを旧バージョンに読ませない
	// ための境界でもある。
	// v6→v7は項目を足すだけである。v7は、v0.39.7までのsshcが知らないVPNの方式
	// （openvpn、ikev2）のプロファイルを旧バージョンに読ませないための境界である。旧バージョンは知らない
	// 方式を断るので、バージョンで先に断る方が、何が起きたかが利用者に分かる。
	// v7→v8は、WireGuardのプロファイルに、Endpointのサーバー（wireguard.servers）を足す。
	// 設定ファイルはVaultにあり、項目の形（server、peerPublicKey、address）のプロファイルは
	// 保存し直すまで項目のまま読む（metadata_wireguard.go）。v8は、項目の無い設定ファイルの
	// 形のプロファイルを旧バージョンに読ませないための境界である。
	// v8→v9は、IKEv2のサーバーのIDのうち、どのサーバーにも一致する値（`*`、`0.0.0.0`
	// など）を外す（metadata_ikev2.go）。VPNプロファイルのbackendと違う節も外す
	// （metadata_vpnsections.go）。v9は、空白や日本語を含むVPNプロファイル名を
	// 旧バージョンに読ませないための境界でもある。旧バージョンはその名前の metadata を
	// 書けなくなる。グループ設定の ProxyCommand などを複数の値で保存した前の形は、
	// 行の残りの 1 つの値にする（metadata_groupsettings.go）。同じ接続の entry が 2 つ
	// あれば 1 つにする（metadata_duplicatehosts.go）。sshc エンジンの設定（engine 節）は
	// 読まない。このマシンの設定として engine-settings.json へ移すのは、初めて起動した
	// ときの InitialiseEngineSettings である（metadata_engine.go）。engine 節は、書き直すと
	// 消える。VPN プロファイルに、名前から決めた識別子を与える（vpnprofile_id.go）。
	if version.SchemaVersion < 9 {
		clearUnpinnedServerIdentities(&metadata)
		clearForeignVPNSections(&metadata)
		joinSplitRestOfLineSettings(&metadata)
		keepOneEntryPerConnection(&metadata)
		giveVPNProfilesMigratedIDs(&metadata)
	}
	metadata.SchemaVersion = MetadataSchemaVersion
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
	sorted.VPNProfiles = append([]VPNProfile(nil), metadata.VPNProfiles...)
	sort.SliceStable(sorted.VPNProfiles, func(first, second int) bool {
		return sorted.VPNProfiles[first].Name < sorted.VPNProfiles[second].Name
	})
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
	if err := validateShortcutPresets(metadata.ShortcutPresets); err != nil {
		return err
	}
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
		if settings.BrowserScrollbackLines != 0 &&
			(settings.BrowserScrollbackLines < terminal.MinBrowserScrollbackLines ||
				settings.BrowserScrollbackLines > terminal.MaxBrowserScrollbackLines) {
			return fmt.Errorf("%w: browserScrollbackLines %d", ErrMetadataTerminal, settings.BrowserScrollbackLines)
		}
		if settings.LocalShellProfile != "" && !validShellProfileID(settings.LocalShellProfile) {
			return fmt.Errorf("%w: localShellProfile", ErrMetadataTerminal)
		}
		// 上限は接続ログの段の数で決まるので、sshclient.MaxVerbosity を見る。api/openapi.yaml の
		// EmbeddedTerminal・TerminalSettings の verbosity の maximum もこの値にそろえる。
		if settings.Verbosity < 0 || settings.Verbosity > sshclient.MaxVerbosity {
			return fmt.Errorf("%w: verbosity %d", ErrMetadataTerminal, settings.Verbosity)
		}
		if settings.Reconnect != nil && (*settings.Reconnect < 0 || *settings.Reconnect > terminal.MaxReconnects) {
			return fmt.Errorf("%w: reconnect %d", ErrMetadataTerminal, *settings.Reconnect)
		}
		if len(settings.StartDirectory) > maxStartDirectoryLength {
			return fmt.Errorf("%w: startDirectory", ErrMetadataTerminal)
		}
		if err := validateAppearance(settings.Appearance); err != nil {
			return err
		}
	}
	if settings := metadata.Backgrounds; settings != nil {
		if settings.CapacityMiB < MinBackgroundCapacityMiB || settings.CapacityMiB > MaxBackgroundCapacityMiB {
			return fmt.Errorf("%w: background capacity %d", ErrMetadataTerminal, settings.CapacityMiB)
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
			// ProxyCommand などは行の残りを 1 つの値で持つ（metadata_groupsettings.go）。
			// 複数の値は schema 9 より前の形で、読み込みの移行だけが 1 つの値にする。
			// 今の形で受け付けると、空白でつないで書いた行と、同じ値を前の形として
			// 移行した行とが食い違う。
			if effective.TakesRestOfLine(setting.Keyword) && len(setting.Values) > 1 {
				return fmt.Errorf("%w: %s takes the rest of the line as one value", ErrMetadataGroup, setting.Keyword)
			}
		}
	}
	if err := validateVPNProfiles(metadata.VPNProfiles); err != nil {
		return err
	}
	identities := make(map[HostIdentity]bool, len(metadata.Hosts))
	for _, host := range metadata.Hosts {
		if _, err := checkRelative(host.Identity.Path); err != nil {
			return err
		}
		if identities[host.Identity] {
			return fmt.Errorf("%w: %s %s", ErrMetadataDuplicateHost, host.Identity.Path, host.Identity.Alias)
		}
		identities[host.Identity] = true
		if host.VPN != "" {
			// 名前の形だけを見る。指している先があるかは、繋ぐときに確かめる。
			// 参照が外れただけで metadata 全体を保存できなくしない。
			if err := vpn.ValidateName(host.VPN); err != nil {
				return fmt.Errorf("%w: %w", ErrMetadataVPN, err)
			}
		}
		if host.Identity.Alias == "" {
			return ErrMetadataPath
		}
		if !remoteos.Valid(host.OS) || !remoteos.Valid(host.DetectedOS) {
			return errors.New("metadata operating system is invalid")
		}
		if host.Encoding != "" {
			canonical, err := textencoding.Parse(host.Encoding)
			if err != nil || string(canonical) != host.Encoding {
				return ErrMetadataEncoding
			}
		}
		if host.OSC52 != "" && host.OSC52 != "allow" && host.OSC52 != "deny" {
			return ErrMetadataOSC52
		}
		if err := validateAppearance(host.Appearance); err != nil {
			return err
		}
		for _, text := range append([]string{host.Note, host.Colour}, host.Tags...) {
			if containsSecretMarker(text) {
				return ErrMetadataSecret
			}
		}
	}
	return nil
}

func validShellProfileID(value string) bool {
	if len(value) > 64 {
		return false
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '-' {
			continue
		}
		return false
	}
	return value != ""
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

// hostMetadataIndex は、識別子 identity の entry の位置を返す。無ければ -1 を返す。
func hostMetadataIndex(hosts []HostMetadata, identity HostIdentity) int {
	for index, host := range hosts {
		if host.Identity == identity {
			return index
		}
	}
	return -1
}

// RelocateHostIdentities は、fromPath の entry をすべて toPath へ付け直す。toPath に
// 同じ別名の entry が残っていれば（多くは接続先が消えた orphan）捨てる。残すと同じ
// 識別子の entry が 2 つになる。
func RelocateHostIdentities(metadata Metadata, fromPath, toPath string) Metadata {
	moving := map[string]bool{}
	for _, host := range metadata.Hosts {
		if host.Identity.Path == fromPath {
			moving[host.Identity.Alias] = true
		}
	}
	relocated := metadata
	relocated.Hosts = make([]HostMetadata, 0, len(metadata.Hosts))
	for _, host := range metadata.Hosts {
		switch {
		case host.Identity.Path == fromPath:
			host.Identity.Path = toPath
			host.Orphan = false
		case host.Identity.Path == toPath && moving[host.Identity.Alias]:
			continue
		}
		relocated.Hosts = append(relocated.Hosts, host)
	}
	return relocated
}

// RenameHostIdentity は、from の entry を to へ付け直す。to に entry が残っていれば
// （多くは接続先が消えた orphan）捨てる。残すと同じ識別子の entry が 2 つになる。
// from に entry が無ければ何も付け直さないので、to の entry もそのまま残す。
func RenameHostIdentity(metadata Metadata, from, to HostIdentity) Metadata {
	moving := hostMetadataIndex(metadata.Hosts, from) >= 0
	renamed := metadata
	renamed.Hosts = make([]HostMetadata, 0, len(metadata.Hosts))
	for _, host := range metadata.Hosts {
		switch {
		case host.Identity == from:
			host.Identity = to
			host.Orphan = false
		case moving && host.Identity == to:
			continue
		}
		renamed.Hosts = append(renamed.Hosts, host)
	}
	return renamed
}
