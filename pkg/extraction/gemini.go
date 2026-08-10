package extraction

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/fernangcortes/ponto-real-go/pkg/apperr"
	"github.com/fernangcortes/ponto-real-go/pkg/models"
)

const geminiAPIBase = "https://generativelanguage.googleapis.com/v1beta/models"

// GeminiExtractor implementa Extractor usando a API do Gemini.
type GeminiExtractor struct {
	APIKey string
	Model  string // "gemini-2.5-flash", "gemini-3.1-flash-lite-preview" ou "gemini-3.1-pro-preview"
	client *http.Client
	// baseURL permite apontar para um servidor de teste; vazio usa a API real.
	baseURL string
}

// NewGeminiExtractor cria um novo extractor Gemini.
func NewGeminiExtractor(apiKey, model string) *GeminiExtractor {
	if model == "" {
		model = "gemini-2.5-flash"
	}
	return &GeminiExtractor{
		APIKey: apiKey,
		Model:  model,
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func (g *GeminiExtractor) base() string {
	if g.baseURL != "" {
		return g.baseURL
	}
	return geminiAPIBase
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
func (g *GeminiExtractor) Extract(ctx context.Context, fileBytes []byte, mimeType string) (*models.Timesheet, error) {
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

	url := fmt.Sprintf("%s/%s:generateContent", g.base(), g.Model)

	return comTentativas(ctx, func(ctx context.Context, tentativa int) (*models.Timesheet, error) {
		return g.tentar(ctx, url, jsonBody, tentativa)
	})
}

// tentar faz UMA chamada ao Gemini. O corpo da resposta é fechado ao fim desta
// função — antes o defer ficava dentro do laço de retry e só liberava as três
// conexões quando o Extract inteiro terminava.
func (g *GeminiExtractor) tentar(ctx context.Context, url string, jsonBody []byte, tentativa int) (*models.Timesheet, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, naoRepetir(fmt.Errorf("erro ao criar requisição: %w", err))
	}
	req.Header.Set("Content-Type", "application/json")
	// Chave no cabeçalho, não na query string: URL costuma parar em log de
	// proxy e em histórico, e a chave iria junto.
	req.Header.Set("x-goog-api-key", g.APIKey)

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("erro na requisição HTTP (tentativa %d): %w", tentativa, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler resposta (tentativa %d): %w", tentativa, err)
	}

	var gemResp geminiResponse
	if err := json.Unmarshal(body, &gemResp); err != nil {
		return nil, fmt.Errorf("erro ao parsear resposta (tentativa %d): %w", tentativa, err)
	}

	if gemResp.Error != nil {
		apiErr := fmt.Errorf("%w — Gemini [%d]: %s", apperr.ErrProvedorRecusou, gemResp.Error.Code, gemResp.Error.Message)
		if gemResp.Error.Code == http.StatusTooManyRequests {
			// Rate limit: espera um pouco mais que o backoff padrão antes de
			// devolver o erro como repetível.
			if err := esperar(ctx, esperaRateLimit); err != nil {
				return nil, err
			}
			return nil, apiErr
		}
		return nil, naoRepetir(apiErr)
	}

	if len(gemResp.Candidates) == 0 || len(gemResp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("resposta vazia do Gemini (tentativa %d)", tentativa)
	}

	// Limpar possíveis wrappers de markdown (```json...```)
	textResponse := cleanJSONResponse(gemResp.Candidates[0].Content.Parts[0].Text)

	var timesheet models.Timesheet
	if err := json.Unmarshal([]byte(textResponse), &timesheet); err != nil {
		return nil, fmt.Errorf("resposta do Gemini não é JSON válido (tentativa %d): %w\nResposta: %s",
			tentativa, err, truncate(textResponse, 500))
	}

	if len(timesheet.Dias) == 0 {
		return nil, fmt.Errorf("o Gemini retornou 0 dias (tentativa %d)", tentativa)
	}

	return &timesheet, nil
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
