package application

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"sshc/internal/storage"
)

// 端末の背景画像。

// BackgroundsDirectory は、画像を置く場所である。ワークスペース相対。容量の上限を
// 決める storage と同じ場所を指すよう、storage の定数を使う。
const BackgroundsDirectory = storage.BackgroundsDirectory

const (
	MinBackgroundCapacityMiB     = 1
	DefaultBackgroundCapacityMiB = 16
	MaxBackgroundCapacityMiB     = 1024
	// MaxBackgroundBytes は、利用者が明示的に許可できる画像 1 枚の絶対上限である。
	MaxBackgroundBytes = storage.MaxAssetFileSize
)

const (
	// imageHeaderLength は、imageType が型を見分けるのに読む先頭の長さである。
	// 印がいちばん長い WebP（"RIFF" と長さと "WEBP"）が 12 バイト。
	imageHeaderLength = 12
	// backgroundTemporaryPrefix は、受け取っている途中の画像を書く一時ファイルの名前の
	// 先頭である。storage.IsTemporaryName が認める ".sshc-" で始めるので、書いている
	// 途中やクラッシュで残ったファイルが、背景の一覧にも同期にも入らない。
	backgroundTemporaryPrefix = ".sshc-background-"
)

var (
	// ErrBackgroundTooLarge は、大きすぎる画像を断る。
	ErrBackgroundTooLarge = errors.New("that image is larger than this application stores")
	// ErrBackgroundsFull は、合計の上限に達したことを報告する。
	ErrBackgroundsFull = errors.New("there is no room left for another background")
	// ErrNotAnImage は、画像に見えないバイト列を断る。
	ErrNotAnImage = errors.New("those bytes are not an image this application shows")
	// ErrBackgroundUnreadable は、追加する画像の本文を最後まで読めなかったことを
	// 報告する。読み取りの元のエラーも errors.Is と errors.As で取り出せる。
	ErrBackgroundUnreadable = errors.New("the image to add could not be read to the end")
	// ErrUnknownBackground は、置かれていない画像を指した要求を報告する。
	ErrUnknownBackground = errors.New("there is no background by that name")
	// ErrBackgroundAlreadyExists は、明示的な名前変更が既存画像を上書きしようとしたことを報告する。
	ErrBackgroundAlreadyExists = errors.New("a background with that name already exists")
	// ErrBackgroundCapacity は、設定可能な保存容量の範囲外を報告する。
	ErrBackgroundCapacity = errors.New("the background capacity is outside the supported range")
)

// Background は、置いてある画像 1 枚である。
type Background struct {
	Name  string `json:"name"`
	Bytes int    `json:"bytes"`
	Type  string `json:"type"`
}

// BackgroundCapacityMiB は保存済みの上限を返す。未設定は16 MiBである。
func (s *Service) BackgroundCapacityMiB() int {
	metadata, _, err := s.metadata.Load()
	if err != nil || metadata.Backgrounds == nil {
		return DefaultBackgroundCapacityMiB
	}
	value := metadata.Backgrounds.CapacityMiB
	if value < MinBackgroundCapacityMiB || value > MaxBackgroundCapacityMiB {
		return DefaultBackgroundCapacityMiB
	}
	return value
}

func (s *Service) backgroundCapacityBytes() int64 {
	return int64(s.BackgroundCapacityMiB()) << 20
}

// SetBackgroundCapacityMiB changes only the image-library quota. Existing
// images are never rewritten or removed when the quota is lowered.
func (s *Service) SetBackgroundCapacityMiB(value int) (SaveResult, error) {
	if value < MinBackgroundCapacityMiB || value > MaxBackgroundCapacityMiB {
		return SaveResult{}, ErrBackgroundCapacity
	}
	s.saveMutex.Lock()
	defer s.saveMutex.Unlock()
	metadata, precondition, err := s.metadata.Load()
	if err != nil {
		return SaveResult{}, err
	}
	metadata.Backgrounds = &BackgroundSettings{CapacityMiB: value}
	if err := s.metadata.EnsureDirectory(); err != nil {
		return SaveResult{}, err
	}
	change, err := s.metadata.Change(metadata, precondition)
	if err != nil {
		return SaveResult{}, err
	}
	result, err := s.manager.Commit(storage.Request{
		Operation: "terminal.backgrounds.capacity",
		Changes:   []storage.Change{change},
	})
	if err != nil {
		return SaveResult{}, err
	}
	return SaveResult{TransactionID: result.ID, Written: result.Written}, nil
}

