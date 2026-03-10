package llamacpp

import (
	"context"
	"encoding/base64"
	"os"
	"testing"
	"time"
)

func TestSmoke_VLMRescue(t *testing.T) {
	if os.Getenv("VARGASPARSE_SMOKE") != "1" || os.Getenv("VARGASPARSE_TEST_OLLAMA") != "1" {
		t.Skip("set VARGASPARSE_SMOKE=1 and VARGASPARSE_TEST_OLLAMA=1 to run VLM smoke test")
	}

	engine, err := NewEngine("llava")
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer func() { _ = engine.Close() }()

	// 1x1 transparent PNG
	const onePixelPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMB/axW8QAAAABJRU5ErkJggg=="
	_, err = base64.StdEncoding.DecodeString(onePixelPNGBase64)
	if err != nil {
		t.Fatalf("invalid fixture base64: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	text, err := engine.ExtractMarkdownWithRetry(ctx, onePixelPNGBase64, SystemPromptMarkdown, 1)
	if err != nil {
		t.Fatalf("ExtractMarkdownWithRetry: %v", err)
	}
	if text == "" {
		t.Fatal("expected non-empty response from VLM smoke call")
	}
}
