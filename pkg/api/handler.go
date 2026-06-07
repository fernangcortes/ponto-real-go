package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/fernangcortes/ponto-real-go/pkg/extraction"
	"github.com/fernangcortes/ponto-real-go/pkg/models"
	"github.com/fernangcortes/ponto-real-go/pkg/repository"
	"github.com/fernangcortes/ponto-real-go/pkg/service"
)

// Handler contém os handlers HTTP da API.
type Handler struct {
	service      *service.TimesheetService
	settingsRepo repository.SettingsRepository
	settings     models.AppSettings
}

// NewHandler cria um novo Handler com as dependências injetadas.
func NewHandler(
	service *service.TimesheetService,
	settingsRepo repository.SettingsRepository,
) *Handler {
	h := &Handler{
		service:      service,
		settingsRepo: settingsRepo,
	}

	// Carregar configurações salvas
	s, err := settingsRepo.Load()
	if err == nil {
		h.settings = s
	}

	// Normalizar provider
	if h.settings.Provider == "" {
		h.settings.Provider = "gemini"
	}

	// Sobrescrever/preencher com Env Vars se estiverem vazias
	if h.settings.GeminiAPIKey == "" {
		h.settings.GeminiAPIKey = os.Getenv("GEMINI_API_KEY")
	}
	if h.settings.OpenRouterAPIKey == "" {
		h.settings.OpenRouterAPIKey = os.Getenv("OPENROUTER_API_KEY")
	}

	return h
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
}

// Health retorna status do servidor.
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	hasKey := false
	if h.settings.Provider == "openrouter" {
		hasKey = h.settings.OpenRouterAPIKey != ""
	} else {
		hasKey = h.settings.GeminiAPIKey != ""
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":       "ok",
		"version":      "0.3.0",
		"name":         "Ponto Real Go",
		"gemini_ready": hasKey,
	})
}

// GetSettings retorna as configurações salvas (mascarando as chaves).
func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	maskKey := func(key string) string {
		if key == "" {
			return ""
		}
		if len(key) > 8 {
			return key[:4] + "..." + key[len(key)-4:]
		}
		return "****"
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"provider":              h.settings.Provider,
		"has_gemini_key":        h.settings.GeminiAPIKey != "",
		"masked_gemini_key":     maskKey(h.settings.GeminiAPIKey),
		"has_openrouter_key":    h.settings.OpenRouterAPIKey != "",
		"masked_openrouter_key": maskKey(h.settings.OpenRouterAPIKey),
	})
}

