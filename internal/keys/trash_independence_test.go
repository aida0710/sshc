package keys

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// ゴミ箱の一覧は、鍵の一覧と同じ重さを持たない。
//
// ここが inventory に訊くのは Kind と Fingerprint だけである（restoreBlockers を
// 見よ）。かつては Inventory() を呼んでおり、あれは走査に加えて ssh_config を
// 解決して参照を紐付ける。画面は鍵の一覧とこれを同時に読むので、Include を辿る
// 一番重い仕事が毎回二度走っていた。実測で 31ms が 7ms になった。
//
// 壊れた設定でも落ちないことでは、これを確かめられない。Inventory() は
// 解決の失敗を飲んで走査の結果だけを返すので、どちらの実装でも落ちない。
// 確かめられるのは「呼んでいないこと」そのものである。
func TestListingTheTrashDoesNotResolveTheConfiguration(t *testing.T) {
	set := token.NewFileSet()
	file, err := parser.ParseFile(set, "trash.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}

	var listTrash *ast.FuncDecl
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Name.Name == "ListTrash" {
			listTrash = function
		}
	}
	if listTrash == nil {
		t.Fatal("trash.go に ListTrash が無い")
	}

	ast.Inspect(listTrash, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if ok && selector.Sel.Name == "Inventory" {
			t.Errorf("ListTrash が %s.Inventory を呼んでいる: 走査だけでよく、"+
				"ssh_config の解決は一度も読まれない", exprText(selector.X))
		}
		return true
	})
}

func exprText(expression ast.Expr) string {
	identifier, ok := expression.(*ast.Ident)
	if !ok {
		return strings.TrimSpace("(式)")
	}
	return identifier.Name
}
