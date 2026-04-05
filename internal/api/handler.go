package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/fernangcortes/ponto-real-go/internal/extraction"
	"github.com/fernangcortes/ponto-real-go/internal/models"
	"github.com/fernangcortes/ponto-real-go/internal/rules"
)

const settingsFile = "settings.json"

// AppSettings contém configurações salvas localmente.
type AppSettings struct {
	GeminiAPIKey string `json:"gemini_api_key"`
}

// Handler contém os handlers HTTP da API.
type Handler struct {
	Engine *rules.Engine
	APIKey string
}

// NewHandler cria um novo Handler com o motor de regras.
func NewHandler(engine *rules.Engine) *Handler {
	h := &Handler{
		Engine: engine,
		APIKey: os.Getenv("GEMINI_API_KEY"),
	}
	// Se não tem env var, tentar carregar do settings.json
	if h.APIKey == "" {
		if s, err := loadSettings(); err == nil && s.GeminiAPIKey != "" {
			h.APIKey = s.GeminiAPIKey
			fmt.Println("[API] Gemini API key carregada de settings.json")
		}
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
	hasKey := h.APIKey != ""
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":       "ok",
		"version":      "0.2.0",
		"name":         "Ponto Real Go",
		"gemini_ready": hasKey,
	})
}

// GetSettings retorna as configurações salvas (mascarando a API key).
func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	masked := ""
	if h.APIKey != "" {
		if len(h.APIKey) > 8 {
			masked = h.APIKey[:4] + "..." + h.APIKey[len(h.APIKey)-4:]
		} else {
			masked = "****"
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"has_key":    h.APIKey != "",
		"masked_key": masked,
	})
}

