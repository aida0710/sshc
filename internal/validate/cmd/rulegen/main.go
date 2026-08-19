// Command rulegen は、ブラウザにも同じ答えを出してほしい規則を web 側へ配る。
//
// **配るのは 2 つである。** ひとつは表——予約語・パターン・上限——で、これは
// TypeScript の定数として出す。もうひとつは適合コーパスで、入力の一覧と、それらに
// 対する Go の判定を持つ。
//
// **守る契約は「ブラウザはサーバーより厳しくしない」である。** コーパスがあれば、
// それを検査として書ける——Go が通す入力を web が断ったら赤くなる。逆向き（web の
// 方が緩い）は赤くしない: サーバーが正しく断り、利用者は理由を受け取るからである。
//
// 整形（OpenSSH の引用）だけは別で、緩い厳しいの軸が無い。**同じ入力から同じ文字列
// が出る**ことを見る——出なければ、画面が見せているものと保存されるものが違う。
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sshc/internal/config"
	"sshc/internal/validate"
)

// corpus は、両方の側へ突き合わせてほしい入力の全体である。
//
// **並びは決めてある。** 生成物が呼び出しごとに変われば、verify-generated が意味の
// 無い差分を出す。
type corpus struct {
	GroupName []verdict     `json:"groupName"`
	HostName  []verdict     `json:"hostName"`
	Alias     []verdict     `json:"alias"`
	Port      []portVerdict `json:"port"`
	Render    []renderCase  `json:"render"`
	Parse     []parseCase   `json:"parse"`
}

type verdict struct {
	Input string `json:"input"`
	Valid bool   `json:"valid"`
	// Why は、この入力を一覧に入れた理由である。**赤くなった人が読むのはここである。**
	Why string `json:"why"`
}

type portVerdict struct {
	Input int    `json:"input"`
	Valid bool   `json:"valid"`
	Why   string `json:"why"`
}

type renderCase struct {
	Input []string `json:"input"`
	// Output は、Go が書き出す 1 行。Refused なら空である。
	Output  string `json:"output"`
	Refused bool   `json:"refused"`
	Why     string `json:"why"`
}

type parseCase struct {
	Input string `json:"input"`
	// Values は Go が読み取った引数。Refused なら null である。
	Values  []string `json:"values"`
	Refused bool     `json:"refused"`
	Why     string   `json:"why"`
}

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	if err := write(root); err != nil {
		fmt.Fprintln(os.Stderr, "rulegen:", err)
		os.Exit(1)
	}
}

func write(root string) error {
	directory := filepath.Join(root, "web", "src", "rules")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(directory, "generated.ts"), []byte(constants()), 0o644); err != nil {
		return err
	}
	body, err := json.MarshalIndent(build(), "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(directory, "corpus.generated.json"), append(body, '\n'), 0o644)
}

func constants() string {
	var out strings.Builder
	out.WriteString(`// 生成物である。手で編集しない。
//
// 出どころは internal/validate で、配っているのは cmd/rulegen である。
// 変えるならあちらを変えて make generate を走らせること。
//
// **パターンは Go の RE2 と JavaScript が同じ意味で読む書き方に限ってある。**
// 文字クラス・アンカー・回数指定だけで、後方参照も先読みも無い。

`)
	fmt.Fprintf(&out, "export const groupSegmentPattern = /%s/;\n", validate.GroupSegmentPattern)
	fmt.Fprintf(&out, "export const aliasPattern = /%s/;\n", validate.AliasPattern)
	fmt.Fprintf(&out, "export const hostnamePattern = /%s/;\n\n", validate.HostnamePattern)
	fmt.Fprintf(&out, "export const maxGroupSegments = %d;\n", validate.MaxGroupSegments)
	fmt.Fprintf(&out, "export const maxGroupSegmentBytes = %d;\n", validate.MaxGroupSegmentBytes)
	fmt.Fprintf(&out, "export const maxAliasLength = %d;\n", validate.MaxAliasLength)
	fmt.Fprintf(&out, "export const maxHostnameLength = %d;\n\n", validate.MaxHostnameLength)
	out.WriteString("// **大小文字を区別しない。** 既定の macOS ボリュームは \"Config\" と\n")
	out.WriteString("// \"config\" を同じディレクトリエントリとして扱う。\n")
	out.WriteString("export const reservedGroupNames: ReadonlySet<string> = new Set([\n")
	for _, name := range validate.ReservedGroupNames {
		fmt.Fprintf(&out, "  %q,\n", name)
	}
	out.WriteString("]);\n")
	return out.String()
}

// groupNames は、グループ名について両方の側へ突き合わせてほしい入力である。
//
// **食い違いが実際に出ていたものを先に置く。** 予約語の 4 つは、画面が緑を出して
// サーバーが断っていた組で、先頭の `-` は逆に画面だけが断っていた組である。
var groupNames = []struct{ input, why string }{
	{"work", "ふつうの名前"},
	{"work/eu", "入れ子"},
	{"rc", "予約語。画面が緑を出してサーバーが断っていた 4 つのひとつ"},
	{"environment", "同上"},
	{"known_hosts2", "同上"},
	{"authorized_keys2", "同上"},
	{"config", "予約語。両方が断っていた"},
	{"CONFIG", "予約語の大小違い。macOS は同じディレクトリエントリとして扱う"},
	{"-foo", "先頭の `-`。画面だけが断っていた——いまは両方が断る"},
	{".hidden", "先頭の `.`"},
	{"a/b/c/d/e/f", "深さちょうど"},
	{"a/b/c/d/e/f/g", "深さ超過"},
	{"", "空"},
	{"with space", "空白"},
	{"日本語", "ASCII の外"},
	{"a..b", "連続する点。ディレクトリ名としては通る"},
	{strings.Repeat("a", 64), "長さちょうど"},
	{strings.Repeat("a", 65), "長さ超過"},
}

