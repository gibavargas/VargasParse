package table

import (
	"math"
	"sort"
	"strings"

	"github.com/ledongthuc/pdf"
)

// BBox represents a bounding box in PDF points.
type BBox struct {
	X_min, Y_min float64
	X_max, Y_max float64
}

// Cell represents an extracted table cell natively.
type Cell struct {
	Box     BBox
	Row     int
	Col     int
	RowSpan int
	ColSpan int
	Text    string
}

// Table maps a grid struct mapped from the PDF vector stream.
type Table struct {
	Box   BBox
	Cells []Cell
	Rows  int
	Cols  int
}

type textToken struct {
	X float64
	Y float64
	W float64
	S string
}

type row struct {
	Y      float64
	Tokens []textToken
}

// ExtractTable detects tables from positioned page text. It uses a deterministic
// heuristic: rows with >=2 aligned columns and recurring layout form a table.
func ExtractTable(page pdf.Page) []Table {
	content := page.Content()
	tokens := make([]textToken, 0, len(content.Text))
	for _, t := range content.Text {
		s := strings.TrimSpace(t.S)
		if s == "" {
			continue
		}
		tokens = append(tokens, textToken{X: t.X, Y: t.Y, W: t.W, S: s})
	}
	return ExtractTableFromTokens(tokens)
}

// ExtractTableFromTokens is testable logic used by ExtractTable.
func ExtractTableFromTokens(tokens []textToken) []Table {
	if len(tokens) < 6 {
		return nil
	}

	rows := clusterRows(tokens)
	if len(rows) < 2 {
		return nil
	}

	candidates := make([]row, 0, len(rows))
	for _, r := range rows {
		if len(r.Tokens) >= 2 {
			candidates = append(candidates, r)
		}
	}
	if len(candidates) < 2 {
		return nil
	}

	colAnchors := inferColumnAnchors(candidates)
	if len(colAnchors) < 2 {
		return nil
	}

	// Keep only rows that map to at least 2 columns.
	usable := make([]row, 0, len(candidates))
	for _, r := range candidates {
		mapped := 0
		for _, tok := range r.Tokens {
			if nearestColumn(tok.X, colAnchors) >= 0 {
				mapped++
			}
		}
		if mapped >= 2 {
			usable = append(usable, r)
		}
	}
	if len(usable) < 2 {
		return nil
	}

	t := Table{Rows: len(usable), Cols: len(colAnchors)}
	minX, minY := math.MaxFloat64, math.MaxFloat64
	maxX, maxY := -math.MaxFloat64, -math.MaxFloat64

	for ri, r := range usable {
		cellTexts := make([][]string, len(colAnchors))
		cellBoxes := make([]BBox, len(colAnchors))
		for i := range cellBoxes {
			cellBoxes[i] = BBox{X_min: math.MaxFloat64, Y_min: math.MaxFloat64, X_max: -math.MaxFloat64, Y_max: -math.MaxFloat64}
		}

		for _, tok := range r.Tokens {
			ci := nearestColumn(tok.X, colAnchors)
			if ci < 0 {
				continue
			}
			cellTexts[ci] = append(cellTexts[ci], tok.S)
			x2 := tok.X + math.Max(tok.W, 1)
			if tok.X < cellBoxes[ci].X_min {
				cellBoxes[ci].X_min = tok.X
			}
			if r.Y-4 < cellBoxes[ci].Y_min {
				cellBoxes[ci].Y_min = r.Y - 4
			}
			if x2 > cellBoxes[ci].X_max {
				cellBoxes[ci].X_max = x2
			}
			if r.Y+4 > cellBoxes[ci].Y_max {
				cellBoxes[ci].Y_max = r.Y + 4
			}
		}

		for ci := range colAnchors {
			text := strings.TrimSpace(strings.Join(cellTexts[ci], " "))
			if text == "" {
				continue
			}
			box := cellBoxes[ci]
			if box.X_min == math.MaxFloat64 {
				box = BBox{X_min: colAnchors[ci], Y_min: r.Y - 4, X_max: colAnchors[ci] + 1, Y_max: r.Y + 4}
			}

			if box.X_min < minX {
				minX = box.X_min
			}
			if box.Y_min < minY {
				minY = box.Y_min
			}
			if box.X_max > maxX {
				maxX = box.X_max
			}
			if box.Y_max > maxY {
				maxY = box.Y_max
			}

			t.Cells = append(t.Cells, Cell{
				Box:     box,
				Row:     ri,
				Col:     ci,
				RowSpan: 1,
				ColSpan: 1,
				Text:    text,
			})
		}
	}

	if len(t.Cells) < 4 {
		return nil
	}

	t.Box = BBox{X_min: minX, Y_min: minY, X_max: maxX, Y_max: maxY}
	return []Table{t}
}

