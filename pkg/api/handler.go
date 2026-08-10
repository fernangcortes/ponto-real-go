package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/fernangcortes/ponto-real-go/pkg/apperr"
	"github.com/fernangcortes/ponto-real-go/pkg/extraction"
	"github.com/fernangcortes/ponto-real-go/pkg/models"
	"github.com/fernangcortes/ponto-real-go/pkg/repository"
	"github.com/fernangcortes/ponto-real-go/pkg/service"
	"github.com/fernangcortes/ponto-real-go/pkg/version"
)

// uploadMaxBytes é o teto do arquivo enviado (10 MB).
const uploadMaxBytes = 10 * 1024 * 1024

// Handler contém os handlers HTTP da API.
//
// A responsabilidade aqui é só de transporte: ler a requisição, chamar o
// serviço, traduzir o resultado em status e JSON. Escolha de modelo, validação
// de tipo de arquivo e checagem de chave são regra de aplicação e vivem em
// pkg/service.
type Handler struct {
	service        *service.TimesheetService
	settings       *settingsStore
	justificativas *justificativasStore
}

// NewHandler cria um novo Handler com as dependências injetadas.
func NewHandler(service *service.TimesheetService, settingsRepo repository.SettingsRepository,
	justificativasRepo repository.JustificativasRepository) *Handler {
	return &Handler{
		service:        service,
		settings:       newSettingsStore(settingsRepo),
		justificativas: newJustificativasStore(justificativasRepo),
	}
}

// RegisterRoutes registra todas as rotas da API no mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/health", h.Health)
	mux.HandleFunc("GET /api/rules", h.GetRules)
	mux.HandleFunc("GET /api/models", h.GetModels)
	mux.HandleFunc("GET /api/settings", h.GetSettings)
	mux.HandleFunc("POST /api/settings", h.SaveSettings)
	mux.HandleFunc("POST /api/process", h.Process)
	mux.HandleFunc("POST /api/validate", h.Validate)
	mux.HandleFunc("POST /api/upload", h.Upload)
	// Persistência por mês
	mux.HandleFunc("GET /api/months", h.ListMonths)
	mux.HandleFunc("GET /api/month/{mesAno}", h.LoadMonth)
	mux.HandleFunc("POST /api/month/{mesAno}", h.SaveMonth)
	// Biblioteca de frases de justificativa, compartilhada entre os meses
	mux.HandleFunc("GET /api/justificativas", h.GetJustificativas)
	mux.HandleFunc("POST /api/justificativas", h.SaveJustificativas)
}

// decodeBody lê o corpo JSON da requisição.
func decodeBody(r *http.Request, destino any) error {
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(destino); err != nil {
		return fmt.Errorf("%w: JSON malformado: %v", apperr.ErrRequisicaoInvalida, err)
	}
	return nil
}

// Health retorna status do servidor.
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	_, apiKey := h.settings.ChaveDoProvedorAtivo()
	writeJSON(w, http.StatusOK, map[string]any{
		"status":       "ok",
		"version":      version.Atual,
		"name":         "Ponto Real Go",
		"gemini_ready": apiKey != "",
	})
}

// GetSettings retorna as configurações salvas (mascarando as chaves).
func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	atual := h.settings.Get()
	writeJSON(w, http.StatusOK, map[string]any{
		"provider":              atual.Provider,
		"has_gemini_key":        atual.GeminiAPIKey != "",
		"masked_gemini_key":     mascarar(atual.GeminiAPIKey),
		"has_openrouter_key":    atual.OpenRouterAPIKey != "",
		"masked_openrouter_key": mascarar(atual.OpenRouterAPIKey),
	})
}

// mascarar mostra só as pontas da chave, o suficiente para o usuário reconhecer
// qual configurou sem que a chave inteira trafegue de volta.
func mascarar(chave string) string {
	if chave == "" {
		return ""
	}
	if len(chave) > 8 {
		return chave[:4] + "..." + chave[len(chave)-4:]
	}
	return "****"
}

