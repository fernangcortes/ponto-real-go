package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fernangcortes/ponto-real-go/pkg/apperr"
	"github.com/fernangcortes/ponto-real-go/pkg/extraction"
	"github.com/fernangcortes/ponto-real-go/pkg/models"
	"github.com/fernangcortes/ponto-real-go/pkg/repository"
	"github.com/fernangcortes/ponto-real-go/pkg/rules"
	"github.com/fernangcortes/ponto-real-go/pkg/service"
)

// Testes de caracterização da camada HTTP: descrevem o comportamento ATUAL,
// incluindo o que é discutível. Onde o teste trava um defeito conhecido, o
// comentário diz qual é e em que fase do plano ele deve mudar.

// --- Dublês ---

type fakeExtractor struct {
	ts     *models.Timesheet
	err    error
	gotCtx context.Context
}

func (f *fakeExtractor) Extract(ctx context.Context, fileBytes []byte, mimeType string) (*models.Timesheet, error) {
	f.gotCtx = ctx
	if f.err != nil {
		return nil, f.err
	}
	// Devolve uma cópia: o serviço muta o resultado, e um dublê compartilhado
	// entre subtestes não pode acumular essas mutações.
	cp := *f.ts
	cp.Dias = append([]models.DayRecord(nil), f.ts.Dias...)
	return &cp, nil
}

type fakeFactory struct {
	ext          extraction.Extractor
	err          error
	gotProvider  string
	gotAPIKey    string
	gotModel     string
	createCalled bool
}

func (f *fakeFactory) Create(provider, apiKey, model string) (extraction.Extractor, error) {
	f.createCalled = true
	f.gotProvider, f.gotAPIKey, f.gotModel = provider, apiKey, model
	if f.err != nil {
		return nil, f.err
	}
	return f.ext, nil
}

type fakeRepo struct {
	dados    map[string]*models.MonthData
	salvos   []models.MonthData
	listErr  error
	listVals []models.MonthSummary
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{dados: map[string]*models.MonthData{}}
}

func (r *fakeRepo) Save(data models.MonthData) error {
	r.salvos = append(r.salvos, data)
	cp := data
	r.dados[data.MesAno] = &cp
	return nil
}

func (r *fakeRepo) Load(mesAno string) (*models.MonthData, error) {
	d, ok := r.dados[mesAno]
	if !ok {
		// Precisa envolver a sentinela como o repositório real, senão o handler
		// não tem como devolver 404.
		return nil, fmt.Errorf("%w: %s", apperr.ErrMesNaoEncontrado, mesAno)
	}
	return d, nil
}

func (r *fakeRepo) List() ([]models.MonthSummary, error) {
	return r.listVals, r.listErr
}

// fakeSettingsRepo é acessado por várias goroutines em
// TestSettingsConcorrentesNaoDaoRace, então precisa do próprio lock — senão o
// -race acusaria o dublê em vez do código sob teste.
type fakeSettingsRepo struct {
	mu       sync.Mutex
	settings models.AppSettings
	loadErr  error
	salvos   []models.AppSettings
	saveErr  error
}

func (s *fakeSettingsRepo) Load() (models.AppSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settings, s.loadErr
}

func (s *fakeSettingsRepo) Save(settings models.AppSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.saveErr != nil {
		return s.saveErr
	}
	s.salvos = append(s.salvos, settings)
	s.settings = settings
	return nil
}

// ultimo devolve o estado corrente sob lock.
func (s *fakeSettingsRepo) ultimo() models.AppSettings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settings
}

// gravacoes devolve quantas vezes Save foi chamado.
func (s *fakeSettingsRepo) gravacoes() []models.AppSettings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]models.AppSettings(nil), s.salvos...)
}

// --- Montagem ---

// fakeJustificativasRepo guarda a biblioteca em memória. Tem lock pelo mesmo
// motivo do fakeSettingsRepo: o store é lido e escrito por goroutines
// diferentes, e um dublê sem sincronização faria o -race acusar o teste.
type fakeJustificativasRepo struct {
	mu      sync.Mutex
	atual   models.BibliotecaJustificativas
	saveErr error
}

func (j *fakeJustificativasRepo) Load() (models.BibliotecaJustificativas, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.atual, nil
}

func (j *fakeJustificativasRepo) Save(b models.BibliotecaJustificativas) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.saveErr != nil {
		return j.saveErr
	}
	b.Frases = repository.NormalizarFrases(b.Frases)
	b.UpdatedAt = time.Now()
	j.atual = b
	return nil
}

type harness struct {
	srv            http.Handler
	repo           *fakeRepo
	settings       *fakeSettingsRepo
	justificativas *fakeJustificativasRepo
	factory        *fakeFactory
}