func clusterRows(tokens []textToken) []row {
	sorted := append([]textToken(nil), tokens...)
	sort.Slice(sorted, func(i, j int) bool {
		if math.Abs(sorted[i].Y-sorted[j].Y) > 0.001 {
			return sorted[i].Y > sorted[j].Y
		}
		return sorted[i].X < sorted[j].X
	})

	const yTol = 3.5
	rows := make([]row, 0)
	for _, tok := range sorted {
		if len(rows) == 0 || math.Abs(rows[len(rows)-1].Y-tok.Y) > yTol {
			rows = append(rows, row{Y: tok.Y, Tokens: []textToken{tok}})
			continue
		}
		r := &rows[len(rows)-1]
		r.Tokens = append(r.Tokens, tok)
	}

	for i := range rows {
		sort.Slice(rows[i].Tokens, func(a, b int) bool {
			return rows[i].Tokens[a].X < rows[i].Tokens[b].X
		})
	}
	return rows
}

func inferColumnAnchors(rows []row) []float64 {
	xs := make([]float64, 0)
	for _, r := range rows {
		for _, tok := range r.Tokens {
			xs = append(xs, tok.X)
		}
	}
	if len(xs) < 2 {
		return nil
	}
	sort.Float64s(xs)

	const xTol = 18.0
	clusters := make([][]float64, 0)
	for _, x := range xs {
		if len(clusters) == 0 {
			clusters = append(clusters, []float64{x})
			continue
		}
		last := clusters[len(clusters)-1]
		centroid := mean(last)
		if math.Abs(centroid-x) <= xTol {
			clusters[len(clusters)-1] = append(clusters[len(clusters)-1], x)
		} else {
			clusters = append(clusters, []float64{x})
		}
	}

	anchors := make([]float64, 0, len(clusters))
	for _, c := range clusters {
		if len(c) >= 2 {
			anchors = append(anchors, mean(c))
		}
	}
	sort.Float64s(anchors)
	return anchors
}

func nearestColumn(x float64, anchors []float64) int {
	if len(anchors) == 0 {
		return -1
	}
	best := -1
	bestDist := math.MaxFloat64
	for i, a := range anchors {
		d := math.Abs(a - x)
		if d < bestDist {
			bestDist = d
			best = i
		}
	}
	if bestDist > 28.0 {
		return -1
	}
	return best
}

func mean(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := 0.0
	for _, x := range v {
		s += x
	}
	return s / float64(len(v))
}

// intersect returns the intersection percentage of two bounding boxes.
func intersectRatio(a, b BBox) float64 {
	xOverlap := math.Max(0, math.Min(a.X_max, b.X_max)-math.Max(a.X_min, b.X_min))
	yOverlap := math.Max(0, math.Min(a.Y_max, b.Y_max)-math.Max(a.Y_min, b.Y_min))
	aArea := (a.X_max - a.X_min) * (a.Y_max - a.Y_min)
	bArea := (b.X_max - b.X_min) * (b.Y_max - b.Y_min)
	if aArea == 0 && bArea == 0 {
		return 0
	}
	minArea := math.Min(aArea, bArea)
	if minArea == 0 {
		return 0
	}
	return (xOverlap * yOverlap) / minArea
}
