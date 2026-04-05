package extraction

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ponto-real-go/internal/models"
)

const geminiAPIBase = "https://generativelanguage.googleapis.com/v1beta/models"

// GeminiExtractor implementa Extractor usando a API do Gemini.
type GeminiExtractor struct {
	APIKey string
	Model  string // "gemini-3.1-flash-lite-preview" ou "gemini-3.1-pro-preview"
	client *http.Client
}

// NewGeminiExtractor cria um novo extractor Gemini.
func NewGeminiExtractor(apiKey, model string) *GeminiExtractor {
	if model == "" {
		model = "gemini-3.1-flash-lite-preview"
	}
	return &GeminiExtractor{
		APIKey: apiKey,
		Model:  model,
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// geminiRequest é o payload do request para a API generateContent.
type geminiRequest struct {
	Contents         []geminiContent        `json:"contents"`
	GenerationConfig map[string]interface{} `json:"generationConfig,omitempty"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text       string        `json:"text,omitempty"`
	InlineData *geminiInline `json:"inline_data,omitempty"`
}

type geminiInline struct {
	MimeType string `json:"mime_type"`
	Data     string `json:"data"` // base64
}

// geminiResponse é o payload de resposta da API.
type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

// Extract envia a imagem/PDF ao Gemini e retorna o Timesheet extraído.
func (g *GeminiExtractor) Extract(fileBytes []byte, mimeType string) (*models.Timesheet, error) {
	// Codificar arquivo em base64
	b64Data := base64.StdEncoding.EncodeToString(fileBytes)

	// Montar request
	reqBody := geminiRequest{
		Contents: []geminiContent{
			{
				Parts: []geminiPart{
					{
						InlineData: &geminiInline{
							MimeType: mimeType,
							Data:     b64Data,
						},
					},
					{
						Text: ExtractionPrompt,
					},
				},
			},
		},
		GenerationConfig: map[string]interface{}{
			"temperature":      0.1,
			"topP":             0.95,
			"maxOutputTokens":  8192,
			"responseMimeType": "application/json",
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar request: %w", err)
	}

	// Montar URL
	url := fmt.Sprintf("%s/%s:generateContent?key=%s", geminiAPIBase, g.Model, g.APIKey)

	// Fazer request com retry
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt*2) * time.Second)
		}

		resp, err := g.client.Post(url, "application/json", bytes.NewReader(jsonBody))
		if err != nil {
			lastErr = fmt.Errorf("erro na requisição HTTP (tentativa %d): %w", attempt+1, err)
			continue
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = fmt.Errorf("erro ao ler resposta (tentativa %d): %w", attempt+1, err)
			continue
		}

		// Parse da resposta
		var gemResp geminiResponse
		if err := json.Unmarshal(body, &gemResp); err != nil {
			lastErr = fmt.Errorf("erro ao parsear resposta (tentativa %d): %w", attempt+1, err)
			continue
		}

		// Verificar erro da API
		if gemResp.Error != nil {
			lastErr = fmt.Errorf("erro da API Gemini [%d]: %s", gemResp.Error.Code, gemResp.Error.Message)
			if gemResp.Error.Code == 429 {
				time.Sleep(5 * time.Second) // rate limit
				continue
			}
			return nil, lastErr // erro não-retryable
		}

		// Extrair texto da resposta
		if len(gemResp.Candidates) == 0 || len(gemResp.Candidates[0].Content.Parts) == 0 {
			lastErr = fmt.Errorf("resposta vazia do Gemini (tentativa %d)", attempt+1)
			continue
		}

		textResponse := gemResp.Candidates[0].Content.Parts[0].Text

		// Limpar possíveis wrappers de markdown (```json...```)
		textResponse = cleanJSONResponse(textResponse)

		// Parse do JSON extraído
		var timesheet models.Timesheet
		if err := json.Unmarshal([]byte(textResponse), &timesheet); err != nil {
			lastErr = fmt.Errorf("resposta do Gemini não é JSON válido (tentativa %d): %w\nResposta: %s", attempt+1, err, truncate(textResponse, 500))
			continue
		}

		// Validação básica
		if len(timesheet.Dias) == 0 {
			lastErr = fmt.Errorf("Gemini retornou 0 dias (tentativa %d)", attempt+1)
			continue
		}

		return &timesheet, nil
	}

	return nil, fmt.Errorf("falha após 3 tentativas: %w", lastErr)
}

// cleanJSONResponse remove wrappers de markdown da resposta.
func cleanJSONResponse(s string) string {
	s = strings.TrimSpace(s)
	// Remover ```json ... ```
	if strings.HasPrefix(s, "```json") {
		s = strings.TrimPrefix(s, "```json")
		s = strings.TrimSuffix(s, "```")
		s = strings.TrimSpace(s)
	} else if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSuffix(s, "```")
		s = strings.TrimSpace(s)
	}
	return s
}

// truncate corta uma string para o comprimento máximo.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
