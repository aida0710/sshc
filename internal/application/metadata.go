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
)

const (
	// MetadataSchemaVersion はこのビルドが書き込む唯一のバージョンである。
	// より高いバージョンを持つファイルは黙ってダウングレードされず拒否される。
	//
	// バージョン 2 でグループメンバーシップを廃止した。グループは ~/.ssh/connections
	// 配下のディレクトリであり、エントリファイルの生成領域がどのグループが
	// 存在するかを宣言するので、hosts[].group にも groups[].parent にも
	// 語ることはもう何も残っていない。バージョン 1 の文書はデコードされ、その
	// 2 つのフィールドを単に失う。json.Unmarshal は構造体にもう存在しないものを無視するからだ。
	//
	// バージョン 3 で端末の選択を廃止した。端末はこのアプリケーションの中で開く
	// ようになり、外部の端末アプリケーションを起こす経路そのものが無くなったので、
	// terminal と customTerminal には語ることが残っていない。バージョン 2 の文書は
	// 同じやり方でデコードされ、その 2 つを失う。
	//
	// **新しい設定に terminal というキーは使えない。** バージョン 2 の文書はそこに
	// 文字列を持っており、同じキーをオブジェクトへ変えると json.Unmarshal は文書
	// 全体で失敗する。グループも色もお気に入りも道連れに読めなくなるので、
	// 埋め込みターミナルの設定は embeddedTerminal という別のキーに置く。
	MetadataSchemaVersion = 3
	// MetadataFileName はワークスペースの状態ディレクトリに置かれ、設定ツリーには
	// 置かれないので、SSH 設定として読まれることは決してない。
	MetadataFileName = "metadata.json"
	// DefaultGroupsFile はグループ継承がコンパイルされる先の設定ファイルで
	// ある。設定ツリーの内側にとどまるので、手で編集できる普通の
	// OpenSSH 設定である。
	DefaultGroupsFile = "groups.sshc.conf"
)

var (
	ErrMetadataVersion  = errors.New("metadata schema version is newer than this build supports")
	ErrMetadataSecret   = errors.New("metadata may not contain key material")
	ErrMetadataPath     = errors.New("metadata host path must be relative to the ssh directory")
	ErrMetadataGroup    = errors.New("metadata group definition is invalid")
	ErrMetadataTerminal = errors.New("metadata terminal settings are invalid")
)

// secretMarkers は、整理用のフィールドに鍵材料を保存しようとした痕跡を示す
// 部分文字列である。metadata には鍵のためのフィールドは無いので、出現した
// こと自体が大声で拒否する価値のある間違いである。
var secretMarkers = []string{"-----BEGIN", "PRIVATE KEY", "ssh-rsa ", "ssh-ed25519 ", "ecdsa-sha2-"}

// HostIdentity はホストの UI 上の識別子である。正規化された設定ファイル
// のパスと、そこで宣言された具体的な主 Host alias を組にしたものだ。
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
//
// 設定そのものの中に表現を持たないものだけを運ぶ。グループのメンバーシップは
// ファイルが置かれているディレクトリであり、note は Host 行の上のコメントな
// ので、どちらもここには無い。
type HostMetadata struct {
	Identity  HostIdentity `json:"identity"`
	Tags      []string     `json:"tags,omitempty"`
	Colour    string       `json:"colour,omitempty"`
	Note      string       `json:"note,omitempty"`
	Favourite bool         `json:"favourite,omitempty"`
	Order     int          `json:"order,omitempty"`
	Orphan    bool         `json:"orphan,omitempty"`
	// Appearance は、この接続を開いたときの端末の見た目である。
	//
	// **ポインタなのは、空の節を書かないためである。** encoding/json の
	// omitempty は構造体には効かない——値で持つと、何も選んでいない接続にも
	// `"appearance":{}` が並ぶ。
	Appearance *TerminalAppearance `json:"appearance,omitempty"`
}

