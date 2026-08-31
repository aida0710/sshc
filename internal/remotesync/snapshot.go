// Package remotesync は、ワークスペース全体をオブジェクトストアへ運び、また戻す。
//
// このパッケージが sync ではなく remotesync という名前なのは、標準ライブラリが
// すでにその名前を持っており、ここでミューテックスが必要なファイルが、二つのうち
// どちらかに別名を付けなければならなくなるのを避けるためである。
//
// オブジェクトひとつ、スナップショットひとつ、原子的な PUT ひとつ。オブジェクトを
// ファイルごとに分ければファイル単位の衝突検出がただで手に入るが、それは誤りだ。
// 設定は集合としてしか意味を持たないからである。~/.ssh/config は
// "Include connections/work/*.conf" と言い、metadata.json はディレクトリを持たねば
// ならないグループを指定し、IdentityFile 行は鍵ファイルを指定する。ファイルを
// 独立にアップロードすれば、どのマシンにも存在しなかった状態、まだ存在しない
// ファイルに到達する Include をリモートが保持するウィンドウができ、そのウィンドウの中で pull
// したマシンは、単に古いだけでなく整合してもいない設定を受け取ることに
// なる。
package remotesync

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"

	"sshc/internal/storage"
)

// ManifestName は、すべてのスナップショットの最初のエントリ。
const ManifestName = "manifest.json"

// SchemaVersion は、マニフェスト文書のバージョン。
//
// バージョン 5 は、revision、parent、messageを必須契約とする。過去形式を読み分ける
// 分岐は持たず、この版が書く形式だけを受け付ける。
const SchemaVersion = 5

// MaxCommitMessageRunes は、履歴へ保存する一行メッセージの最大長。
const MaxCommitMessageRunes = 240

// MaxSnapshotBytes は、Read が展開する量に上限を設ける。スナップショットは ~/.ssh
// である。これに近づくものは、ワークスペースではなく展開爆弾だ。
const MaxSnapshotBytes = 64 << 20

// MaxEntries は、ひとつのスナップショットが運べるファイル数に上限を設ける。
const MaxEntries = 4096

// MaxManifestAncestors bounds authenticated ancestry carried by new snapshots.
// This avoids opening one Argon2-protected history object per missed update.
const MaxManifestAncestors = 50

var (
	// ErrNotASnapshot は、そもそもスナップショットでないバイト列を報告する。
	ErrNotASnapshot = errors.New("these bytes are not an sshc snapshot")
	// ErrUnsupportedVersion は、このビルドと異なるschemaのスナップショットを報告する。
	ErrUnsupportedVersion = errors.New("this snapshot schema is not supported")
	// ErrUnsafePath は、ワークスペースから抜け出すパスを持つエントリを報告する。
	// スナップショットは信用できない入力であり、tar の中の "../" は最も古い手口で
	// ある。
	ErrUnsafePath = errors.New("a snapshot entry names a path outside the workspace")
	// ErrUnsafeMode は、このアプリケーションが書かない権限ビットを持つエントリを報告
	// する。スナップショットが秘密鍵の権限を広げられてはならない。
	ErrUnsafeMode = errors.New("a snapshot entry has permissions this application does not write")
	// ErrSnapshotTooLarge は、上限を超えるスナップショットを報告する。
	ErrSnapshotTooLarge = errors.New("the snapshot is larger than this application will read")
	// ErrManifestMismatch は、ダイジェストがマニフェストと一致しないファイル、または
	// マニフェストに載っていないファイルを報告する。
	ErrManifestMismatch = errors.New("the snapshot's files do not match its manifest")
	// ErrCommitMessage は、長すぎる、または複数行のコミットメッセージを報告する。
	ErrCommitMessage = errors.New("the snapshot commit message is not valid")
)

// Entry は、スナップショット内のファイルひとつを記述する。
type Entry struct {
	// Path はワークスペース相対でスラッシュ区切り。storage.Workspace がすでに使って
	// いる用語である。
	Path string `json:"path"`
	// SHA256 は内容の 16 進ダイジェスト。これにより pull は、すべてのファイルを二度
	// 展開せずに、どれが異なるかを判別できる。
	SHA256 string `json:"sha256"`
	// Mode を運ぶのは、ビットの誤った秘密鍵が、OpenSSH の拒む秘密鍵になるからで
	// ある。受け付けるのは 0600 と 0700 だけ。
	Mode string `json:"mode"`
}