func newHarness(t *testing.T, settings models.AppSettings, ext extraction.Extractor) *harness {
	t.Helper()
	// NewHandler cai nas variáveis de ambiente quando a chave da settings está
	// vazia; zeramos para o teste não depender da máquina onde roda.
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("OPENROUTER_API_KEY", "")

	repo := newFakeRepo()
	settingsRepo := &fakeSettingsRepo{settings: settings}
	justRepo := &fakeJustificativasRepo{}
	factory := &fakeFactory{ext: ext}

	svc := service.NewTimesheetService(rules.NewEngineWithDefaults(), repo, factory)
	h := NewHandler(svc, settingsRepo, justRepo)

	return &harness{
		srv: BuildHandler(h), repo: repo, settings: settingsRepo,
		justificativas: justRepo, factory: factory,
	}
}

func (h *harness) do(t *testing.T, method, path string, body io.Reader, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, body)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	h.srv.ServeHTTP(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("resposta não é objeto JSON (%d): %s", rec.Code, rec.Body.String())
	}
	return out
}

func exigirStatus(t *testing.T, rec *httptest.ResponseRecorder, esperado int) {
	t.Helper()
	if rec.Code != esperado {
		t.Fatalf("status = %d; esperado %d. Corpo: %s", rec.Code, esperado, rec.Body.String())
	}
}

// pngValido é o cabeçalho mágico que detectMimeType usa para reconhecer PNG.
var pngValido = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

// uploadBody monta um corpo multipart. mimeType vazio faz o campo ir como
// application/octet-stream, forçando o servidor a inferir o tipo.
func uploadBody(t *testing.T, filename string, conteudo []byte, mimeType string, campos map[string]string) (io.Reader, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	hdr := textproto.MIMEHeader{}
	hdr.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, filename))
	if mimeType != "" {
		hdr.Set("Content-Type", mimeType)
	}
	part, err := w.CreatePart(hdr)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(conteudo); err != nil {
		t.Fatal(err)
	}

	for k, v := range campos {
		if err := w.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf, w.FormDataContentType()
}

func folhaExemplo() *models.Timesheet {
	return &models.Timesheet{
		Version:  1,
		MesAno:   "06/2026",
		Servidor: models.ServerInfo{Nome: "FULANO DE TAL"},
		Dias: []models.DayRecord{
			{Dia: 1, DiaSemana: "Seg", Entrada1: "08:02", Saida1: "12:03", Entrada2: "13:05", Saida2: "17:31"},
			{Dia: 2, DiaSemana: "Ter", Entrada1: "08:07", Saida2: "17:22"},
		},
	}
}

// --- Health ---

func TestHealthReportaChaveDoProvedorAtivo(t *testing.T) {
	casos := []struct {
		nome     string
		settings models.AppSettings
		pronto   bool
	}{
		{"gemini com chave", models.AppSettings{Provider: extraction.ProviderGemini, GeminiAPIKey: "k"}, true},
		{"gemini sem chave", models.AppSettings{Provider: extraction.ProviderGemini}, false},
		{"openrouter com chave", models.AppSettings{Provider: extraction.ProviderOpenRouter, OpenRouterAPIKey: "k"}, true},
		// Olha só a chave do provedor ATIVO: chave do outro provedor não conta.
		{"openrouter com chave do gemini", models.AppSettings{Provider: extraction.ProviderOpenRouter, GeminiAPIKey: "k"}, false},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			h := newHarness(t, c.settings, nil)
			rec := h.do(t, "GET", "/api/health", nil, "")
			exigirStatus(t, rec, http.StatusOK)

			body := decode(t, rec)
			if body["status"] != "ok" {
				t.Errorf("status = %v", body["status"])
			}
			if body["gemini_ready"] != c.pronto {
				t.Errorf("gemini_ready = %v; esperado %v", body["gemini_ready"], c.pronto)
			}
		})
	}
}

// Provider vazio vira "gemini" na construção do handler.
func TestProviderVazioViraGemini(t *testing.T) {
	h := newHarness(t, models.AppSettings{GeminiAPIKey: "k"}, nil)

	rec := h.do(t, "GET", "/api/models", nil, "")
	exigirStatus(t, rec, http.StatusOK)

	if p := decode(t, rec)["provider"]; p != "gemini" {
		t.Errorf("provider = %v; esperado gemini", p)
	}
}

// --- Settings ---

func TestGetSettingsNuncaVazaAChaveCrua(t *testing.T) {
	const chave = "sk-or-v1-abcdefghijkl"
	h := newHarness(t, models.AppSettings{Provider: extraction.ProviderOpenRouter, OpenRouterAPIKey: chave}, nil)

	rec := h.do(t, "GET", "/api/settings", nil, "")
	exigirStatus(t, rec, http.StatusOK)

	if strings.Contains(rec.Body.String(), chave) {
		t.Fatalf("a chave crua vazou na resposta: %s", rec.Body.String())
	}

	body := decode(t, rec)
	if body["has_openrouter_key"] != true {
		t.Errorf("has_openrouter_key = %v; esperado true", body["has_openrouter_key"])
	}
	if got := body["masked_openrouter_key"]; got != "sk-o...ijkl" {
		t.Errorf("masked_openrouter_key = %v; esperado sk-o...ijkl", got)
	}
}