// imageType は、バイト列の先頭からその型を返す。画像でなければ空である。
func imageType(contents []byte) (mediaType string, extension string) {
	switch {
	case len(contents) >= 8 && string(contents[:8]) == "\x89PNG\r\n\x1a\n":
		return "image/png", "png"
	case len(contents) >= 3 && contents[0] == 0xFF && contents[1] == 0xD8 && contents[2] == 0xFF:
		return "image/jpeg", "jpg"
	case len(contents) >= 12 && string(contents[:4]) == "RIFF" && string(contents[8:12]) == "WEBP":
		return "image/webp", "webp"
	case len(contents) >= 6 && (string(contents[:6]) == "GIF87a" || string(contents[:6]) == "GIF89a"):
		return "image/gif", "gif"
	}
	return "", ""
}

// requestedStem は、希望された表記を、こちらが書いてよい名前に均す。使える文字が
// 残らなければ空を返す。そのときは digestStem で中身から名前を作る。
func requestedStem(suggested string) string {
	trimmed := strings.TrimSpace(suggested)
	if dot := strings.LastIndex(trimmed, "."); dot > 0 && len(trimmed)-dot <= 6 {
		trimmed = trimmed[:dot]
	}
	var builder strings.Builder
	previousDash := false
	for _, letter := range strings.ToLower(trimmed) {
		switch {
		case letter >= 'a' && letter <= 'z', letter >= '0' && letter <= '9':
			builder.WriteRune(letter)
			previousDash = false
		case builder.Len() > 0 && !previousDash:
			builder.WriteByte('-')
			previousDash = true
		}
		if builder.Len() >= 48 {
			break
		}
	}
	return strings.Trim(builder.String(), "-")
}

// digestStem は、希望された表記から名前を作れなかった画像に、中身の SHA-256 の
// 先頭 4 バイトから名前を付ける。
func digestStem(sum []byte) string {
	return "background-" + hex.EncodeToString(sum[:4])
}

func (s *Service) backgroundsRoot() string {
	return filepath.Join(s.workspace.Root(), filepath.FromSlash(BackgroundsDirectory))
}

// Backgrounds は、置いてある画像を名前順に返す。
func (s *Service) Backgrounds() ([]Background, error) {
	entries, err := s.workspace.FileSystem().ReadDir(s.backgroundsRoot())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return []Background{}, nil
		}
		return nil, err
	}
	found := make([]Background, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || storage.IsTemporaryName(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.Size() < 0 || info.Size() > int64(MaxBackgroundBytes) {
			continue
		}
		contents, err := storage.ReadFilePrefix(s.workspace.FileSystem(), filepath.Join(s.backgroundsRoot(), entry.Name()), imageHeaderLength)
		if err != nil {
			continue
		}
		mediaType, _ := imageType(contents)
		if mediaType == "" {
			continue
		}
		found = append(found, Background{Name: entry.Name(), Bytes: int(info.Size()), Type: mediaType})
	}
	sort.Slice(found, func(one, other int) bool { return found[one].Name < found[other].Name })
	return found, nil
}

// AddBackground は、body を読みながら 1 枚の背景として置く。
//
// 本文はメモリに載せない。型は先頭だけで見分け、本文は置き場所の一時ファイルへ
// 書きながら空き容量で打ち切る。名前は書き終えてから決める。
func (s *Service) AddBackground(suggested string, body io.Reader) (Background, error) {
	head, err := readImageHeader(body)
	if err != nil {
		return Background{}, err
	}
	mediaType, extension := imageType(head)
	if mediaType == "" {
		return Background{}, ErrNotAnImage
	}

	stem := requestedStem(suggested)
	contents := io.MultiReader(bytes.NewReader(head), body)
	// 中身から名前を作るときだけ、書きながらハッシュを求める。
	var digest hash.Hash
	if stem == "" {
		digest = sha256.New()
		contents = io.TeeReader(contents, digest)
	}
	staged, err := s.stageBackground(contents)
	if err != nil {
		return Background{}, err
	}
	defer staged.Discard()
	if digest != nil {
		stem = digestStem(digest.Sum(nil))
	}

	name, err := s.publishBackground(staged, stem, extension)
	if err != nil {
		return Background{}, err
	}
	return Background{Name: name, Bytes: int(staged.Size()), Type: mediaType}, nil
}

// readImageHeader は、imageType が型を見分けるのに要る先頭だけを読む。それより
// 短い本文は、全体が先頭である。
func readImageHeader(body io.Reader) ([]byte, error) {
	head := make([]byte, 0, imageHeaderLength)
	for len(head) < imageHeaderLength {
		count, err := body.Read(head[len(head):imageHeaderLength])
		head = head[:len(head)+count]
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrBackgroundUnreadable, err)
		}
	}
	return head, nil
}

