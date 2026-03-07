package ocr

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeCommand struct {
	combinedFn func() ([]byte, error)
	outputFn   func() ([]byte, error)
}

func (c fakeCommand) CombinedOutput() ([]byte, error) {
	if c.combinedFn == nil {
		return nil, nil
	}
	return c.combinedFn()
}

func (c fakeCommand) Output() ([]byte, error) {
	if c.outputFn == nil {
		return nil, nil
	}
	return c.outputFn()
}

func TestBuildTesseractLang(t *testing.T) {
	tests := []struct {
		hint string
		want string
	}{
		{"", "por+eng"},
		{"auto", "por+eng"},
		{"eng", "eng"},
		{"por,eng", "por+eng"},
		{" por , eng , spa ", "por+eng+spa"},
	}

	for _, tc := range tests {
		if got := buildTesseractLang(tc.hint); got != tc.want {
			t.Fatalf("hint=%q got=%q want=%q", tc.hint, got, tc.want)
		}
	}
}

func TestNewOCRMissingDependencies(t *testing.T) {
	origLookPath := lookPath
	defer func() { lookPath = origLookPath }()

	lookPath = func(name string) (string, error) {
		if name == "pdftoppm" {
			return "", errors.New("missing")
		}
		return "/usr/bin/" + name, nil
	}
	if _, err := NewOCR(); err == nil || !strings.Contains(err.Error(), "pdftoppm not found") {
		t.Fatalf("expected pdftoppm error, got %v", err)
	}

	lookPath = func(name string) (string, error) {
		if name == "tesseract" {
			return "", errors.New("missing")
		}
		return "/usr/bin/" + name, nil
	}
	if _, err := NewOCR(); err == nil || !strings.Contains(err.Error(), "tesseract not found") {
		t.Fatalf("expected tesseract error, got %v", err)
	}
}

func TestProcessPage_ContextCanceledBeforeStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	o := &OCR{
		pdftoppmPath:  "pdftoppm",
		tesseractPath: "tesseract",
		mkTempDir: func(dir, pattern string) (string, error) {
			t.Fatal("mkTempDir should not be called")
			return "", nil
		},
	}

	_, _, err := o.ProcessPage(ctx, "doc.pdf", 0, "eng")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
}

func TestProcessPage_MkTempError(t *testing.T) {
	o := &OCR{
		pdftoppmPath:  "pdftoppm",
		tesseractPath: "tesseract",
		mkTempDir: func(dir, pattern string) (string, error) {
			return "", errors.New("mktemp failed")
		},
	}

	_, _, err := o.ProcessPage(context.Background(), "doc.pdf", 0, "eng")
	if err == nil || !strings.Contains(err.Error(), "mktemp") {
		t.Fatalf("expected mktemp error, got %v", err)
	}
}

func TestProcessPage_PdfToPpmFailure(t *testing.T) {
	tmp := t.TempDir()
	o := &OCR{
		pdftoppmPath:  "pdftoppm",
		tesseractPath: "tesseract",
		mkTempDir: func(dir, pattern string) (string, error) {
			return tmp, nil
		},
		removeAll: func(path string) error { return nil },
		makeCommand: func(ctx context.Context, name string, args ...string) command {
			if name != "pdftoppm" {
				t.Fatalf("unexpected command %q", name)
			}
			return fakeCommand{combinedFn: func() ([]byte, error) {
				return []byte("bad raster"), errors.New("exit status 1")
			}}
		},
	}

	_, _, err := o.ProcessPage(context.Background(), "doc.pdf", 0, "eng")
	if err == nil || !strings.Contains(err.Error(), "pdftoppm") || !strings.Contains(err.Error(), "bad raster") {
		t.Fatalf("expected pdftoppm error with output, got %v", err)
	}
}

func TestProcessPage_GlobError(t *testing.T) {
	tmp := t.TempDir()
	o := &OCR{
		pdftoppmPath:  "pdftoppm",
		tesseractPath: "tesseract",
		mkTempDir: func(dir, pattern string) (string, error) {
			return tmp, nil
		},
		removeAll: func(path string) error { return nil },
		glob: func(pattern string) ([]string, error) {
			return nil, errors.New("glob failed")
		},
		makeCommand: func(ctx context.Context, name string, args ...string) command {
			return fakeCommand{combinedFn: func() ([]byte, error) { return []byte("ok"), nil }}
		},
	}

	_, _, err := o.ProcessPage(context.Background(), "doc.pdf", 0, "eng")
	if err == nil || !strings.Contains(err.Error(), "glob png") {
		t.Fatalf("expected glob error, got %v", err)
	}
}

