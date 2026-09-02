package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"sshc/internal/api"
	"sshc/internal/handoff"
	"sshc/internal/httpserver"
)

const (
	maxEngineAPIResponse = 2 << 20
	engineAPITimeout     = 30 * time.Minute
	engineCloseTimeout   = 5 * time.Second
)

var (
	errEngineIdentityMismatch = errors.New("running engine identity does not match its handoff")
	errEngineVaultMissing     = errors.New("the running engine has no vault")
	errEngineVaultLocked      = errors.New("the running engine vault is locked")
	errEngineInvalidResponse  = errors.New("the running engine returned an invalid response")
	errEngineResponseTooLarge = errors.New("the running engine response is too large")
)

// engineProblem is a sanitized, stable failure from the running engine. It
// intentionally excludes response messages and transport text because either
// can reflect a request credential or secret payload.
type engineProblem struct {
	Status         int
	Code           string
	Retryable      bool
	OutcomeUnknown bool
	cause          error
}

func (problem engineProblem) Error() string {
	if problem.Status != 0 {
		return fmt.Sprintf("engine request failed (status %d, code %s)", problem.Status, problem.Code)
	}
	return fmt.Sprintf("engine request failed (code %s)", problem.Code)
}

func (problem engineProblem) Unwrap() error { return problem.cause }

type engineAPI struct {
	origin string
	secret string
	cookie http.Cookie
	csrf   string
	client *http.Client

	closeMu sync.Mutex
	closed  bool
}

type engineStatusWire struct {
	Owner           handoff.Owner `json:"owner"`
	Version         string        `json:"version"`
	ProtocolVersion int           `json:"protocolVersion"`
	Vault           *bool         `json:"vault"`
	Unlocked        *bool         `json:"unlocked"`
	Sessions        *int          `json:"sessions"`
}

// openEngineAPI validates one exact handoff and engine before minting a
// command-scoped normal API session. It never rereads the handoff mid-command.
func openEngineAPI(ctx context.Context, stateDir string, base *http.Client) (*engineAPI, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	found, err := readHandoff(stateDir)
	if err != nil {
		return nil, err
	}
	client := engineCommandClient(base)

	var status engineStatusWire
	if err := handoffJSON(ctx, client, found, http.MethodGet, httpserver.StatusPath, &status); err != nil {
		return nil, err
	}
	if status.Vault == nil || status.Unlocked == nil || status.Sessions == nil ||
		*status.Sessions < 0 || (!*status.Vault && *status.Unlocked) {
		return nil, errEngineInvalidResponse
	}
	if status.Owner != found.Owner || status.Version != found.Version ||
		status.ProtocolVersion != found.ProtocolVersion {
		return nil, errEngineIdentityMismatch
	}
	if !*status.Vault {
		return nil, errEngineVaultMissing
	}
	if !*status.Unlocked {
		return nil, errEngineVaultLocked
	}

	request, err := newHandoffRequest(ctx, found, http.MethodPost, httpserver.CLISessionPath, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		if response != nil {
			discardEngineResponse(response)
		}
		return nil, transportProblem(err, false)
	}
	if response.StatusCode != http.StatusOK {
		return nil, decodeEngineProblem(response)
	}
	var cookie *http.Cookie
	for _, candidate := range response.Cookies() {
		if candidate.Name == httpserver.SessionCookie && candidate.Value != "" {
			copy := *candidate
			cookie = &copy
			break
		}
	}
	var credentials api.BootstrapResponse
	decodeErr := decodeEngineJSONResponse(response, &credentials)
	if decodeErr != nil && ctx.Err() != nil {
		decodeErr = transportProblem(ctx.Err(), false)
	}
	if decodeErr != nil || cookie == nil || credentials.CsrfToken == "" {
		if cookie != nil {
			partial := &engineAPI{origin: found.URL, secret: found.Secret, cookie: *cookie, client: client}
			_ = partial.Close()
		}
		if decodeErr != nil {
			return nil, decodeErr
		}
		return nil, errEngineInvalidResponse
	}
	return &engineAPI{
		origin: found.URL,
		secret: found.Secret,
		cookie: *cookie,
		csrf:   credentials.CsrfToken,
		client: client,
	}, nil
}

func engineCommandClient(base *http.Client) *http.Client {
	client := noRedirectClient(base)
	client.Timeout = engineAPITimeout
	return client
}

func noRedirectClient(base *http.Client) *http.Client {
	if base == nil {
		base = http.DefaultClient
	}
	cloned := *base
	cloned.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &cloned
}

