package keys

import (
	"context"
	"crypto/rand"
	"errors"
	"testing"
)

// fakeToolchain は固定の絶対パスで答えるので、どのテストも、開発者のマシンに
// たまたま入っている OpenSSH のプログラムに依存しない。
type fakeToolchain struct {
	err error
}

func (fake fakeToolchain) KeyGen() (string, error) { return fake.resolve("/usr/bin/ssh-keygen") }

func (fake fakeToolchain) resolve(path string) (string, error) {
	if fake.err != nil {
		return "", fake.err
	}
	return path, nil
}

// **一覧に載っているものは必ず作れなければならない。** 載っているのに作れない
// 項目は、画面のボタンが必ず失敗するということである。
func TestEveryInProcessVariantCanActuallyBeGenerated(t *testing.T) {
	catalogue := CatalogueReader{}.Read(context.Background())
	if len(catalogue.Variants) == 0 {
		t.Fatal("the catalogue is empty")
	}
	for _, variant := range catalogue.Variants {
		if !variant.InProcess {
			continue
		}
		if _, err := GeneratePrivateKey(variant.Algorithm, variant.Bits, rand.Reader); err != nil {
			t.Errorf("the catalogue offers %s/%d but generating it = %v",
				variant.Algorithm, variant.Bits, err)
		}
	}
}

// **インストール済みの OpenSSH には尋ねない。** ここが並べているのは
// 「ここで生成できる鍵」であり、あちらが何に対応しているかは別の問いである。
func TestTheCatalogueDoesNotDependOnTheInstalledOpenSSH(t *testing.T) {
	without := CatalogueReader{}.Read(context.Background())
	broken := CatalogueReader{Toolchain: fakeToolchain{err: errors.New("no programs here")}}.
		Read(context.Background())

	if len(without.Variants) != len(broken.Variants) {
		t.Fatalf("a missing toolchain changed the answer: %d vs %d",
			len(without.Variants), len(broken.Variants))
	}
	for _, variant := range without.Variants {
		if !variant.InProcess {
			t.Errorf("without ssh-keygen the catalogue still offered %s", variant.Algorithm)
		}
	}
}

// ハードウェア鍵だけは ssh-keygen が要る。無ければ出さない——押せば必ず失敗する
// ボタンを画面に出さない。
func TestHardwareVariantsAppearOnlyWhenKeygenIsAvailable(t *testing.T) {
	present := CatalogueReader{Toolchain: fakeToolchain{}}.Read(context.Background())
	found := map[Algorithm]bool{}
	for _, variant := range present.Variants {
		found[variant.Algorithm] = true
		if variant.Algorithm == AlgorithmEd25519SK && variant.Reason != ReasonHardwareToken {
			t.Errorf("a hardware variant does not say why it is not in process: %#v", variant)
		}
	}
	if !found[AlgorithmEd25519SK] || !found[AlgorithmECDSASK] {
		t.Fatalf("ssh-keygen is available but the hardware variants are missing: %#v", present.Variants)
	}

	absent := CatalogueReader{Toolchain: fakeToolchain{err: errors.New("absent")}}.
		Read(context.Background())
	for _, variant := range absent.Variants {
		if variant.Algorithm == AlgorithmEd25519SK || variant.Algorithm == AlgorithmECDSASK {
			t.Fatalf("a hardware variant was offered without ssh-keygen: %#v", variant)
		}
	}
}

func TestHardwareCommandProducesAnUnambiguousArgumentList(t *testing.T) {
	command, err := HardwareCommand(AlgorithmEd25519SK, "id_yubikey", "aida@laptop", "/Users/example/.ssh")
	if err != nil {
		t.Fatalf("HardwareCommand error = %v", err)
	}
	want := []string{"ssh-keygen", "-t", "ed25519-sk", "-C", "aida@laptop", "-f", "/Users/example/.ssh/id_yubikey"}
	if len(command) != len(want) {
		t.Fatalf("command = %#v, want %#v", command, want)
	}
	for index := range want {
		if command[index] != want[index] {
			t.Fatalf("command[%d] = %q, want %q", index, command[index], want[index])
		}
	}

	rejections := []struct {
		name      string
		algorithm Algorithm
		fileName  string
		comment   string
		wantError error
	}{
		{"software algorithm", AlgorithmEd25519, "id_ed25519", "aida@laptop", ErrUnsupportedAlgorithm},
		{"traversal in name", AlgorithmEd25519SK, "../escape", "aida@laptop", ErrInvalidFileName},
		{"option injection in name", AlgorithmEd25519SK, "-oProxyCommand=id", "aida@laptop", ErrInvalidFileName},
		{"shell metacharacter in comment", AlgorithmECDSASK, "id_yubikey", "aida; rm -rf /", ErrInvalidComment},
	}
	for _, test := range rejections {
		t.Run(test.name, func(t *testing.T) {
			if _, err := HardwareCommand(test.algorithm, test.fileName, test.comment, "/Users/example/.ssh"); !errors.Is(err, test.wantError) {
				t.Fatalf("error = %v, want %v", err, test.wantError)
			}
		})
	}
}
