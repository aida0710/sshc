package httpserver

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// このファイルは、このパッケージのソースを読み、経路の登録と、各ハンドラが自分で書く
// 拒否の状態コードを取り出す。openapi.yaml と突き合わせる検査は
// problem_status_contract_test.go にある。

// problemConstructors は、拒否を problem+json で書く関数である。どれも状態コードを
// problemStatusArgument の位置の引数で受け取る。
var problemConstructors = map[string]bool{"problem": true, "problemDetail": true, "problemWith": true}

const problemStatusArgument = 1

// problemMediaType は、problemConstructors が書く本文の Content-Type である。
const problemMediaType = "application/problem+json"

// errorStatusByName は、ハンドラが http の定数名で書く拒否の状態コードである。
// ソースは型検査せずに読むので、定数の値はここで引く。
var errorStatusByName = map[string]int{
	"StatusBadRequest":                   http.StatusBadRequest,
	"StatusUnauthorized":                 http.StatusUnauthorized,
	"StatusForbidden":                    http.StatusForbidden,
	"StatusNotFound":                     http.StatusNotFound,
	"StatusMethodNotAllowed":             http.StatusMethodNotAllowed,
	"StatusConflict":                     http.StatusConflict,
	"StatusPreconditionFailed":           http.StatusPreconditionFailed,
	"StatusRequestEntityTooLarge":        http.StatusRequestEntityTooLarge,
	"StatusUnsupportedMediaType":         http.StatusUnsupportedMediaType,
	"StatusRequestedRangeNotSatisfiable": http.StatusRequestedRangeNotSatisfiable,
	"StatusUnprocessableEntity":          http.StatusUnprocessableEntity,
	"StatusTooManyRequests":              http.StatusTooManyRequests,
	"StatusInternalServerError":          http.StatusInternalServerError,
	"StatusNotImplemented":               http.StatusNotImplemented,
	"StatusBadGateway":                   http.StatusBadGateway,
	"StatusServiceUnavailable":           http.StatusServiceUnavailable,
	"StatusGatewayTimeout":               http.StatusGatewayTimeout,
	"StatusInsufficientStorage":          http.StatusInsufficientStorage,
}

// operationMethods は、経路の登録と openapi.yaml の操作に使う HTTP メソッドである。
var operationMethods = map[string]bool{
	http.MethodGet: true, http.MethodPost: true, http.MethodPut: true, http.MethodPatch: true, http.MethodDelete: true,
}

// routePathHelpers は、経路の登録でパスを包む関数である。どれも引数のパスを group の中の
// 相対パスに直すだけなので、API の経路かどうかは引数のパスで決める。
var routePathHelpers = map[string]bool{"cliRoute": true}

// handlerAdapters は、経路の登録でハンドラを包む関数である。包む関数が自分で書く拒否も、
// その経路の拒否として数える。
var handlerAdapters = map[string]bool{"profileNamed": true}

// echoPathParameter は、echo の経路の `:id` を、openapi.yaml の `{id}` に書き直すために探す。
var echoPathParameter = regexp.MustCompile(`:([A-Za-z][A-Za-z0-9]*)`)

// sourceRoute は、ソースに書かれた API の経路の登録 1 つである。
type sourceRoute struct {
	// Operation は openapi.yaml と同じ "PATCH /api/v1/terminal/sessions/{id}" の形である。
	Operation string
	// Handler は "TerminalHandlers.Rename" の形のメソッド名である。
	Handler string
	// Adapter は、Handler を包む handlerAdapters の関数の名前である。包まなければ空である。
	Adapter string
}

// packageSource は、このパッケージのテスト以外のソースである。
type packageSource struct {
	fileSet   *token.FileSet
	files     []*ast.File
	constants map[string]string
	functions map[string]*ast.FuncDecl
	// methods は "TerminalHandlers.Rename" の形で引く。
	methods map[string]*ast.FuncDecl
}

