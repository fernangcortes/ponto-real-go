package extraction

import (
	"github.com/fernangcortes/ponto-real-go/pkg/models"
)

// Extractor é a interface para extração de dados de folhas de ponto.
// Permite trocar a implementação (Gemini Vision, Document AI, etc.) sem alterar o resto do código.
type Extractor interface {
	// Extract recebe os bytes do arquivo e seu MIME type, retorna um Timesheet estruturado.
	Extract(fileBytes []byte, mimeType string) (*models.Timesheet, error)
}

// AvailableModels retorna a lista de modelos padrão (backward compatibility).
func AvailableModels() []ModelOption {
	return GeminiModels()
}

// GeminiModels retorna os modelos da API direta do Google Gemini.
func GeminiModels() []ModelOption {
	return []ModelOption{
		{
			ID:          "gemini-2.5-flash",
			Name:        "⚡ Gemini 2.5 Flash",
			Description: "Recomendado. Nova geração com maior precisão e rapidez para leitura de tabelas.",
			Speed:       "Rápido",
		},
		{
			ID:          "gemini-3.1-flash-lite-preview",
			Name:        "⚡ Gemini 3.1 Flash Lite",
			Description: "Mais rápido e econômico. Bom para folhas de ponto com layout padrão.",
			Speed:       "Rápido",
		},
		{
			ID:          "gemini-3.1-pro-preview",
			Name:        "🧠 Gemini 3.1 Pro",
			Description: "Mais preciso. Recomendado para folhas com layout complexo ou baixa qualidade.",
			Speed:       "Moderado",
		},
	}
}

// OpenRouterModels retorna os modelos disponíveis via OpenRouter.
func OpenRouterModels() []ModelOption {
	return []ModelOption{
		{
			ID:          "google/gemini-2.5-flash",
			Name:        "⚡ Gemini 2.5 Flash",
			Description: "Recomendado. Nova geração da família Flash com maior precisão e context window.",
			Speed:       "Rápido",
		},
		{
			ID:          "google/gemini-2.0-flash-001",
			Name:        "⚡ Gemini 2.0 Flash",
			Description: "Excelente custo-benefício. Leitura rápida e precisa de tabelas estruturadas.",
			Speed:       "Rápido",
		},
		{
			ID:          "openai/gpt-4o-mini",
			Name:        "🧠 GPT-4o mini",
			Description: "Econômico e altamente preciso na aderência de esquemas JSON estruturados.",
			Speed:       "Rápido",
		},
		{
			ID:          "qwen/qwen2.5-vl-72b-instruct",
			Name:        "👁️ Qwen 2.5 VL 72B",
			Description: "Estado da arte para OCR e tabelas densas, fotos desalinhadas ou digitalizações de baixa qualidade.",
			Speed:       "Moderado",
		},
		{
			ID:          "meta/llama-3.2-11b-vision-instruct",
			Name:        "🦙 Llama 3.2 11B Vision",
			Description: "Modelo leve e ultra-econômico para detecção e OCR básicos.",
			Speed:       "Rápido",
		},
	}
}

// ModelOption descreve um modelo disponível para o front-end.
type ModelOption struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Speed       string `json:"speed"`
}
