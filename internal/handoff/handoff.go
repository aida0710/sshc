// Package handoff は、動作中のアプリケーションが同じバイナリのコマンドライン
// 起動に自分の居場所を伝えるための仕組みである。
package handoff

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
)

// FileName は、アプリケーションの状態ディレクトリ内のハンドオフファイル。
const FileName = "cli"

// mutationLockName は、公開ファイルと別 inode に固定して保持する。cli 自体を lock
// すると Rename のたびに lock の対象 inode が替わり、Write と Remove を直列化
// できなくなる。
const mutationLockName = ".cli.mutation.lock"

// secretLength は、秘密の元になるランダムバイト数。
const secretLength = 32

// handoffDocumentMaxSize is intentionally separate from the general storage
// limit. A handoff contains only one loopback endpoint and one short bearer
// secret, so accepting megabytes would only create a same-user allocation
// hazard. 4 KiB leaves ample room for future schema fields.
const handoffDocumentMaxSize = 4 << 10

// Owner は、engine の生存期間を引き受ける相手を表す。
//
// **いまは 1 つしかない。** かつては desktop（Electron の外殻が engine を子として
// 抱える）と headless（端末や supervisor が持つ）に分かれていた。外殻が無くなり、
// engine を生かしておくのは常に人（tmux でも systemd でも）になったので、区別も
// 消えた。**値そのものは残す** ——handoff は engine の身元でもあり、別の engine を
// 掴んでいないことをここで確かめている。
type Owner string

const (
	OwnerEngine Owner = "engine"

	SchemaVersion   = 1
	ProtocolVersion = 1
)

var (
	// ErrInvalid は、同じ版であっても接続先として安全でない文書を表す。
	ErrInvalid = errors.New("invalid handoff document")
	// ErrSchemaVersion は、文書の構造を CLI が理解できないことを表す。
	ErrSchemaVersion = errors.New("unsupported handoff schema version")
	// ErrProtocolVersion は、CLI と実行中 app の通信規約が一致しないことを表す。
	ErrProtocolVersion = errors.New("unsupported handoff protocol version")
	// ErrDocumentTooLarge は、ハンドオフ専用の小さい入力上限を超えた文書を表す。
	ErrDocumentTooLarge = errors.New("handoff document is too large")
)

// Handoff は、この実行がどこで待ち受けているか、誰が所有しているか、そして
// 呼び出し側がファイルを読んだことを何が証明するかを保持する。
type Handoff struct {
	SchemaVersion   int    `json:"schemaVersion"`
	URL             string `json:"url"`
	Secret          string `json:"secret"`
	Owner           Owner  `json:"owner"`
	PID             int    `json:"pid"`
	Version         string `json:"version"`
	ProtocolVersion int    `json:"protocolVersion"`
}

// HeaderName は、コマンドラインからのリクエストに秘密を載せる。
const HeaderName = "X-SSHC-CLI"

// Mint は、一回の実行のための秘密を返す。
func Mint(random io.Reader) (string, error) {
	return mint(random, base64.RawURLEncoding.EncodeToString)
}

func mint(random io.Reader, encode func([]byte) string) (string, error) {
	raw := make([]byte, secretLength)
	defer zeroBytes(raw)
	if _, err := io.ReadFull(random, raw); err != nil {
		return "", err
	}
	return encode(raw), nil
}

// Write は検証済みの文書を同じディレクトリ内で原子的に置き換える。
//
// 一時ファイルを別の場所に置くと Rename が copy になり、読み手が途中の JSON を
// 見られる。したがって公開直前まで同じディレクトリに隠し、同期してから名前だけを
// 差し替える。
func Write(directory string, document Handoff) error {
	return write(directory, document, defaultWriteOperations())
}

type writeOperations struct {
	marshal         func(any) ([]byte, error)
	ensureDirectory func(string) error
	createTemp      func(string, string) (*os.File, error)
	replace         func(string, string) error
	syncDirectory   func(string) error
}

type handoffFileOperations struct {
	open   func(string) (*os.File, error)
	read   func(io.Reader) ([]byte, error)
	remove func(*os.File, string) error
}