// Manifest は、スナップショットの索引。
type Manifest struct {
	SchemaVersion int    `json:"schemaVersion"`
	CreatedAt     string `json:"createdAt"`
	// Origin は、これを書いたインストールを識別する。インターフェースが「これは別の
	// マシンから来た」と言えるようにするためだ。不透明な ID であって、決してホスト名
	// ではない。バケットを読める者なら誰でも見られるオブジェクトの中のホスト名は、
	// 不要な開示である。
	Origin string `json:"origin"`
	// Revision はcreatedAt、origin、parent、filesから決まる内容ID。暗号化された
	// manifest内にだけ置き、S3 providerへfile digestや系譜を平文公開しない。
	Revision string `json:"revision"`
	// ParentRevision は、このsnapshotを作る直前にlocalが採用していた世代。
	// 空なら履歴のrootである。
	ParentRevision string `json:"parentRevision,omitempty"`
	// Ancestors begins with ParentRevision and continues toward the root. It is
	// encrypted and covered by Revision, so the object store does not learn it.
	Ancestors []string `json:"ancestors,omitempty"`
	// Message はremoteでは暗号化manifest内だけに保存し、object metadataへ複製しない。
	Message string  `json:"message"`
	Files   []Entry `json:"files"`
}

type revisionDocument struct {
	CreatedAt      string   `json:"createdAt"`
	Origin         string   `json:"origin"`
	ParentRevision string   `json:"parentRevision,omitempty"`
	Ancestors      []string `json:"ancestors,omitempty"`
	Message        string   `json:"message"`
	Files          []Entry  `json:"files"`
}

// RevisionFor returns the stable content identity of a manifest. Revision and
// schemaVersion are excluded because they describe, rather than comprise, the
// revision contents.
func RevisionFor(manifest Manifest) (string, error) {
	files := append([]Entry(nil), manifest.Files...)
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	document, err := json.Marshal(revisionDocument{
		CreatedAt: manifest.CreatedAt, Origin: manifest.Origin,
		ParentRevision: manifest.ParentRevision, Ancestors: manifest.Ancestors,
		Message: manifest.Message, Files: files,
	})
	if err != nil {
		return "", err
	}
	return Digest(document), nil
}

// FinalizeManifest prepares a new manifest for writing and records its parent.
func FinalizeManifest(manifest *Manifest, parentRevision string) error {
	if parentRevision != "" && !validRevision(parentRevision) {
		return ErrManifestMismatch
	}
	manifest.SchemaVersion = SchemaVersion
	manifest.ParentRevision = parentRevision
	if parentRevision != "" && len(manifest.Ancestors) == 0 {
		manifest.Ancestors = []string{parentRevision}
	}
	if err := validateAncestors(parentRevision, manifest.Ancestors); err != nil {
		return err
	}
	message, err := NormalizeCommitMessage(manifest.Message)
	if err != nil {
		return err
	}
	manifest.Message = message
	revision, err := RevisionFor(*manifest)
	if err != nil {
		return err
	}
	manifest.Revision = revision
	return nil
}

// NormalizeCommitMessage trims the editable one-line message and validates its
// encrypted manifest representation.
func NormalizeCommitMessage(message string) (string, error) {
	message = strings.TrimSpace(message)
	if message == "" || len([]rune(message)) > MaxCommitMessageRunes || strings.ContainsAny(message, "\r\n") {
		return "", ErrCommitMessage
	}
	for _, character := range message {
		if character < 0x20 || character == 0x7f {
			return "", ErrCommitMessage
		}
	}
	return message, nil
}

func validRevision(revision string) bool {
	if len(revision) != 64 {
		return false
	}
	_, err := hex.DecodeString(revision)
	return err == nil
}

func validateAncestors(parent string, ancestors []string) error {
	if parent == "" {
		if len(ancestors) != 0 {
			return ErrManifestMismatch
		}
		return nil
	}
	if len(ancestors) == 0 || len(ancestors) > MaxManifestAncestors || ancestors[0] != parent {
		return ErrManifestMismatch
	}
	seen := make(map[string]struct{}, len(ancestors))
	for _, revision := range ancestors {
		if !validRevision(revision) {
			return ErrManifestMismatch
		}
		if _, exists := seen[revision]; exists {
			return ErrManifestMismatch
		}
		seen[revision] = struct{}{}
	}
	return nil
}

