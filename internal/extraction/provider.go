package extraction

import (
	"github.com/fernangcortes/ponto-real-go/internal/models"
)

// Extractor é a interface para extração de dados de folhas de ponto.
// Permite trocar a implementação (Gemini Vision, Document AI, etc.) sem alterar o resto do código.
type Extractor interface {
	// Extract recebe os bytes do arquivo e seu MIME type, retorna um Timesheet estruturado.
	Extract(fileBytes []byte, mimeType string) (*models.Timesheet, error)
}

// AvailableModels retorna a lista de modelos disponíveis para extração.
func AvailableModels() []ModelOption {
	return []ModelOption{
		{
			ID:          "gemini-3.1-flash-lite-preview",
			Name:        "Gemini 3.1 Flash Lite (Preview)",
			Description: "Mais rápido e econômico. Bom para folhas de ponto com layout padrão.",
			Speed:       "Rápido",
		},
		{
			ID:          "gemini-3.1-pro-preview",
			Name:        "Gemini 3.1 Pro (Preview)",
			Description: "Mais preciso. Recomendado para folhas com layout complexo ou baixa qualidade.",
			Speed:       "Moderado",
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
