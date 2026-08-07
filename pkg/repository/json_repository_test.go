package repository

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/fernangcortes/ponto-real-go/pkg/apperr"
	"github.com/fernangcortes/ponto-real-go/pkg/models"
)

// newRepo devolve um repositório apontando para um diretório temporário.
func newRepo(t *testing.T) *JSONTimesheetRepository {
	t.Helper()
	return NewJSONTimesheetRepository(t.TempDir())
}

func mesExemplo(mesAno, nome string) models.MonthData {
	return models.MonthData{
		MesAno:   mesAno,
		Servidor: models.ServerInfo{Nome: nome},
		Dias: []models.MonthDayRecord{
			{DayRecord: models.DayRecord{Dia: 1, DiaSemana: "Qui", Entrada1: "08:02", Saida1: "12:03", Entrada2: "13:05", Saida2: "17:31"}},
			{DayRecord: models.DayRecord{Dia: 2, DiaSemana: "Sex"}},
		},
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	r := newRepo(t)

	original := mesExemplo("06/2026", "FULANO DE TAL")
	if err := r.Save(original); err != nil {
		t.Fatalf("Save: %v", err)
	}

	lido, err := r.Load("06/2026")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if lido.MesAno != "06/2026" {
		t.Errorf("MesAno = %q; esperado %q", lido.MesAno, "06/2026")
	}
	if lido.Servidor.Nome != "FULANO DE TAL" {
		t.Errorf("Servidor.Nome = %q", lido.Servidor.Nome)
	}
	if len(lido.Dias) != 2 {
		t.Fatalf("len(Dias) = %d; esperado 2", len(lido.Dias))
	}
	if lido.Dias[0].Entrada1 != "08:02" || lido.Dias[0].Saida2 != "17:31" {
		t.Errorf("horários do dia 1 não sobreviveram: %+v", lido.Dias[0].DayRecord)
	}
}

// Save carimba UpdatedAt sozinho, ignorando o que veio no argumento.
func TestSaveCarimbaUpdatedAt(t *testing.T) {
	r := newRepo(t)

	if err := r.Save(mesExemplo("06/2026", "FULANO")); err != nil {
		t.Fatalf("Save: %v", err)
	}

	lido, err := r.Load("06/2026")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if lido.UpdatedAt.IsZero() {
		t.Error("UpdatedAt ficou zerado; Save deveria carimbar o horário da gravação")
	}
}

// A barra do mês vira underscore no nome do arquivo, senão viraria subdiretório.
func TestNomeDoArquivoTrocaBarraPorUnderscore(t *testing.T) {
	dir := t.TempDir()
	r := NewJSONTimesheetRepository(dir)

	if err := r.Save(mesExemplo("06/2026", "FULANO")); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "06_2026.json")); err != nil {
		t.Errorf("esperado arquivo 06_2026.json no diretório de dados: %v", err)
	}
}

func TestSaveSobrescreveMesExistente(t *testing.T) {
	r := newRepo(t)

	if err := r.Save(mesExemplo("06/2026", "PRIMEIRO")); err != nil {
		t.Fatalf("Save 1: %v", err)
	}
	if err := r.Save(mesExemplo("06/2026", "SEGUNDO")); err != nil {
		t.Fatalf("Save 2: %v", err)
	}

	lido, err := r.Load("06/2026")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if lido.Servidor.Nome != "SEGUNDO" {
		t.Errorf("Servidor.Nome = %q; esperado a segunda gravação", lido.Servidor.Nome)
	}

	meses, err := r.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(meses) != 1 {
		t.Errorf("len(List()) = %d; sobrescrever não pode criar um segundo mês", len(meses))
	}
}

func TestLoadMesInexistenteRetornaErro(t *testing.T) {
	r := newRepo(t)

	if _, err := r.Load("01/1999"); err == nil {
		t.Error("Load de mês inexistente deveria retornar erro")
	}
}