func TestGetSettingsMascaraChaveCurta(t *testing.T) {
	h := newHarness(t, models.AppSettings{Provider: extraction.ProviderGemini, GeminiAPIKey: "curta"}, nil)

	rec := h.do(t, "GET", "/api/settings", nil, "")
	exigirStatus(t, rec, http.StatusOK)

	if got := decode(t, rec)["masked_gemini_key"]; got != "****" {
		t.Errorf("masked_gemini_key = %v; esperado ****", got)
	}
}

// Chave vazia no payload significa "não mexi neste campo" — o front-end manda o
// placeholder mascarado de volta, e sobrescrever apagaria a chave real.
func TestSaveSettingsChaveVaziaPreservaAAtual(t *testing.T) {
	h := newHarness(t, models.AppSettings{Provider: extraction.ProviderGemini, GeminiAPIKey: "chave-original"}, nil)

	rec := h.do(t, "POST", "/api/settings",
		strings.NewReader(`{"provider":"gemini","gemini_api_key":""}`), "application/json")
	exigirStatus(t, rec, http.StatusOK)

	if len(h.settings.gravacoes()) != 1 {
		t.Fatalf("gravações = %d; esperado 1", len(h.settings.gravacoes()))
	}
	if got := h.settings.gravacoes()[0].GeminiAPIKey; got != "chave-original" {
		t.Errorf("GeminiAPIKey = %q; a chave existente deveria ter sido preservada", got)
	}
}

func TestSaveSettingsChaveNovaSubstitui(t *testing.T) {
	h := newHarness(t, models.AppSettings{Provider: extraction.ProviderGemini, GeminiAPIKey: "antiga"}, nil)

	rec := h.do(t, "POST", "/api/settings",
		strings.NewReader(`{"provider":"openrouter","open_router_api_key":"  nova  "}`), "application/json")
	exigirStatus(t, rec, http.StatusOK)

	salvo := h.settings.gravacoes()[0]
	if salvo.OpenRouterAPIKey != "nova" {
		t.Errorf("OpenRouterAPIKey = %q; esperado %q (com trim)", salvo.OpenRouterAPIKey, "nova")
	}
	if salvo.Provider != "openrouter" {
		t.Errorf("Provider = %q", salvo.Provider)
	}
	if salvo.GeminiAPIKey != "antiga" {
		t.Errorf("GeminiAPIKey = %q; a chave do outro provedor não deveria sumir", salvo.GeminiAPIKey)
	}
}

func TestSaveSettingsProviderVazioViraGemini(t *testing.T) {
	h := newHarness(t, models.AppSettings{Provider: extraction.ProviderOpenRouter}, nil)

	rec := h.do(t, "POST", "/api/settings", strings.NewReader(`{"provider":"  "}`), "application/json")
	exigirStatus(t, rec, http.StatusOK)

	if got := h.settings.gravacoes()[0].Provider; got != "gemini" {
		t.Errorf("Provider = %q; esperado gemini", got)
	}
}

func TestSaveSettingsJSONInvalido(t *testing.T) {
	h := newHarness(t, models.AppSettings{Provider: extraction.ProviderGemini}, nil)

	rec := h.do(t, "POST", "/api/settings", strings.NewReader(`{quebrado`), "application/json")
	exigirStatus(t, rec, http.StatusBadRequest)

	if len(h.settings.gravacoes()) != 0 {
		t.Error("payload inválido não pode gravar settings")
	}
}

// --- Rules e Models ---

func TestGetRulesDevolveConfigDoMotor(t *testing.T) {
	h := newHarness(t, models.AppSettings{Provider: extraction.ProviderGemini}, nil)

	rec := h.do(t, "GET", "/api/rules", nil, "")
	exigirStatus(t, rec, http.StatusOK)

	body := decode(t, rec)
	if body["carga_horaria_diaria_min"] != float64(480) {
		t.Errorf("carga_horaria_diaria_min = %v; esperado 480", body["carga_horaria_diaria_min"])
	}
	if body["almoco_minimo_min"] != float64(60) {
		t.Errorf("almoco_minimo_min = %v; esperado 60", body["almoco_minimo_min"])
	}
}

func TestGetModelsListaOsDoisProvedores(t *testing.T) {
	h := newHarness(t, models.AppSettings{Provider: extraction.ProviderGemini, GeminiAPIKey: "k"}, nil)

	rec := h.do(t, "GET", "/api/models", nil, "")
	exigirStatus(t, rec, http.StatusOK)

	body := decode(t, rec)
	for _, campo := range []string{"gemini_models", "openrouter_models"} {
		lista, ok := body[campo].([]any)
		if !ok || len(lista) == 0 {
			t.Errorf("%s vazio ou ausente", campo)
		}
	}
	if body["has_gemini_key"] != true {
		t.Errorf("has_gemini_key = %v; esperado true", body["has_gemini_key"])
	}
}

