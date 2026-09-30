package httpserver

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// openapi.yaml を読んだ側が、どの操作で何の拒否に備えればよいかを契約から読めるようにする。
// 宣言の無い状態コードは、呼び出し側が呼び出しごとに手で扱うことになる。

// problemResponseRef は、problem+json の本文を持つ共通の応答である。
const problemResponseRef = "#/components/responses/Problem"

// sessionEntryOperations は、Security.Middleware がセッションも Vault のロックも確かめずに
// 通す操作である。どちらもセッションを作るための操作なので、401 の代わりにハンドラが自分で
// 断り、Vault のロック中の 409 も返さない。
var sessionEntryOperations = map[string]bool{
	"POST /api/v1/session/bootstrap": true,
	"POST /api/v1/session/recover":   true,
}

// Security.Middleware は、ハンドラより先にどの操作でも拒否を返しうる。403 は Host、
// Fetch Metadata、Origin、CSRF のどれかが合わないとき、401 はセッションが無いとき、
// 409 vault_locked は Vault がロックされているときである。呼び出し側はどの操作でも
// この 3 つに備えるので、操作ごとに宣言する。
func TestEveryOperationDeclaresTheRefusalsOfTheSecurityMiddleware(t *testing.T) {
	declared := declaredResponses(t)
	for _, operation := range sortedOperations(declared) {
		method, path, _ := strings.Cut(operation, " ")
		wanted := []int{http.StatusForbidden}
		if !sessionEntryOperations[operation] {
			wanted = append(wanted, http.StatusUnauthorized)
			if !gateExempt(method, path) {
				wanted = append(wanted, http.StatusConflict)
			}
		}
		for _, status := range wanted {
			response, ok := declared[operation][strconv.Itoa(status)]
			if !ok {
				t.Errorf("%s: Security.Middleware が返す %d を宣言していない", operation, status)
				continue
			}
			if !response.carriesProblem() {
				t.Errorf("%s: %d の応答に %s の本文を宣言していない", operation, status, problemMediaType)
			}
		}
	}
}

// ハンドラが自分で書く拒否の状態コードを、その操作に宣言していることを確かめる。
// どこまで読むかは handlerErrorStatuses を参照。エラーの値から状態コードを決める関数
// （sftpProblem など）の分は読まないので、手で宣言する。
func TestEveryOperationDeclaresTheProblemsItsHandlerWrites(t *testing.T) {
	source := readPackageSource(t)
	declared := declaredResponses(t)
	found := map[string]bool{}
	for _, route := range source.apiRoutes(t) {
		found[route.Operation] = true
		responses, ok := declared[route.Operation]
		if !ok {
			t.Errorf("%s: 経路は登録されているが、openapi.yaml に操作が無い", route.Operation)
			continue
		}
		statuses := source.handlerErrorStatuses(t, route)
		for _, status := range sortedStatuses(statuses) {
			if _, ok := responses[strconv.Itoa(status)]; !ok {
				t.Errorf("%s: %s は %d を返すが、この操作に宣言していない", route.Operation, route.Handler, status)
			}
		}
	}
	// 登録の書き方が変わってソースから読めなくなった経路は、何も比べずに通ってしまう。
	for _, operation := range sortedOperations(declared) {
		if !found[operation] {
			t.Errorf("%s: ソースから経路の登録を読めなかった", operation)
		}
	}
}

// problem+json を書く関数が増えたら problemConstructors にも足す。足さないと、その関数で
// 書く状態コードを上の検査が読まない。
func TestProblemConstructorsListEveryFunctionThatWritesAProblem(t *testing.T) {
	writers := readPackageSource(t).functionsWritingProblemBodies()
	for _, name := range writers {
		if !problemConstructors[name] {
			t.Errorf("%s は %s の本文を書くが、problemConstructors に無い", name, problemMediaType)
		}
	}
	if len(writers) == 0 {
		t.Error("problem+json を書く関数をひとつも見つけられなかった。探し方が壊れている")
	}
}

type declaredResponse struct {
	Ref     string               `yaml:"$ref"`
	Content map[string]yaml.Node `yaml:"content"`
}

func (response declaredResponse) carriesProblem() bool {
	if response.Ref == problemResponseRef {
		return true
	}
	_, ok := response.Content[problemMediaType]
	return ok
}

// declaredResponses は、"PATCH /api/v1/terminal/sessions/{id}" の形の操作ごとに、
// 状態コードから宣言した応答を引く。
func declaredResponses(t *testing.T) map[string]map[string]declaredResponse {
	t.Helper()
	operations := map[string]map[string]declaredResponse{}
	for path, item := range readOpenAPIPaths(t) {
		for method, node := range item {
			method = strings.ToUpper(method)
			if !operationMethods[method] {
				continue
			}
			var operation struct {
				Responses map[string]declaredResponse `yaml:"responses"`
			}
			if err := node.Decode(&operation); err != nil {
				t.Fatalf("%s %s: %v", method, path, err)
			}
			operations[method+" "+path] = operation.Responses
		}
	}
	// 読み方を壊したときに、何も比べずに通らないようにする。
	if len(operations) < 100 {
		t.Fatalf("openapi.yaml から操作を %d 個しか読めなかった", len(operations))
	}
	return operations
}

func sortedOperations(operations map[string]map[string]declaredResponse) []string {
	names := make([]string, 0, len(operations))
	for name := range operations {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedStatuses(statuses map[int]bool) []int {
	sorted := make([]int, 0, len(statuses))
	for status := range statuses {
		sorted = append(sorted, status)
	}
	sort.Ints(sorted)
	return sorted
}