func TestProcessPage_NoRasterizedPage(t *testing.T) {
	tmp := t.TempDir()
	o := &OCR{
		pdftoppmPath:  "pdftoppm",
		tesseractPath: "tesseract",
		mkTempDir: func(dir, pattern string) (string, error) {
			return tmp, nil
		},
		removeAll: func(path string) error { return nil },
		glob: func(pattern string) ([]string, error) {
			return []string{}, nil
		},
		makeCommand: func(ctx context.Context, name string, args ...string) command {
			if name != "pdftoppm" {
				t.Fatalf("unexpected command %q", name)
			}
			return fakeCommand{combinedFn: func() ([]byte, error) { return []byte("ok"), nil }}
		},
	}

	_, _, err := o.ProcessPage(context.Background(), "doc.pdf", 0, "eng")
	if err == nil || !strings.Contains(err.Error(), "no rasterized page") {
		t.Fatalf("expected no rasterized page error, got %v", err)
	}
}

func TestProcessPage_TesseractFailure(t *testing.T) {
	tmp := t.TempDir()
	o := &OCR{
		pdftoppmPath:  "pdftoppm",
		tesseractPath: "tesseract",
		mkTempDir: func(dir, pattern string) (string, error) {
			return tmp, nil
		},
		removeAll: func(path string) error { return nil },
		glob: func(pattern string) ([]string, error) {
			return []string{"/tmp/page-1.png"}, nil
		},
		makeCommand: func(ctx context.Context, name string, args ...string) command {
			switch name {
			case "pdftoppm":
				return fakeCommand{combinedFn: func() ([]byte, error) { return []byte("ok"), nil }}
			case "tesseract":
				return fakeCommand{outputFn: func() ([]byte, error) { return nil, errors.New("ocr failed") }}
			default:
				t.Fatalf("unexpected command %q", name)
				return fakeCommand{}
			}
		},
	}

	_, _, err := o.ProcessPage(context.Background(), "doc.pdf", 0, "por,eng")
	if err == nil || !strings.Contains(err.Error(), "tesseract") {
		t.Fatalf("expected tesseract error, got %v", err)
	}
}

func TestProcessPage_Success(t *testing.T) {
	tmp := t.TempDir()
	o := &OCR{
		pdftoppmPath:  "pdftoppm",
		tesseractPath: "tesseract",
		mkTempDir: func(dir, pattern string) (string, error) {
			return tmp, nil
		},
		removeAll: func(path string) error { return nil },
		glob: func(pattern string) ([]string, error) {
			return []string{"/tmp/page-1.png"}, nil
		},
		makeCommand: func(ctx context.Context, name string, args ...string) command {
			switch name {
			case "pdftoppm":
				return fakeCommand{combinedFn: func() ([]byte, error) { return []byte("ok"), nil }}
			case "tesseract":
				return fakeCommand{outputFn: func() ([]byte, error) { return []byte("recognized text"), nil }}
			default:
				t.Fatalf("unexpected command %q", name)
				return fakeCommand{}
			}
		},
	}

	text, conf, err := o.ProcessPage(context.Background(), "doc.pdf", 1, "por,eng")
	if err != nil {
		t.Fatalf("ProcessPage error: %v", err)
	}
	if strings.TrimSpace(text) != "recognized text" {
		t.Fatalf("unexpected text: %q", text)
	}
	if conf != 0 {
		t.Fatalf("unexpected confidence: %v", conf)
	}
}

func TestProcessPage_ContextCanceledDuringRaster(t *testing.T) {
	tmp := t.TempDir()
	o := &OCR{
		pdftoppmPath:  "pdftoppm",
		tesseractPath: "tesseract",
		mkTempDir: func(dir, pattern string) (string, error) {
			return tmp, nil
		},
		removeAll: func(path string) error { return nil },
		makeCommand: func(ctx context.Context, name string, args ...string) command {
			if name != "pdftoppm" {
				t.Fatalf("unexpected command %q", name)
			}
			return fakeCommand{combinedFn: func() ([]byte, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			}}
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, _, err := o.ProcessPage(ctx, "doc.pdf", 0, "eng")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "context deadline exceeded") {
		t.Fatalf("expected context deadline error, got %v", err)
	}
}