// --- Upload ---

func TestUploadSemChaveRecusa(t *testing.T) {
	for _, provider := range []string{"gemini", "openrouter"} {
		t.Run(provider, func(t *testing.T) {
			h := newHarness(t, models.AppSettings{Provider: provider}, &fakeExtractor{ts: folhaExemplo()})

			body, ct := uploadBody(t, "ponto.png", pngValido, "image/png", nil)
			rec := h.do(t, "POST", "/api/upload", body, ct)
			exigirStatus(t, rec, http.StatusBadRequest)

			if h.factory.createCalled {
				t.Error("sem chave configurada, o extrator não deveria ser construído")
			}
		})
	}
}

func TestUploadTipoNaoSuportado(t *testing.T) {
	h := newHarness(t, models.AppSettings{Provider: extraction.ProviderGemini, GeminiAPIKey: "k"}, &fakeExtractor{ts: folhaExemplo()})

	body, ct := uploadBody(t, "planilha.xlsx", []byte("qualquer coisa"), "application/vnd.ms-excel", nil)
	rec := h.do(t, "POST", "/api/upload", body, ct)
	exigirStatus(t, rec, http.StatusBadRequest)

	if !strings.Contains(decode(t, rec)["error"].(string), "não suportado") {
		t.Errorf("mensagem de erro inesperada: %s", rec.Body.String())
	}
	if h.factory.createCalled {
		t.Error("tipo inválido não pode chegar ao extrator")
	}
}

// Quando o navegador manda application/octet-stream, o tipo é inferido pelo nome
// do arquivo e pelos magic bytes.
func TestUploadInfereMimeQuandoNaoInformado(t *testing.T) {
	h := newHarness(t, models.AppSettings{Provider: extraction.ProviderGemini, GeminiAPIKey: "k"}, &fakeExtractor{ts: folhaExemplo()})

	body, ct := uploadBody(t, "ponto.png", pngValido, "", nil)
	rec := h.do(t, "POST", "/api/upload", body, ct)
	exigirStatus(t, rec, http.StatusOK)

	if !h.factory.createCalled {
		t.Error("PNG inferido deveria ter chegado ao extrator")
	}
}

func TestUploadModeloInvalido(t *testing.T) {
	h := newHarness(t, models.AppSettings{Provider: extraction.ProviderGemini, GeminiAPIKey: "k"}, &fakeExtractor{ts: folhaExemplo()})

	body, ct := uploadBody(t, "ponto.png", pngValido, "image/png",
		map[string]string{"model": "modelo-que-nao-existe"})
	rec := h.do(t, "POST", "/api/upload", body, ct)
	exigirStatus(t, rec, http.StatusBadRequest)

	if h.factory.createCalled {
		t.Error("modelo inválido não pode chegar ao extrator")
	}
}

func TestUploadUsaModeloPadraoPorProvedor(t *testing.T) {
	casos := map[string]string{
		"gemini":     "gemini-2.5-flash",
		"openrouter": "google/gemini-2.5-flash",
	}

	for provider, esperado := range casos {
		t.Run(provider, func(t *testing.T) {
			settings := models.AppSettings{Provider: provider, GeminiAPIKey: "k", OpenRouterAPIKey: "k"}
			h := newHarness(t, settings, &fakeExtractor{ts: folhaExemplo()})

			body, ct := uploadBody(t, "ponto.png", pngValido, "image/png", nil)
			exigirStatus(t, h.do(t, "POST", "/api/upload", body, ct), http.StatusOK)

			if h.factory.gotModel != esperado {
				t.Errorf("modelo = %q; esperado %q", h.factory.gotModel, esperado)
			}
			if h.factory.gotProvider != provider {
				t.Errorf("provedor = %q; esperado %q", h.factory.gotProvider, provider)
			}
		})
	}
}

