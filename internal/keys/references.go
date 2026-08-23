package keys

import (
	"path/filepath"
	"strings"

	"sshc/internal/config"
	"sshc/internal/storage"
)

// Reference は、鍵ファイルを指定する設定ディレクティブひとつ。
type Reference struct {
	Directive    string
	ConfigPath   string
	Line         int
	Condition    string
	HostPatterns []string
	Value        string
}

// UnresolvedReference は、エンジンが引数の推測を拒むディレクティブ。UI が、
// でっちあげの結果ではなく本当の理由を表示できるようにするためである。
type UnresolvedReference struct {
	Directive  string
	Value      string
	ConfigPath string
	Line       int
	Reason     string
}

// 未解決の理由コード。
const (
	ReasonUnsupportedToken = "unsupported_token"
	ReasonRelativePath     = "relative_path"
	ReasonOutsideWorkspace = "outside_workspace"
)

// referencedDirectives は、鍵ファイルまたはエージェントを指定するクライアント
// ディレクティブ。それ以外のディレクティブは、このインデックスでは無視される。
var referencedDirectives = []string{"IdentityFile", "CertificateFile", "IdentityAgent"}

// ReferenceIndex は、ワークスペース相対のパスを、それを指定するディレクティブに対応付ける。
type ReferenceIndex struct {
	byRelativePath map[string][]Reference
	agent          []Reference
	unresolved     []UnresolvedReference
}

func (index *ReferenceIndex) For(relativePath string) []Reference {
	return index.byRelativePath[relativePath]
}

func (index *ReferenceIndex) AgentDelegations() []Reference { return index.agent }

func (index *ReferenceIndex) Unresolved() []UnresolvedReference { return index.unresolved }

// BuildReferenceIndex は、Include グラフが到達したすべてのファイルを走査し、どの
// Host がどの鍵ファイルを指定しているかを記録する。
func BuildReferenceIndex(graph *config.Graph, workspace *storage.Workspace) *ReferenceIndex {
	index := &ReferenceIndex{byRelativePath: make(map[string][]Reference)}
	for _, path := range graph.Order {
		node := graph.Nodes[path]
		if node == nil || node.File == nil {
			continue
		}
		for lineIndex, line := range node.File.Lines {
			if line.Kind != config.LineDirective {
				continue
			}
			directive, matched := matchDirective(line.Keyword)
			if !matched {
				continue
			}
			block := node.File.BlockAt(lineIndex)
			condition := node.File.Condition(block)
			patterns := make([]string, 0, len(block.Patterns))
			for _, pattern := range block.Patterns {
				patterns = append(patterns, pattern.Raw)
			}
			for _, value := range line.Values() {
				index.record(workspace, directive, value, path, lineIndex+1, condition, patterns)
			}
		}
	}
	return index
}

func matchDirective(keyword string) (string, bool) {
	for _, directive := range referencedDirectives {
		if config.EqualKeyword(keyword, directive) {
			return directive, true
		}
	}
	return "", false
}

func (index *ReferenceIndex) record(
	workspace *storage.Workspace,
	directive, value, configPath string,
	line int,
	condition string,
	patterns []string,
) {
	reference := Reference{
		Directive:    directive,
		ConfigPath:   configPath,
		Line:         line,
		Condition:    condition,
		HostPatterns: patterns,
		Value:        value,
	}
	if directive == "IdentityAgent" {
		index.agent = append(index.agent, reference)
		if value == "none" || value == "SSH_AUTH_SOCK" {
			return
		}
	}

	// Home に対して展開し、Root に対して比較する。~/.ssh がリンク経由で到達される
	// とき、その両者を同じ空間にするのが Normalise である。
	expanded, reason := expandKeyPath(value, workspace.Home())
	absolute := workspace.Normalise(expanded)
	if reason != "" {
		index.unresolved = append(index.unresolved, UnresolvedReference{
			Directive: directive, Value: value, ConfigPath: configPath, Line: line, Reason: reason,
		})
		return
	}
	if !workspace.Contains(absolute) {
		index.unresolved = append(index.unresolved, UnresolvedReference{
			Directive: directive, Value: value, ConfigPath: configPath, Line: line, Reason: ReasonOutsideWorkspace,
		})
		return
	}
	relative, err := filepath.Rel(workspace.Root(), absolute)
	if err != nil {
		index.unresolved = append(index.unresolved, UnresolvedReference{
			Directive: directive, Value: value, ConfigPath: configPath, Line: line, Reason: ReasonOutsideWorkspace,
		})
		return
	}
	// 鍵はワークスペース相対の識別子で引かれる。その表記はスラッシュ区切りで
	// あり、Inventory が付ける RelativePath と同じものでなければならない。ここだけ
	// このファイルシステムの区切り文字にすると、Windows では For がひとつも当たらず、
	// 鍵を名指す IdentityFile は「誰にも名指されていない」ことになる。
	key := filepath.ToSlash(relative)
	index.byRelativePath[key] = append(index.byRelativePath[key], reference)
}