func newHandoffRequest(
	ctx context.Context, found handoff.Handoff, method, path string, body io.Reader,
) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, method, found.URL+path, body)
	if err != nil {
		return nil, err
	}
	request.Header.Set(handoff.HeaderName, found.Secret)
	return request, nil
}

func handoffJSON(
	ctx context.Context,
	client *http.Client,
	found handoff.Handoff,
	method, path string,
	target any,
) error {
	request, err := newHandoffRequest(ctx, found, method, path, nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		if response != nil {
			discardEngineResponse(response)
		}
		return transportProblem(err, false)
	}
	if response.StatusCode != http.StatusOK {
		return decodeEngineProblem(response)
	}
	if err := decodeEngineJSONResponse(response, target); err != nil {
		if ctx.Err() != nil {
			return transportProblem(ctx.Err(), false)
		}
		return err
	}
	return nil
}

func (engine *engineAPI) getJSON(ctx context.Context, path string, target any) error {
	return engine.doAPI(ctx, http.MethodGet, path, nil, target)
}

func (engine *engineAPI) sendJSON(
	ctx context.Context, method, path string, value, target any,
) error {
	return engine.sendJSONWithAction(ctx, method, path, "", value, target)
}

func (engine *engineAPI) sendJSONWithAction(
	ctx context.Context, method, path, actionToken string, value, target any,
) error {
	var payload []byte
	var err error
	if value != nil {
		payload, err = json.Marshal(value)
		if err != nil {
			return errEngineInvalidResponse
		}
	}
	defer zeroBytes(payload)
	var body io.Reader
	if payload != nil {
		body = &oneShotSecretPayload{body: payload}
	}
	return engine.doAPIWithAction(ctx, method, path, actionToken, body, target)
}

// sendSecretJSON takes ownership of payload and zeroes it on every exit. The
// one-shot reader deliberately has no GetBody, preventing transparent replay.
func (engine *engineAPI) sendSecretJSON(
	ctx context.Context, method, path string, payload []byte, target any,
) error {
	defer zeroBytes(payload)
	var body io.Reader
	if payload != nil {
		body = &oneShotSecretPayload{body: payload}
	}
	return engine.doAPI(ctx, method, path, body, target)
}

func (engine *engineAPI) doAPI(
	ctx context.Context, method, path string, body io.Reader, target any,
) error {
	return engine.doAPIWithAction(ctx, method, path, "", body, target)
}

func (engine *engineAPI) doAPIWithAction(
	ctx context.Context, method, path, actionToken string, body io.Reader, target any,
) error {
	request, err := http.NewRequestWithContext(ctx, method, engine.origin+path, body)
	if err != nil {
		return errEngineInvalidResponse
	}
	request.AddCookie(&engine.cookie)
	request.Header.Set(httpserver.CSRFHeader, engine.csrf)
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	if actionToken != "" {
		request.Header.Set(httpserver.ActionHeader, actionToken)
	}
	mutation := method != http.MethodGet && method != http.MethodHead
	if mutation {
		request.Header.Set("Origin", engine.origin)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := engine.client.Do(request)
	if err != nil {
		if response != nil {
			discardEngineResponse(response)
		}
		return transportProblem(err, mutation)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return decodeEngineProblem(response)
	}
	if target == nil {
		return consumeEngineResponse(response, mutation)
	}
	if err := decodeEngineJSONResponse(response, target); err != nil {
		if ctx.Err() != nil {
			return transportProblem(ctx.Err(), mutation)
		}
		return responseProblem(err, response.StatusCode, mutation)
	}
	return nil
}

// doRaw sends or receives a streaming API body. Successful callers own the
// response body; failures are reduced to the same sanitized engineProblem used
// by JSON commands so file contents and request URLs never reach diagnostics.
func (engine *engineAPI) doRaw(
	ctx context.Context, method, path, contentType string, body io.Reader,
) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, engine.origin+path, body)
	if err != nil {
		return nil, errEngineInvalidResponse
	}
	request.AddCookie(&engine.cookie)
	request.Header.Set(httpserver.CSRFHeader, engine.csrf)
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	mutation := method != http.MethodGet && method != http.MethodHead
	if mutation {
		request.Header.Set("Origin", engine.origin)
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	// A file transfer may legitimately take longer than the JSON command
	// timeout. Its context (including Ctrl+C) owns cancellation instead.
	client := *engine.client
	client.Timeout = 0
	response, err := client.Do(request)
	if err != nil {
		if response != nil {
			discardEngineResponse(response)
		}
		return nil, transportProblem(err, mutation)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, decodeEngineProblem(response)
	}
	return response, nil
}

