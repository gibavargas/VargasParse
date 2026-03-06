package renderer

import (
	"fmt"
	"image"

	"github.com/gen2brain/go-fitz"
)

// PDFRasterizer handles converting PDF documents into high-resolution images
// suitable for Vision Model object detection and OCR.
type PDFRasterizer struct {
	Doc *fitz.Document
}

// NewPDFRasterizer opens the PDF file leveraging the MuPDF bindings via go-fitz.
func NewPDFRasterizer(pdfPath string) (*PDFRasterizer, error) {
	doc, err := fitz.New(pdfPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open PDF %s: %w", pdfPath, err)
	}

	return &PDFRasterizer{
		Doc: doc,
	}, nil
}

// Close cleans up MuPDF C pointers.
func (r *PDFRasterizer) Close() error {
	if r.Doc != nil {
		return r.Doc.Close()
	}
	return nil
}

// NumPages returns the total pages in the PDF.
func (r *PDFRasterizer) NumPages() int {
	return r.Doc.NumPage()
}

// RenderPage renders the specified page index into a Go Image.
// Providing a Dpi > 0 scales the rendering (e.g. 150 for Layout, 300+ for OCR).
func (r *PDFRasterizer) RenderPage(pageIndex int, dpi float64) (image.Image, error) {
	if pageIndex < 0 || pageIndex >= r.NumPages() {
		return nil, fmt.Errorf("page index out of range: %d", pageIndex)
	}
	if dpi <= 0 {
		dpi = 150
	}
	// Clamp to practical OCR/VLM range.
	if dpi < 72 {
		dpi = 72
	}
	if dpi > 600 {
		dpi = 600
	}
	// fitz DPI scaling is relative to 72 default usually. We use ImageDPI here.
	img, err := r.Doc.ImageDPI(pageIndex, dpi)
	if err != nil {
		return nil, fmt.Errorf("failed to render page %d: %w", pageIndex, err)
	}
	return img, nil
}