func readPackageSource(t *testing.T) packageSource {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	source := packageSource{
		fileSet:   token.NewFileSet(),
		constants: map[string]string{},
		functions: map[string]*ast.FuncDecl{},
		methods:   map[string]*ast.FuncDecl{},
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(source.fileSet, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		source.files = append(source.files, file)
		source.indexDeclarations(file)
	}
	if len(source.methods) == 0 {
		t.Fatal("ソースからメソッドをひとつも読めなかった")
	}
	return source
}

func (source packageSource) indexDeclarations(file *ast.File) {
	for _, declaration := range file.Decls {
		switch declaration := declaration.(type) {
		case *ast.GenDecl:
			if declaration.Tok == token.CONST {
				source.indexStringConstants(declaration)
			}
		case *ast.FuncDecl:
			if declaration.Recv == nil {
				source.functions[declaration.Name.Name] = declaration
			} else {
				source.methods[typeName(declaration.Recv.List[0].Type)+"."+declaration.Name.Name] = declaration
			}
		}
	}
}

func (source packageSource) indexStringConstants(declaration *ast.GenDecl) {
	for _, specification := range declaration.Specs {
		value := specification.(*ast.ValueSpec)
		for index, name := range value.Names {
			if index >= len(value.Values) {
				continue
			}
			literal, ok := value.Values[index].(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				continue
			}
			if unquoted, err := strconv.Unquote(literal.Value); err == nil {
				source.constants[name.Name] = unquoted
			}
		}
	}
}

// apiRoutes は、`engine.PATCH("/api/v1/...", handlers.Rename)` の形の登録を集める。
// handlers の型は、登録する関数の引数か、`handlers := Handlers{...}` のような代入から引く。
func (source packageSource) apiRoutes(t *testing.T) []sourceRoute {
	t.Helper()
	var routes []sourceRoute
	for _, file := range source.files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			routes = append(routes, source.apiRoutesIn(t, function)...)
		}
	}
	sort.Slice(routes, func(i, j int) bool { return routes[i].Operation < routes[j].Operation })
	return routes
}

func (source packageSource) apiRoutesIn(t *testing.T, function *ast.FuncDecl) []sourceRoute {
	variableTypes := map[string]string{}
	for _, field := range function.Type.Params.List {
		for _, name := range field.Names {
			variableTypes[name.Name] = typeName(field.Type)
		}
	}
	var routes []sourceRoute
	ast.Inspect(function.Body, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.AssignStmt:
			source.recordAssignedType(node, variableTypes)
		case *ast.CallExpr:
			if route, ok := source.parseAPIRoute(t, node, variableTypes); ok {
				routes = append(routes, route)
			}
		}
		return true
	})
	return routes
}

func (source packageSource) recordAssignedType(assignment *ast.AssignStmt, variableTypes map[string]string) {
	if len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
		return
	}
	variable, ok := assignment.Lhs[0].(*ast.Ident)
	if !ok {
		return
	}
	switch value := assignment.Rhs[0].(type) {
	case *ast.CompositeLit:
		variableTypes[variable.Name] = typeName(value.Type)
	case *ast.CallExpr:
		constructor, ok := value.Fun.(*ast.Ident)
		if !ok {
			return
		}
		declaration := source.functions[constructor.Name]
		if declaration != nil && declaration.Type.Results != nil && len(declaration.Type.Results.List) == 1 {
			variableTypes[variable.Name] = typeName(declaration.Type.Results.List[0].Type)
		}
	}
}

func (source packageSource) parseAPIRoute(t *testing.T, call *ast.CallExpr, variableTypes map[string]string) (sourceRoute, bool) {
	registration, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !operationMethods[registration.Sel.Name] || len(call.Args) != 2 {
		return sourceRoute{}, false
	}
	path, ok := source.stringValue(call.Args[0])
	if !ok {
		t.Errorf("%s: 経路の登録のパスを文字列として読めない", source.fileSet.Position(call.Pos()))
		return sourceRoute{}, false
	}
	if !strings.HasPrefix(path, "/api/") {
		return sourceRoute{}, false
	}
	handlerExpression, adapter := unwrapHandlerAdapter(call.Args[1])
	handler, isSelector := handlerExpression.(*ast.SelectorExpr)
	variable, isVariable := selectorVariable(handler)
	if !isSelector || !isVariable || variableTypes[variable] == "" {
		t.Errorf("%s: %s のハンドラを `変数.メソッド` の形で読めない", source.fileSet.Position(call.Pos()), path)
		return sourceRoute{}, false
	}
	return sourceRoute{
		Operation: registration.Sel.Name + " " + echoPathParameter.ReplaceAllString(path, "{$1}"),
		Handler:   variableTypes[variable] + "." + handler.Sel.Name,
		Adapter:   adapter,
	}, true
}

// unwrapHandlerAdapter は、`profileNamed(handlers.Update)` のように handlerAdapters で包んだ
// ハンドラから、包まれたハンドラと包む関数の名前を取り出す。包んでいなければそのまま返す。
func unwrapHandlerAdapter(expression ast.Expr) (ast.Expr, string) {
	call, ok := expression.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return expression, ""
	}
	adapter, ok := call.Fun.(*ast.Ident)
	if !ok || !handlerAdapters[adapter.Name] {
		return expression, ""
	}
	return call.Args[0], adapter.Name
}

func (source packageSource) stringValue(expression ast.Expr) (string, bool) {
	switch expression := expression.(type) {
	case *ast.BasicLit:
		if expression.Kind != token.STRING {
			return "", false
		}
		value, err := strconv.Unquote(expression.Value)
		return value, err == nil
	case *ast.Ident:
		value, ok := source.constants[expression.Name]
		return value, ok
	case *ast.CallExpr:
		helper, ok := expression.Fun.(*ast.Ident)
		if !ok || !routePathHelpers[helper.Name] || len(expression.Args) != 1 {
			return "", false
		}
		return source.stringValue(expression.Args[0])
	}
	return "", false
}