// SaveSettings salva a API key em settings.json.
func (h *Handler) SaveSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		GeminiAPIKey string `json:"gemini_api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "JSON inválido"})
		return
	}
	defer r.Body.Close()

	key := strings.TrimSpace(req.GeminiAPIKey)
	if key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "API key não pode ser vazia"})
		return
	}

	// Salvar no arquivo
	s := AppSettings{GeminiAPIKey: key}
	if err := saveSettings(s); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Erro ao salvar: " + err.Error()})
		return
	}

	// Atualizar em memória
	h.APIKey = key
	fmt.Println("[API] Gemini API key salva em settings.json")

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"message": "API key salva com sucesso!",
	})
}

// loadSettings carrega settings.json do diretório do executável.
func loadSettings() (AppSettings, error) {
	var s AppSettings
	path := settingsFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		return s, err
	}
	err = json.Unmarshal(data, &s)
	return s, err
}

// saveSettings salva settings.json no diretório do executável.
func saveSettings(s AppSettings) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(settingsFilePath(), data, 0600)
}

// settingsFilePath retorna o caminho do settings.json (mesmo dir do executável ou CWD).
func settingsFilePath() string {
	exe, err := os.Executable()
	if err != nil {
		return settingsFile
	}
	return filepath.Join(filepath.Dir(exe), settingsFile)
}

// GetModels retorna os modelos disponíveis para extração.
func (h *Handler) GetModels(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"models":  extraction.AvailableModels(),
		"has_key": h.APIKey != "",
	})
}

// Upload recebe um PDF/PNG via multipart, extrai dados via Gemini e retorna Timesheet.
func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	if h.APIKey == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "API key não configurada. Clique no ⚙️ no topo para configurar sua chave Gemini.",
		})
		return
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
		model = "gemini-3.1-flash-lite-preview"
	}

	// Validar modelo
	validModels := map[string]bool{
		"gemini-3.1-flash-lite-preview": true,
		"gemini-3.1-pro-preview":        true,
	}
	if !validModels[model] {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("Modelo inválido: %s", model),
		})
		return
	}

	fmt.Printf("[API] Upload: %s (%s, %d bytes) modelo: %s\n", header.Filename, mimeType, len(fileBytes), model)

	// Etapa 1: Extrair dados brutos via Vision
	fmt.Println("[API] Etapa 1/2: Extraindo dados...")
	extractor := extraction.NewGeminiExtractor(h.APIKey, model)
	timesheet, err := extractor.Extract(fileBytes, mimeType)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Erro na extração: " + err.Error(),
		})
		return
	}
	fmt.Printf("[API] Extração: %d dias extraídos\n", len(timesheet.Dias))

	// Etapa 2: Ajustar horários faltantes (lógica determinística)
	fmt.Println("[API] Etapa 2/2: Ajustando horários faltantes...")
	adjuster := extraction.NewRulesAdjuster(h.Engine)
	adjusted := adjuster.Adjust(timesheet)

	fmt.Printf("[API] Ajuste concluído: %d dias processados\n", len(adjusted.Dias))

	// Classificar dias e calcular resumo
	for i := range adjusted.Dias {
		adjusted.Dias[i].Tipo = h.Engine.ClassifyDay(&adjusted.Dias[i])
	}
	summary := h.Engine.CalculateSummary(adjusted.Dias)

	resp := models.ProcessResponse{
		Timesheet: *adjusted,
		Summary:   summary,
	}

	// Auto-save: salvar o mês processado em disco
	if adjusted.MesAno != "" {
		monthDays := make([]MonthDayRecord, len(adjusted.Dias))
		for i, d := range adjusted.Dias {
			monthDays[i] = MonthDayRecord{DayRecord: d}
		}
		monthData := MonthData{
			MesAno:   adjusted.MesAno,
			Servidor: adjusted.Servidor,
			Dias:     monthDays,
		}
		if err := SaveMonth(monthData); err != nil {
			fmt.Printf("[WARN] Erro ao auto-salvar mês: %v\n", err)
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// GetRules retorna as regras de cálculo atuais.
func (h *Handler) GetRules(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.Engine.Config)
}

// Process recebe dias de uma folha de ponto, classifica, calcula saldo e retorna resumo.
func (h *Handler) Process(w http.ResponseWriter, r *http.Request) {
	var req models.ProcessRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "JSON inválido: " + err.Error(),
		})
		return
	}
	defer r.Body.Close()

	if len(req.Dias) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Nenhum dia fornecido",
		})
		return
	}

	// Classificar cada dia
	for i := range req.Dias {
		req.Dias[i].Tipo = h.Engine.ClassifyDay(&req.Dias[i])
	}

	// Calcular resumo
	summary := h.Engine.CalculateSummary(req.Dias)

	resp := models.ProcessResponse{
		Timesheet: models.Timesheet{
			Version: 1,
			Dias:    req.Dias,
		},
		Summary: summary,
	}

	writeJSON(w, http.StatusOK, resp)
}

// ValidateRequest é o payload para validação de um dia.
type ValidateRequest struct {
	Day models.DayRecord `json:"day"`
}

// ValidateResponse é a resposta da validação.
type ValidateResponse struct {
	Valid   bool     `json:"valid"`
	Errors  []string `json:"errors,omitempty"`
	Worked  string   `json:"worked,omitempty"`
	Lunch   string   `json:"lunch,omitempty"`
	Balance string   `json:"balance,omitempty"`
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

	errs := h.Engine.ValidateDay(&req.Day)
	worked := h.Engine.CalculateDayWorked(&req.Day)
	lunch := h.Engine.CalculateLunchDuration(&req.Day)

	resp := ValidateResponse{
		Valid:   len(errs) == 0,
		Errors:  errs,
		Worked:  rules.MinutesToTimeUnsigned(worked),
		Lunch:   rules.MinutesToTimeUnsigned(lunch),
		Balance: rules.MinutesToTime(worked - h.Engine.Config.CargaHorariaDiaria),
	}

	writeJSON(w, http.StatusOK, resp)
}

// --- Persistência por Mês ---

// ListMonths retorna todos os meses salvos.
func (h *Handler) ListMonths(w http.ResponseWriter, r *http.Request) {
	months, err := ListMonths()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if months == nil {
		months = []MonthSummary{}
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

	data, err := LoadMonth(mesAno)
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

	var data MonthData
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "JSON inválido: " + err.Error()})
		return
	}
	defer r.Body.Close()

	data.MesAno = mesAno

	if err := SaveMonth(data); err != nil {
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
	// Primeiro tentar pela extensão
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
	// Fallback: magic bytes
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