// expandKeyPath は、IdentityFile 形式の引数を絶対パスへ解決する。
//
// 展開するのは '%d' と先頭の '~/' だけである。接続先ホストが決まる前に意味の
// 定まる形式はそれだけだからだ。相対パスは推測せずに報告する。OpenSSH はそれを
// ssh プロセスの作業ディレクトリに対して解決するが、このアプリケーションには
// それが分からない。
func expandKeyPath(value, home string) (absolute string, reason string) {
	if value == "" {
		return "", ReasonUnsupportedToken
	}
	expanded := value
	if strings.ContainsRune(expanded, '%') {
		var builder strings.Builder
		for index := 0; index < len(expanded); index++ {
			if expanded[index] != '%' {
				builder.WriteByte(expanded[index])
				continue
			}
			index++
			if index >= len(expanded) {
				return "", ReasonUnsupportedToken
			}
			switch expanded[index] {
			case '%':
				builder.WriteByte('%')
			case 'd':
				builder.WriteString(home)
			default:
				return "", ReasonUnsupportedToken
			}
		}
		expanded = builder.String()
	}
	switch {
	case expanded == "~":
		expanded = home
	case strings.HasPrefix(expanded, "~/"):
		expanded = filepath.Join(home, expanded[2:])
	case strings.HasPrefix(expanded, "~"):
		return "", ReasonUnsupportedToken
	case !filepath.IsAbs(expanded):
		return "", ReasonRelativePath
	}
	return filepath.Clean(expanded), ""
}

// AttachReferences は、各ファイルを指定している Host をそのインベントリの item
// へ写し、エンジンが解決できなかったディレクティブを記録する。
func (inventory *Inventory) AttachReferences(index *ReferenceIndex) {
	for itemIndex := range inventory.Items {
		item := &inventory.Items[itemIndex]
		item.References = index.For(item.RelativePath)
	}
	inventory.AgentDelegations = index.AgentDelegations()
	inventory.UnresolvedReferences = index.Unresolved()
}

// ExpandsTo は、IdentityFile 形式の引数がまさにこのファイルを指定しているかを
// 報告する。他のパッケージがその問いを尋ねられる唯一の手段である。expandKeyPath は
// 非公開のままにしてあるので、このエンジンが何の推測を拒むか（相対パスや未知の
// トークン）は一か所で決まる。
// 素のホームではなくワークスペースを渡す。結果はルート配下のパスとの比較であり、
// 比較を行う前に二つの表記を突き合わせておく必要があるからだ。ホームだけを渡すと、
// 展開された "~/.ssh/…" と、解決済みのルートから組み立てたパスとを比較した
// 呼び出し側は、それらは別のファイルであると告げられて
// しまった。
func ExpandsTo(workspace *storage.Workspace, value, absolute string) bool {
	expanded, reason := expandKeyPath(value, workspace.Home())
	return reason == "" && workspace.Normalise(expanded) == workspace.Normalise(absolute)
}

// ResolveWorkspaceKeyPath は参照インデックスと同じ制限で IdentityFile を解決し、Vault の
// subject と、シンボリックリンクを解決した安定パスを返す。相対パスと ~/.ssh 外の
// パスは推測しない。
func ResolveWorkspaceKeyPath(workspace *storage.Workspace, value string) (relative, promptPath string, ok bool) {
	expanded, reason := expandKeyPath(value, workspace.Home())
	if reason != "" {
		return "", "", false
	}
	normalised := workspace.Normalise(filepath.Clean(expanded))
	if !workspace.Contains(normalised) || normalised == workspace.Root() {
		return "", "", false
	}
	relative, err := filepath.Rel(workspace.Root(), normalised)
	if err != nil || relative == "." || relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", "", false
	}
	return filepath.ToSlash(relative), filepath.Clean(normalised), true
}