var hostNames = []struct{ input, why string }{
	{"example.com", "DNS 名"},
	{"203.0.113.10", "IPv4"},
	{"::1", "圧縮 IPv6。先頭が ':' なので文字集合では判定できない"},
	{"2001:db8::1", "圧縮 IPv6"},
	{"fe80::1%eth0", "zone 識別子。Go は断る"},
	{"[2001:db8::1]", "角括弧付き。Go は断る"},
	{"::ffff:01.2.3.4", "IPv4 射影の先頭ゼロ。net.ParseIP は断る"},
	{"::ffff:1.2.3.4", "IPv4 射影"},
	{"-lead.example", "先頭の `-`"},
	{"trail-.example", "末尾の `-`"},
	{"", "空"},
	{strings.Repeat("a", 256), "長さ超過"},
}

var aliases = []struct{ input, why string }{
	{"bastion", "ふつう"},
	{"build-01", "ハイフン"},
	{"-oProxyCommand=id", "先頭の `-`。option に化ける綴り"},
	{"has space", "空白"},
	{`quote"inside`, "二重引用符"},
	{"*", "パターン。alias ではない"},
	{"", "空"},
	{strings.Repeat("a", 65), "長さ超過"},
}

var ports = []struct {
	input int
	why   string
}{
	{22, "既定"},
	{1, "下限"},
	{65535, "上限"},
	{0, "範囲外"},
	{65536, "範囲外"},
	{-1, "負"},
}

var renderCases = []struct {
	input []string
	why   string
}{
	{[]string{"simple"}, "そのまま"},
	{[]string{"has space"}, "空白があるので引用する"},
	{[]string{""}, "空の値も引用で表す"},
	{[]string{"#comment"}, "`#` 始まりは引用しないとコメントになる"},
	{[]string{`quote"inside`}, "二重引用符。OpenSSH に綴りが無いので Go は断る"},
	{[]string{"line\nbreak"}, "改行。同上"},
	{[]string{"a", "b c"}, "複数の値"},
	{[]string{"tab\there"}, "タブがあるので引用する"},
}

var parseCases = []struct{ input, why string }{
	{"a b", "ふつう"},
	{`a "b c"`, "引用された値"},
	{`"unbalanced`, "閉じない引用。Go は非構造化として扱う"},
	{`a"b`, "語の途中の引用符"},
	{`"a"b`, "引用の直後に文字が続く"},
	{"  spaced  out  ", "前後と間の空白"},
	{"", "空"},
}

func build() corpus {
	built := corpus{}
	for _, item := range groupNames {
		built.GroupName = append(built.GroupName, verdict{
			Input: item.input, Valid: validate.GroupName(item.input) == nil, Why: item.why,
		})
	}
	for _, item := range hostNames {
		built.HostName = append(built.HostName, verdict{
			Input: item.input, Valid: validate.Hostname(item.input) == nil, Why: item.why,
		})
	}
	for _, item := range aliases {
		built.Alias = append(built.Alias, verdict{
			Input: item.input, Valid: validate.Alias(item.input) == nil, Why: item.why,
		})
	}
	for _, item := range ports {
		built.Port = append(built.Port, portVerdict{
			Input: item.input, Valid: validate.Port(item.input) == nil, Why: item.why,
		})
	}
	for _, item := range renderCases {
		built.Render = append(built.Render, renderCase{
			Input: item.input, Output: renderLine(item.input), Refused: renderLine(item.input) == "" && len(item.input) > 0 && refuses(item.input), Why: item.why,
		})
	}
	for _, item := range parseCases {
		values, refused := parseLine(item.input)
		built.Parse = append(built.Parse, parseCase{
			Input: item.input, Values: values, Refused: refused, Why: item.why,
		})
	}
	return built
}

// renderLine は、値の並びを 1 行の引数部分として書き出す。断られたら空を返す。
func renderLine(values []string) string {
	var parts []string
	for _, value := range values {
		argument, err := config.RenderArgument("", value)
		if err != nil {
			return ""
		}
		parts = append(parts, argument.Raw)
	}
	return strings.Join(parts, " ")
}

func refuses(values []string) bool {
	for _, value := range values {
		if _, err := config.RenderArgument("", value); err != nil {
			return true
		}
	}
	return false
}

// parseLine は、引数部分ひとつを Go に読ませる。
//
// **公開の入口を通す。** 行を組み立てて config.Parse に渡し、読み取れた値を見る。
// 非構造化として扱われた行は Values を持たないので、それが断りである。
func parseLine(input string) ([]string, bool) {
	file := config.Parse([]byte("Host " + input + "\n"))
	if len(file.Lines) == 0 || file.Lines[0].Kind != config.LineDirective {
		return nil, true
	}
	values := file.Lines[0].Values()
	if values == nil {
		return []string{}, false
	}
	return values, false
}
