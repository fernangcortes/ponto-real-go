package rules

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fernangcortes/ponto-real-go/pkg/models"
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
		{Dia: 3, DiaSemana: "Sáb"},                  // folga
		{Dia: 6, DiaSemana: "Ter", Saldo: "-08:00"}, // falta
		{Dia: 9, DiaSemana: "Sex", Entrada1: "09:12", Saida1: "12:41", // completo
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

// As regras embutidas são carregadas com panic se o JSON for inválido — o que é
// aceitável porque é erro de build, não condição de execução. Este teste é o que
// garante que o erro apareça no CI e não no deploy.
func TestRegrasEmbutidasSaoValidas(t *testing.T) {
	e := NewEngineWithDefaults()

	if e.Config.CargaHorariaDiaria <= 0 {
		t.Errorf("CargaHorariaDiaria = %d; deve ser positiva", e.Config.CargaHorariaDiaria)
	}
	if e.Config.AlmocoMinimo <= 0 {
		t.Errorf("AlmocoMinimo = %d; deve ser positivo", e.Config.AlmocoMinimo)
	}
	if e.Config.AlmocoGeradoMin < e.Config.AlmocoMinimo {
		t.Errorf("AlmocoGeradoMin (%d) não pode ser menor que AlmocoMinimo (%d): "+
			"geraria dia que a própria ValidateDay recusa",
			e.Config.AlmocoGeradoMin, e.Config.AlmocoMinimo)
	}
	if e.Config.AlmocoGeradoMax < e.Config.AlmocoGeradoMin {
		t.Errorf("faixa de almoço invertida: %d..%d", e.Config.AlmocoGeradoMin, e.Config.AlmocoGeradoMax)
	}
	if e.Config.NomeInstituicao == "" {
		t.Error("NomeInstituicao vazio")
	}
}

// Um arquivo externo sobrescreve as embutidas quando pedido explicitamente.
func TestNewEngineLeArquivoExterno(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rules.json")
	conteudo := `{"carga_horaria_diaria_min":360,"almoco_minimo_min":30,
		"almoco_gerado_min_min":30,"almoco_gerado_max_min":45,"nome_instituicao":"OUTRA"}`
	if err := os.WriteFile(path, []byte(conteudo), 0644); err != nil {
		t.Fatal(err)
	}

	e, err := NewEngine(path)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	if e.Config.CargaHorariaDiaria != 360 || e.Config.NomeInstituicao != "OUTRA" {
		t.Errorf("config = %+v; não veio do arquivo externo", e.Config)
	}
}

func TestNewEngineArquivoInexistente(t *testing.T) {
	if _, err := NewEngine(filepath.Join(t.TempDir(), "nao-existe.json")); err == nil {
		t.Error("esperado erro para arquivo inexistente")
	}
}

// Regressão de 72 horas: "Férias" não estava no vocabulário do motor, e a regra
// "dia útil sem batimento = falta" descontava a jornada inteira de cada dia de
// férias homologadas. Num mês com 9 dias, isso somava 72h de débito inexistente.
func TestFeriasNaoSaoFalta(t *testing.T) {
	e := NewEngineWithDefaults()

	dias := []models.DayRecord{
		{Dia: 13, DiaSemana: "Seg", Motivo: "Férias - Estatutário"},
		{Dia: 14, DiaSemana: "Ter", Motivo: "FERIAS"},
	}

	summary := e.CalculateSummary(dias)

	for _, d := range dias {
		if d.Tipo != models.DayTypeFerias {
			t.Errorf("dia %d classificado como %q; esperado ferias", d.Dia, d.Tipo)
		}
	}
	if summary.TotalFaltas != 0 {
		t.Errorf("TotalFaltas = %d; férias homologadas não são falta", summary.TotalFaltas)
	}
	if summary.SaldoTotalRealMinutos != 0 {
		t.Errorf("SaldoTotalRealMinutos = %d; férias não geram déficit", summary.SaldoTotalRealMinutos)
	}
}

// "FERIADO" não pode ser lido como "FERIAS" nem vice-versa.
func TestFeriasNaoSeConfundeComFeriado(t *testing.T) {
	casos := map[string]ObsKind{
		"FÉRIAS":               ObsFerias,
		"Férias - Estatutário": ObsFerias,
		"FERIADO MUNICIPAL":    ObsFeriado,
		"PONTO FACULTATIVO":    ObsPontoFacultativo,
	}

	for texto, esperado := range casos {
		if got := ClassifyObservacao(texto); got != esperado {
			t.Errorf("ClassifyObservacao(%q) = %q; esperado %q", texto, got, esperado)
		}
	}
}

// A jornada de uma dispensa vem do ato que a concedeu e varia caso a caso.
// Antes o motor arbitrava a jornada cheia enquanto o front-end arbitrava metade
// dela — dois números que não vinham de lugar nenhum e discordavam.
func TestDispensaSemCargaInformadaEhNeutra(t *testing.T) {
	e := NewEngineWithDefaults()

	dias := []models.DayRecord{
		{
			Dia: 3, DiaSemana: "Qua", Motivo: "DISPENSA PARA CURSO",
			Entrada1: "08:00", Saida1: "12:00", Entrada2: "13:00", Saida2: "17:00",
			Saldo: "-04:00",
		},
	}

	summary := e.CalculateSummary(dias)

	if summary.SaldoTotalRealMinutos != 0 {
		t.Errorf("SaldoTotalRealMinutos = %d; sem jornada informada o dia é neutro", summary.SaldoTotalRealMinutos)
	}
	if !dias[0].Revisar || dias[0].RevisarMotivo != MsgRevisarCargaDispensa {
		t.Errorf("dia deveria pedir conferência da jornada; Revisar=%v motivo=%q",
			dias[0].Revisar, dias[0].RevisarMotivo)
	}
}

func TestDispensaComCargaInformadaApuraContraEla(t *testing.T) {
	e := NewEngineWithDefaults()

	// 4h trabalhadas contra 2h exigidas pelo ato: sobram 2h.
	dias := []models.DayRecord{
		{
			Dia: 3, DiaSemana: "Qua", Motivo: "DISPENSA PARA CURSO",
			Entrada1: "08:00", Saida1: "10:00", Entrada2: "11:00", Saida2: "13:00",
			CargaEsperada: 120,
		},
	}

	summary := e.CalculateSummary(dias)

	if summary.SaldoTotalRealMinutos != 120 {
		t.Errorf("SaldoTotalRealMinutos = %d; esperado 120", summary.SaldoTotalRealMinutos)
	}
	if dias[0].Revisar {
		t.Error("com a jornada informada o dia não precisa mais de conferência")
	}
}

// O Saldo Oficial é a soma FIEL do que a ficha imprimiu.
//
// Antes o motor recalculava os dias em que o sistema gerou horário: o dia
// impresso como -06:47 (faltou a entrada) entrava como +01:52 depois do
// horário gerado. O número deixava de ser o retrato do problema e contradizia
// o rótulo mostrado na interface, "soma dos saldos originais da imagem".
func TestSaldoOficialNaoRecalculaDiaAjustado(t *testing.T) {
	e := NewEngineWithDefaults()

	dias := []models.DayRecord{
		{
			Dia: 1, DiaSemana: "Seg",
			Entrada1: "09:11", Saida1: "11:30", Entrada2: "12:33", Saida2: "20:06",
			// A entrada foi gerada pelo sistema; as demais vieram da ficha.
			Bloqueio: []int{0, 1, 1, 1},
			Saldo:    "-06:47",
		},
	}

	summary := e.CalculateSummary(dias)

	if got := summary.SaldoTotalMinutos; got != -407 {
		t.Errorf("SaldoTotalMinutos = %d (%s); esperado -407 (-06:47), o que a ficha imprimiu",
			got, MinutesToTime(got))
	}
	// O saldo real, esse sim, reflete o horário gerado: 09:52 trabalhadas.
	if got := summary.SaldoTotalRealMinutos; got != 112 {
		t.Errorf("SaldoTotalRealMinutos = %d; esperado 112 (+01:52)", got)
	}
	if summary.DiasAjustados != 1 {
		t.Errorf("DiasAjustados = %d; esperado 1", summary.DiasAjustados)
	}
}

// Todo dia entra no Saldo Oficial, inclusive os neutros que o laço encerra cedo.
func TestSaldoOficialIncluiDiaNeutro(t *testing.T) {
	e := NewEngineWithDefaults()

	dias := []models.DayRecord{
		// Dispensa sem jornada informada: neutra no saldo real, mas a ficha
		// imprimiu -04:00 e isso continua valendo no total oficial.
		{
			Dia: 3, DiaSemana: "Qua", Motivo: "DISPENSA PARA CURSO",
			Entrada1: "08:00", Saida1: "12:00", Saldo: "-04:00",
		},
	}

	summary := e.CalculateSummary(dias)

	if summary.SaldoTotalMinutos != -240 {
		t.Errorf("SaldoTotalMinutos = %d; esperado -240 (o que a ficha imprimiu)", summary.SaldoTotalMinutos)
	}
	if summary.SaldoTotalRealMinutos != 0 {
		t.Errorf("SaldoTotalRealMinutos = %d; sem jornada informada o dia é neutro", summary.SaldoTotalRealMinutos)
	}
}

// A escolha manual do usuário no seletor da linha prevalece sobre a leitura da
// observação. Antes o DayTypeOverride vivia só em MonthDayRecord, então o
// /api/process não o enxergava e reclassificava por conta própria.
func TestEscolhaManualPrevaleceSobreObservacao(t *testing.T) {
	e := NewEngineWithDefaults()

	casos := map[string]models.DayType{
		"fds":        models.DayTypeFolga,
		"folga":      models.DayTypeFolga,
		"feriado":    models.DayTypeFeriado,
		"convocacao": models.DayTypeRecesso,
		"dispensa":   models.DayTypeDispensa,
		"reduzido":   models.DayTypeExpedienteReduzido,
		"ferias":     models.DayTypeFerias,
	}

	for escolha, esperado := range casos {
		t.Run(escolha, func(t *testing.T) {
			// Dia completo e sem observação nenhuma: só a escolha manual decide.
			d := models.DayRecord{
				Dia: 9, DiaSemana: "Qua",
				Entrada1: "08:00", Saida1: "12:00", Entrada2: "13:00", Saida2: "17:00",
				DayTypeOverride: escolha,
			}
			if got := e.ClassifyDay(&d); got != esperado {
				t.Errorf("ClassifyDay() = %q; esperado %q", got, esperado)
			}
		})
	}
}

// "Útil" derruba a detecção: é como o usuário desfaz uma leitura equivocada.
func TestEscolherUtilDerrubaADeteccao(t *testing.T) {
	e := NewEngineWithDefaults()

	d := models.DayRecord{
		Dia: 9, DiaSemana: "Qua", Motivo: "DISPENSA PARA CURSO",
		Entrada1: "08:00", Saida1: "12:00", Entrada2: "13:00", Saida2: "17:00",
	}

	if got := e.ClassifyDay(&d); got != models.DayTypeDispensa {
		t.Fatalf("sem escolha manual = %q; esperado dispensa", got)
	}

	d.DayTypeOverride = "util"
	if got := e.ClassifyDay(&d); got != models.DayTypeCompleto {
		t.Errorf("com \"util\" explícito = %q; esperado completo", got)
	}
}

// "Útil" num sábado também vale: o usuário está dizendo que trabalhou.
func TestUtilExplicitoVenceOFimDeSemana(t *testing.T) {
	e := NewEngineWithDefaults()

	d := models.DayRecord{
		Dia: 6, DiaSemana: "Sáb",
		Entrada1: "08:00", Saida1: "12:00", Entrada2: "13:00", Saida2: "17:00",
		DayTypeOverride: "util",
	}

	if got := e.ClassifyDay(&d); got != models.DayTypeCompleto {
		t.Errorf("ClassifyDay() = %q; esperado completo", got)
	}
}
