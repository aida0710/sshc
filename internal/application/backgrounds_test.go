package application

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshc/internal/storage"
)

func png(payload string) []byte {
	return append([]byte("\x89PNG\r\n\x1a\n"), []byte(payload)...)
}

// **名前を決めるのはこちらであって、送ってきた側ではない。**
//
// 送られた綴りをそのままファイル名にすれば、`../`・隠しファイル・拡張子の詐称が
// すべてそこから入る。ここは受け取った希望が、こちらの書く名前に均されることを
// 確かめる。
func TestTheServerNamesTheFileItWrites(t *testing.T) {
	service, workspace := newTerminalService(t)

	for _, probe := range []struct{ suggested, want string }{
		{"../../../etc/passwd", "etc-passwd.png"},
		{"/absolute/path.png", "absolute-path-png.png"},
		{".hidden", "hidden.png"},
		{"Office Wall.JPG", "office-wall-jpg.png"},
		{"..", ""},
		{"", ""},
	} {
		background, err := service.AddBackground(probe.suggested, png(probe.suggested+"payload"))
		if err != nil {
			t.Fatalf("%q: %v", probe.suggested, err)
		}
		if probe.want != "" && background.Name != probe.want {
			t.Fatalf("%q became %q, want %q", probe.suggested, background.Name, probe.want)
		}
		// **どの名前でも、置かれる先は背景の置き場の直下でなければならない。**
		written := filepath.Join(workspace.Root(), filepath.FromSlash(BackgroundsDirectory), background.Name)
		if _, err := os.Stat(written); err != nil {
			t.Fatalf("%q was written somewhere else: %v", probe.suggested, err)
		}
		if strings.ContainsAny(background.Name, "/\\") || strings.HasPrefix(background.Name, ".") {
			t.Fatalf("%q became an unsafe name %q", probe.suggested, background.Name)
		}
	}
}

// **拡張子でも Content-Type でも判断しない。** どちらも送ってきた側が名乗るもの
// である。中身の先頭が答える。
func TestOnlyBytesThatLookLikeAnImageAreStored(t *testing.T) {
	service, _ := newTerminalService(t)

	if _, err := service.AddBackground("payload.png", []byte("<html><script>alert(1)</script>")); !errors.Is(err, ErrNotAnImage) {
		t.Fatalf("err = %v, want it refused as not an image", err)
	}
	// SVG は書類であって画像ではない。中に script を書ける。
	if _, err := service.AddBackground("art.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)); !errors.Is(err, ErrNotAnImage) {
		t.Fatalf("err = %v, want svg refused", err)
	}
	background, err := service.AddBackground("photo.txt", png("real"))
	if err != nil {
		t.Fatal(err)
	}
	// **名乗った拡張子ではなく、中身の型が名前を決める。**
	if !strings.HasSuffix(background.Name, ".png") || background.Type != "image/png" {
		t.Fatalf("background = %#v, want the extension to come from the bytes", background)
	}
}

func TestBackgroundsRoundTripAndCanBeRemoved(t *testing.T) {
	service, _ := newTerminalService(t)

	added, err := service.AddBackground("office", png("bytes"))
	if err != nil {
		t.Fatal(err)
	}
	contents, mediaType, err := service.BackgroundContents(added.Name)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(contents, png("bytes")) || mediaType != "image/png" {
		t.Fatalf("read back %d bytes as %q", len(contents), mediaType)
	}

	if err := service.RemoveBackground(added.Name); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.BackgroundContents(added.Name); !errors.Is(err, ErrUnknownBackground) {
		t.Fatalf("err = %v, want it gone", err)
	}
}

// **置いていないものは読めない。** 名前は一覧と突き合わせるので、綴りの検査を
// 別に書かなくてよい。
func TestReadingRefusesNamesThatWereNeverStored(t *testing.T) {
	service, _ := newTerminalService(t)
	if _, err := service.AddBackground("office", png("bytes")); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"../metadata.json", "../../config", "office", "", "office.png/../../x"} {
		if _, _, err := service.BackgroundContents(name); !errors.Is(err, ErrUnknownBackground) {
			t.Fatalf("%q was readable: %v", name, err)
		}
	}
}

func TestTwoImagesWithTheSameNameBothSurvive(t *testing.T) {
	service, _ := newTerminalService(t)

	first, err := service.AddBackground("wall", png("one"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.AddBackground("wall", png("two"))
	if err != nil {
		t.Fatal(err)
	}
	if first.Name == second.Name {
		t.Fatalf("both were called %q, one overwrote the other", first.Name)
	}
	if _, _, err := service.BackgroundContents(first.Name); err != nil {
		t.Fatalf("the first one is gone: %v", err)
	}
}

// **スナップショットには上限がある。** 背景だけでそこを埋めると、鍵も設定も
// 旅に出られなくなる。
func TestThereIsARoofOverWhatTheBackgroundsMayWeigh(t *testing.T) {
	service, _ := newTerminalService(t)

	if _, err := service.AddBackground("huge", png(strings.Repeat("x", MaxBackgroundBytes))); !errors.Is(err, ErrBackgroundTooLarge) {
		t.Fatalf("err = %v, want a single oversized image refused", err)
	}

	// **保存層が読める大きさを超えない。** 超えると、置けても読み戻せない
	// ——上限を storage.MaxFileSize から導いているのはそのためである。
	if MaxBackgroundBytes > storage.MaxFileSize {
		t.Fatalf("MaxBackgroundBytes = %d, larger than the layer will read back (%d)", MaxBackgroundBytes, storage.MaxFileSize)
	}

	// 上限に近い画像を並べる。合計が屋根を超えたところで断られる。
	chunk := png(strings.Repeat("x", MaxBackgroundBytes-64))
	var lastErr error
	for round := 0; round < (MaxBackgroundsBytes/MaxBackgroundBytes)+2; round++ {
		if _, lastErr = service.AddBackground("wall", chunk); lastErr != nil {
			break
		}
	}
	if !errors.Is(lastErr, ErrBackgroundsFull) {
		t.Fatalf("err = %v, want the total to be capped", lastErr)
	}
}
