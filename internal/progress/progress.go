// Package progress provides real-time terminal progress UI
// for the PDF extraction pipeline.
package progress

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

type progressMsg Event
type completeMsg struct{}

type tuiModel struct {
	total     int
	processed int
	stats     Stats
	recent    []Event
	width     int
	done      bool
}

var (
	colorGreen  = color.New(color.FgGreen, color.Bold)
	colorYellow = color.New(color.FgYellow, color.Bold)
	colorRed    = color.New(color.FgRed, color.Bold)
	colorGray   = color.New(color.FgHiBlack)
	colorCyan   = color.New(color.FgCyan, color.Bold)
	colorWhite  = color.New(color.FgWhite, color.Bold)

	styleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("62")).
			Padding(0, 1)
	styleSubtitle = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	styleGood     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	styleWarn     = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	styleBad      = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	stylePanel    = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(0, 1)
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
	if tty {
		return printerTUI(events, total)
	}
	return printerPlain(events, total)
}

func printerPlain(events <-chan Event, total int) (done chan struct{}, stats *Stats) {
	stats = &Stats{}
	done = make(chan struct{})

	go func() {
		defer close(done)
		processed := 0
		for ev := range events {
			processed++
			applyStat(stats, ev.Method)

			switch ev.Method {
			case MethodFast:
				fmt.Printf("  ✓ Page %4d  fast-extract   (conf: %.0f%%)  %s\n", ev.PageNum, ev.Score*100, FormatDuration(ev.Duration))
			case MethodOCR:
				fmt.Printf("  ⚡ Page %4d  OCR fallback   (conf: %.0f%%)  %s\n", ev.PageNum, ev.Score*100, FormatDuration(ev.Duration))
			case MethodCompare:
				fmt.Printf("  ⚖  Page %4d  compare        (conf: %.0f%%)  %s\n", ev.PageNum, ev.Score*100, FormatDuration(ev.Duration))
			case MethodOCRFail:
				fmt.Printf("  ✗ Page %4d  OCR FAILED                    %s\n", ev.PageNum, FormatDuration(ev.Duration))
			case MethodSkip:
				fmt.Printf("  · Page %4d  skipped\n", ev.PageNum)
			}
			if ev.Warning != "" {
				fmt.Printf("       ⚠ %s\n", ev.Warning)
			}
			fmt.Printf("       %s\n", bar(processed, total, 30))
		}
	}()

	return done, stats
}

func printerTUI(events <-chan Event, total int) (done chan struct{}, stats *Stats) {
	stats = &Stats{}
	done = make(chan struct{})
	model := tuiModel{total: total, width: 90}
	prog := tea.NewProgram(model, tea.WithAltScreen())

	go func() {
		finalModel, err := prog.Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "progress TUI error: %v\n", err)
			close(done)
			return
		}
		if m, ok := finalModel.(tuiModel); ok {
			*stats = m.stats
		}
		close(done)
	}()

	go func() {
		for ev := range events {
			prog.Send(progressMsg(ev))
		}
		prog.Send(completeMsg{})
	}()

	return done, stats
}

func applyStat(stats *Stats, method string) {
	switch method {
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
}

func (m tuiModel) Init() tea.Cmd { return nil }

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil
	case progressMsg:
		ev := Event(msg)
		m.processed++
		applyStat(&m.stats, ev.Method)
		m.recent = append(m.recent, ev)
		if len(m.recent) > 8 {
			m.recent = m.recent[len(m.recent)-8:]
		}
		return m, nil
	case completeMsg:
		m.done = true
		return m, tea.Quit
	}
	return m, nil
}

func (m tuiModel) View() string {
	barWidth := m.width - 28
	if barWidth < 18 {
		barWidth = 18
	}

	head := styleTitle.Render(" VargasParse Live Dashboard ")
	sub := styleSubtitle.Render(bar(m.processed, m.total, barWidth))

	statsLine := fmt.Sprintf(
		"%s  %s  %s  %s",
		styleGood.Render(fmt.Sprintf("fast: %d", m.stats.Fast)),
		styleWarn.Render(fmt.Sprintf("compare: %d", m.stats.Compare)),
		styleWarn.Render(fmt.Sprintf("ocr: %d", m.stats.OCR)),
		styleBad.Render(fmt.Sprintf("fail: %d", m.stats.OCRFail)),
	)

	var recent strings.Builder
	recent.WriteString("Recent Pages\n")
	for _, ev := range m.recent {
		method := ev.Method
		style := styleSubtitle
		switch ev.Method {
		case MethodFast:
			style = styleGood
		case MethodCompare, MethodOCR:
			style = styleWarn
		case MethodOCRFail:
			style = styleBad
		}
		line := fmt.Sprintf("p%-4d %-10s conf=%3.0f%% dur=%-8s", ev.PageNum, method, ev.Score*100, FormatDuration(ev.Duration))
		if ev.Warning != "" {
			line += "  ⚠ " + ev.Warning
		}
		recent.WriteString(style.Render(line))
		recent.WriteString("\n")
	}

	footer := styleSubtitle.Render("Ctrl+C to abort")
	if m.done {
		footer = styleGood.Render("Completed")
	}

	top := stylePanel.Width(m.width - 2).Render(fmt.Sprintf("%s\n%s\n%s", head, sub, statsLine))
	bottom := stylePanel.Width(m.width - 2).Render(recent.String())
	return fmt.Sprintf("%s\n%s\n%s\n", top, bottom, footer)
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
