package application

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"sshc/internal/storage"
)

// 端末の背景画像。

// BackgroundsDirectory は、画像を置く場所である。ワークスペース相対。
const BackgroundsDirectory = "sshc/backgrounds"

const (
	MinBackgroundCapacityMiB     = 1
	DefaultBackgroundCapacityMiB = 16
	MaxBackgroundCapacityMiB     = 1024
	// MaxBackgroundBytes は、利用者が明示的に許可できる画像 1 枚の絶対上限である。
	MaxBackgroundBytes = MaxBackgroundCapacityMiB << 20
)

var (
	// ErrBackgroundTooLarge は、大きすぎる画像を断る。
	ErrBackgroundTooLarge = errors.New("that image is larger than this application stores")
	// ErrBackgroundsFull は、合計の上限に達したことを報告する。
	ErrBackgroundsFull = errors.New("there is no room left for another background")
	// ErrNotAnImage は、画像に見えないバイト列を断る。
	ErrNotAnImage = errors.New("those bytes are not an image this application shows")
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

// safeStem は、希望された表記を、こちらが書いてよい名前に均す。
func safeStem(suggested string, contents []byte) string {
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
	stem := strings.Trim(builder.String(), "-")
	if stem != "" {
		return stem
	}
	sum := sha256.Sum256(contents)
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
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.Size() < 0 || info.Size() > int64(MaxBackgroundBytes) {
			continue
		}
		contents, err := storage.ReadFilePrefix(s.workspace.FileSystem(), filepath.Join(s.backgroundsRoot(), entry.Name()), 12)
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

// AddBackground は、送られてきたバイト列を 1 枚の背景として置く。
func (s *Service) AddBackground(suggested string, contents []byte) (Background, error) {
	if len(contents) > MaxBackgroundBytes {
		return Background{}, ErrBackgroundTooLarge
	}
	mediaType, extension := imageType(contents)
	if mediaType == "" {
		return Background{}, ErrNotAnImage
	}

	existing, err := s.Backgrounds()
	if err != nil {
		return Background{}, err
	}
	total := len(contents)
	taken := map[string]bool{}
	for _, background := range existing {
		total += background.Bytes
		taken[background.Name] = true
	}
	if int64(total) > s.backgroundCapacityBytes() {
		return Background{}, ErrBackgroundsFull
	}

	stem := safeStem(suggested, contents)
	name := stem + "." + extension
	for attempt := 2; taken[name]; attempt++ {
		name = fmt.Sprintf("%s-%d.%s", stem, attempt, extension)
	}

	root := s.backgroundsRoot()
	if err := s.workspace.EnsureDirectory(root); err != nil {
		return Background{}, err
	}
	target, err := s.workspace.ResolveForWrite(filepath.Join(root, name))
	if err != nil {
		return Background{}, err
	}
	if err := storage.WriteAtomicFile(s.workspace.FileSystem(), target, "background", storage.FilePermission, contents); err != nil {
		return Background{}, err
	}
	return Background{Name: name, Bytes: len(contents), Type: mediaType}, nil
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
	next := safeStem(suggested, contents) + "." + extension
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

// RemoveBackground は、その画像を捨てる。
func (s *Service) RemoveBackground(name string) error {
	s.saveMutex.Lock()
	defer s.saveMutex.Unlock()

	existing, err := s.Backgrounds()
	if err != nil {
		return err
	}
	for _, background := range existing {
		if background.Name == name {
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
			referenced := false
			if terminal := metadata.EmbeddedTerminal; terminal != nil && terminal.Appearance != nil && terminal.Appearance.Background == name {
				terminal.Appearance.Background = ""
				if terminal.Appearance.Empty() {
					terminal.Appearance = nil
				}
				referenced = true
			}
			for index := range metadata.Hosts {
				appearance := metadata.Hosts[index].Appearance
				if appearance != nil && appearance.Background == name {
					appearance.Background = ""
					if appearance.Empty() {
						metadata.Hosts[index].Appearance = nil
					}
					referenced = true
				}
			}
			if referenced {
				change, err := s.metadata.Change(metadata, precondition)
				if err != nil {
					return err
				}
				request.Changes = append(request.Changes, change)
			}
			_, err = s.manager.Commit(request)
			return err
		}
	}
	return ErrUnknownBackground
}
