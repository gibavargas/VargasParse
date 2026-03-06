package table

import "testing"

func TestExtractTableFromTokens(t *testing.T) {
	tokens := []textToken{
		{X: 10, Y: 100, W: 20, S: "Name"},
		{X: 80, Y: 100, W: 20, S: "Age"},
		{X: 10, Y: 90, W: 20, S: "Alice"},
		{X: 80, Y: 90, W: 20, S: "30"},
		{X: 10, Y: 80, W: 20, S: "Bob"},
		{X: 80, Y: 80, W: 20, S: "28"},
	}

	tables := ExtractTableFromTokens(tokens)
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	if tables[0].Rows < 2 || tables[0].Cols < 2 {
		t.Fatalf("unexpected shape rows=%d cols=%d", tables[0].Rows, tables[0].Cols)
	}
	if len(tables[0].Cells) < 4 {
		t.Fatalf("expected >=4 cells, got %d", len(tables[0].Cells))
	}
}

func TestExtractTableFromTokens_NoTable(t *testing.T) {
	tokens := []textToken{
		{X: 10, Y: 100, W: 50, S: "This"},
		{X: 40, Y: 100, W: 50, S: "is"},
		{X: 55, Y: 100, W: 50, S: "a"},
		{X: 65, Y: 100, W: 50, S: "sentence"},
	}
	if tables := ExtractTableFromTokens(tokens); len(tables) != 0 {
		t.Fatalf("expected no table, got %d", len(tables))
	}
}
