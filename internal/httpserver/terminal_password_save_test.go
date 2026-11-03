package httpserver

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"sshc/internal/api"
	"sshc/internal/application"
	"sshc/internal/secret"
	"sshc/internal/terminal"
)

// passwordSavingReadinessPTY is an SSH connection that authenticated with a
// password the person typed, and waits on whether to save it.
type passwordSavingReadinessPTY struct {
	*readinessPTY
	mutex sync.Mutex
	typed *terminal.TypedPassword
}

func (p *passwordSavingReadinessPTY) TypedPassword() (terminal.TypedPassword, bool) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if p.typed == nil {
		return terminal.TypedPassword{}, false
	}
	return *p.typed, true
}

func (p *passwordSavingReadinessPTY) ForgetTypedPassword() {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.typed = nil
}

type recordedTypedPasswordAnswers struct {
	mutex    sync.Mutex
	saved    []terminal.TypedPassword
	declined []terminal.TypedPassword
	refusal  error
}

func (r *recordedTypedPasswordAnswers) SaveTypedPassword(typed terminal.TypedPassword) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r.refusal != nil {
		return r.refusal
	}
	r.saved = append(r.saved, typed)
	return nil
}

func (r *recordedTypedPasswordAnswers) DeclineTypedPassword(typed terminal.TypedPassword) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.declined = append(r.declined, typed)
	return nil
}

const typedPasswordFixture = "typed-at-the-prompt"

// openAskingAboutPassword は、打ったパスワードの保存を尋ねているセッションを開き、問いが一覧に
// 出るまで待って、その ID を返す。
func openAskingAboutPassword(t *testing.T, fixture *terminalFixture) string {
	t.Helper()
	fixture.typedPassword = typedPasswordFixture
	response, body := fixture.do(t, http.MethodPost, "/api/v1/terminal/sessions", `{"kind":"ssh","alias":"web-01"}`)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("open = %d: %s", response.StatusCode, body)
	}
	var opened api.OpenTerminalSessionResponse
	if err := json.Unmarshal([]byte(body), &opened); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		_, body = fixture.do(t, http.MethodGet, "/api/v1/terminal/sessions", "")
		var listed api.TerminalSessionList
		if err := json.Unmarshal([]byte(body), &listed); err != nil {
			t.Fatal(err)
		}
		if question := listed.Sessions[0].PasswordSaveQuestion; question != nil && *question {
			return opened.Session.Id
		}
	}
	t.Fatal("the question never reached the list")
	return ""
}

func TestTheListSaysAPasswordWaitsOnAnAnswerAndNeverSaysThePassword(t *testing.T) {
	fixture := newTerminalFixture(t, terminal.Limits{MaxSessions: 4, Scrollback: 1 << 12})
	id := openAskingAboutPassword(t, fixture)

	_, body := fixture.do(t, http.MethodGet, "/api/v1/terminal/sessions", "")
	if strings.Contains(body, typedPasswordFixture) {
		t.Fatalf("the list holds the password: %s", body)
	}
	_, body = fixture.do(t, http.MethodPost, "/api/v1/terminal/sessions/"+id+"/password-save", `{"save":true}`)
	if strings.Contains(body, typedPasswordFixture) {
		t.Fatalf("the answer holds the password: %s", body)
	}
}

func TestSavingATypedPasswordHandsItOverOnceAndEndsTheQuestion(t *testing.T) {
	fixture := newTerminalFixture(t, terminal.Limits{MaxSessions: 4, Scrollback: 1 << 12})
	id := openAskingAboutPassword(t, fixture)
	path := "/api/v1/terminal/sessions/" + id + "/password-save"

	response, body := fixture.do(t, http.MethodPost, path, `{"save":true}`)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("answer = %d: %s", response.StatusCode, body)
	}
	var answered api.TerminalSessionList
	if err := json.Unmarshal([]byte(body), &answered); err != nil {
		t.Fatal(err)
	}
	if answered.Sessions[0].PasswordSaveQuestion != nil {
		t.Fatalf("still asking: %s", body)
	}
	saved := fixture.typedAnswers.saved
	if len(saved) != 1 || saved[0].Alias != "web-01" || saved[0].Password != typedPasswordFixture || saved[0].Binding != "binding-of-web-01" {
		t.Fatalf("saved = %+v", saved)
	}

	response, body = fixture.do(t, http.MethodPost, path, `{"save":true}`)
	if response.StatusCode != http.StatusConflict || !strings.Contains(body, "terminal_no_password_save_question") {
		t.Fatalf("second answer = %d: %s", response.StatusCode, body)
	}
}

func TestDecliningEndsTheQuestionWithoutSaving(t *testing.T) {
	fixture := newTerminalFixture(t, terminal.Limits{MaxSessions: 4, Scrollback: 1 << 12})
	id := openAskingAboutPassword(t, fixture)
	response, body := fixture.do(t, http.MethodPost, "/api/v1/terminal/sessions/"+id+"/password-save", `{"save":false}`)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("answer = %d: %s", response.StatusCode, body)
	}
	if len(fixture.typedAnswers.saved) != 0 || len(fixture.typedAnswers.declined) != 1 {
		t.Fatalf("saved %+v, declined %+v", fixture.typedAnswers.saved, fixture.typedAnswers.declined)
	}
}

