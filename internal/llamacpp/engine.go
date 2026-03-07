package llamacpp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"time"
)

var (
	defaultAPIBaseURL = "http://127.0.0.1:11434/api"
	defaultHealthURL  = "http://127.0.0.1:11434/"
	serveCommand      = func() *exec.Cmd { return exec.Command("ollama", "serve") }
	sleepFn           = time.Sleep
	newHTTPClient     = func(timeout time.Duration) *http.Client { return &http.Client{Timeout: timeout} }

	signalProcess   = func(p *os.Process, s os.Signal) error { return p.Signal(s) }
	killProcess     = func(p *os.Process) error { return p.Kill() }
	waitCmd         = func(cmd *exec.Cmd) error { return cmd.Wait() }
	shutdownTimeout = 2 * time.Second
)

// Engine manages the local Ollama LLM daemon for purely offline Vision parsing.
type Engine struct {
	cmd        *exec.Cmd
	modelName  string
	apiBaseURL string
	healthURL  string
	client     *http.Client
}

// NewEngine safely boots up the Ollama daemon as a child process if it isn't already
// running independently, ensuring it's available for offline extraction.
func NewEngine(modelName string) (*Engine, error) {
	e := &Engine{
		modelName:  modelName,
		apiBaseURL: defaultAPIBaseURL,
		healthURL:  defaultHealthURL,
		client:     newHTTPClient(600 * time.Second), // VLMs can take a while on CPUs
	}

	// Try to ping the local ollama server first. If it's already running, we just attach.
	if err := e.ping(); err == nil {
		fmt.Println("🚀 Attached to existing local Ollama daemon.")
		return e, nil
	}

	// If it's not running, we boot it as a managed subprocess.
	e.cmd = serveCommand()
	if err := e.cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start ollama subprocess: %w. Is Ollama installed?", err)
	}

	// Wait for healthcheck
	healthy := false
	for i := 0; i < 15; i++ {
		sleepFn(1 * time.Second)
		if err := e.ping(); err == nil {
			healthy = true
			break
		}
	}

	if !healthy {
		e.Close()
		return nil, fmt.Errorf("ollama subprocess failed to become healthy")
	}

	fmt.Println("🚀 Succesfully booted managed Ollama daemon.")
	return e, nil
}

func (e *Engine) ping() error {
	resp, err := e.client.Get(e.healthURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("bad status: %d", resp.StatusCode)
	}
	return nil
}

// ExtractMarkdown converts a base64 encoded PNG/JPEG image slice into a predicted string
// representing the document's content, formatted in Markdown syntax, using the local VLM.
func (e *Engine) ExtractMarkdown(ctx context.Context, imgBase64 string, systemPrompt string) (string, error) {
	reqBody, err := json.Marshal(map[string]interface{}{
		"model":  e.modelName,
		"prompt": systemPrompt,
		"images": []string{imgBase64},
		"stream": false,
		"options": map[string]interface{}{
			"temperature": 0.1, // high determinism for document transcription
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to marshal ollama request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/generate", e.apiBaseURL), bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama inferencing failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama returned status %d: %s", resp.StatusCode, string(b))
	}

	var resData struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&resData); err != nil {
		return "", fmt.Errorf("failed to decode ollama response: %w", err)
	}

	return resData.Response, nil
}

// ExtractMarkdownWithRetry retries extraction for transient failures while
// preserving cancellation semantics through ctx.
func (e *Engine) ExtractMarkdownWithRetry(ctx context.Context, imgBase64, systemPrompt string, attempts int) (string, error) {
	if attempts < 1 {
		attempts = 1
	}

	var lastErr error
	for i := 0; i < attempts; i++ {
		text, err := e.ExtractMarkdown(ctx, imgBase64, systemPrompt)
		if err == nil {
			return text, nil
		}
		lastErr = err
		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", err
		}
		time.Sleep(300 * time.Millisecond)
	}
	return "", fmt.Errorf("vlm extraction failed after %d attempt(s): %w", attempts, lastErr)
}

// Close gracefully stops the Ollama subprocess if VargasParse instantiated it.
func (e *Engine) Close() error {
	if e.cmd == nil || e.cmd.Process == nil {
		return nil
	}

	fmt.Println("🛑 Shutting down managed Ollama daemon.")
	proc := e.cmd.Process
	done := make(chan struct{})
	go func(cmd *exec.Cmd) {
		_ = waitCmd(cmd)
		close(done)
	}(e.cmd)

	if err := signalProcess(proc, os.Interrupt); err != nil {
		_ = killProcess(proc)
		select {
		case <-done:
		case <-time.After(500 * time.Millisecond):
		}
		e.cmd = nil
		return nil
	}

	select {
	case <-done:
	case <-time.After(shutdownTimeout):
		_ = killProcess(proc)
		select {
		case <-done:
		case <-time.After(500 * time.Millisecond):
		}
	}

	e.cmd = nil
	return nil
}