func TestUploadSucessoProcessaESalvaOMes(t *testing.T) {
	h := newHarness(t, models.AppSettings{Provider: extraction.ProviderGemini, GeminiAPIKey: "chave-secreta"},
		&fakeExtractor{ts: folhaExemplo()})

	body, ct := uploadBody(t, "junho2026.png", pngValido, "image/png", nil)
	rec := h.do(t, "POST", "/api/upload", body, ct)
	exigirStatus(t, rec, http.StatusOK)

	var resp models.ProcessResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("resposta não decodifica em ProcessResponse: %v", err)
	}

	if resp.Timesheet.MesAno != "06/2026" {
		t.Errorf("MesAno = %q", resp.Timesheet.MesAno)
	}
	if len(resp.Timesheet.Dias) != 2 {
		t.Fatalf("len(Dias) = %d; esperado 2", len(resp.Timesheet.Dias))
	}
	// O dia 2 veio com 2 batimentos e deve sair com os 4 preenchidos pelo adjuster.
	d2 := resp.Timesheet.Dias[1]
	if d2.Saida1 == "" || d2.Entrada2 == "" {
		t.Errorf("dia parcial não foi ajustado: %+v", d2)
	}
	if len(d2.Bloqueio) != 4 {
		t.Errorf("Bloqueio = %v; esperado 4 posições marcando original/gerado", d2.Bloqueio)
	}

	if h.factory.gotAPIKey != "chave-secreta" {
		t.Errorf("chave repassada ao factory = %q", h.factory.gotAPIKey)
	}
	if len(h.repo.salvos) != 1 || h.repo.salvos[0].MesAno != "06/2026" {
		t.Errorf("o mês processado deveria ter sido auto-salvo; gravações: %+v", h.repo.salvos)
	}
}

// Erro de credencial é problema que o usuário resolve nas Configurações, então
// precisa chegar como 4xx. Antes toda falha de extração virava 500 e o
// front-end não conseguia distinguir "sua chave está errada" de "o servidor
// quebrou".
func TestUploadChaveInvalidaVira400(t *testing.T) {
	h := newHarness(t, models.AppSettings{Provider: extraction.ProviderGemini, GeminiAPIKey: "k"},
		&fakeExtractor{err: fmt.Errorf("%w — Gemini [401]: API key not valid", apperr.ErrProvedorRecusou)})

	body, ct := uploadBody(t, "ponto.png", pngValido, "image/png", nil)
	rec := h.do(t, "POST", "/api/upload", body, ct)
	exigirStatus(t, rec, http.StatusBadRequest)

	// A mensagem do provedor precisa chegar ao usuário, senão ele não sabe o
	// que corrigir.
	if msg := decode(t, rec)["error"].(string); !strings.Contains(msg, "API key not valid") {
		t.Errorf("mensagem = %q; deveria repassar o motivo do provedor", msg)
	}
}

// Extração que nunca produz folha utilizável não é culpa do usuário nem do
// nosso servidor: 502 aponta para o serviço de fora.
func TestUploadExtracaoInutilizavelVira502(t *testing.T) {
	h := newHarness(t, models.AppSettings{Provider: extraction.ProviderGemini, GeminiAPIKey: "k"},
		&fakeExtractor{err: fmt.Errorf("%w após 3 tentativas", apperr.ErrExtracaoFalhou)})

	body, ct := uploadBody(t, "ponto.png", pngValido, "image/png", nil)
	rec := h.do(t, "POST", "/api/upload", body, ct)
	exigirStatus(t, rec, http.StatusBadGateway)
}

// Falha inesperada continua sendo 500: o padrão só se aplica ao que não foi
// classificado.
func TestUploadErroDesconhecidoVira500(t *testing.T) {
	h := newHarness(t, models.AppSettings{Provider: extraction.ProviderGemini, GeminiAPIKey: "k"},
		&fakeExtractor{err: errors.New("algo totalmente inesperado")})

	body, ct := uploadBody(t, "ponto.png", pngValido, "image/png", nil)
	rec := h.do(t, "POST", "/api/upload", body, ct)
	exigirStatus(t, rec, http.StatusInternalServerError)
}

func TestUploadSemCampoFile(t *testing.T) {
	h := newHarness(t, models.AppSettings{Provider: extraction.ProviderGemini, GeminiAPIKey: "k"}, &fakeExtractor{ts: folhaExemplo()})

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if err := w.WriteField("model", "gemini-2.5-flash"); err != nil {
		t.Fatal(err)
	}
	w.Close()

	rec := h.do(t, "POST", "/api/upload", &buf, w.FormDataContentType())
	exigirStatus(t, rec, http.StatusBadRequest)
}

// --- Process e Validate ---
//
// Estas duas rotas existem, estão roteadas e testadas — e nenhum fetch do
// front-end as chama hoje. São a base da Fase 4 (fonte única de verdade).

func TestProcessSemDiasRecusa(t *testing.T) {
	h := newHarness(t, models.AppSettings{Provider: extraction.ProviderGemini}, nil)

	rec := h.do(t, "POST", "/api/process", strings.NewReader(`{"dias":[]}`), "application/json")
	exigirStatus(t, rec, http.StatusBadRequest)
}

func TestProcessJSONInvalido(t *testing.T) {
	h := newHarness(t, models.AppSettings{Provider: extraction.ProviderGemini}, nil)

	rec := h.do(t, "POST", "/api/process", strings.NewReader(`{quebrado`), "application/json")
	exigirStatus(t, rec, http.StatusBadRequest)
}