// Build は、与えられたファイルを圧縮アーカイブに詰める。
//
// contents は、ワークスペース相対のパスをキーとする。何を含めるかは呼び出し側が
// 決める。このパッケージは推測を拒む。「どのファイルが設定の一部なのか」は Include
// グラフの返す問いであり、このパッケージからはそれが見えないからで
// ある。
func Build(manifest Manifest, contents map[string][]byte) ([]byte, error) {
	manifest.SchemaVersion = SchemaVersion
	message, err := NormalizeCommitMessage(manifest.Message)
	if err != nil {
		return nil, err
	}
	manifest.Message = message
	if len(manifest.Files) > MaxEntries {
		return nil, ErrSnapshotTooLarge
	}
	sort.Slice(manifest.Files, func(i, j int) bool { return manifest.Files[i].Path < manifest.Files[j].Path })
	for _, entry := range manifest.Files {
		if err := checkPath(entry.Path); err != nil {
			return nil, err
		}
		if err := checkMode(entry.Mode); err != nil {
			return nil, err
		}
		if _, ok := contents[entry.Path]; !ok {
			return nil, ErrManifestMismatch
		}
	}
	expectedRevision, err := RevisionFor(manifest)
	if err != nil {
		return nil, err
	}
	if manifest.Revision == "" {
		manifest.Revision = expectedRevision
	}
	if manifest.Revision != expectedRevision ||
		(manifest.ParentRevision != "" && !validRevision(manifest.ParentRevision)) ||
		validateAncestors(manifest.ParentRevision, manifest.Ancestors) != nil {
		return nil, ErrManifestMismatch
	}
	document, err := json.Marshal(manifest)
	if err != nil {
		return nil, err
	}
	if len(document) > storage.MaxFileSize {
		return nil, ErrSnapshotTooLarge
	}
	total := int64(len(document))
	for _, entry := range manifest.Files {
		if len(contents[entry.Path]) > storage.MaxFileSize {
			return nil, ErrSnapshotTooLarge
		}
		total += int64(len(contents[entry.Path]))
		if total > MaxSnapshotBytes {
			return nil, ErrSnapshotTooLarge
		}
	}

	var compressed bytes.Buffer
	zip := gzip.NewWriter(&compressed)
	archive := tar.NewWriter(zip)

	if err := writeEntry(archive, ManifestName, document, 0o600); err != nil {
		return nil, err
	}
	for _, entry := range manifest.Files {
		if err := writeEntry(archive, entry.Path, contents[entry.Path], modeBits(entry.Mode)); err != nil {
			return nil, err
		}
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	if err := zip.Close(); err != nil {
		return nil, err
	}
	return compressed.Bytes(), nil
}

// Read はアーカイブを展開し、そのマニフェストと内容を返す。
//
// すべてのバイトを敵対的なものとして扱う。アーカイブはバケットから届き、そのバケット
// に書ける者なら誰でも、その中身を選べるからだ。
func Read(archive []byte) (Manifest, map[string][]byte, error) {
	zip, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return Manifest{}, nil, ErrNotASnapshot
	}
	defer func() { _ = zip.Close() }()

	reader := tar.NewReader(io.LimitReader(zip, MaxSnapshotBytes+1))
	var manifest Manifest
	seenManifest := false
	contents := map[string][]byte{}
	total := 0

	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Manifest{}, nil, ErrNotASnapshot
		}
		if header.Typeflag != tar.TypeReg {
			// このアーカイブの中のディレクトリ、シンボリックリンク、デバイスは、ここでは
			// 何の意味も持たず、好意的に解釈すべきものでもない。
			return Manifest{}, nil, ErrUnsafePath
		}
		if header.Size < 0 || header.Size > storage.MaxFileSize {
			return Manifest{}, nil, ErrSnapshotTooLarge
		}
		if len(contents) >= MaxEntries {
			return Manifest{}, nil, ErrSnapshotTooLarge
		}
		body, err := io.ReadAll(io.LimitReader(reader, storage.MaxFileSize+1))
		if err != nil {
			return Manifest{}, nil, ErrNotASnapshot
		}
		total += len(body)
		if total > MaxSnapshotBytes {
			return Manifest{}, nil, ErrSnapshotTooLarge
		}
		if len(body) > storage.MaxFileSize {
			return Manifest{}, nil, ErrSnapshotTooLarge
		}

		if header.Name == ManifestName {
			var version struct {
				SchemaVersion int `json:"schemaVersion"`
			}
			if err := json.Unmarshal(body, &version); err != nil {
				return Manifest{}, nil, ErrNotASnapshot
			}
			if version.SchemaVersion != SchemaVersion {
				return Manifest{}, nil, ErrUnsupportedVersion
			}
			decoder := json.NewDecoder(bytes.NewReader(body))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&manifest); err != nil {
				return Manifest{}, nil, ErrNotASnapshot
			}
			seenManifest = true
			continue
		}
		if err := checkPath(header.Name); err != nil {
			return Manifest{}, nil, err
		}
		if inboundReserved(header.Name) {
			return Manifest{}, nil, ErrUnsafePath
		}
		contents[header.Name] = body
	}

	if !seenManifest {
		return Manifest{}, nil, ErrNotASnapshot
	}
	message, err := NormalizeCommitMessage(manifest.Message)
	if err != nil || message != manifest.Message {
		return Manifest{}, nil, ErrManifestMismatch
	}
	if len(manifest.Files) != len(contents) {
		return Manifest{}, nil, ErrManifestMismatch
	}
	for _, entry := range manifest.Files {
		if err := checkPath(entry.Path); err != nil {
			return Manifest{}, nil, err
		}
		if inboundReserved(entry.Path) {
			return Manifest{}, nil, ErrUnsafePath
		}
		if err := checkMode(entry.Mode); err != nil {
			return Manifest{}, nil, err
		}
		body, ok := contents[entry.Path]
		if !ok {
			return Manifest{}, nil, ErrManifestMismatch
		}
		if Digest(body) != entry.SHA256 {
			return Manifest{}, nil, ErrManifestMismatch
		}
	}
	revision, err := RevisionFor(manifest)
	if err != nil {
		return Manifest{}, nil, ErrManifestMismatch
	}
	if manifest.Revision != revision ||
		(manifest.ParentRevision != "" && !validRevision(manifest.ParentRevision)) ||
		validateAncestors(manifest.ParentRevision, manifest.Ancestors) != nil {
		return Manifest{}, nil, ErrManifestMismatch
	}
	return manifest, contents, nil
}

