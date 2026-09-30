package vpnrefusal

import (
	"maps"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"sshc/internal/vpn"
)

// 日本語で言える理由は、どれも英語でも言える。足し忘れると、CLI だけ汎用の文になる。
func TestEveryJapaneseSentenceHasAnEnglishOne(t *testing.T) {
	compare := func(name string, japaneseKeys, englishKeys []string) {
		t.Helper()
		slices.Sort(japaneseKeys)
		slices.Sort(englishKeys)
		if !slices.Equal(japaneseKeys, englishKeys) {
			t.Errorf("%s: 日本語 = %v, 英語 = %v", name, japaneseKeys, englishKeys)
		}
	}
	compare("codes", slices.Collect(maps.Keys(japanese.codes)), slices.Collect(maps.Keys(english.codes)))
	compare("fields", reasonKeys(japanese.fields), reasonKeys(english.fields))
	compare("destinations", reasonKeys(japanese.destinations), reasonKeys(english.destinations))
	compare("directives", reasonKeys(japanese.directives), reasonKeys(english.directives))
	compare("routes", failureKeys(japanese.routes), failureKeys(english.routes))
	compare("targets", failureKeys(japanese.targets), failureKeys(english.targets))
}

func reasonKeys(table map[vpn.Reason]string) []string {
	var keys []string
	for reason := range table {
		keys = append(keys, string(reason))
	}
	return keys
}

func failureKeys(table map[vpn.FailureReason]string) []string {
	var keys []string
	for reason := range table {
		keys = append(keys, string(reason))
	}
	return keys
}

// englishCatalogEntry は、画面の英語のカタログの1行（"vpn.<group>.<reason>": "<文>",）である。
var englishCatalogEntry = regexp.MustCompile(`^\s*"vpn\.(failure|targetFailure|destination|field|configLine|configFile)\.([a-z0-9_]+)":\s*(".*"),$`)

// readEnglishCatalog は、画面の英語のカタログから、VPN の理由の文を group ごとに読む。
func readEnglishCatalog(t *testing.T) map[string]map[string]string {
	t.Helper()
	contents, err := os.ReadFile("../../web/src/i18n/messages/en.ts")
	if err != nil {
		t.Fatal(err)
	}
	catalog := map[string]map[string]string{}
	for _, line := range strings.Split(string(contents), "\n") {
		match := englishCatalogEntry.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		text, err := strconv.Unquote(match[3])
		if err != nil {
			t.Fatalf("%s: %v", line, err)
		}
		if catalog[match[1]] == nil {
			catalog[match[1]] = map[string]string{}
		}
		catalog[match[1]][match[2]] = text
	}
	for _, group := range []string{"failure", "targetFailure", "destination", "field", "configLine", "configFile"} {
		if len(catalog[group]) == 0 {
			t.Fatalf("en.ts から vpn.%s.* を読めない", group)
		}
	}
	return catalog
}

// CLI の英語の文は、画面の英語と同じ文面にする。同じ失敗で、画面と CLI の言い方が
// 分かれない。
func TestTheEnglishSentencesMatchTheScreen(t *testing.T) {
	catalog := readEnglishCatalog(t)
	expect := func(key, got, want string) {
		t.Helper()
		if got != want {
			t.Errorf("%s:\n CLI  = %q\n 画面 = %q", key, got, want)
		}
	}
	for reason, want := range catalog["failure"] {
		expect("vpn.failure."+reason, english.routes[vpn.FailureReason(reason)], want)
	}
	for reason, want := range catalog["targetFailure"] {
		expect("vpn.targetFailure."+reason, english.targets[vpn.FailureReason(reason)], want)
	}
	for reason, want := range catalog["destination"] {
		expect("vpn.destination."+reason, english.destinations[vpn.Reason(reason)], want)
	}
	for reason, want := range catalog["field"] {
		expect("vpn.field."+reason, english.fields[vpn.Reason(reason)], strings.ReplaceAll(want, "{limit}", "%d"))
	}
	placeholders := strings.NewReplacer("{line}", "12", "{directive}", "up")
	for reason, want := range catalog["configLine"] {
		refusal := Refusal{Reason: reason, Line: 12, Directive: "up"}
		expect("vpn.configLine."+reason, english.configLineSentence(refusal, "{reason}"), placeholders.Replace(want))
	}
	for reason, want := range catalog["configFile"] {
		refusal := Refusal{Reason: reason, Directive: "up"}
		expect("vpn.configFile."+reason, english.configLineSentence(refusal, ""), placeholders.Replace(want))
	}
}

// CLI の文は、何に失敗したかを先に書き、理由を続ける。
func TestAnEnglishRouteFailureSaysWhatFailedFirst(t *testing.T) {
	sentence := EnglishSentence(Refusal{Code: CodeRouteFailed, Reason: string(vpn.FailureHandshakeTimeout)})

	if sentence != "Connecting to the VPN failed. The handshake failed. Check the keys and the server." {
		t.Fatalf("sentence = %q", sentence)
	}
}

// 設定ファイルの誤りは、項目と行番号を添えて言う。
func TestAnEnglishConfigLineNamesTheFieldAndTheLine(t *testing.T) {
	sentence := EnglishSentence(Refusal{
		Code: CodeProfileInvalid, Field: "openvpn.config", Reason: string(vpn.ReasonRunsCommand),
		Line: 3, Directive: "up",
	})

	want := `openvpn.config: Line 3: "up" runs a command or loads a program, so it cannot be used.`
	if sentence != want {
		t.Fatalf("sentence = %q, want %q", sentence, want)
	}
}

// 上限のある理由は、上限を入れて言う。
func TestAnEnglishLimitIsFilledIn(t *testing.T) {
	sentence := EnglishSentence(Refusal{Code: CodeProfileInvalid, Field: "name", Reason: string(vpn.ReasonTooLong), Limit: 48})

	if sentence != "name: Too long: up to 48 characters." {
		t.Fatalf("sentence = %q", sentence)
	}
}