// EngineSettings は、engine そのものの設定である。
//
// **受け口の番号は起動時にしか読まれない。** 変えても、次に engine を起こすまで
// 効かない——画面はそう言う。
//
// 0 は「決めていない」であり、そのとき engine は 30000〜60000 から無作為に引く。
// **番号は秘密ではない** ——走査すれば見つかるし handoff にも書いてある。固定
// できることは安全を下げる取引ではない。ただし固定した番号は先に握られうるので、
// 既定は無作為のままにしてある。
type EngineSettings struct {
	Port int `json:"port,omitempty"`
}

// TerminalAppearance は、端末の見た目の選択である。
//
// **名前で持ち、色そのものは持たない。** 配色と字体の中身は画面が一度だけ
// 定義する。engine はこれを読まない——PTY は色を知らない。
//
// **知らない名前は既定へ戻す。** 手で書かれた綴りひとつが、タグもお気に入りも
// 道連れに読めなくしてよい理由はない。TerminalLimits が範囲外の数字にしている
// のと同じ扱いである。
type TerminalAppearance struct {
	Palette string `json:"palette,omitempty"`
	Font    string `json:"font,omitempty"`
	// Background は、置いてある画像の名前である。**中身は運ばない。**
	Background string `json:"background,omitempty"`
	// BackgroundTint は、画像の上にかぶせる濃さ（0〜100）である。
	//
	// **ポインタなのは、0 が有効な選択だからである。** 「かぶせない」と
	// 「選んでいない」は違う——値で持つと、素のまま見たい人の選択が、
	// 上の段の設定に毎回上書きされる。
	BackgroundTint *int `json:"backgroundTint,omitempty"`
}

// Empty は、何も選ばれていないことを答える。
func (appearance TerminalAppearance) Empty() bool { return appearance == TerminalAppearance{} }

func (host HostMetadata) Alias() string { return host.Identity.Alias }

// GroupMetadata は 1 個のグループ名に付随する見た目（presentation）である。
// グループ自体はディレクトリであり、その階層構造がその名前そのものなので、これは
// ディレクトリが運べないものである。Settings は普通の Host ブロックへとコンパイルされる。
type GroupMetadata struct {
	Name   string `json:"name"`
	Colour string `json:"colour,omitempty"`
	Note   string `json:"note,omitempty"`
	Order  int    `json:"order,omitempty"`
	// Hidden は、他のグループを保持することが目的で、connections ツリーに
	// それ自体として示すものが何もないグループについて、その見出しを
	// connections ツリーから除く。Colour や Order と同様にこれは見た目であり、
	// このエンジンはそれを運ぶだけで決して読まない。Include 行も、
	// ディレクトリも、ssh が返すあらゆる答えも手つかずのままである。
	Hidden   bool      `json:"hidden,omitempty"`
	Settings []Setting `json:"settings,omitempty"`
}

// EmbeddedTerminal は、埋め込みターミナルの設定である。
//
// 0 は「書かれていない」であって「0 本」ではない。読み取り側は範囲の外の値と
// 同じように既定へ戻す。
type EmbeddedTerminal struct {
	MaxSessions     int `json:"maxSessions,omitempty"`
	ScrollbackBytes int `json:"scrollbackBytes,omitempty"`
	// FontSize は画面が字を描く大きさである。**この engine は使わない**——
	// PTY は px を知らない。持っているのは、端末ごとに読みやすい大きさが
	// 違うからで、指で持つ画面と机の上の画面では同じ数字が別の意味になる。
	FontSize int `json:"fontSize,omitempty"`
	// nil は既定の on、false は明示的な off である。bool に omitempty を直接
	// 付けると false が消え、再起動後に on へ戻ってしまう。
	// Verbosity は、接続の途中経過をどこまで端末へ書くかである。0 は無言。
	Verbosity int `json:"verbosity,omitempty"`
	// Reconnect は、輸送が落ちたときに繋ぎ直しを試みる回数である。
	//
	// **ポインタなのは、0 が有効な選択だからである。** 「繋ぎ直さない」と
	// 「選んでいない」は違う——値で持つと、切ったつもりの人が既定に戻される。
	// 範囲の外は、読むときに既定へ戻す。
	Reconnect       *int  `json:"reconnect,omitempty"`
	CopyOnSelect    *bool `json:"copyOnSelect,omitempty"`
	RightClickPaste *bool `json:"rightClickPaste,omitempty"`
	// StartDirectory は、ローカルシェルが始まる場所である。
	//
	// 空は「書かれていない」であり、そのとき始まるのは home である。
	// **エンジンの作業ディレクトリは継がない**——あれはエンジンを起こした
	// ものがたまたま居た場所で、利用者はそれを選んでいない。
	//
	// `~` の綴りのまま持つ。**home の綴りを設定に焼き付けない**ので、
	// 別の機械へ持って行っても同じ意味になる。
	StartDirectory string `json:"startDirectory,omitempty"`
	// Appearance は、どの接続にも選ばれていないときの見た目である。
	// **接続ごとの選択の方が強い。**
	Appearance *TerminalAppearance `json:"appearance,omitempty"`
}