// stageBackground は、画像の本文を置き場所の一時ファイルへ書く。いま空いている
// 分を超えたところで読むのをやめる。
func (s *Service) stageBackground(contents io.Reader) (*storage.StagedFile, error) {
	existing, err := s.Backgrounds()
	if err != nil {
		return nil, err
	}
	room := s.backgroundRoom(existing)
	root := s.backgroundsRoot()
	if err := s.workspace.EnsureDirectory(root); err != nil {
		return nil, err
	}
	staged, err := s.workspace.FileSystem().StageFile(storage.StageRequest{
		Directory:  root,
		Prefix:     backgroundTemporaryPrefix,
		Permission: storage.FilePermission,
		Source:     contents,
		Maximum:    room,
	})
	switch {
	case errors.Is(err, storage.ErrFileTooLarge):
		return nil, backgroundOverflow(room)
	case errors.Is(err, storage.ErrSourceUnreadable):
		return nil, fmt.Errorf("%w: %w", ErrBackgroundUnreadable, err)
	case err != nil:
		return nil, err
	}
	return staged, nil
}

// backgroundRoom は、existing を置いたうえで、あと何バイト置けるかを返す。容量を
// 下げて既に超えていれば 0 である。
func (s *Service) backgroundRoom(existing []Background) int64 {
	used := int64(0)
	for _, background := range existing {
		used += int64(background.Bytes)
	}
	return max(s.backgroundCapacityBytes()-used, 0)
}

// backgroundOverflow は、本文が空きを超えたときの断り方を返す。1 枚の絶対上限まで
// 空いていてなお超えたなら、画像そのものが大きすぎる。そうでなければ、置き場所の
// 空きが足りない。
func backgroundOverflow(room int64) error {
	if room >= MaxBackgroundBytes {
		return ErrBackgroundTooLarge
	}
	return ErrBackgroundsFull
}

// publishBackground は、書き終えた画像に名前を付けて置き、その名前を返す。
//
// 本文を受け取っている間は錠を持たない。そのため同時に受け取った 2 枚が、同じ名前を
// 選んだり、合わせて容量を超えたりしうる。ここで錠を取って一覧を読み直し、名前と
// 容量を確かめてから置くまでを、ほかの追加・名前の変更・削除と重ねない。
func (s *Service) publishBackground(staged *storage.StagedFile, stem, extension string) (string, error) {
	s.saveMutex.Lock()
	defer s.saveMutex.Unlock()

	existing, err := s.Backgrounds()
	if err != nil {
		return "", err
	}
	if staged.Size() > s.backgroundRoom(existing) {
		return "", ErrBackgroundsFull
	}
	name := unusedBackgroundName(stem, extension, existing)
	target, err := s.workspace.ResolveForWrite(filepath.Join(s.backgroundsRoot(), name))
	if err != nil {
		return "", err
	}
	if err := staged.Publish(target); err != nil {
		return "", err
	}
	return name, nil
}

// unusedBackgroundName は、stem と extension から、existing のどれとも重ならない
// 名前を作る。
func unusedBackgroundName(stem, extension string, existing []Background) string {
	taken := make(map[string]bool, len(existing))
	for _, background := range existing {
		taken[background.Name] = true
	}
	name := stem + "." + extension
	for attempt := 2; taken[name]; attempt++ {
		name = fmt.Sprintf("%s-%d.%s", stem, attempt, extension)
	}
	return name
}

// BackgroundContents は、その画像のバイト列と型を返す。
func (s *Service) BackgroundContents(name string) ([]byte, string, error) {
	existing, err := s.Backgrounds()
	if err != nil {
		return nil, "", err
	}
	for _, background := range existing {
		if background.Name != name {
			continue
		}
		contents, err := storage.ReadFileLimited(s.workspace.FileSystem(), filepath.Join(s.backgroundsRoot(), name), int64(MaxBackgroundBytes))
		if err != nil {
			return nil, "", err
		}
		return contents, background.Type, nil
	}
	return nil, "", ErrUnknownBackground
}

