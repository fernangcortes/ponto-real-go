package rules

import (
	"testing"

	"github.com/ponto-real-go/internal/models"
)

func TestTimeToMinutes(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"08:00", 480},
		{"12:30", 750},
		{"00:00", 0},
		{"23:59", 1439},
		{"", 0},
		{"**:**", 0},
		{"invalid", 0},
	}
	for _, tt := range tests {
		got := TimeToMinutes(tt.input)
		if got != tt.expected {
			t.Errorf("TimeToMinutes(%q) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}

func TestMinutesToTime(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "+00:00"},
		{480, "+08:00"},
		{-480, "-08:00"},
		{-28, "-00:28"},
		{90, "+01:30"},
	}
	for _, tt := range tests {
		got := MinutesToTime(tt.input)
		if got != tt.expected {
			t.Errorf("MinutesToTime(%d) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestParseSaldo(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"-08:00", -480},
		{"+01:20", 80},
		{"00:00", 0},
		{"-00:15", -15},
		{"", 0},
		{"00:18", 18},
	}
	for _, tt := range tests {
		got := ParseSaldo(tt.input)
		if got != tt.expected {
			t.Errorf("ParseSaldo(%q) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}

func TestClassifyDay(t *testing.T) {
	e := NewEngineWithDefaults()

	tests := []struct {
		name     string
		day      models.DayRecord
		expected models.DayType
	}{
		{
			name:     "Sábado = folga",
			day:      models.DayRecord{Dia: 3, DiaSemana: "Sáb"},
			expected: models.DayTypeFolga,
		},
		{
			name:     "Domingo = folga",
			day:      models.DayRecord{Dia: 4, DiaSemana: "Dom"},
			expected: models.DayTypeFolga,
		},
		{
			name:     "Dia útil sem registro = falta",
			day:      models.DayRecord{Dia: 6, DiaSemana: "Ter", Saldo: "-08:00"},
			expected: models.DayTypeFalta,
		},
		{
			name: "Dia completo com 4 pontos",
			day: models.DayRecord{
				Dia: 9, DiaSemana: "Sex",
				Entrada1: "09:12", Saida1: "12:41", Entrada2: "13:44", Saida2: "18:46",
			},
			expected: models.DayTypeCompleto,
		},
		{
			name: "Dia parcial com 1 ponto",
			day: models.DayRecord{
				Dia: 12, DiaSemana: "Seg", Entrada1: "08:02", Saldo: "-08:00",
			},
			expected: models.DayTypeParcial,
		},
		{
			name: "Dispensa para curso",
			day: models.DayRecord{
				Dia: 23, DiaSemana: "Sex", Entrada1: "09:20", Saida1: "12:53",
				Motivo: "DISPENSA PARA FREQUÊNCIA A CURSO DE DOUTORADO",
			},
			expected: models.DayTypeDispensa,
		},
		{
			name: "Recesso",
			day: models.DayRecord{
				Dia: 5, DiaSemana: "Seg", Ocorrencia: "08:00", Motivo: "RECESSO (OCOR.)",
			},
			expected: models.DayTypeRecesso,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.ClassifyDay(&tt.day)
			if got != tt.expected {
				t.Errorf("ClassifyDay() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestValidateDay(t *testing.T) {
	e := NewEngineWithDefaults()

	// Dia válido — 8h15m, almoço 1h03m
	valid := models.DayRecord{
		Entrada1: "09:12", Saida1: "12:41", Entrada2: "13:44", Saida2: "18:46",
	}
	errs := e.ValidateDay(&valid)
	if len(errs) != 0 {
		t.Errorf("ValidateDay(valid) returned errors: %v", errs)
	}

	// Dia com almoço curto (30min)
	shortLunch := models.DayRecord{
		Entrada1: "08:00", Saida1: "12:00", Entrada2: "12:30", Saida2: "16:30",
	}
	errs = e.ValidateDay(&shortLunch)
	if len(errs) == 0 {
		t.Error("ValidateDay(shortLunch) should return error for lunch < 1h")
	}

	// Dia incompleto — não deve validar
	incomplete := models.DayRecord{
		Entrada1: "08:02",
	}
	errs = e.ValidateDay(&incomplete)
	if len(errs) != 0 {
		t.Errorf("ValidateDay(incomplete) should return no errors, got: %v", errs)
	}
}

func TestCalculateDayWorked(t *testing.T) {
	e := NewEngineWithDefaults()

	// Dia 9: 09:12-12:41 (209min) + 13:44-18:46 (302min) = 511min = 8h31m
	day := models.DayRecord{
		Entrada1: "09:12", Saida1: "12:41", Entrada2: "13:44", Saida2: "18:46",
	}
	got := e.CalculateDayWorked(&day)
	if got != 511 {
		t.Errorf("CalculateDayWorked() = %d, want 511", got)
	}
}

func TestCalculateSummary(t *testing.T) {
	e := NewEngineWithDefaults()

	days := []models.DayRecord{
		{Dia: 3, DiaSemana: "Sáb"},                                          // folga
		{Dia: 6, DiaSemana: "Ter", Saldo: "-08:00"},                         // falta
		{Dia: 9, DiaSemana: "Sex", Entrada1: "09:12", Saida1: "12:41",      // completo
			Entrada2: "13:44", Saida2: "18:46", Saldo: "00:00",
			Bloqueio: []int{1, 1, 1, 1}},
	}

	summary := e.CalculateSummary(days)

	if summary.TotalFaltas != 1 {
		t.Errorf("TotalFaltas = %d, want 1", summary.TotalFaltas)
	}
	if summary.DiasCompletos != 1 {
		t.Errorf("DiasCompletos = %d, want 1", summary.DiasCompletos)
	}
}
