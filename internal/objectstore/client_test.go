package objectstore_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"sshc/internal/objectstore"
)

// AWS が公開している SigV4 テストスイートの資格情報。署名そのものはもう SDK の
// 責任なので既知解テストは持たないが、鍵が回線へ漏れていないことを確かめる
// テストには、探すべき具体的な文字列が要る。
const (
	suiteAccessKeyID     = "AKIDEXAMPLE"
	suiteSecretAccessKey = "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY"
)

func suiteCredentials() objectstore.Credentials {
	return objectstore.Credentials{AccessKeyID: suiteAccessKeyID, SecretAccessKey: suiteSecretAccessKey}
}

// ここのテストはすべて httptest に対して走る。このパッケージのものが Cloudflare
// や、その他のどのネットワークにも到達することはない。
func newClient(t *testing.T, handler http.HandlerFunc) (objectstore.Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	return objectstore.Client{
		HTTP:     server.Client(),
		Endpoint: server.URL,
		Bucket:   "sshc",
		Region:   "auto",
		Creds:    suiteCredentials(),
	}, server
}

func TestPutSendsTheConditionItWasGiven(t *testing.T) {
	// compare-and-swap こそが、このクライアントが存在する理由のすべてである。条件が
	// 回線に届いていなければ、「auto」同期は他のマシンを踏み潰しうるのに、それを
	// 告げるものは何もない。
	var ifMatch, ifNoneMatch, method, path string
	client, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		ifMatch, ifNoneMatch = r.Header.Get("If-Match"), r.Header.Get("If-None-Match")
		w.Header().Set("ETag", `"new"`)
		w.WriteHeader(http.StatusOK)
	})

	if _, err := client.Put(context.Background(), "snapshot", []byte("body"), `"old"`, ""); err != nil {
		t.Fatalf("Put = %v", err)
	}
	if method != http.MethodPut || path != "/sshc/snapshot" {
		t.Errorf("request = %s %s", method, path)
	}
	if ifMatch != `"old"` || ifNoneMatch != "" {
		t.Errorf("If-Match = %q, If-None-Match = %q", ifMatch, ifNoneMatch)
	}

	if _, err := client.Put(context.Background(), "snapshot", []byte("body"), "", "*"); err != nil {
		t.Fatalf("Put = %v", err)
	}
	if ifNoneMatch != "*" || ifMatch != "" {
		t.Errorf("If-Match = %q, If-None-Match = %q", ifMatch, ifNoneMatch)
	}
}

func TestPutRefusesBothConditionsAtOnceWithoutSendingAnything(t *testing.T) {
	reached := false
	client, _ := newClient(t, func(http.ResponseWriter, *http.Request) { reached = true })

	if _, err := client.Put(context.Background(), "k", nil, `"a"`, "*"); !errors.Is(err, objectstore.ErrBothConditions) {
		t.Fatalf("Put = %v, want ErrBothConditions", err)
	}
	if reached {
		t.Error("a programming error still reached the network")
	}
}

func TestDeleteRemovesOnlyTheNamedObject(t *testing.T) {
	var method, path string
	client, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	})

	if err := client.Delete(context.Background(), "snapshots/candidate"); err != nil {
		t.Fatalf("Delete = %v", err)
	}
	if method != http.MethodDelete || path != "/sshc/snapshots/candidate" {
		t.Fatalf("request = %s %s", method, path)
	}
}

func TestAFailedConditionIsItsOwnError(t *testing.T) {
	// 「誰かが先に到達した」は失敗ではなく結果であり、呼び出し側はそれを失敗と
	// 見分けられなければならない。
	for _, status := range []int{http.StatusPreconditionFailed, http.StatusConflict} {
		client, _ := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
		})
		if _, err := client.Put(context.Background(), "k", []byte("b"), `"old"`, ""); !errors.Is(err, objectstore.ErrPreconditionFailed) {
			t.Errorf("HTTP %d gave %v, want ErrPreconditionFailed", status, err)
		}
	}
}

func TestGetMapsNotFoundAndReturnsTheETag(t *testing.T) {
	missing, _ := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	if _, err := missing.Get(context.Background(), "k"); !errors.Is(err, objectstore.ErrNotFound) {
		t.Errorf("Get = %v, want ErrNotFound", err)
	}

	present, _ := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `"v1"`)
		_, _ = w.Write([]byte("the snapshot"))
	})
	object, err := present.Get(context.Background(), "k")
	if err != nil {
		t.Fatalf("Get = %v", err)
	}
	if string(object.Body) != "the snapshot" || object.ETag != `"v1"` {
		t.Errorf("object = %#v", object)
	}
}