// handlerErrorStatuses は、ハンドラが problemConstructors で書く状態コードを集める。
//
// ハンドラが同じ receiver で呼ぶメソッド（h.list() など）もたどる。sftpProblem のように
// エラーの値から状態コードを決める関数はたどらない。どの状態コードになるかはエラーの値で
// 決まり、ソースだけからは分からないからである。
func (source packageSource) handlerErrorStatuses(t *testing.T, route sourceRoute) map[int]bool {
	t.Helper()
	collector := errorStatusCollector{t: t, source: source, visited: map[string]bool{}, statuses: map[int]bool{}}
	collector.collect(route.Handler)
	if route.Adapter != "" {
		collector.collectAdapter(route.Adapter)
	}
	return collector.statuses
}

// errorStatusCollector は、1 つのハンドラからたどったメソッドの状態コードを集める。
type errorStatusCollector struct {
	t        *testing.T
	source   packageSource
	visited  map[string]bool
	statuses map[int]bool
}

func (collector errorStatusCollector) collect(method string) {
	if collector.visited[method] {
		return
	}
	collector.visited[method] = true
	declaration := collector.source.methods[method]
	if declaration == nil || declaration.Body == nil {
		collector.t.Errorf("ソースに %s が無い", method)
		return
	}
	owner, _, _ := strings.Cut(method, ".")
	receiver := receiverName(declaration)
	ast.Inspect(declaration.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch function := call.Fun.(type) {
		case *ast.Ident:
			collector.recordProblem(call, function)
		case *ast.SelectorExpr:
			variable, isVariable := selectorVariable(function)
			sibling := owner + "." + function.Sel.Name
			if isVariable && receiver != "" && variable == receiver && collector.source.methods[sibling] != nil {
				collector.collect(sibling)
			}
		}
		return true
	})
}

// collectAdapter は、handlerAdapters の関数が自分で書く状態コードを集める。
func (collector errorStatusCollector) collectAdapter(name string) {
	declaration := collector.source.functions[name]
	if declaration == nil || declaration.Body == nil {
		collector.t.Errorf("ソースに %s が無い", name)
		return
	}
	ast.Inspect(declaration.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if function, ok := call.Fun.(*ast.Ident); ok {
			collector.recordProblem(call, function)
		}
		return true
	})
}

// recordProblem は、呼び出しが problemConstructors なら、その状態コードを記録する。
func (collector errorStatusCollector) recordProblem(call *ast.CallExpr, function *ast.Ident) {
	if problemConstructors[function.Name] && len(call.Args) > problemStatusArgument {
		collector.record(call.Args[problemStatusArgument])
	}
}

// record は、problemConstructors に渡した状態コードの引数を値にして記録する。
func (collector errorStatusCollector) record(argument ast.Expr) {
	position := collector.source.fileSet.Position(argument.Pos())
	constant, ok := argument.(*ast.SelectorExpr)
	if !ok {
		collector.t.Errorf("%s: 状態コードが http の定数ではないので、ハンドラが何を返すか読めない", position)
		return
	}
	status, known := errorStatusByName[constant.Sel.Name]
	if !known {
		collector.t.Errorf("%s: http.%s を errorStatusByName に足す", position, constant.Sel.Name)
		return
	}
	collector.statuses[status] = true
}

// functionsWritingProblemBodies は、problemMediaType の文字列を使う関数の名前を返す。
func (source packageSource) functionsWritingProblemBodies() []string {
	var names []string
	for name, declaration := range source.functions {
		if declaration.Body != nil && mentionsString(declaration.Body, problemMediaType) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func mentionsString(node ast.Node, wanted string) bool {
	found := false
	ast.Inspect(node, func(node ast.Node) bool {
		if literal, ok := node.(*ast.BasicLit); ok && literal.Kind == token.STRING {
			if value, err := strconv.Unquote(literal.Value); err == nil && value == wanted {
				found = true
			}
		}
		return !found
	})
	return found
}

func selectorVariable(selector *ast.SelectorExpr) (string, bool) {
	if selector == nil {
		return "", false
	}
	variable, ok := selector.X.(*ast.Ident)
	if !ok {
		return "", false
	}
	return variable.Name, true
}

func receiverName(declaration *ast.FuncDecl) string {
	if declaration.Recv == nil || len(declaration.Recv.List[0].Names) == 0 {
		return ""
	}
	return declaration.Recv.List[0].Names[0].Name
}

func typeName(expression ast.Expr) string {
	switch expression := expression.(type) {
	case *ast.StarExpr:
		return typeName(expression.X)
	case *ast.Ident:
		return expression.Name
	}
	return ""
}
