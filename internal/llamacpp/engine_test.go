package llamacpp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func httpResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func testEngine(url string, rt roundTripFunc) *Engine {
	return &Engine{
		modelName:  "llava",
		apiBaseURL: url,
		healthURL:  "http://local.test/health",
		client: &http.Client{
			Timeout:   2 * time.Second,
			Transport: rt,
		},
	}
}

func TestExtractMarkdownSuccess(t *testing.T) {
	var seenModel string
	e := testEngine("http://local.test/api", func(r *http.Request) (*http.Response, error) {
		defer r.Body.Close()
		if r.Method != http.MethodPost {
			t.Fatalf("method=%s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/generate") {
			t.Fatalf("path=%s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		seenModel, _ = body["model"].(string)
		return httpResponse(http.StatusOK, `{"response":"ok text"}`), nil
	})

	got, err := e.ExtractMarkdown(context.Background(), "abc123", SystemPromptMarkdown)
	if err != nil {
		t.Fatalf("ExtractMarkdown error: %v", err)
	}
	if got != "ok text" {
		t.Fatalf("response=%q", got)
	}
	if seenModel != "llava" {
		t.Fatalf("model=%q", seenModel)
	}
}

func TestExtractMarkdownNon200(t *testing.T) {
	e := testEngine("http://local.test/api", func(r *http.Request) (*http.Response, error) {
		return httpResponse(http.StatusInternalServerError, "broken"), nil
	})

	_, err := e.ExtractMarkdown(context.Background(), "abc123", SystemPromptMarkdown)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "status 500") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExtractMarkdownWithRetryEventuallySucceeds(t *testing.T) {
	var calls int32
	e := testEngine("http://local.test/api", func(r *http.Request) (*http.Response, error) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			return httpResponse(http.StatusServiceUnavailable, "transient"), nil
		}
		return httpResponse(http.StatusOK, `{"response":"recovered"}`), nil
	})

	got, err := e.ExtractMarkdownWithRetry(context.Background(), "abc123", SystemPromptMarkdown, 2)
	if err != nil {
		t.Fatalf("ExtractMarkdownWithRetry error: %v", err)
	}
	if got != "recovered" {
		t.Fatalf("response=%q", got)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("calls=%d want 2", calls)
	}
}

func TestExtractMarkdownWithRetryContextCanceled(t *testing.T) {
	e := testEngine("http://local.test/api", func(r *http.Request) (*http.Response, error) {
		<-r.Context().Done()
		return nil, r.Context().Err()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := e.ExtractMarkdownWithRetry(ctx, "abc123", SystemPromptMarkdown, 3)
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "context deadline exceeded") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPing(t *testing.T) {
	e := testEngine("http://local.test/api", func(r *http.Request) (*http.Response, error) {
		return httpResponse(http.StatusOK, ""), nil
	})
	if err := e.ping(); err != nil {
		t.Fatalf("ping error: %v", err)
	}

	e.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return httpResponse(http.StatusInternalServerError, ""), nil
	})
	if err := e.ping(); err == nil {
		t.Fatal("expected ping bad status error")
	}
}

func TestNewEngineAttachsToExistingHealthcheck(t *testing.T) {
	origHealth := defaultHealthURL
	origAPI := defaultAPIBaseURL
	origServe := serveCommand
	origClient := newHTTPClient
	defer func() {
		defaultHealthURL = origHealth
		defaultAPIBaseURL = origAPI
		serveCommand = origServe
		newHTTPClient = origClient
	}()

	defaultHealthURL = "http://local.test/health"
	defaultAPIBaseURL = "http://local.test/api"
	serveCommand = func() *exec.Cmd {
		t.Fatal("serveCommand should not be called when healthcheck is up")
		return exec.Command("sh", "-c", "exit 1")
	}
	newHTTPClient = func(timeout time.Duration) *http.Client {
		return &http.Client{
			Timeout: timeout,
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				return httpResponse(http.StatusOK, ""), nil
			}),
		}
	}

	e, err := NewEngine("llava")
	if err != nil {
		t.Fatalf("NewEngine error: %v", err)
	}
	if e == nil {
		t.Fatal("expected engine")
	}
}