// Metadata は~/.ssh/sshc/metadata.json の全体である。
type Metadata struct {
	SchemaVersion int    `json:"schemaVersion"`
	GroupsFile    string `json:"groupsFile,omitempty"`
	// EmbeddedTerminal はポインタである。書かれていない文書と、既定と同じ値が
	// 明示的に書かれた文書を、書き戻すときに区別できるようにするためだ。
	EmbeddedTerminal *EmbeddedTerminal `json:"embeddedTerminal,omitempty"`
	// Engine は engine そのものの設定である。**端末のものではない。**
	//
	// ポインタなのは、書かれていない文書と、既定と同じ値が明示的に書かれた文書を
	// 区別するためである。
	Engine *EngineSettings `json:"engine,omitempty"`
	Groups []GroupMetadata `json:"groups,omitempty"`
	Hosts  []HostMetadata  `json:"hosts,omitempty"`
}

// TerminalStartDirectory は、ローカルシェルが始まる場所を、書かれたままの
// 綴りで返す。空なら書かれていない。
func (metadata Metadata) TerminalStartDirectory() string {
	if metadata.EmbeddedTerminal == nil {
		return ""
	}
	return metadata.EmbeddedTerminal.StartDirectory
}

// TerminalLimits は、保存された設定を埋め込みターミナルの語彙へ移す。
//
// 範囲の外は拒否ではなく既定へ戻す。ここは読み取りであり、手で書かれた数字ひとつが
// 色もタグもお気に入りも道連れに読めなくしてよい理由はない。書き込み側は
// ValidateMetadata が範囲の外を拒否し続ける。
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

