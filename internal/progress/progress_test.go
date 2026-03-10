package progress

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestApplyStat(t *testing.T) {
	s := &Stats{}
	applyStat(s, MethodFast)
	applyStat(s, MethodOCR)
	applyStat(s, MethodCompare)
	applyStat(s, MethodOCRFail)
	applyStat(s, MethodSkip)

	if s.Fast != 1 || s.OCR != 1 || s.Compare != 1 || s.OCRFail != 1 || s.Skip != 1 {
		t.Fatalf("unexpected stats: %+v", s)
	}
}

func TestBar(t *testing.T) {
	got := bar(3, 10, 10)
	if !strings.Contains(got, "3/10") || !strings.Contains(got, "30%") {
		t.Fatalf("unexpected bar output: %q", got)
	}
}

func TestFormatDuration(t *testing.T) {
	if got := FormatDuration(950 * time.Millisecond); got != "950ms" {
		t.Fatalf("got %q", got)
	}
	if got := FormatDuration(1500 * time.Millisecond); got != "1.50s" {
		t.Fatalf("got %q", got)
	}
}

func TestTUIModelUpdateAndView(t *testing.T) {
	m := tuiModel{total: 20, width: 80}
	for i := 1; i <= 10; i++ {
		var method string
		switch i % 4 {
		case 0:
			method = MethodOCRFail
		case 1:
			method = MethodFast
		case 2:
			method = MethodCompare
		default:
			method = MethodOCR
		}

		updated, _ := m.Update(progressMsg(Event{
			PageNum:  i,
			Method:   method,
			Score:    0.88,
			Duration: 120 * time.Millisecond,
		}))
		m = updated.(tuiModel)
	}

	if m.processed != 10 {
		t.Fatalf("processed=%d want 10", m.processed)
	}
	if len(m.recent) != 8 {
		t.Fatalf("recent len=%d want 8", len(m.recent))
	}

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(tuiModel)
	if m.width != 120 {
		t.Fatalf("width=%d want 120", m.width)
	}

	view := m.View()
	if !strings.Contains(view, "VargasParse Live Dashboard") {
		t.Fatalf("unexpected view output: %q", view)
	}
	if !strings.Contains(view, "Recent Pages") {
		t.Fatalf("missing recent section: %q", view)
	}
}

func TestPrinterPlain(t *testing.T) {
	events := make(chan Event, 2)
	done, stats := Printer(events, 2, false)

	events <- Event{PageNum: 1, Method: MethodFast, Score: 0.9, Duration: 10 * time.Millisecond}
	events <- Event{PageNum: 2, Method: MethodOCRFail, Duration: 11 * time.Millisecond, Warning: "ocr fail"}
	close(events)
	<-done

	if stats.Fast != 1 || stats.OCRFail != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}

func TestOutputHelpersSmoke(t *testing.T) {
	_ = IsTTY()

	PrintBanner()
	PrintConfig(123, 42*time.Millisecond, "sample.pdf", 9, 2, "balanced")
	PrintSummary(&Stats{Fast: 7, OCR: 1, Compare: 1, OCRFail: 0, Skip: 0}, 2*time.Second, "out.txt", 9, 9)

	m := tuiModel{}
	if cmd := m.Init(); cmd != nil {
		t.Fatalf("expected nil init command, got %#v", cmd)
	}
}