func TestProcessClassificaECalculaResumo(t *testing.T) {
	h := newHarness(t, models.AppSettings{Provider: extraction.ProviderGemini}, nil)

	payload := `{"dias":[
		{"d":1,"w":"Seg","e1":"08:00","s1":"12:00","e2":"13:00","s2":"17:00"},
		{"d":6,"w":"Sáb"},
		{"d":8,"w":"Seg"}
	]}`
	rec := h.do(t, "POST", "/api/process", strings.NewReader(payload), "application/json")
	exigirStatus(t, rec, http.StatusOK)

	var resp models.ProcessResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	tipos := []models.DayType{
		models.DayTypeCompleto, // 4 batimentos
		models.DayTypeFolga,    // sábado
		models.DayTypeFalta,    // dia útil sem batimento
	}
	for i, esperado := range tipos {
		if resp.Timesheet.Dias[i].Tipo != esperado {
			t.Errorf("Dias[%d].Tipo = %q; esperado %q", i, resp.Timesheet.Dias[i].Tipo, esperado)
		}
	}

	if resp.Summary.DiasCompletos != 1 {
		t.Errorf("DiasCompletos = %d; esperado 1", resp.Summary.DiasCompletos)
	}
	if resp.Summary.TotalFaltas != 1 {
		t.Errorf("TotalFaltas = %d; esperado 1", resp.Summary.TotalFaltas)
	}
}

func TestValidateDiaCompleto(t *testing.T) {
	h := newHarness(t, models.AppSettings{Provider: extraction.ProviderGemini}, nil)

	payload := `{"day":{"d":1,"w":"Seg","e1":"08:00","s1":"12:00","e2":"13:00","s2":"17:00"}}`
	rec := h.do(t, "POST", "/api/validate", strings.NewReader(payload), "application/json")
	exigirStatus(t, rec, http.StatusOK)

	var resp models.ValidateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Valid {
		t.Errorf("Valid = false; erros: %v", resp.Errors)
	}
	if resp.Worked != "08:00" || resp.Lunch != "01:00" || resp.Balance != "+00:00" {
		t.Errorf("Worked=%q Lunch=%q Balance=%q", resp.Worked, resp.Lunch, resp.Balance)
	}
}

func TestValidateAlmocoCurtoDevolveErro(t *testing.T) {
	h := newHarness(t, models.AppSettings{Provider: extraction.ProviderGemini}, nil)

	payload := `{"day":{"d":1,"w":"Seg","e1":"08:00","s1":"12:00","e2":"12:30","s2":"16:30"}}`
	rec := h.do(t, "POST", "/api/validate", strings.NewReader(payload), "application/json")
	exigirStatus(t, rec, http.StatusOK)

	var resp models.ValidateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Valid || len(resp.Errors) == 0 {
		t.Errorf("almoço de 30min deveria invalidar o dia; resposta: %+v", resp)
	}
}

// --- Persistência por mês ---

// Lista vazia sai como [] e não null, senão o front-end quebra ao iterar.
func TestListMonthsVazioDevolveArrayNaoNull(t *testing.T) {
	h := newHarness(t, models.AppSettings{Provider: extraction.ProviderGemini}, nil)

	rec := h.do(t, "GET", "/api/months", nil, "")
	exigirStatus(t, rec, http.StatusOK)

	if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
		t.Errorf("corpo = %q; esperado []", got)
	}
}

func TestListMonthsErroDoRepositorio(t *testing.T) {
	h := newHarness(t, models.AppSettings{Provider: extraction.ProviderGemini}, nil)
	h.repo.listErr = fmt.Errorf("disco indisponível")

	rec := h.do(t, "GET", "/api/months", nil, "")
	exigirStatus(t, rec, http.StatusInternalServerError)
}

// Na URL o mês vem como 06_2026 porque a barra separaria segmentos de path.
func TestLoadMonthConverteUnderscoreEmBarra(t *testing.T) {
	h := newHarness(t, models.AppSettings{Provider: extraction.ProviderGemini}, nil)
	h.repo.dados["06/2026"] = &models.MonthData{
		MesAno:   "06/2026",
		Servidor: models.ServerInfo{Nome: "FULANO"},
		Dias: []models.MonthDayRecord{
			{DayRecord: models.DayRecord{Dia: 1, DiaSemana: "Seg", Entrada1: "08:00", Saida1: "12:00", Entrada2: "13:00", Saida2: "17:00"}},
		},
	}

	rec := h.do(t, "GET", "/api/month/06_2026", nil, "")
	exigirStatus(t, rec, http.StatusOK)

	var data models.MonthData
	if err := json.Unmarshal(rec.Body.Bytes(), &data); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if data.MesAno != "06/2026" {
		t.Errorf("MesAno = %q", data.MesAno)
	}
	// LoadMonth reavalia os diagnósticos na leitura, então o tipo vem preenchido.
	if data.Dias[0].Tipo != models.DayTypeCompleto {
		t.Errorf("Tipo = %q; LoadMonth deveria reclassificar na leitura", data.Dias[0].Tipo)
	}
}