// checkPath は、ワークスペース内の素朴な相対パスでないものをすべて拒否する。
// filepath.Clean より意図的に厳しい。整形が必要なパスとは、誰かが組み立てたパスで
// あり、ここはそれについて好意的になる場所では
// ない。
func checkPath(name string) error {
	if name == "" || name == "." {
		return ErrUnsafePath
	}
	if strings.HasPrefix(name, "/") || strings.Contains(name, "\\") {
		return ErrUnsafePath
	}
	if name != path.Clean(name) {
		return ErrUnsafePath
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return ErrUnsafePath
		}
	}
	return nil
}

// checkMode は、このアプリケーションが書く二つの権限セットだけを受け付ける。それ
// 以外は正規化せずに拒否する。これにより、スナップショットが秘密鍵の権限を、
// OpenSSH はまだ読むが別のユーザーも読める程度にまで広げることはできない。
func checkMode(mode string) error {
	if mode != "0600" && mode != "0700" {
		return ErrUnsafeMode
	}
	return nil
}

func modeBits(mode string) fs.FileMode {
	if mode == "0700" {
		return 0o700
	}
	return 0o600
}

func writeEntry(archive *tar.Writer, name string, body []byte, mode fs.FileMode) error {
	if err := archive.WriteHeader(&tar.Header{
		Typeflag: tar.TypeReg,
		Name:     name,
		Mode:     int64(mode),
		Size:     int64(len(body)),
		// 更新時刻も、所有者も、グループも運ばない。それらを運ぶスナップショットは、
		// 理由もなくマシンごとに異なることになり、それを書いたマシンについて少しばかり
		// 開示してしまう。
	}); err != nil {
		return err
	}
	_, err := archive.Write(body)
	return err
}
