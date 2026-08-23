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
	// MaxBackgroundBytes は、画像 1 枚の上限である。
	MaxBackgroundBytes = storage.MaxFileSize
	// MaxBackgroundsBytes は、置いてある画像の合計の上限である。
	MaxBackgroundsBytes = 16 << 20
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
)

// Background は、置いてある画像 1 枚である。
type Background struct {
	Name  string `json:"name"`
	Bytes int    `json:"bytes"`
	Type  string `json:"type"`
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
		if err != nil || info.Size() > MaxBackgroundBytes {
			continue
		}
		contents, err := s.workspace.FileSystem().ReadFile(filepath.Join(s.backgroundsRoot(), entry.Name()))
		if err != nil {
			continue
		}
		mediaType, _ := imageType(contents)
		if mediaType == "" {
			continue
		}
		found = append(found, Background{Name: entry.Name(), Bytes: len(contents), Type: mediaType})
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
	if total > MaxBackgroundsBytes {
		return Background{}, ErrBackgroundsFull
	}

	stem := safeStem(suggested, contents)
	name := stem + "." + extension
	for attempt := 2; taken[name]; attempt++ {
		name = fmt.Sprintf("%s-%d.%s", stem, attempt, extension)
	}

	root := s.backgroundsRoot()
	if err := s.workspace.FileSystem().MkdirAll(root, storage.DirectoryPermission); err != nil {
		return Background{}, err
	}
	temporary, err := s.workspace.FileSystem().WriteTemp(root, "background", storage.FilePermission, contents)
	if err != nil {
		return Background{}, err
	}
	if err := s.workspace.FileSystem().Rename(temporary, filepath.Join(root, name)); err != nil {
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
		contents, err := s.workspace.FileSystem().ReadFile(filepath.Join(s.backgroundsRoot(), name))
		if err != nil {
			return nil, "", err
		}
		return contents, background.Type, nil
	}
	return nil, "", ErrUnknownBackground
}

// RemoveBackground は、その画像を捨てる。
func (s *Service) RemoveBackground(name string) error {
	existing, err := s.Backgrounds()
	if err != nil {
		return err
	}
	for _, background := range existing {
		if background.Name == name {
			return s.workspace.FileSystem().Remove(filepath.Join(s.backgroundsRoot(), name))
		}
	}
	return ErrUnknownBackground
}
