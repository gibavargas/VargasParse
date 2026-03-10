package pipeline

import (
	"fmt"

	"github.com/ledongthuc/pdf"
)

// PageCount returns the number of pages in a PDF.
func PageCount(pdfPath string) (int, error) {
	f, r, err := pdf.Open(pdfPath)
	if err != nil {
		return 0, fmt.Errorf("open pdf: %w", err)
	}
	defer f.Close()
	return r.NumPage(), nil
}
