package ocr

import "testing"

func TestBuildTesseractLang(t *testing.T) {
	tests := []struct {
		hint string
		want string
	}{
		{"", "por+eng"},
		{"auto", "por+eng"},
		{"eng", "eng"},
		{"por,eng", "por+eng"},
		{" por , eng , spa ", "por+eng+spa"},
	}

	for _, tc := range tests {
		if got := buildTesseractLang(tc.hint); got != tc.want {
			t.Fatalf("hint=%q got=%q want=%q", tc.hint, got, tc.want)
		}
	}
}