func TestLoadMonthInexistenteDevolve404(t *testing.T) {
	h := newHarness(t, models.AppSettings{Provider: extraction.ProviderGemini}, nil)

	rec := h.do(t, "GET", "/api/month/01_1999", nil, "")
	exigirStatus(t, rec, http.StatusNotFound)
}

// O MesAno do corpo é ignorado: quem manda é o caminho da URL.
func TestSaveMonthUsaMesAnoDaURL(t *testing.T) {
	h := newHarness(t, models.AppSettings{Provider: extraction.ProviderGemini}, nil)

	payload := `{"mes_ano":"99/9999","servidor":{"nome":"FULANO"},"dias":[{"d":1,"w":"Seg"}]}`
	rec := h.do(t, "POST", "/api/month/06_2026", strings.NewReader(payload), "application/json")
	exigirStatus(t, rec, http.StatusOK)

	if len(h.repo.salvos) != 1 {
		t.Fatalf("gravações = %d; esperado 1", len(h.repo.salvos))
	}
	if got := h.repo.salvos[0].MesAno; got != "06/2026" {
		t.Errorf("MesAno gravado = %q; esperado o da URL (06/2026)", got)
	}
}

func TestSaveMonthJSONInvalido(t *testing.T) {
	h := newHarness(t, models.AppSettings{Provider: extraction.ProviderGemini}, nil)

	rec := h.do(t, "POST", "/api/month/06_2026", strings.NewReader(`{quebrado`), "application/json")
	exigirStatus(t, rec, http.StatusBadRequest)

	if len(h.repo.salvos) != 0 {
		t.Error("payload inválido não pode gravar")
	}
}

// --- Middlewares ---

func TestCORSRespondePreflight(t *testing.T) {
	h := newHarness(t, models.AppSettings{Provider: extraction.ProviderGemini}, nil)

	rec := h.do(t, "OPTIONS", "/api/health", nil, "")
	exigirStatus(t, rec, http.StatusOK)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q; esperado *", got)
	}
}

func TestRespostasSaoJSONUTF8(t *testing.T) {
	h := newHarness(t, models.AppSettings{Provider: extraction.ProviderGemini}, nil)

	rec := h.do(t, "GET", "/api/health", nil, "")
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}
}

// O /api/process passou a receber o mês para corrigir o dia da semana pelo
// calendário real. Sem isso o motor confia no campo "w" lido do documento — e é
// ele que decide se um dia sem batimento é falta ou folga.
func TestProcessUsaOCalendarioRealQuandoRecebeOMes(t *testing.T) {
	h := newHarness(t, models.AppSettings{Provider: extraction.ProviderGemini}, nil)

	// 6 de junho de 2026 é sábado, mas o documento veio dizendo "Seg".
	// Sem batimento algum: pelo campo lido seria falta; pelo calendário, folga.
	payload := `{"mes_ano":"06/2026","dias":[{"d":6,"w":"Seg","saldo":"-08:00"}]}`

	rec := h.do(t, "POST", "/api/process", strings.NewReader(payload), "application/json")
	exigirStatus(t, rec, http.StatusOK)

	var resp models.ProcessResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if got := resp.Timesheet.Dias[0].DiaSemana; got != "Sáb" {
		t.Errorf("DiaSemana = %q; o calendário deveria ter corrigido para Sáb", got)
	}
	if got := resp.Timesheet.Dias[0].Tipo; got != models.DayTypeFolga {
		t.Errorf("Tipo = %q; sábado sem batimento é folga, não falta", got)
	}
	if resp.Summary.TotalFaltas != 0 {
		t.Errorf("TotalFaltas = %d; esperado 0", resp.Summary.TotalFaltas)
	}
}

// Sem o mês informado o comportamento antigo continua valendo: o campo "w" manda.
func TestProcessSemMesConfiaNoCampoLido(t *testing.T) {
	h := newHarness(t, models.AppSettings{Provider: extraction.ProviderGemini}, nil)

	payload := `{"dias":[{"d":6,"w":"Seg","saldo":"-08:00"}]}`
	rec := h.do(t, "POST", "/api/process", strings.NewReader(payload), "application/json")
	exigirStatus(t, rec, http.StatusOK)

	var resp models.ProcessResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := resp.Timesheet.Dias[0].Tipo; got != models.DayTypeFalta {
		t.Errorf("Tipo = %q; sem calendário, segunda sem batimento é falta", got)
	}
}

