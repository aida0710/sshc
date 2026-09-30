package buildcontract

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// workflowDocument は、GitHub Actions の workflow のうち、契約テストが見る部分である。
// ci.yml、release.yml、pages.yml の契約テストが同じ型で読む。
type workflowDocument struct {
	On          workflowNames          `yaml:"on"`
	Permissions map[string]string      `yaml:"permissions"`
	Jobs        map[string]workflowJob `yaml:"jobs"`
}

type workflowJob struct {
	Name        string            `yaml:"name"`
	RunsOn      string            `yaml:"runs-on"`
	Needs       workflowNames     `yaml:"needs"`
	Permissions map[string]string `yaml:"permissions"`
	// Uses は、job 全体を別の workflow（reusable workflow）に任せるときの参照である。
	Uses     string            `yaml:"uses"`
	Strategy *workflowStrategy `yaml:"strategy"`
	Steps    []workflowStep    `yaml:"steps"`
}

type workflowStrategy struct {
	FailFast *bool          `yaml:"fail-fast"`
	Matrix   workflowMatrix `yaml:"matrix"`
}

type workflowMatrix struct {
	Include []workflowMatrixEntry `yaml:"include"`
}

type workflowMatrixEntry struct {
	OS   string `yaml:"os"`
	Name string `yaml:"name"`
}

type workflowStep struct {
	Name            string         `yaml:"name"`
	If              string         `yaml:"if"`
	Uses            string         `yaml:"uses"`
	Run             string         `yaml:"run"`
	Shell           string         `yaml:"shell"`
	With            map[string]any `yaml:"with"`
	ContinueOnError *bool          `yaml:"continue-on-error"`
}

// workflowNames は、1つの名前でも名前の並びでも書ける値（on、needs）である。
// on はイベントごとの設定を持つ mapping でも書けるので、そのキーを名前として読む。
type workflowNames []string

func (names *workflowNames) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		*names = workflowNames{node.Value}
		return nil
	case yaml.SequenceNode:
		var list []string
		if err := node.Decode(&list); err != nil {
			return err
		}
		*names = list
		return nil
	case yaml.MappingNode:
		keys := make(workflowNames, 0, len(node.Content)/2)
		for index := 0; index < len(node.Content); index += 2 {
			keys = append(keys, node.Content[index].Value)
		}
		*names = keys
		return nil
	}
	return fmt.Errorf("line %d: expected a name, a list of names, or a mapping", node.Line)
}

func workflowsDirectory() string {
	return filepath.Join("..", "..", ".github", "workflows")
}

// readWorkflowFile は、workflow のファイルを読み、読んだ文書と元の文字列を返す。
func readWorkflowFile(t *testing.T, path string) (workflowDocument, string) {
	t.Helper()
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	document, err := decodeWorkflowDocument(source)
	if err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return document, string(source)
}

func decodeWorkflowDocument(source []byte) (workflowDocument, error) {
	var document workflowDocument
	if err := yaml.Unmarshal(source, &document); err != nil {
		return workflowDocument{}, err
	}
	if document.Jobs == nil {
		return workflowDocument{}, fmt.Errorf("jobs mapping is missing")
	}
	return document, nil
}