func TestNewEngineServeStartFailure(t *testing.T) {
	origHealth := defaultHealthURL
	origServe := serveCommand
	origSleep := sleepFn
	origClient := newHTTPClient
	defer func() {
		defaultHealthURL = origHealth
		serveCommand = origServe
		sleepFn = origSleep
		newHTTPClient = origClient
	}()

	defaultHealthURL = "http://local.test/down"
	serveCommand = func() *exec.Cmd {
		return exec.Command("definitely-missing-ollama-binary")
	}
	sleepFn = func(time.Duration) {}
	newHTTPClient = func(timeout time.Duration) *http.Client {
		return &http.Client{
			Timeout: timeout,
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				return nil, errors.New("dial failed")
			}),
		}
	}

	_, err := NewEngine("llava")
	if err == nil {
		t.Fatal("expected start failure")
	}
}

func TestCloseNoProcess(t *testing.T) {
	e := &Engine{}
	if err := e.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}
}

func TestCloseManagedGraceful(t *testing.T) {
	origSignal := signalProcess
	origKill := killProcess
	origWait := waitCmd
	origTimeout := shutdownTimeout
	defer func() {
		signalProcess = origSignal
		killProcess = origKill
		waitCmd = origWait
		shutdownTimeout = origTimeout
	}()

	var signalCalled, killCalled, waitCalled bool
	signalProcess = func(p *os.Process, s os.Signal) error {
		signalCalled = true
		return nil
	}
	killProcess = func(p *os.Process) error {
		killCalled = true
		return nil
	}
	waitCmd = func(cmd *exec.Cmd) error {
		waitCalled = true
		return nil
	}
	shutdownTimeout = 10 * time.Millisecond

	e := &Engine{cmd: &exec.Cmd{Process: &os.Process{Pid: 1234}}}
	if err := e.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}
	if !signalCalled {
		t.Fatal("expected signal to be called")
	}
	if !waitCalled {
		t.Fatal("expected wait to be called")
	}
	if killCalled {
		t.Fatal("did not expect kill in graceful path")
	}
	if e.cmd != nil {
		t.Fatal("expected managed command to be cleared after close")
	}
}

func TestCloseManagedTimeoutForcesKill(t *testing.T) {
	origSignal := signalProcess
	origKill := killProcess
	origWait := waitCmd
	origTimeout := shutdownTimeout
	defer func() {
		signalProcess = origSignal
		killProcess = origKill
		waitCmd = origWait
		shutdownTimeout = origTimeout
	}()

	waitRelease := make(chan struct{})
	var killCalled bool
	signalProcess = func(p *os.Process, s os.Signal) error { return nil }
	waitCmd = func(cmd *exec.Cmd) error {
		<-waitRelease
		return nil
	}
	killProcess = func(p *os.Process) error {
		if !killCalled {
			close(waitRelease)
		}
		killCalled = true
		return nil
	}
	shutdownTimeout = 10 * time.Millisecond

	e := &Engine{cmd: &exec.Cmd{Process: &os.Process{Pid: 4321}}}
	if err := e.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}
	if !killCalled {
		t.Fatal("expected kill after shutdown timeout")
	}
}

func TestCloseManagedSignalFailureFallsBackToKill(t *testing.T) {
	origSignal := signalProcess
	origKill := killProcess
	origWait := waitCmd
	defer func() {
		signalProcess = origSignal
		killProcess = origKill
		waitCmd = origWait
	}()

	waitRelease := make(chan struct{})
	var killCalled bool
	signalProcess = func(p *os.Process, s os.Signal) error {
		return errors.New("signal failed")
	}
	waitCmd = func(cmd *exec.Cmd) error {
		<-waitRelease
		return nil
	}
	killProcess = func(p *os.Process) error {
		if !killCalled {
			close(waitRelease)
		}
		killCalled = true
		return nil
	}

	e := &Engine{cmd: &exec.Cmd{Process: &os.Process{Pid: 9876}}}
	if err := e.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}
	if !killCalled {
		t.Fatal("expected kill when signal fails")
	}
}
