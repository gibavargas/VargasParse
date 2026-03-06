package quality

import (
	"testing"
)

func TestCleanAndTokenize(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantLen   int
		wantFirst string
	}{
		{"empty", "", 0, ""},
		{"numbers only", "R$ 1500.00", 0, ""},
		{"single char", "a b c", 0, ""},
		{"normal text", "Hello World", 2, "hello"},
		{"with numbers", "Total: R$ 1500 items", 2, "total"},
		{"portuguese", "São Paulo é uma cidade", 4, "são"},
		{"mixed garbage", "Hello !!! World", 2, "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := CleanAndTokenize(tt.input)
			if len(tokens) != tt.wantLen {
				t.Errorf("got %d tokens, want %d — tokens: %v", len(tokens), tt.wantLen, tokens)
			}
			if tt.wantFirst != "" && len(tokens) > 0 && tokens[0] != tt.wantFirst {
				t.Errorf("first token = %q, want %q", tokens[0], tt.wantFirst)
			}
		})
	}
}

func TestAssessQuality_Accept(t *testing.T) {
	dict := map[string]bool{
		"the": true, "quick": true, "brown": true,
		"fox": true, "jumps": true, "over": true,
		"lazy": true, "dog": true,
	}

	text := "The quick brown fox jumps over the lazy dog"
	r := AssessQuality(text, dict)

	if r.Decision != Accept {
		t.Errorf("Decision = %v, want Accept (confidence=%.2f)", r.Decision, r.Confidence)
	}
	if r.Confidence < 0.80 {
		t.Errorf("Confidence = %.2f, want >= 0.80", r.Confidence)
	}
}

func TestAssessQuality_Reject_Garbage(t *testing.T) {
	dict := map[string]bool{"hello": true}

	// Simulated CMap garbage
	text := "cid(123) cid(456) \x00\x01\x02 !!!### garbage"
	r := AssessQuality(text, dict)

	if r.Decision == Accept {
		t.Errorf("Decision = Accept, want Compare or Reject for garbage text (confidence=%.2f)", r.Confidence)
	}
}

func TestAssessQuality_Empty(t *testing.T) {
	r := AssessQuality("", nil)
	if r.Decision != Reject {
		t.Errorf("Decision = %v, want Reject for empty text", r.Decision)
	}
}

func TestAssessQuality_FewTokens_Clean(t *testing.T) {
	dict := map[string]bool{"ok": true}

	// Only 1 token — should still accept if clean
	r := AssessQuality("OK", dict)
	if r.Decision != Accept {
		t.Errorf("Decision = %v, want Accept for few clean tokens (confidence=%.2f)", r.Decision, r.Confidence)
	}
}

func TestDecision_String(t *testing.T) {
	if Accept.String() != "accept" {
		t.Errorf("Accept.String() = %q", Accept.String())
	}
	if Compare.String() != "compare" {
		t.Errorf("Compare.String() = %q", Compare.String())
	}
	if Reject.String() != "reject" {
		t.Errorf("Reject.String() = %q", Reject.String())
	}
}

func TestLoadDictionary(t *testing.T) {
	dict := LoadDictionary("hello\nworld\n\n  TEST  \n")
	if len(dict) != 3 {
		t.Errorf("got %d words, want 3", len(dict))
	}
	if !dict["hello"] || !dict["world"] || !dict["test"] {
		t.Errorf("missing expected words: %v", dict)
	}
}