func (engine *engineAPI) issueAction(
	ctx context.Context, kind, target string,
) (api.IssueActionResponse, error) {
	var answer api.IssueActionResponse
	err := engine.sendJSON(ctx, http.MethodPost, "/api/v1/actions", api.IssueActionRequest{
		Kind: kind, Target: target,
	}, &answer)
	if err != nil {
		return api.IssueActionResponse{}, err
	}
	if answer.Token == "" || answer.ExpiresAt == "" {
		return api.IssueActionResponse{}, errEngineInvalidResponse
	}
	return answer, nil
}

// Close best-effort revokes the command session. It is safe to call more than
// once; only the first call reaches the engine.
func (engine *engineAPI) Close() error {
	if engine == nil {
		return nil
	}
	engine.closeMu.Lock()
	defer engine.closeMu.Unlock()
	if engine.closed {
		return nil
	}
	engine.closed = true
	defer func() {
		engine.secret = ""
		engine.csrf = ""
		engine.cookie.Value = ""
	}()

	ctx, cancel := context.WithTimeout(context.Background(), engineCloseTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		engine.origin+httpserver.CLISessionPath, nil)
	if err != nil {
		return errEngineInvalidResponse
	}
	request.Header.Set(handoff.HeaderName, engine.secret)
	request.AddCookie(&engine.cookie)
	response, err := engine.client.Do(request)
	if err != nil {
		if response != nil {
			discardEngineResponse(response)
		}
		return transportProblem(err, false)
	}
	if response.StatusCode != http.StatusNoContent {
		return decodeEngineProblem(response)
	}
	return consumeEngineResponse(response, false)
}

type oneShotSecretPayload struct {
	body   []byte
	offset int
}

func (reader *oneShotSecretPayload) Read(destination []byte) (int, error) {
	if reader.offset >= len(reader.body) {
		return 0, io.EOF
	}
	written := copy(destination, reader.body[reader.offset:])
	reader.offset += written
	return written, nil
}

func decodeEngineJSONResponse(response *http.Response, target any) error {
	body, err := readAndCloseEngineResponse(response)
	defer zeroBytes(body)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errEngineInvalidResponse
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errEngineInvalidResponse
	}
	return nil
}

func consumeEngineResponse(response *http.Response, mutation bool) error {
	body, err := readAndCloseEngineResponse(response)
	defer zeroBytes(body)
	if err != nil {
		return responseProblem(err, response.StatusCode, mutation)
	}
	if len(bytes.TrimSpace(body)) != 0 {
		return responseProblem(errEngineInvalidResponse, response.StatusCode, mutation)
	}
	return nil
}

func readAndCloseEngineResponse(response *http.Response) ([]byte, error) {
	if response == nil || response.Body == nil {
		return nil, errEngineInvalidResponse
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxEngineAPIResponse+1))
	if err != nil {
		zeroBytes(body)
		return nil, errEngineInvalidResponse
	}
	if len(body) > maxEngineAPIResponse {
		zeroBytes(body)
		return nil, errEngineResponseTooLarge
	}
	return body, nil
}

func discardEngineResponse(response *http.Response) {
	body, _ := readAndCloseEngineResponse(response)
	zeroBytes(body)
}

func decodeEngineProblem(response *http.Response) error {
	status := 0
	if response != nil {
		status = response.StatusCode
	}
	body, err := readAndCloseEngineResponse(response)
	defer zeroBytes(body)
	code := "http_error"
	if err == nil {
		var problem api.Problem
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.DisallowUnknownFields()
		if decodeErr := decoder.Decode(&problem); decodeErr == nil && problem.Code != "" {
			var trailing any
			if trailingErr := decoder.Decode(&trailing); errors.Is(trailingErr, io.EOF) {
				code = problem.Code
			}
		}
	} else if errors.Is(err, errEngineResponseTooLarge) {
		code = "response_too_large"
	}
	return engineProblem{Status: status, Code: code, Retryable: retryableStatus(status)}
}

func responseProblem(err error, status int, mutation bool) error {
	code := "invalid_response"
	if errors.Is(err, errEngineResponseTooLarge) {
		code = "response_too_large"
	}
	return engineProblem{
		Status: status, Code: code, Retryable: false, OutcomeUnknown: mutation,
	}
}

func transportProblem(err error, mutation bool) error {
	problem := engineProblem{Code: "transport_error", Retryable: true, OutcomeUnknown: mutation}
	switch {
	case errors.Is(err, context.Canceled):
		problem.Retryable = false
		problem.cause = context.Canceled
	case errors.Is(err, context.DeadlineExceeded):
		problem.cause = context.DeadlineExceeded
	}
	return problem
}

func retryableStatus(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusTooEarly ||
		status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}
