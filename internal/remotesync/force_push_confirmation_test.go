package remotesync_test

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"sshc/internal/remotesync"
)

// requestDeadlines は、同期先へ送った要求それぞれの context に期限があったかを記録する。
type requestDeadlines struct {
	base http.RoundTripper

	mu        sync.Mutex
	deadlines []time.Time
	missing   int
}

func (r *requestDeadlines) RoundTrip(request *http.Request) (*http.Response, error) {
	r.mu.Lock()
	if deadline, ok := request.Context().Deadline(); ok {
		r.deadlines = append(r.deadlines, deadline)
	} else {
		r.missing++
	}
	r.mu.Unlock()
	return r.base.RoundTrip(request)
}

// confirmationDeadlineCeiling は、確認の問い合わせに付く期限として許す最も遠い先。
// objectstore の要求の上限（30分）ではなく、確認の画面の前で待てる長さであることを確かめる。
const confirmationDeadlineCeiling = time.Minute

// 強制 push の確認は同期先の今の ETag を問い合わせるだけで、利用者は確認の画面の前で
// 待っている。呼び出し側（HTTP の入口）が期限を付けなくても、問い合わせには
// 確認用の短い期限が付く。
func TestForcePushConfirmationBoundsTheBucketQueryWithoutACallerDeadline(t *testing.T) {
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{"config": "Host first\n"})
	if _, err := machine.service.PushUsing(context.Background(), keyOf(syncPassphrase), ""); err != nil {
		t.Fatal(err)
	}
	recorder := &requestDeadlines{base: machine.client.HTTP.Transport}
	machine.client.HTTP = &http.Client{Transport: recorder}

	asked := time.Now()
	if _, err := machine.service.ForcePushConfirmation(context.Background(), remotesync.ForcePushTarget); err != nil {
		t.Fatal(err)
	}

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.missing != 0 {
		t.Fatalf("%d bucket requests had no deadline", recorder.missing)
	}
	if len(recorder.deadlines) == 0 {
		t.Fatal("the confirmation did not ask the bucket for the live ETag")
	}
	for _, deadline := range recorder.deadlines {
		if deadline.After(asked.Add(confirmationDeadlineCeiling)) {
			t.Errorf("deadline %s after the request, want within %s", deadline.Sub(asked), confirmationDeadlineCeiling)
		}
	}
}