// write takes an operations value so failure tests can replace one operation
// without racing other package tests through mutable global state.
func write(directory string, document Handoff, operations writeOperations) error {
	if err := validate(document); err != nil {
		return err
	}
	marshal := operations.marshal
	if marshal == nil {
		marshal = json.Marshal
	}
	body, err := marshal(document)
	defer zeroBytes(body)
	if err != nil {
		return err
	}
	if len(body) > handoffDocumentMaxSize {
		return ErrDocumentTooLarge
	}
	if err := operations.ensureDirectory(directory); err != nil {
		return err
	}
	release, err := lockMutation(directory)
	if err != nil {
		return err
	}
	defer release()
	temporary, err := operations.createTemp(directory, "."+FileName+".tmp-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(body); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	finalPath := filepath.Join(directory, FileName)
	if err := operations.replace(temporaryPath, finalPath); err != nil {
		return err
	}
	return operations.syncDirectory(directory)
}

// Read は、動作中のアプリケーションが残した検証済みの文書を返す。
//
// mutation lock は取らない。Rename により、見えるのは常に完全な旧文書か新文書
// だけである。**Remove の側は違う** —— あちらは lock を保持したまま
// readValidatedHandleWith を呼び、比較と削除の間に Write が割り込めないように
// する。
func Read(directory string) (Handoff, error) {
	return readValidatedWith(directory, defaultHandoffFileOperations().open)
}

func readValidatedWith(directory string, open func(string) (*os.File, error)) (Handoff, error) {
	document, file, err := readValidatedHandle(filepath.Join(directory, FileName), open)
	if err != nil {
		return Handoff{}, err
	}
	if err := file.Close(); err != nil {
		return Handoff{}, err
	}
	return document, nil
}

func readValidatedHandle(path string, open func(string) (*os.File, error)) (Handoff, *os.File, error) {
	operations := defaultHandoffFileOperations()
	operations.open = open
	return readValidatedHandleWith(path, operations)
}

func readValidatedHandleWith(path string, operations handoffFileOperations) (Handoff, *os.File, error) {
	file, err := operations.open(path)
	if err != nil {
		return Handoff{}, nil, err
	}
	read := operations.read
	if read == nil {
		read = readHandoffBody
	}
	body, err := read(file)
	defer zeroBytes(body)
	if err != nil {
		_ = file.Close()
		return Handoff{}, nil, err
	}
	var document Handoff
	if err := json.Unmarshal(body, &document); err != nil {
		_ = file.Close()
		return Handoff{}, nil, fmt.Errorf("%w: cannot decode document", ErrInvalid)
	}
	if err := validate(document); err != nil {
		_ = file.Close()
		return Handoff{}, nil, err
	}
	return document, file, nil
}

func readHandoffBody(reader io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, handoffDocumentMaxSize+1))
	if err != nil {
		return body, err
	}
	if len(body) > handoffDocumentMaxSize {
		return body, ErrDocumentTooLarge
	}
	return body, nil
}

func zeroBytes(contents []byte) {
	for index := range contents {
		contents[index] = 0
	}
}

// Remove は、そこに残っているのがこの実行の秘密を持つ文書だけを取り除く。
//
// URL や PID は次の実行で偶然再利用され得るが、実行ごとに発行する秘密は再利用
// されない。そのため secret だけが、後から来た別 engine を消さない所有権になる。
func Remove(directory, secret string) error {
	return removeWith(directory, secret, defaultHandoffFileOperations())
}

func removeWith(directory, secret string, operations handoffFileOperations) error {
	release, err := lockMutation(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer release()

	path := filepath.Join(directory, FileName)
	found, file, err := readValidatedHandleWith(path, operations)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if found.Secret != secret {
		return file.Close()
	}
	err = operations.remove(file, path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func validate(document Handoff) error {
	if document.SchemaVersion != SchemaVersion {
		return fmt.Errorf("%w: got %d, want %d", ErrSchemaVersion, document.SchemaVersion, SchemaVersion)
	}
	if document.ProtocolVersion != ProtocolVersion {
		return fmt.Errorf("%w: got %d, want %d", ErrProtocolVersion, document.ProtocolVersion, ProtocolVersion)
	}
	if document.Owner != OwnerEngine {
		return fmt.Errorf("%w: unknown owner", ErrInvalid)
	}
	if document.Secret == "" {
		return fmt.Errorf("%w: empty secret", ErrInvalid)
	}
	if document.PID <= 0 {
		return fmt.Errorf("%w: pid must be positive", ErrInvalid)
	}
	if document.Version == "" {
		return fmt.Errorf("%w: empty version", ErrInvalid)
	}
	if err := validateLoopbackURL(document.URL); err != nil {
		return err
	}
	return nil
}

func validateLoopbackURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%w: cannot parse URL", ErrInvalid)
	}
	if parsed.Scheme != "http" || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("%w: URL must be a bare HTTP loopback URL", ErrInvalid)
	}
	host := parsed.Hostname()
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("%w: URL host is not loopback", ErrInvalid)
	}
	return nil
}