// List ordena do mais recente para o mais antigo.
func TestListOrdenaDoMaisRecente(t *testing.T) {
	r := newRepo(t)

	for _, m := range []string{"06/2026", "07/2026", "05/2026"} {
		if err := r.Save(mesExemplo(m, "FULANO")); err != nil {
			t.Fatalf("Save %s: %v", m, err)
		}
	}

	meses, err := r.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(meses) != 3 {
		t.Fatalf("len(List()) = %d; esperado 3", len(meses))
	}

	esperado := []string{"07/2026", "06/2026", "05/2026"}
	for i, e := range esperado {
		if meses[i].MesAno != e {
			t.Errorf("List()[%d].MesAno = %q; esperado %q", i, meses[i].MesAno, e)
		}
	}
}

// Regressão: a ordenação comparava "MM/AAAA" como string, e o mês vindo na
// frente fazia 12/2025 passar por mais recente que 01/2026 — o ano era ignorado
// justamente na virada do ano, que é quando a ordem importa.
func TestListOrdenaPorDataNaViradaDoAno(t *testing.T) {
	r := newRepo(t)

	for _, m := range []string{"01/2026", "12/2025", "02/2026", "11/2025"} {
		if err := r.Save(mesExemplo(m, "FULANO")); err != nil {
			t.Fatalf("Save %s: %v", m, err)
		}
	}

	meses, err := r.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	esperado := []string{"02/2026", "01/2026", "12/2025", "11/2025"}
	for i, e := range esperado {
		if meses[i].MesAno != e {
			t.Fatalf("ordem = %v; esperado %v", nomesDosMeses(meses), esperado)
		}
	}
}

// Mês com formato irreconhecível não pode derrubar a ordenação; vai para o fim.
func TestListMesComFormatoInvalidoVaiParaOFim(t *testing.T) {
	r := newRepo(t)

	for _, m := range []string{"06/2026", "bagunca", "07/2026"} {
		if err := r.Save(mesExemplo(m, "FULANO")); err != nil {
			t.Fatalf("Save %s: %v", m, err)
		}
	}

	meses, err := r.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(meses) != 3 {
		t.Fatalf("len(List()) = %d; esperado 3", len(meses))
	}
	if meses[len(meses)-1].MesAno != "bagunca" {
		t.Errorf("ordem = %v; o inválido deveria ficar por último", nomesDosMeses(meses))
	}
}

func nomesDosMeses(meses []models.MonthSummary) []string {
	out := make([]string, len(meses))
	for i, m := range meses {
		out[i] = m.MesAno
	}
	return out
}

func TestListEmDiretorioVazio(t *testing.T) {
	r := newRepo(t)

	meses, err := r.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(meses) != 0 {
		t.Errorf("len(List()) = %d; esperado 0", len(meses))
	}
}

// Arquivos que não são .json e JSON corrompido são ignorados em silêncio, em vez
// de derrubar a listagem inteira.
func TestListIgnoraArquivosInvalidos(t *testing.T) {
	dir := t.TempDir()
	r := NewJSONTimesheetRepository(dir)

	if err := r.Save(mesExemplo("06/2026", "FULANO")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "leiame.txt"), []byte("nada"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "07_2026.json"), []byte("{quebrado"), 0644); err != nil {
		t.Fatal(err)
	}

	meses, err := r.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(meses) != 1 || meses[0].MesAno != "06/2026" {
		t.Errorf("List() = %+v; esperado apenas o mês válido 06/2026", meses)
	}
}

// O repositório JSON não consulta mais o ambiente: a decisão de ter ou não
// disco durável é de quem monta o grafo, e o comportamento "não guardar nada"
// virou uma implementação explícita.
//
// Antes isso era um os.Getenv("VERCEL") repetido em cinco pontos aqui dentro.
func TestJSONNaoDependeDoAmbiente(t *testing.T) {
	dir := t.TempDir()
	// Mesmo com a variável do Vercel presente, o repositório JSON grava: o que
	// decide é qual implementação foi injetada, não a variável.
	t.Setenv("VERCEL", "1")
	r := NewJSONTimesheetRepository(dir)

	if err := r.Save(mesExemplo("06/2026", "FULANO")); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "06_2026.json")); err != nil {
		t.Errorf("o repositório JSON deveria gravar independentemente do ambiente: %v", err)
	}
}

