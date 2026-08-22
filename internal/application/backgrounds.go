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
//
// **封の中を旅する。** remotesync.Collect が sshc/backgrounds/ を集めるので、
// 一度置いた画像は同期した他の端末にも配られる。Android はサンドボックスの外を
// 見られないので、**そこへ画像を持ち込む道はこれしかない。**
//
// 名前を付けるのはこちらであって、送ってきた側ではない。**送られてきた綴りを
// そのままファイル名にしない** ——`../` も、隠しファイルも、拡張子の詐称も、
// すべてそこから入る。受け取るのは希望であって、決めるのはここである。

// BackgroundsDirectory は、画像を置く場所である。ワークスペース相対。
const BackgroundsDirectory = "sshc/backgrounds"

const (
	// MaxBackgroundBytes は、画像 1 枚の上限である。
	//
	// **保存層の上限から導く。** storage.MaxFileSize を超えるものは、書けても
	// 読み戻せない——別々に数を書くと、いつか片方だけが動いて「置けたのに
	// 一覧に出ない画像」ができる。実際にそうなった。
	MaxBackgroundBytes = storage.MaxFileSize
	// MaxBackgroundsBytes は、置いてある画像の合計の上限である。
	//
	// **スナップショットには上限がある。** 背景だけでそこを埋めると、鍵も設定も
	// 旅に出られなくなる——同期が死ぬより、画像が置けない方がよい。
	MaxBackgroundsBytes = 16 << 20
)

var (
	// ErrBackgroundTooLarge は、大きすぎる画像を断る。
	ErrBackgroundTooLarge = errors.New("that image is larger than this application stores")
	// ErrBackgroundsFull は、合計の上限に達したことを報告する。
	ErrBackgroundsFull = errors.New("there is no room left for another background")
	// ErrNotAnImage は、画像に見えないバイト列を断る。
	//
	// **拡張子でも Content-Type でも判断しない。** どちらも送ってきた側が
	// 名乗るものである。中身の先頭が答える。
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

// imageType は、バイト列の先頭からその型を答える。画像でなければ空である。
//
// **許すものを並べる。** 「危ないものを弾く」向きに書くと、並べ忘れたものが
// 通る側に落ちる。SVG は入れない——あれは書類であり、中に script を書ける。
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

// safeStem は、希望された綴りを、こちらが書いてよい名前に均す。
//
// **通す文字を並べる。** 点も斜線も通さないので、`..` も隠しファイルも作れない。
// 何も残らなければ、中身から作る——名前が無いことを理由に断るほどのことではない。
func safeStem(suggested string, contents []byte) string {
	var builder strings.Builder
	previousDash := false
	for _, letter := range strings.ToLower(strings.TrimSpace(suggested)) {
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
//
// **画像に見えないものは黙って飛ばす。** ここは読み取りであり、誰かが置いた
// 一枚のせいで一覧そのものが出せなくなってよい理由はない。
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
		// **大きすぎるものは読まずに飛ばす。** 保存層は上限を超えるファイルを
		// 読まない。読ませてから失敗を握り潰すと、「置いたのに一覧に出ない画像」
		// になる——原因の分からない消え方をする。
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
//
// **名前を決めるのはここである。** 希望は綴りの素材にしかならず、拡張子は
// 中身から決まる——`.png` と名乗る HTML は `.html` にすらならず、断られる。
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
//
// **名前は一覧と突き合わせて確かめる。** パスを組み立てる前に、置いてあるものの
// どれかであることを確かめれば、綴りの検査を別に書かなくてよい——書けば、
// いつか片方だけが直る。
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
