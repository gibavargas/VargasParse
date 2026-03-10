package renderer

import (
	"errors"
	"image"
	"strings"
	"testing"
)

type fakeDoc struct {
	pages int
	img   image.Image
	err   error
}

func (d fakeDoc) NumPage() int { return d.pages }
func (d fakeDoc) ImageDPI(pageIndex int, dpi float64) (image.Image, error) {
	if d.err != nil {
		return nil, d.err
	}
	return d.img, nil
}
func (d fakeDoc) Close() error { return nil }

func TestNormalizeDPI(t *testing.T) {
	cases := []struct {
		in   float64
		want float64
	}{
		{0, 150},
		{-12, 150},
		{71, 72},
		{72, 72},
		{150, 150},
		{800, 600},
	}

	for _, tc := range cases {
		got := normalizeDPI(tc.in)
		if got != tc.want {
			t.Fatalf("normalizeDPI(%v)=%v want %v", tc.in, got, tc.want)
		}
	}
}

func TestRenderPageNilDoc(t *testing.T) {
	r := &PDFRasterizer{}
	_, err := r.RenderPage(0, 150)
	if err == nil {
		t.Fatal("expected error for nil document")
	}
	if !strings.Contains(err.Error(), "no open document") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRenderPageBounds(t *testing.T) {
	r := &PDFRasterizer{Doc: fakeDoc{pages: 1, img: image.NewRGBA(image.Rect(0, 0, 1, 1))}}
	_, err := r.RenderPage(-1, 150)
	if err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("expected out of range error, got: %v", err)
	}
	_, err = r.RenderPage(2, 150)
	if err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("expected out of range error, got: %v", err)
	}
}

func TestRenderPageSuccessAndDocError(t *testing.T) {
	r := &PDFRasterizer{Doc: fakeDoc{pages: 1, img: image.NewRGBA(image.Rect(0, 0, 1, 1))}}
	img, err := r.RenderPage(0, 10)
	if err != nil {
		t.Fatalf("RenderPage error: %v", err)
	}
	if img == nil {
		t.Fatal("expected image")
	}

	rErr := &PDFRasterizer{Doc: fakeDoc{pages: 1, err: errors.New("render failed")}}
	_, err = rErr.RenderPage(0, 150)
	if err == nil || !strings.Contains(err.Error(), "failed to render page") {
		t.Fatalf("unexpected render error: %v", err)
	}
}