// RenameBackground は画像と、その画像を参照する全体・接続別の見た目設定を一緒に移す。
// 片方だけ成功すれば保存済みの背景が消えて見えるため、同じstorage transactionに置く。
func (s *Service) RenameBackground(current, suggested string) (Background, error) {
	s.saveMutex.Lock()
	defer s.saveMutex.Unlock()

	existing, err := s.Backgrounds()
	if err != nil {
		return Background{}, err
	}
	var found *Background
	taken := make(map[string]bool, len(existing))
	for index := range existing {
		background := &existing[index]
		taken[background.Name] = true
		if background.Name == current {
			found = background
		}
	}
	if found == nil {
		return Background{}, ErrUnknownBackground
	}
	contents, err := storage.ReadFileLimited(s.workspace.FileSystem(), filepath.Join(s.backgroundsRoot(), current), int64(MaxBackgroundBytes))
	if err != nil {
		return Background{}, err
	}
	mediaType, extension := imageType(contents)
	if mediaType == "" {
		return Background{}, ErrNotAnImage
	}
	stem := requestedStem(suggested)
	if stem == "" {
		sum := sha256.Sum256(contents)
		stem = digestStem(sum[:])
	}
	next := stem + "." + extension
	if next == current {
		return *found, nil
	}
	if taken[next] {
		return Background{}, ErrBackgroundAlreadyExists
	}

	from, err := s.workspace.ResolveForWrite(filepath.Join(s.backgroundsRoot(), current))
	if err != nil {
		return Background{}, err
	}
	to, err := s.workspace.ResolveForWrite(filepath.Join(s.backgroundsRoot(), next))
	if err != nil {
		return Background{}, err
	}
	request := storage.Request{
		Operation: "rename terminal background",
		Moves: []storage.Move{{
			From: from,
			To:   to,
			Precondition: storage.Precondition{
				Exists: true,
				Digest: storage.Digest(contents),
			},
		}},
	}

	metadata, precondition, err := s.metadata.Load()
	if err != nil {
		return Background{}, err
	}
	referenced := false
	if terminal := metadata.EmbeddedTerminal; terminal != nil && terminal.Appearance != nil && terminal.Appearance.Background == current {
		terminal.Appearance.Background = next
		referenced = true
	}
	for index := range metadata.Hosts {
		appearance := metadata.Hosts[index].Appearance
		if appearance != nil && appearance.Background == current {
			appearance.Background = next
			referenced = true
		}
	}
	if referenced {
		change, err := s.metadata.Change(metadata, precondition)
		if err != nil {
			return Background{}, err
		}
		request.Changes = append(request.Changes, change)
	}

	if _, err := s.manager.Commit(request); err != nil {
		return Background{}, err
	}
	return Background{Name: next, Bytes: found.Bytes, Type: mediaType}, nil
}

// RemoveBackground は、その画像を捨てる。その画像を指している外観の指定も外す。
func (s *Service) RemoveBackground(name string) error {
	s.saveMutex.Lock()
	defer s.saveMutex.Unlock()

	existing, err := s.Backgrounds()
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(existing, func(background Background) bool { return background.Name == name }) {
		return ErrUnknownBackground
	}
	target, err := s.workspace.ResolveForWrite(filepath.Join(s.backgroundsRoot(), name))
	if err != nil {
		return err
	}
	contents, err := storage.ReadFileLimited(s.workspace.FileSystem(), target, int64(MaxBackgroundBytes))
	if err != nil {
		return err
	}
	request := storage.Request{
		Operation: "remove terminal background",
		Removals: []storage.Removal{{
			Path: target,
			Precondition: storage.Precondition{
				Exists: true,
				Digest: storage.Digest(contents),
			},
		}},
	}
	metadata, precondition, err := s.metadata.Load()
	if err != nil {
		return err
	}
	if forgetBackground(&metadata, name) {
		change, err := s.metadata.Change(metadata, precondition)
		if err != nil {
			return err
		}
		request.Changes = append(request.Changes, change)
	}
	_, err = s.manager.Commit(request)
	return err
}

// forgetBackground は、埋め込みターミナルと各接続の外観から、背景画像 name の指定を
// 外す。外したものがあれば true を返す。外して空になった外観は、指定ごと消す。
func forgetBackground(metadata *Metadata, name string) bool {
	forgotten := false
	if terminal := metadata.EmbeddedTerminal; terminal != nil && terminal.Appearance != nil && terminal.Appearance.Background == name {
		terminal.Appearance.Background = ""
		if terminal.Appearance.Empty() {
			terminal.Appearance = nil
		}
		forgotten = true
	}
	for index := range metadata.Hosts {
		appearance := metadata.Hosts[index].Appearance
		if appearance == nil || appearance.Background != name {
			continue
		}
		appearance.Background = ""
		if appearance.Empty() {
			metadata.Hosts[index].Appearance = nil
		}
		forgotten = true
	}
	return forgotten
}