// SaveSettings salva a configuração usando o repositório de configurações.
func (h *Handler) SaveSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider         string `json:"provider"`
		GeminiAPIKey     string `json:"gemini_api_key"`
		OpenRouterAPIKey string `json:"open_router_api_key"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeError(w, err)
		return
	}

	if err := h.settings.Atualizar(req.Provider, req.GeminiAPIKey, req.OpenRouterAPIKey); err != nil {
		writeError(w, fmt.Errorf("erro ao salvar configurações: %w", err))
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "Configurações salvas com sucesso!",
	})
}

// GetModels retorna os modelos disponíveis para extração.
func (h *Handler) GetModels(w http.ResponseWriter, r *http.Request) {
	atual := h.settings.Get()
	writeJSON(w, http.StatusOK, map[string]any{
		"provider":          atual.Provider,
		"gemini_models":     extraction.GeminiModels(),
		"openrouter_models": extraction.OpenRouterModels(),
		"has_gemini_key":    atual.GeminiAPIKey != "",
		"has_or_key":        atual.OpenRouterAPIKey != "",
	})
}

// Upload recebe um PDF/PNG via multipart e devolve o ProcessResponse.
func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, uploadMaxBytes)

	if err := r.ParseMultipartForm(uploadMaxBytes); err != nil {
		writeError(w, fmt.Errorf("%w: arquivo muito grande ou formato inválido (máximo 10MB)",
			apperr.ErrRequisicaoInvalida))
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, fmt.Errorf("%w: campo 'file' não encontrado no upload", apperr.ErrRequisicaoInvalida))
		return
	}
	defer file.Close()

	conteudo, err := io.ReadAll(file)
	if err != nil {
		writeError(w, fmt.Errorf("erro ao ler o arquivo enviado: %w", err))
		return
	}

	provider, apiKey := h.settings.ChaveDoProvedorAtivo()

	// O contexto da requisição cancela a chamada ao provedor se o cliente sumir.
	resp, err := h.service.ProcessUpload(r.Context(), service.UploadRequest{
		Arquivo:  conteudo,
		MimeType: header.Header.Get("Content-Type"),
		Filename: header.Filename,
		Modelo:   r.FormValue("model"),
		Provider: provider,
		APIKey:   apiKey,
	})
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// GetRules retorna as regras de cálculo atuais.
func (h *Handler) GetRules(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.service.GetEngineConfig())
}

// Process recebe dias de uma folha, classifica e devolve o resumo.
func (h *Handler) Process(w http.ResponseWriter, r *http.Request) {
	var req models.ProcessRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, err)
		return
	}

	resp, err := h.service.Process(req)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// ValidateRequest é o payload para validação de um dia.
type ValidateRequest struct {
	Day models.DayRecord `json:"day"`
}

// Validate verifica se os horários de um dia são válidos e retorna cálculos.
func (h *Handler) Validate(w http.ResponseWriter, r *http.Request) {
	var req ValidateRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, h.service.Validate(req.Day))
}

// GetJustificativas retorna a biblioteca de frases do usuário.
func (h *Handler) GetJustificativas(w http.ResponseWriter, r *http.Request) {
	biblioteca := h.justificativas.Get()
	// Nunca devolver null: o front-end itera direto sobre a lista.
	if biblioteca.Frases == nil {
		biblioteca.Frases = []models.Justificativa{}
	}
	writeJSON(w, http.StatusOK, biblioteca)
}

// SaveJustificativas substitui a biblioteca inteira pela lista recebida.
//
// O corpo traz o estado completo, e não uma frase a acrescentar: é o que
// permite que apagar funcione. Uma rota de "adicionar" precisaria de outra de
// "remover" e de identificador por frase, sem ganhar nada — a biblioteca é
// pequena e o cliente já a tem inteira na mão.
func (h *Handler) SaveJustificativas(w http.ResponseWriter, r *http.Request) {
	var req models.BibliotecaJustificativas
	if err := decodeBody(r, &req); err != nil {
		writeError(w, err)
		return
	}

	biblioteca, err := h.justificativas.Substituir(req.Frases)
	if err != nil {
		writeError(w, fmt.Errorf("erro ao salvar as justificativas: %w", err))
		return
	}

	if biblioteca.Frases == nil {
		biblioteca.Frases = []models.Justificativa{}
	}
	writeJSON(w, http.StatusOK, biblioteca)
}

// ListMonths retorna todos os meses salvos.
func (h *Handler) ListMonths(w http.ResponseWriter, r *http.Request) {
	months, err := h.service.ListMonths()
	if err != nil {
		writeError(w, fmt.Errorf("erro ao listar meses: %w", err))
		return
	}
	// Nunca devolver null: o front-end itera direto sobre a resposta.
	if months == nil {
		months = []models.MonthSummary{}
	}
	writeJSON(w, http.StatusOK, months)
}

// mesAnoDaURL lê o mês do caminho, onde a barra vira underscore por não poder
// aparecer dentro de um segmento de path.
func mesAnoDaURL(r *http.Request) (string, error) {
	mesAno := strings.ReplaceAll(r.PathValue("mesAno"), "_", "/")
	if mesAno == "" {
		return "", fmt.Errorf("%w: mês/ano obrigatório no caminho", apperr.ErrRequisicaoInvalida)
	}
	return mesAno, nil
}

// LoadMonth carrega um mês específico.
func (h *Handler) LoadMonth(w http.ResponseWriter, r *http.Request) {
	mesAno, err := mesAnoDaURL(r)
	if err != nil {
		writeError(w, err)
		return
	}

	data, err := h.service.LoadMonth(mesAno)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, data)
}

// SaveMonth salva/atualiza o estado de um mês.
func (h *Handler) SaveMonth(w http.ResponseWriter, r *http.Request) {
	mesAno, err := mesAnoDaURL(r)
	if err != nil {
		writeError(w, err)
		return
	}

	var data models.MonthData
	if err := decodeBody(r, &data); err != nil {
		writeError(w, err)
		return
	}

	// Quem manda é o caminho da URL, não o corpo.
	data.MesAno = mesAno

	if err := h.service.SaveMonth(data); err != nil {
		writeError(w, fmt.Errorf("erro ao salvar o mês %s: %w", mesAno, err))
		return
	}

	slog.Info("mês salvo", "mes_ano", mesAno, "dias", len(data.Dias))
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": fmt.Sprintf("Mês %s salvo com sucesso", mesAno),
	})
}
