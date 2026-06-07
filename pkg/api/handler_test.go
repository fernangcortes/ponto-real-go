package api

import (
	"testing"
)

func TestIsValidMesAno(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"04/2026", true},
		{"12/2025", true},
		{"", false},
		{"??/????", false},
		{"04/??/????", false},
		{"04_2026", false},
		{"4/2026", false},
		{"04/26", false},
	}

	for _, tc := range tests {
		result := isValidMesAno(tc.input)
		if result != tc.expected {
			t.Errorf("isValidMesAno(%q) = %v; expected %v", tc.input, result, tc.expected)
		}
	}
}

func TestExtractDateFromFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"abril2026.png", "04/2026"},
		{"ponto_04_2026.pdf", "04/2026"},
		{"2026_05_ponto.png", "05/2026"},
		{"ponto-abr-26.jpeg", "04/2026"},
		{"Frequencia_Marco_2025.pdf", "03/2025"},
		{"ponto.png", ""},
		{"ponto_dez_26.pdf", "12/2026"},
	}

	for _, tc := range tests {
		result := extractDateFromFilename(tc.input)
		if result != tc.expected {
			t.Errorf("extractDateFromFilename(%q) = %q; expected %q", tc.input, result, tc.expected)
		}
	}
}