func TestAnAnswerThatCouldNotBeStoredKeepsTheQuestionOpen(t *testing.T) {
	cases := []struct {
		refusal error
		code    string
	}{
		{secret.ErrLocked, "vault_locked"},
		{application.ErrConnectionChanged, "connection_changed"},
	}
	for _, item := range cases {
		t.Run(item.code, func(t *testing.T) {
			fixture := newTerminalFixture(t, terminal.Limits{MaxSessions: 4, Scrollback: 1 << 12})
			id := openAskingAboutPassword(t, fixture)
			fixture.typedAnswers.refusal = item.refusal
			path := "/api/v1/terminal/sessions/" + id + "/password-save"
			response, body := fixture.do(t, http.MethodPost, path, `{"save":true}`)
			if response.StatusCode != http.StatusConflict || !strings.Contains(body, item.code) {
				t.Fatalf("answer = %d: %s", response.StatusCode, body)
			}
			fixture.typedAnswers.refusal = nil
			response, body = fixture.do(t, http.MethodPost, path, `{"save":true}`)
			if response.StatusCode != http.StatusOK || len(fixture.typedAnswers.saved) != 1 {
				t.Fatalf("answer after the refusal = %d: %s", response.StatusCode, body)
			}
		})
	}
}

func TestAnAnswerWithoutSaveIsNotReadAsNo(t *testing.T) {
	fixture := newTerminalFixture(t, terminal.Limits{MaxSessions: 4, Scrollback: 1 << 12})
	id := openAskingAboutPassword(t, fixture)
	response, _ := fixture.do(t, http.MethodPost, "/api/v1/terminal/sessions/"+id+"/password-save", `{}`)
	if response.StatusCode != http.StatusBadRequest || len(fixture.typedAnswers.declined) != 0 {
		t.Fatalf("answer = %d, declined %+v", response.StatusCode, fixture.typedAnswers.declined)
	}
}

func TestAnAnswerForASessionThatIsNotAskingIsRefused(t *testing.T) {
	fixture := newTerminalFixture(t, terminal.Limits{MaxSessions: 4, Scrollback: 1 << 12})
	response, body := fixture.do(t, http.MethodPost, "/api/v1/terminal/sessions", `{"kind":"ssh","alias":"web-01"}`)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("open = %d: %s", response.StatusCode, body)
	}
	var opened api.OpenTerminalSessionResponse
	if err := json.Unmarshal([]byte(body), &opened); err != nil {
		t.Fatal(err)
	}
	response, body = fixture.do(t, http.MethodPost, "/api/v1/terminal/sessions/"+opened.Session.Id+"/password-save", `{"save":true}`)
	if response.StatusCode != http.StatusConflict || !strings.Contains(body, "terminal_no_password_save_question") {
		t.Fatalf("answer = %d: %s", response.StatusCode, body)
	}
	response, _ = fixture.do(t, http.MethodPost, "/api/v1/terminal/sessions/absent/password-save", `{"save":true}`)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("answer for an unknown session = %d", response.StatusCode)
	}
}

func TestADirectDestinationOpensASessionAndALookalikeDoesNot(t *testing.T) {
	fixture := newTerminalFixture(t, terminal.Limits{MaxSessions: 4, Scrollback: 1 << 12})
	response, body := fixture.do(t, http.MethodPost, "/api/v1/terminal/sessions", `{"kind":"ssh","destination":"deploy@10.0.0.5:2222"}`)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("open = %d: %s", response.StatusCode, body)
	}
	var opened api.OpenTerminalSessionResponse
	if err := json.Unmarshal([]byte(body), &opened); err != nil {
		t.Fatal(err)
	}
	if opened.Session.Alias == nil || *opened.Session.Alias != "deploy@10.0.0.5:2222" || opened.Session.Title != "deploy@10.0.0.5:2222" {
		t.Fatalf("session = %+v", opened.Session)
	}
	refused := map[string]string{
		"a shell word in the user":      `{"kind":"ssh","destination":"de$(id)ploy@10.0.0.5"}`,
		"a connection name":             `{"kind":"ssh","destination":"web-01"}`,
		"a destination as an alias":     `{"kind":"ssh","alias":"deploy@10.0.0.5"}`,
		"an alias and a destination":    `{"kind":"ssh","alias":"web-01","destination":"deploy@10.0.0.5"}`,
		"an alias that reads as a flag": `{"kind":"ssh","alias":"-oProxyCommand=id"}`,
	}
	for name, request := range refused {
		response, body = fixture.do(t, http.MethodPost, "/api/v1/terminal/sessions", request)
		if response.StatusCode != http.StatusBadRequest || !strings.Contains(body, "unsafe_alias") {
			t.Errorf("open with %s = %d: %s", name, response.StatusCode, body)
		}
	}
}