func TestStatAndListReturnMetadataWithoutDownloadingBodies(t *testing.T) {
	modified := "2026-08-25T01:02:03.000Z"
	client, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead:
			w.Header().Set("ETag", `"live"`)
			w.Header().Set("Content-Length", "42")
			w.Header().Set("Last-Modified", "Tue, 25 Aug 2026 01:02:03 GMT")
		case r.Method == http.MethodGet && r.URL.Query().Get("list-type") == "2":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>`+
				`<ListBucketResult><Name>sshc</Name><Prefix>snapshots/</Prefix><KeyCount>1</KeyCount><MaxKeys>1000</MaxKeys><IsTruncated>false</IsTruncated>`+
				`<Contents><Key>snapshots/2026-08-25.tar.gz.enc</Key><LastModified>`+modified+`</LastModified><ETag>&quot;history&quot;</ETag><Size>41</Size><StorageClass>STANDARD</StorageClass></Contents></ListBucketResult>`)
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	})

	info, err := client.Stat(context.Background(), "workspace.tar.gz.enc")
	if err != nil {
		t.Fatal(err)
	}
	if info.ETag != `"live"` || info.Size != 42 || info.LastModified.IsZero() {
		t.Fatalf("Stat = %#v", info)
	}
	listed, truncated, err := client.ListNewest(context.Background(), "snapshots/", 10)
	if err != nil {
		t.Fatal(err)
	}
	if truncated {
		t.Fatal("List unexpectedly reported truncation")
	}
	if len(listed) != 1 || listed[0].Key != "snapshots/2026-08-25.tar.gz.enc" || listed[0].Size != 41 {
		t.Fatalf("List = %#v", listed)
	}
}

func TestListNewestScansPastOldEntriesWithBoundedResults(t *testing.T) {
	client, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Query().Get("list-type") != "2" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>`+
			`<ListBucketResult><Name>sshc</Name><Prefix>snapshots/</Prefix><KeyCount>3</KeyCount><MaxKeys>1000</MaxKeys><IsTruncated>false</IsTruncated>`+
			`<Contents><Key>snapshots/a</Key><LastModified>2026-08-23T01:00:00.000Z</LastModified><ETag>&quot;a&quot;</ETag><Size>1</Size></Contents>`+
			`<Contents><Key>snapshots/b</Key><LastModified>2026-08-24T01:00:00.000Z</LastModified><ETag>&quot;b&quot;</ETag><Size>2</Size></Contents>`+
			`<Contents><Key>snapshots/c</Key><LastModified>2026-08-25T01:00:00.000Z</LastModified><ETag>&quot;c&quot;</ETag><Size>3</Size></Contents>`+
			`</ListBucketResult>`)
	})

	listed, truncated, err := client.ListNewest(context.Background(), "snapshots/", 2)
	if err != nil {
		t.Fatal(err)
	}
	if !truncated || len(listed) != 2 {
		t.Fatalf("ListNewest = %#v, truncated %v", listed, truncated)
	}
	keys := map[string]bool{}
	for _, item := range listed {
		keys[item.Key] = true
	}
	if !keys["snapshots/b"] || !keys["snapshots/c"] || keys["snapshots/a"] {
		t.Fatalf("ListNewest kept %#v, want b and c", listed)
	}
}

func TestEveryRequestIsSignedAndCarriesNoCredentialInTheURL(t *testing.T) {
	var seen []*http.Request
	client, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Clone(context.Background()))
		w.Header().Set("ETag", `"v"`)
	})

	if _, err := client.Get(context.Background(), "k"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Head(context.Background(), "k"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Put(context.Background(), "k", []byte("b"), "", "*"); err != nil {
		t.Fatal(err)
	}
	if err := client.Delete(context.Background(), "k"); err != nil {
		t.Fatal(err)
	}

	if len(seen) != 4 {
		t.Fatalf("requests = %d, want 4", len(seen))
	}
	for _, request := range seen {
		authorization := request.Header.Get("Authorization")
		if !strings.HasPrefix(authorization, "AWS4-HMAC-SHA256 Credential=") {
			t.Errorf("%s %s is unsigned", request.Method, request.URL.Path)
		}
		// 空でないことだけでは足りない。"UNSIGNED-PAYLOAD" も空ではないが、それは
		// 本文に署名していないという意味である。送るものは誰かの ~/.ssh の
		// スナップショットなので、実際のハッシュが載っていなければならない。
		payloadHash := request.Header.Get("X-Amz-Content-Sha256")
		if len(payloadHash) != sha256.Size*2 {
			t.Errorf("%s %s の X-Amz-Content-Sha256 が %q で、SHA-256 の 16 進ダイジェストではない",
				request.Method, request.URL.Path, payloadHash)
		}
		if strings.Contains(request.URL.RawQuery, "Signature") ||
			strings.Contains(request.URL.String(), suiteSecretAccessKey) {
			t.Errorf("%s carries a credential in the URL", request.URL.String())
		}
	}
}

func TestAPlaintextEndpointIsRefused(t *testing.T) {
	// 本文はここへ届く前に暗号化されているが、資格情報はそうではない。
	client := objectstore.Client{
		Endpoint: "http://example.com",
		Bucket:   "sshc",
		Region:   "auto",
		Creds:    suiteCredentials(),
	}
	if _, err := client.Get(context.Background(), "k"); !errors.Is(err, objectstore.ErrInsecureEndpoint) {
		t.Fatalf("Get = %v, want ErrInsecureEndpoint", err)
	}
}

func TestAnOversizedObjectIsRefusedBeforeItIsSent(t *testing.T) {
	reached := false
	client, _ := newClient(t, func(http.ResponseWriter, *http.Request) { reached = true })

	oversized := make([]byte, objectstore.MaxObjectBytes+1)
	if _, err := client.Put(context.Background(), "k", oversized, "", "*"); !errors.Is(err, objectstore.ErrObjectTooLarge) {
		t.Fatalf("Put = %v, want ErrObjectTooLarge", err)
	}
	if reached {
		t.Error("an oversized body was sent anyway")
	}
}

func TestAnOversizedAdvertisedDownloadIsRefusedWithoutReadingItsBody(t *testing.T) {
	body := &observedBody{}
	client := objectstore.Client{
		HTTP: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Etag":           []string{`"large"`},
					"Content-Length": []string{strconv.FormatInt(objectstore.MaxObjectBytes+1, 10)},
				},
				Body:          body,
				ContentLength: objectstore.MaxObjectBytes + 1,
				Request:       request,
			}, nil
		})},
		Endpoint: "http://127.0.0.1",
		Bucket:   "sshc",
		Region:   "auto",
		Creds:    suiteCredentials(),
	}

	if _, err := client.Get(context.Background(), "k"); !errors.Is(err, objectstore.ErrObjectTooLarge) {
		t.Fatalf("Get = %v, want ErrObjectTooLarge", err)
	}
	if got := body.reads.Load(); got != 0 {
		t.Fatalf("oversized response body was read %d times", got)
	}
	if !body.closed.Load() {
		t.Fatal("oversized response body was not closed")
	}
}

