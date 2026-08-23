package acceptance_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshc/internal/secret"
)

// **説明は、実装と同じだけ古くなる。** そして古くなった説明は、間違った
// コードより長く生き残る——誰も走らせないからである。ここで固定するのは
// 「読んだ人が実際にできること」であり、その一つひとつが今日の入口に対応する。
//
// 語そのものを検査するのは乱暴だが、代わりが無い。文章の意味を機械が読むより、
// **決めごとを表す語が消えたら落ちる**方が、はるかに安く同じ効果を持つ。

func repositoryFile(t *testing.T, parts ...string) string {
	t.Helper()
	path := filepath.Join(append([]string{"..", ".."}, parts...)...)
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", filepath.Join(parts...), err)
	}
	return string(contents)
}

// 旧版の語は、消えたのではなく置き換わった。**残っていれば、読んだ人は
// 存在しない入口を打つ。**
func TestNoDocumentationTeachesTheRemovedEntryPoints(t *testing.T) {
	// docs/superpowers は設計と計画の記録であり、そこには「何を消したか」が
	// 書かれている。歴史を書いた文書から歴史の語を消させない。
	for _, path := range [][]string{{"README.md"}, {"docs", "manual-acceptance.md"}} {
		name := filepath.Join(path...)
		contents, err := os.ReadFile(filepath.Join(append([]string{"..", ".."}, path...)...))
		if err != nil {
			continue
		}
		for _, removed := range []string{"--own-engine", "-open=false", "sshc engine start", "make desktop-dist"} {
			if strings.Contains(string(contents), removed) &&
				!strings.Contains(string(contents), removed+"` は未定義") &&
				!strings.Contains(string(contents), "旧版") {
				t.Errorf("%s still teaches %q", name, removed)
			}
		}
	}
}

// 読んだ人が最初に知る必要があるのは、**誰が engine を持つか**である。
// ここが曖昧なままだと、裸の `sshc` を supervisor に登録する人が出る。
func TestTheReadmeSaysWhoOwnsTheEngine(t *testing.T) {
	readme := repositoryFile(t, "README.md")

	for phrase, why := range map[string]string{
		"sshc engine": "the way to own an engine from a terminal",
		"sshc vault":  "where passwords are typed",
		"sshc run":    "how a written procedure reaches a host",
	} {
		if !strings.Contains(readme, phrase) {
			t.Errorf("README never mentions %q (%s)", phrase, why)
		}
	}

	// **裸の `sshc` は engine を起こさない。** これを書いていないと、
	// 「起動すればエンジンが上がる」という以前の理解が残る。
	if !strings.Contains(readme, "エンジンは起こしません") {
		t.Error("README does not say that bare sshc starts no engine")
	}
}

// 保管庫の約束は、数字を伴って書かれていなければ意味が無い。
//
// **数字を書き写さない。** ここに "8 時間" と直に書いていたせいで、時計を 12 時間へ
// 延ばしても検査は緑のままで、README だけが古い約束を語り続けた。定数から綴りを
// 組み立てれば、次に延ばした人は README を直すまで赤を見る。
func TestTheReadmeStatesTheVaultRules(t *testing.T) {
	readme := repositoryFile(t, "README.md")

	stated := fmt.Sprintf("%d 時間", int(secret.IdleTimeout.Hours()))
	if !strings.Contains(readme, stated) {
		t.Errorf("README does not state the idle timeout %q; internal/secret.IdleTimeout is %v",
			stated, secret.IdleTimeout)
	}
	if !strings.Contains(readme, "端末からしか受け取りません") {
		t.Error("README does not say vault passwords are typed only on a terminal")
	}
}

// **ログイン時起動は OS のものである。** sshc は unit も service も
// スケジュールタスクも作らない。三つの OS それぞれについて、どこで登録するかを
// 書いていなければ、書いていないのと同じである。
func TestTheReadmeSendsAutostartToTheOperatingSystem(t *testing.T) {
	readme := repositoryFile(t, "README.md")

	if !strings.Contains(readme, "ログイン時起動は OS") {
		t.Error("README does not say autostart belongs to the operating system")
	}
	for _, where := range []string{"ログイン項目", "自動起動", "スタートアップ アプリ"} {
		if !strings.Contains(readme, where) {
			t.Errorf("README does not say where to register autostart: %q missing", where)
		}
	}
	// **Dock を Linux の案内に使わない。** あれは macOS のものである。
	linux := sectionOf(readme, "### Linux", "###")
	if strings.Contains(linux, "Dock") {
		t.Error("the Linux guidance mentions the Dock, which is a macOS affordance")
	}
}

// sectionOf は、見出しから次の見出しまでを返す。無ければ空である。
func sectionOf(text, heading, next string) string {
	start := strings.Index(text, heading)
	if start < 0 {
		return ""
	}
	rest := text[start+len(heading):]
	if end := strings.Index(rest, next); end >= 0 {
		return rest[:end]
	}
	return rest
}