// DecodeMetadata は metadata.json をパースする。内容が無いか空であれば
// 新規の文書を作り、より新しいスキーマバージョンは書き換えず拒否する。
func DecodeMetadata(contents []byte) (Metadata, error) {
	if len(strings.TrimSpace(string(contents))) == 0 {
		return NewMetadata(), nil
	}
	var metadata Metadata
	if err := json.Unmarshal(contents, &metadata); err != nil {
		return Metadata{}, err
	}
	if metadata.SchemaVersion > MetadataSchemaVersion {
		return Metadata{}, ErrMetadataVersion
	}
	if metadata.SchemaVersion == 0 {
		metadata.SchemaVersion = MetadataSchemaVersion
	}
	if metadata.GroupsFile == "" {
		metadata.GroupsFile = DefaultGroupsFile
	}
	// **読んだものを書き換えない。** 範囲の外を既定へ戻すのは TerminalLimits()
	// の仕事であり、あちらは読むたびに戻す——ここでも戻すと、戻した値が
	// 構造体に残り、次に何かを保存したときにファイルへ焼き付く。
	//
	// 実際そうなっていた。開始位置を二度保存すると、利用者が一度も選んで
	// いない maxSessions と scrollbackBytes が metadata に現れる。**既定を
	// 設定ファイルへ書くと、既定を変えた日にその人だけ取り残される。**
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
	// 上限は範囲の中でなければ書けない。読み取りが既定へ戻すのとは対称ではない
	// ——書き込みはこのアプリケーション自身の操作であり、そこに範囲外が現れたら
	// それは利用者の古いファイルではなく、こちらの間違いだからである。
	//
	// **0 は「書かれていない」である。** この節には上限以外のものも入るので
	// (開始位置)、上限に触れずにそこだけを書く文書が成立する。0 を範囲外として
	// 断ると、その文書が書けない。読み取り側は前からそう読んでいる。
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
		// 名前はディレクトリパスなので、このアプリケーションが自ら作成
		// してよいと思えるものでなければならない。ここで拒否することで、
		// 手で編集された metadata ファイルが"../escape"を名乗り、信じ込まされることを防ぐ。
		if names[strings.ToLower(group.Name)] || ValidateGroupName(group.Name) != nil {
			return ErrMetadataGroup
		}
		// 大文字小文字を区別しない。大文字小文字だけが異なる 2 つのグループ名は、
		// デフォルトの macOS ボリュームでは 1 つのディレクトリになるからだ。
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

// MetadataStore はワークスペースの状態ディレクトリ内で metadata.json を
// 読み、ステージする。直接書き込むことは決してない。変更は、それが記述する
// 設定ファイルを書き込むのと同じ storage transaction によってコミットされる。
type MetadataStore struct {
	workspace *storage.Workspace
}

func NewMetadataStore(workspace *storage.Workspace) *MetadataStore {
	return &MetadataStore{workspace: workspace}
}

func (store *MetadataStore) Path() string {
	return filepath.Join(store.workspace.StateDir(), MetadataFileName)
}

// EnsureDirectory は状態ディレクトリを作成し、最初の metadata 書き込みが
// その親を解決できるようにする。
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

// ReconcileMetadata はホストが消えた entry に印を付ける。entry を別の
// ホストに向け直すことは決してない。消えたターゲットは orphan となり、
// ユーザーが意図的に再関連付けしなければならない。
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

// ClearHostNote は 1 個のホストの entry から note を取り除き、他のすべての
// フィールドと他のすべての entry は手つかずのままにする。
//
// note とコメントは 2 箇所に書かれた同じものなので、コメントを保存すると
// そのホストの note は同じ transaction で退役する。ユーザーが編集するたびに
// ホストごとにこれを行うことで、すべてのファイルを一度に書き換える
// migration を介さずに収束する。identity しか残っていない entry は
// saysNothing は、その entry に残しておく価値が無いことを答える。
//
// **数え上げない。** ここはかつて `len(Tags) == 0 && Colour == "" && !Favourite
// && Order == 0` と項目を並べていた。並べると、増やした項目をここへ足し忘れた
// 日に、**note を消しただけでその設定ごと消える。** 見た目を足したときが
// まさにそれだった。
//
// identity は entry の宛名なので数に入れない。orphan は「向こうが消えた」と
// いう観測であって、人が語ったことではない——それだけが残った entry は、
// 以前と同じく捨てる。
func saysNothing(host HostMetadata) bool {
	host.Identity = HostIdentity{}
	host.Orphan = false
	return reflect.DeepEqual(host, HostMetadata{})
}

// 捨てられる。何も語らない entry は残しておく価値が無いからだ。
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

// RenameHostIdentity は 1 個のホストの entry を移動し、他のすべての entry は
// 手つかずのままにする。呼び出し側は、rename を実行した設定変更と
// 同じ transaction でその結果をコミットする。
// RelocateHostIdentities は 1 個のファイルに宣言されたすべての entry の
// パスを書き換え、各 alias は保つ。これは、ちょうど 1 個の identity を
// 移動する RenameHostIdentity のファイル単位版である。
//
// 書き換えられるのは完全一致したパスだけである。前方一致にすると、"work"への rename が
// "workshop"を飲み込んでしまう。ディレクトリを移動する呼び出し側は各ファイルを渡す。
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
