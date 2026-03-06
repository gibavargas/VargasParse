// Package progress provides real-time terminal progress UI
// for the PDF extraction pipeline.
package progress

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
)

const (
	MethodFast    = "fast"
	MethodOCR     = "ocr"
	MethodOCRFail = "ocr-fail"
	MethodSkip    = "skip"
	MethodCompare = "compare" // ran both native+OCR, picked best
)

// Event is sent by workers to the progress printer goroutine.
type Event struct {
	PageNum  int
	Method   string
	Score    float64
	Duration time.Duration
	Warning  string // optional warning message
}

// Stats holds aggregate counts per method.
type Stats struct {
	Fast    int
	OCR     int
	Compare int
	OCRFail int
	Skip    int
}

var (
	colorGreen  = color.New(color.FgGreen, color.Bold)
	colorYellow = color.New(color.FgYellow, color.Bold)
	colorRed    = color.New(color.FgRed, color.Bold)
	colorGray   = color.New(color.FgHiBlack)
	colorCyan   = color.New(color.FgCyan, color.Bold)
	colorWhite  = color.New(color.FgWhite, color.Bold)
)

// IsTTY returns true if stdout is a terminal.
func IsTTY() bool {
	stat, _ := os.Stdout.Stat()
	return (stat.Mode() & os.ModeCharDevice) != 0
}

// FormatDuration formats a duration for display.
func FormatDuration(d time.Duration) string {
	if d >= time.Second {
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
	return fmt.Sprintf("%dms", d.Milliseconds())
}

func bar(done, total, width int) string {
	if total == 0 {
		return ""
	}
	ratio := float64(done) / float64(total)
	filled := int(ratio * float64(width))
	if filled > width {
		filled = width
	}
	return fmt.Sprintf("[%s%s] %d/%d (%.0f%%)",
		strings.Repeat("█", filled),
		strings.Repeat("░", width-filled),
		done, total, ratio*100)
}

// PrintBanner prints the startup banner.
func PrintBanner() {
	fmt.Println()
	colorCyan.Println("████████████████████████████████████████")
	colorWhite.Println("  🔍  VargasParse — PDF Text Extractor  ")
	colorCyan.Println("████████████████████████████████████████")
	fmt.Println()
}

// PrintConfig prints configuration info.
func PrintConfig(dictWords int, dictDuration time.Duration, pdfName string, numPages, numWorkers int, profile string) {
	colorGreen.Printf("📖 Dictionary: %d words (%s)\n", dictWords, FormatDuration(dictDuration))
	fmt.Printf("📄 PDF: %s  (%d pages)\n", pdfName, numPages)
	fmt.Printf("🖥  Workers: %d  |  Profile: %s\n\n", numWorkers, profile)
}

// Printer receives events and renders live progress to the terminal.
// Close the events channel when all pages are done; the returned
// channel is closed when the printer has finished flushing.
func Printer(events <-chan Event, total int, tty bool) (done chan struct{}, stats *Stats) {
	stats = &Stats{}
	done = make(chan struct{})

	go func() {
		defer close(done)
		processed := 0
		barWidth := 30

		for ev := range events {
			processed++
			switch ev.Method {
			case MethodFast:
				stats.Fast++
			case MethodOCR:
				stats.OCR++
			case MethodCompare:
				stats.Compare++
			case MethodOCRFail:
				stats.OCRFail++
			case MethodSkip:
				stats.Skip++
			}

			if tty {
				fmt.Printf("\r%-60s\n", bar(processed, total, barWidth))
			}

			switch ev.Method {
			case MethodFast:
				colorGreen.Printf("  ✓ ")
				fmt.Printf("Page %4d  fast-extract   (conf: %.0f%%)  %s\n",
					ev.PageNum, ev.Score*100, FormatDuration(ev.Duration))
			case MethodOCR:
				colorYellow.Printf("  ⚡ ")
				fmt.Printf("Page %4d  OCR fallback   (conf: %.0f%%)  %s\n",
					ev.PageNum, ev.Score*100, FormatDuration(ev.Duration))
			case MethodCompare:
				colorYellow.Printf("  ⚖  ")
				fmt.Printf("Page %4d  compare        (conf: %.0f%%)  %s\n",
					ev.PageNum, ev.Score*100, FormatDuration(ev.Duration))
			case MethodOCRFail:
				colorRed.Printf("  ✗ ")
				fmt.Printf("Page %4d  OCR FAILED                    %s\n",
					ev.PageNum, FormatDuration(ev.Duration))
			case MethodSkip:
				colorGray.Printf("  · ")
				fmt.Printf("Page %4d  skipped\n", ev.PageNum)
			}

			if ev.Warning != "" {
				colorGray.Printf("       ⚠ %s\n", ev.Warning)
			}

			if tty {
				fmt.Printf("\r%-60s", bar(processed, total, barWidth))
			}
		}

		if tty {
			fmt.Println()
		}
	}()
	return done, stats
}

// PrintSummary prints the final summary footer.
func PrintSummary(s *Stats, wallDuration time.Duration, outputPath string, nonEmpty, total int) {
	fmt.Println()
	colorCyan.Println("════════════════════════════════════════")
	colorWhite.Println("  VargasParse — Summary")
	colorCyan.Println("════════════════════════════════════════")
	colorGreen.Printf("  ✓ Fast extract:  %d pages\n", s.Fast)
	if s.Compare > 0 {
		colorYellow.Printf("  ⚖  Compare:      %d pages\n", s.Compare)
	}
	colorYellow.Printf("  ⚡ OCR fallback:  %d pages\n", s.OCR)
	colorRed.Printf("  ✗ OCR failures:  %d pages\n", s.OCRFail)
	if s.Skip > 0 {
		colorGray.Printf("  · Skipped:       %d pages\n", s.Skip)
	}
	colorCyan.Println("════════════════════════════════════════")
	colorGreen.Printf("\n✅ Done in %s → %s\n", FormatDuration(wallDuration), outputPath)
	colorGray.Printf("   %d/%d pages with text content\n\n", nonEmpty, total)
}