// A escolha manual do tipo de dia agora atravessa o /api/process.
func TestProcessRespeitaEscolhaManualDoTipoDeDia(t *testing.T) {
	h := newHarness(t, models.AppSettings{Provider: extraction.ProviderGemini}, nil)

	payload := `{"mes_ano":"06/2026","dias":[
		{"d":8,"w":"Seg","mot":"DISPENSA PARA CURSO",
		 "e1":"08:00","s1":"12:00","e2":"13:00","s2":"17:00",
		 "day_type_override":"util"}
	]}`

	rec := h.do(t, "POST", "/api/process", strings.NewReader(payload), "application/json")
	exigirStatus(t, rec, http.StatusOK)

	var resp models.ProcessResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := resp.Timesheet.Dias[0].Tipo; got != models.DayTypeCompleto {
		t.Errorf("Tipo = %q; a escolha \"util\" do usuário deveria derrubar a detecção de dispensa", got)
	}
}

// --- Biblioteca de justificativas ---

func TestJustificativasComecaVaziaENuncaDevolveNull(t *testing.T) {
	h := newHarness(t, models.AppSettings{}, nil)

	rec := h.do(t, "GET", "/api/justificativas", nil, "")
	exigirStatus(t, rec, http.StatusOK)

	// O front-end itera direto sobre a lista: `null` quebraria a montagem do
	// seletor na primeira execução, quando ainda não há nenhuma frase.
	if !strings.Contains(rec.Body.String(), `"frases":[]`) {
		t.Errorf("esperava lista vazia explícita, veio %s", rec.Body.String())
	}
}

func TestJustificativasRoundTripPelaAPI(t *testing.T) {
	h := newHarness(t, models.AppSettings{}, nil)

	payload := `{"frases":[
		{"texto":"Cumpri a jornada de {jornada} exigida pelo ato.","tipo":"dispensa","usos":2},
		{"texto":"Compareci a reuniao externa.","tipo":"util"}
	]}`
	rec := h.do(t, "POST", "/api/justificativas", strings.NewReader(payload), "application/json")
	exigirStatus(t, rec, http.StatusOK)

	rec = h.do(t, "GET", "/api/justificativas", nil, "")
	var b models.BibliotecaJustificativas
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(b.Frases) != 2 {
		t.Fatalf("esperava 2 frases, veio %d: %+v", len(b.Frases), b.Frases)
	}
	if b.Frases[0].Tipo != "dispensa" || b.Frases[0].Usos != 2 {
		t.Errorf("metadados perdidos: %+v", b.Frases[0])
	}
}

// O POST manda o estado completo: é assim que apagar uma frase funciona.
func TestJustificativasPostSubstituiAListaInteira(t *testing.T) {
	h := newHarness(t, models.AppSettings{}, nil)

	h.do(t, "POST", "/api/justificativas",
		strings.NewReader(`{"frases":[{"texto":"A"},{"texto":"B"}]}`), "application/json")
	h.do(t, "POST", "/api/justificativas",
		strings.NewReader(`{"frases":[{"texto":"A"}]}`), "application/json")

	rec := h.do(t, "GET", "/api/justificativas", nil, "")
	var b models.BibliotecaJustificativas
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(b.Frases) != 1 || b.Frases[0].Texto != "A" {
		t.Errorf("a frase excluída voltou: %+v", b.Frases)
	}
}

func TestJustificativasNormalizaAoSalvar(t *testing.T) {
	h := newHarness(t, models.AppSettings{}, nil)

	payload := `{"frases":[
		{"texto":"  Cumpri a jornada.  ","usos":1},
		{"texto":"cumpri a jornada.","usos":2},
		{"texto":"   "}
	]}`
	rec := h.do(t, "POST", "/api/justificativas", strings.NewReader(payload), "application/json")
	exigirStatus(t, rec, http.StatusOK)

	var b models.BibliotecaJustificativas
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(b.Frases) != 1 {
		t.Fatalf("esperava 1 frase após normalizar, veio %d: %+v", len(b.Frases), b.Frases)
	}
	if b.Frases[0].Texto != "Cumpri a jornada." {
		t.Errorf("texto = %q", b.Frases[0].Texto)
	}
	if b.Frases[0].Usos != 3 {
		t.Errorf("usos = %d, esperava a soma das repetidas (3)", b.Frases[0].Usos)
	}
}

func TestJustificativasJSONMalformadoDa400(t *testing.T) {
	h := newHarness(t, models.AppSettings{}, nil)

	rec := h.do(t, "POST", "/api/justificativas", strings.NewReader(`{"frases":`), "application/json")
	exigirStatus(t, rec, http.StatusBadRequest)
}

// Falha de gravação não pode ser reportada como sucesso: o front-end usaria a
// resposta 200 para limpar a fila de pendências e a frase se perderia.
func TestJustificativasErroDeGravacaoVira500(t *testing.T) {
	h := newHarness(t, models.AppSettings{}, nil)
	h.justificativas.saveErr = errors.New("disco cheio")

	rec := h.do(t, "POST", "/api/justificativas",
		strings.NewReader(`{"frases":[{"texto":"A"}]}`), "application/json")
	exigirStatus(t, rec, http.StatusInternalServerError)
}
