package renderer

import (
	"fmt"
	"image"

	"github.com/gen2brain/go-fitz"
)

// DefaultDPI is the standard rendering resolution used when no DPI is specified.
const DefaultDPI = 150.0

type fitzDoc interface {
	NumPage() int
	ImageDPI(pageIndex int, dpi float64) (image.Image, error)
	Close() error
}

type realFitzDoc struct {
	doc *fitz.Document
}

func (d *realFitzDoc) NumPage() int {
	return d.doc.NumPage()
}

func (d *realFitzDoc) ImageDPI(pageIndex int, dpi float64) (image.Image, error) {
	return d.doc.ImageDPI(pageIndex, dpi)
}

func (d *realFitzDoc) Close() error {
	return d.doc.Close()
}

// PDFRasterizer handles converting PDF documents into high-resolution images
// suitable for Vision Model object detection and OCR.
type PDFRasterizer struct {
	Doc fitzDoc
}

// NewPDFRasterizer opens the PDF file leveraging the MuPDF bindings via go-fitz.
func NewPDFRasterizer(pdfPath string) (*PDFRasterizer, error) {
	doc, err := fitz.New(pdfPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open PDF %s: %w", pdfPath, err)
	}

	return &PDFRasterizer{
		Doc: &realFitzDoc{doc: doc},
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
	if r.Doc == nil {
		return 0
	}
	return r.Doc.NumPage()
}

func normalizeDPI(dpi float64) float64 {
	if dpi <= 0 {
		return DefaultDPI
	}
	// Clamp to practical OCR/VLM range.
	if dpi < 72 {
		return 72
	}
	if dpi > 600 {
		return 600
	}
	return dpi
}

// RenderPage renders the specified page index into a Go Image.
// Providing a Dpi > 0 scales the rendering (e.g. 150 for Layout, 300+ for OCR).
func (r *PDFRasterizer) RenderPage(pageIndex int, dpi float64) (image.Image, error) {
	if r.Doc == nil {
		return nil, fmt.Errorf("rasterizer has no open document")
	}
	if pageIndex < 0 || pageIndex >= r.NumPages() {
		return nil, fmt.Errorf("page index out of range: %d", pageIndex)
	}
	dpi = normalizeDPI(dpi)
	// fitz DPI scaling is relative to 72 default usually. We use ImageDPI here.
	img, err := r.Doc.ImageDPI(pageIndex, dpi)
	if err != nil {
		return nil, fmt.Errorf("failed to render page %d: %w", pageIndex, err)
	}
	return img, nil
}