// SaveSettings salva a configuração usando o repositório de configurações.
func (h *Handler) SaveSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider         string `json:"provider"`
		GeminiAPIKey     string `json:"gemini_api_key"`
		OpenRouterAPIKey string `json:"open_router_api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "JSON inválido"})
		return
	}
	defer r.Body.Close()

	provider := strings.TrimSpace(req.Provider)
	if provider == "" {
		provider = "gemini"
	}

	// Atualizar em memória mantendo chaves antigas se vierem vazias (placeholders)
	geminiKey := strings.TrimSpace(req.GeminiAPIKey)
	if geminiKey != "" {
		h.settings.GeminiAPIKey = geminiKey
	}
	orKey := strings.TrimSpace(req.OpenRouterAPIKey)
	if orKey != "" {
		h.settings.OpenRouterAPIKey = orKey
	}
	h.settings.Provider = provider

	// Salvar no arquivo (se não estiver no Vercel)
	if os.Getenv("VERCEL") != "1" {
		if err := h.settingsRepo.Save(h.settings); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Erro ao salvar settings.json: " + err.Error()})
			return
		}
		fmt.Println("[API] Configurações salvas")
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"message": "Configurações salvas com sucesso!",
	})
}

// GetModels retorna os modelos disponíveis para extração.
func (h *Handler) GetModels(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"provider":          h.settings.Provider,
		"gemini_models":     extraction.GeminiModels(),
		"openrouter_models": extraction.OpenRouterModels(),
		"has_gemini_key":    h.settings.GeminiAPIKey != "",
		"has_or_key":        h.settings.OpenRouterAPIKey != "",
	})
}

// Upload recebe um PDF/PNG via multipart, delega para o serviço de extração e retorna o ProcessResponse.
func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	// Validar chaves com base no provedor
	var apiKey string
	if h.settings.Provider == "openrouter" {
		apiKey = h.settings.OpenRouterAPIKey
		if apiKey == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "Chave da API OpenRouter não configurada. Clique no ⚙️ no topo para configurá-la.",
			})
			return
		}
	} else {
		apiKey = h.settings.GeminiAPIKey
		if apiKey == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "Chave da API Gemini não configurada. Clique no ⚙️ no topo para configurá-la.",
			})
			return
		}
	}

	// Limitar upload a 10MB
	r.Body = http.MaxBytesReader(w, r.Body, 10*1024*1024)

	// Parse multipart
	if err := r.ParseMultipartForm(10 * 1024 * 1024); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Arquivo muito grande ou formato inválido. Máximo: 10MB.",
		})
		return
	}

	// Lê o arquivo
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Campo 'file' não encontrado no upload.",
		})
		return
	}
	defer file.Close()

	fileBytes, err := io.ReadAll(file)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Erro ao ler arquivo.",
		})
		return
	}

	// Detectar MIME type
	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = detectMimeType(header.Filename, fileBytes)
	}

	// Validar MIME type
	allowedTypes := map[string]bool{
		"image/png": true, "image/jpeg": true, "image/webp": true,
		"application/pdf": true,
	}
	if !allowedTypes[mimeType] {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("Tipo de arquivo não suportado: %s. Use PNG, JPEG, WebP ou PDF.", mimeType),
		})
		return
	}

	// Modelo selecionado pelo usuário
	model := r.FormValue("model")
	if model == "" {
		if h.settings.Provider == "openrouter" {
			model = "google/gemini-2.5-flash"
		} else {
			model = "gemini-2.5-flash"
		}
	}

	// Validar modelo com base no provedor
	if h.settings.Provider == "openrouter" {
		validModels := map[string]bool{
			"google/gemini-2.5-flash":            true,
			"google/gemini-2.0-flash-001":        true,
			"openai/gpt-4o-mini":                  true,
			"qwen/qwen2.5-vl-72b-instruct":        true,
			"meta/llama-3.2-11b-vision-instruct": true,
		}
		if !validModels[model] {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": fmt.Sprintf("Modelo OpenRouter inválido: %s", model),
			})
			return
		}
	} else {
		validModels := map[string]bool{
			"gemini-2.5-flash":              true,
			"gemini-3.1-flash-lite-preview": true,
			"gemini-3.1-pro-preview":        true,
		}
		if !validModels[model] {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": fmt.Sprintf("Modelo Gemini inválido: %s", model),
			})
			return
		}
	}

	fmt.Printf("[API] Upload: %s (%s, %d bytes) provedor: %s, modelo: %s\n", header.Filename, mimeType, len(fileBytes), h.settings.Provider, model)

	// Chamar o caso de uso de processamento de folha de ponto
	resp, err := h.service.ProcessUpload(fileBytes, mimeType, header.Filename, model, h.settings.Provider, apiKey)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// GetRules retorna as regras de cálculo atuais.
func (h *Handler) GetRules(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.service.GetEngineConfig())
}

// Process recebe dias de uma folha de ponto, delega classificação/resumo para o serviço e retorna ProcessResponse.
func (h *Handler) Process(w http.ResponseWriter, r *http.Request) {
	var req models.ProcessRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "JSON inválido: " + err.Error(),
		})
		return
	}
	defer r.Body.Close()

	resp, err := h.service.Process(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "JSON inválido: " + err.Error(),
		})
		return
	}
	defer r.Body.Close()

	resp := h.service.Validate(req.Day)
	writeJSON(w, http.StatusOK, resp)
}

// ListMonths retorna todos os meses salvos.
func (h *Handler) ListMonths(w http.ResponseWriter, r *http.Request) {
	months, err := h.service.ListMonths()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if months == nil {
		months = []models.MonthSummary{}
	}
	writeJSON(w, http.StatusOK, months)
}

// LoadMonth carrega um mês específico.
func (h *Handler) LoadMonth(w http.ResponseWriter, r *http.Request) {
	mesAno := r.PathValue("mesAno")
	if mesAno == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "mes_ano obrigatório"})
		return
	}

	// Converter "02_2026" para "02/2026"
	mesAno = strings.ReplaceAll(mesAno, "_", "/")

	data, err := h.service.LoadMonth(mesAno)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Mês não encontrado: " + mesAno})
		return
	}

	writeJSON(w, http.StatusOK, data)
}

// SaveMonth salva/atualiza o estado de um mês.
func (h *Handler) SaveMonth(w http.ResponseWriter, r *http.Request) {
	mesAno := r.PathValue("mesAno")
	if mesAno == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "mes_ano obrigatório"})
		return
	}

	// Converter "02_2026" para "02/2026"
	mesAno = strings.ReplaceAll(mesAno, "_", "/")

	var data models.MonthData
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "JSON inválido: " + err.Error()})
		return
	}
	defer r.Body.Close()

	data.MesAno = mesAno

	if err := h.service.SaveMonth(data); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Erro ao salvar: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"message": fmt.Sprintf("Mês %s salvo com sucesso", mesAno),
	})
}

// writeJSON escreve uma resposta JSON.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// detectMimeType detecta o tipo MIME do arquivo pelo nome ou magic bytes.
func detectMimeType(filename string, data []byte) string {
	lower := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(lower, ".png"):
		return "image/png"
	case strings.HasSuffix(lower, ".jpg"), strings.HasSuffix(lower, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(lower, ".webp"):
		return "image/webp"
	case strings.HasSuffix(lower, ".pdf"):
		return "application/pdf"
	}
	if len(data) >= 4 {
		if data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
			return "image/png"
		}
		if data[0] == 0xFF && data[1] == 0xD8 {
			return "image/jpeg"
		}
		if data[0] == 0x25 && data[1] == 0x50 && data[2] == 0x44 && data[3] == 0x46 {
			return "application/pdf"
		}
	}
	return "application/octet-stream"
}