func TestAnyOtherRejectionCarriesNoResponseBody(t *testing.T) {
	// S3 のエラードキュメントにはバケット名とリクエスト ID が含まれる。どちらも
	// このアプリケーションが表示するメッセージに入れてよいものではない。
	client, _ := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("<Error><BucketName>private-bucket</BucketName></Error>"))
	})

	_, err := client.Get(context.Background(), "k")
	if !errors.Is(err, objectstore.ErrRefused) {
		t.Fatalf("Get = %v, want ErrRefused", err)
	}
	if strings.Contains(err.Error(), "private-bucket") {
		t.Error("the error carries the response body")
	}
}

func TestAnErrorBodyIsDiscardedBeforeTheSDKReadsIt(t *testing.T) {
	body := &observedBody{}
	client := objectstore.Client{
		HTTP: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusForbidden,
				Header:     make(http.Header),
				Body:       body,
			}, nil
		})},
		Endpoint: "http://127.0.0.1",
		Bucket:   "sshc",
		Region:   "auto",
		Creds:    suiteCredentials(),
	}

	if _, err := client.Get(context.Background(), "k"); !errors.Is(err, objectstore.ErrRefused) {
		t.Fatalf("Get = %v, want ErrRefused", err)
	}
	if got := body.reads.Load(); got != 0 {
		t.Errorf("error body was read %d times", got)
	}
	if !body.closed.Load() {
		t.Error("error body was not closed")
	}
}

func TestARequestThatStopsRespondingTimesOut(t *testing.T) {
	// ハンドラはリクエストの context ではなく、この channel で待つ。
	//
	// 以前は `<-r.Context().Done()` で待っており、それはクライアント自身の
	// タイムアウトが閉じるものだった。つまりキャンセルがハンドラを終わらせ、
	// chunked の本文が 0 バイトで「正常に」終わる。その EOF がキャンセルの
	// エラーより先に転送層へ届くと Get は成功してしまい、このテストは CI で
	// 時々落ちていた。応答は決して完成しない、が表明したいことである。
	stop := make(chan struct{})
	client, _ := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `"v"`)
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-stop
	})
	// newClient が server.Close を先に登録しているので、LIFO によりこちらが
	// 先に走る。ハンドラを解放してからでないと Close は戻らない。
	t.Cleanup(func() { close(stop) })
	client.RequestTimeout = 25 * time.Millisecond

	// 表明するのは「タイムアウトで終わったこと」であって、それが何ミリ秒で
	// 起きたかではない。以前は経過時間が 1 秒未満であることを求めており、
	// それは製品ではなく走っているマシンを測っていた。負荷の高いランナーでは
	// スケジューリングだけで 1 秒を超え、CI が理由もなく赤くなる。
	started := time.Now()
	_, err := client.Get(context.Background(), "k")
	if err == nil {
		t.Fatal("Get succeeded after the server stopped responding")
	}
	var timeout interface{ Timeout() bool }
	if !errors.As(err, &timeout) || !timeout.Timeout() {
		t.Fatalf("Get = %v, want a timeout", err)
	}
	// 上限は、ぶら下がっていないことだけを見る保険である。タイムアウトが
	// まったく効いていなければ、この応答は永遠に完成しない。
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Fatalf("Get took %s despite its request timeout", elapsed)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type observedBody struct {
	reads  atomic.Int64
	closed atomic.Bool
}

func (b *observedBody) Read([]byte) (int, error) {
	b.reads.Add(1)
	return 0, io.EOF
}

func (b *observedBody) Close() error {
	b.closed.Store(true)
	return nil
}
