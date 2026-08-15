// Package handoff は、動作中のアプリケーションが同じバイナリのコマンドライン
// 起動に自分の居場所を伝えるための仕組みである。
package handoff

import (
	"crypto/rand"
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

// Owner は、engine の生存期間を引き受ける外殻を表す。
type Owner string

const (
	OwnerDesktop  Owner = "desktop"
	OwnerHeadless Owner = "headless"

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
	raw := make([]byte, secretLength)
	if _, err := io.ReadFull(random, raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
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
	ensureDirectory func(string) error
	createTemp      func(string, string) (*os.File, error)
	replace         func(string, string) error
	syncDirectory   func(string) error
}

// write takes an operations value so failure tests can replace one operation
// without racing other package tests through mutable global state.
func write(directory string, document Handoff, operations writeOperations) error {
	if err := validate(document); err != nil {
		return err
	}
	body, err := json.Marshal(document)
	if err != nil {
		return err
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
func Read(directory string) (Handoff, error) {
	return readValidated(directory)
}

// readValidated は mutation lock を取らない。Read は Rename により常に完全な旧文書
// か新文書だけを見る。一方 Remove は lock を保持したままこれを呼び、比較と削除の
// 間に Write が割り込めないようにする。
func readValidated(directory string) (Handoff, error) {
	body, err := os.ReadFile(filepath.Join(directory, FileName))
	if err != nil {
		return Handoff{}, err
	}
	var document Handoff
	if err := json.Unmarshal(body, &document); err != nil {
		return Handoff{}, fmt.Errorf("decode handoff document: %w", err)
	}
	if err := validate(document); err != nil {
		return Handoff{}, err
	}
	return document, nil
}

// Remove は、そこに残っているのがこの実行の秘密を持つ文書だけを取り除く。
//
// URL や PID は次の実行で偶然再利用され得るが、実行ごとに発行する秘密は再利用
// されない。そのため secret だけが、後から来た別 engine を消さない所有権になる。
func Remove(directory, secret string) error {
	release, err := lockMutation(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer release()

	found, err := readValidated(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if found.Secret != secret {
		return nil
	}
	err = os.Remove(filepath.Join(directory, FileName))
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
	if document.Owner != OwnerDesktop && document.Owner != OwnerHeadless {
		return fmt.Errorf("%w: unknown owner %q", ErrInvalid, document.Owner)
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
		return fmt.Errorf("%w: parse URL: %v", ErrInvalid, err)
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

// Random は、呼び出し側に指定がないときに Write が引く乱数源。
var Random io.Reader = rand.Reader
