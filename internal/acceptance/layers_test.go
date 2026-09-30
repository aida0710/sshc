package acceptance_test

import (
	"go/parser"
	"go/token"
	"path"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const (
	layerInfrastructure = "infrastructure"
	layerPureLogic      = "pure logic"
)

// lowerLayers は、下の2つの層に置いたパッケージを、その層へ対応付ける。
//
// インフラは、ディスク・ネットワーク・OS ロックの原始操作（トランザクション、
// ジャーナル、世代バックアップ、封、条件付き PUT、セッション cookie、ファイルの
// ロック、乱数から作る識別子とトークン）を持つ。
// 純粋ロジックは、OS にもディスクにも依存しない値と規則（ssh_config の無損失パーサと
// 解決器、入力検証、通信契約の型）を持つ。どちらも internal/platform の OS 抽象と、
// 同じ層のパッケージだけを使う。
//
// この2つの層に新しいパッケージを置くときは、ここに足す。足さないと、同じ層の
// パッケージから import されたときに上の層のパッケージとして扱われる。
var lowerLayers = map[string]string{
	"sshc/internal/storage":     layerInfrastructure,
	"sshc/internal/envelope":    layerInfrastructure,
	"sshc/internal/objectstore": layerInfrastructure,
	"sshc/internal/filelock":    layerInfrastructure,
	"sshc/internal/randomid":    layerInfrastructure,
	"sshc/internal/handoff":     layerInfrastructure,
	"sshc/internal/session":     layerInfrastructure,
	"sshc/internal/config":      layerPureLogic,
	"sshc/internal/effective":   layerPureLogic,
	"sshc/internal/validate":    layerPureLogic,
	"sshc/internal/api":         layerPureLogic,
}

// osAbstraction は、どの層からも使ってよい OS 抽象のパッケージの接頭辞である。
const osAbstraction = "sshc/internal/platform"

// goImports は、Go ファイルひとつが import するパスを返す。build tag は見ないので、
// どの OS 向けのファイルも同じように数える。
func goImports(t *testing.T, relative, contents string) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), relative, contents, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse %s: %v", relative, err)
	}
	imports := make([]string, 0, len(file.Imports))
	for _, spec := range file.Imports {
		imported, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			t.Fatalf("import path in %s: %v", relative, err)
		}
		imports = append(imports, imported)
	}
	return imports
}

func isOSAbstraction(imported string) bool {
	return imported == osAbstraction || strings.HasPrefix(imported, osAbstraction+"/")
}

// TestLowerLayersImportOnlyTheirOwnLayerAndTheOSAbstraction は、インフラと純粋ロジックが、
// 上の層のパッケージにも、互いにも依存しないようにする。
//
// storage が ssh_config の解決器を組み立てていた頃、ディスクの原始操作の層が
// config の型とローカルのアカウントを知っていた。層の分け方と実際の依存が
// 食い違っても、散文では気付けない。
func TestLowerLayersImportOnlyTheirOwnLayerAndTheOSAbstraction(t *testing.T) {
	var found []string
	productionGoFiles(t, func(relative, contents string) {
		importer := "sshc/" + path.Dir(relative)
		layer, lower := lowerLayers[importer]
		if !lower {
			return
		}
		for _, imported := range goImports(t, relative, contents) {
			if !strings.HasPrefix(imported, "sshc/") || isOSAbstraction(imported) {
				continue
			}
			if lowerLayers[imported] != layer {
				found = append(found, relative+" ("+layer+") -> "+imported)
			}
		}
	})
	slices.Sort(found)
	if len(found) != 0 {
		t.Errorf("下の層が、自分の層と OS 抽象の外を import している:\n  %s", strings.Join(found, "\n  "))
	}
}

// entryPointDirectories は、argv やアプリの外殻からの入口を持つディレクトリである。
var entryPointDirectories = []string{"cmd/sshc", "mobile"}

// configEngine は、入口が直接使ってはならない ssh_config のパーサと解決器である。
//
// storage は縛らない。cmd/sshc は service の unit ファイルを原子的に書くのに、mobile は
// ワークスペースを開けなかった理由を見分けるのに storage を使う。どちらも ssh_config を
// 解決しない。
var configEngine = []string{
	"sshc/internal/config",
	"sshc/internal/effective",
}

// TestEntryPointsDoNotResolveTheConfigThemselves は、入口が ssh_config のパーサと
// 解決器を直接使わないようにする。
//
// cmd/sshc が自分で設定を解決していた頃、一覧と TUI は engine とは別の解決器を
// 通っており、選んだ先と繋がる先が食い違っていた。設定の上限のような定数ひとつの
// ために import しても、同じ経路が開く。
func TestEntryPointsDoNotResolveTheConfigThemselves(t *testing.T) {
	var found []string
	productionGoFiles(t, func(relative, contents string) {
		entryPoint := slices.ContainsFunc(entryPointDirectories, func(directory string) bool {
			return strings.HasPrefix(relative, directory+"/")
		})
		if !entryPoint {
			return
		}
		for _, imported := range goImports(t, relative, contents) {
			if slices.Contains(configEngine, imported) {
				found = append(found, relative+" -> "+imported)
			}
		}
	})
	slices.Sort(found)
	if len(found) != 0 {
		t.Errorf("入口が ssh_config のパーサか解決器を直接 import している:\n  %s", strings.Join(found, "\n  "))
	}
}