func TestNoopTimesheetDescartaGravacoes(t *testing.T) {
	r := NoopTimesheetRepository{}

	if err := r.Save(mesExemplo("06/2026", "FULANO")); err != nil {
		t.Errorf("Save no-op não deveria falhar: %v", err)
	}

	_, err := r.Load("06/2026")
	if !errors.Is(err, apperr.ErrMesNaoEncontrado) {
		t.Errorf("Load = %v; esperado ErrMesNaoEncontrado para virar 404 na API", err)
	}

	meses, err := r.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// Vazio e não-nil: o handler devolve [] ao front-end, que itera direto.
	if meses == nil || len(meses) != 0 {
		t.Errorf("List = %#v; esperado slice vazio não-nil", meses)
	}
}

func TestNoopSettingsNaoGuardaNada(t *testing.T) {
	r := NoopSettingsRepository{}

	if err := r.Save(models.AppSettings{Provider: "gemini", GeminiAPIKey: "k"}); err != nil {
		t.Errorf("Save no-op não deveria falhar: %v", err)
	}

	if _, err := r.Load(); !errors.Is(err, ErrSemPersistencia) {
		t.Errorf("Load = %v; esperado ErrSemPersistencia", err)
	}
}

// Mês inexistente precisa ser reconhecível como tal para o handler responder
// 404 em vez de 500.
func TestLoadInexistenteEnvolveSentinela(t *testing.T) {
	r := newRepo(t)

	_, err := r.Load("01/1999")
	if !errors.Is(err, apperr.ErrMesNaoEncontrado) {
		t.Errorf("Load = %v; esperado que envolva ErrMesNaoEncontrado", err)
	}
}

// --- Settings ---

// JSONSettingsRepository resolve o caminho a partir de os.Executable(), não do
// diretório de trabalho. Em teste isso aponta para o diretório temporário do
// binário de teste, que o `go test` descarta no fim — por isso o round-trip
// abaixo é seguro, mas note que a escolha do caminho NÃO é injetável.
//
// Essa rigidez é justamente o que impede um teste mais direto e deve ser
// resolvida quando a composição passar para pkg/app (Fase 1.5).
func TestSettingsRoundTrip(t *testing.T) {
	r := NewJSONSettingsRepository("settings_test_tmp.json")
	t.Cleanup(func() { os.Remove(r.filepath()) })

	original := models.AppSettings{
		Provider:         "openrouter",
		OpenRouterAPIKey: "sk-or-v1-exemplo",
	}
	if err := r.Save(original); err != nil {
		t.Fatalf("Save: %v", err)
	}

	lido, err := r.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if lido.Provider != "openrouter" || lido.OpenRouterAPIKey != "sk-or-v1-exemplo" {
		t.Errorf("settings não sobreviveram ao round-trip: %+v", lido)
	}
}

// O arquivo de settings guarda chave de API e por isso é gravado com 0600.
// Em Windows o modo POSIX não é aplicado, então a verificação só roda onde faz sentido.
func TestSettingsGravadoComPermissaoRestrita(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("permissões POSIX não se aplicam no Windows")
	}
	r := NewJSONSettingsRepository("settings_perm_tmp.json")
	t.Cleanup(func() { os.Remove(r.filepath()) })

	if err := r.Save(models.AppSettings{Provider: "gemini", GeminiAPIKey: "segredo"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(r.filepath())
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("permissão = %o; esperado 600 por conter chave de API", perm)
	}
}

func TestSettingsLoadSemArquivoRetornaErro(t *testing.T) {
	r := NewJSONSettingsRepository("settings_inexistente_tmp.json")

	if _, err := r.Load(); err == nil {
		t.Error("Load sem arquivo deveria retornar erro")
	}
}
