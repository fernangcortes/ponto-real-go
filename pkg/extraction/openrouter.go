package extraction

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/fernangcortes/ponto-real-go/pkg/models"
)

const openRouterAPIBase = "https://openrouter.ai/api/v1/chat/completions"

// OpenRouterExtractor implementa Extractor usando a API do OpenRouter.
type OpenRouterExtractor struct {
	APIKey string
	Model  string
	client *http.Client
}

// NewOpenRouterExtractor cria um novo extractor OpenRouter.
func NewOpenRouterExtractor(apiKey, model string) *OpenRouterExtractor {
	if model == "" {
		model = "google/gemini-2.0-flash"
	}
	return &OpenRouterExtractor{
		APIKey: apiKey,
		Model:  model,
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

type openRouterRequest struct {
	Model          string              `json:"model"`
	Messages       []openRouterMessage `json:"messages"`
	Temperature    float64             `json:"temperature"`
	ResponseFormat *responseFormat     `json:"response_format,omitempty"`
}

type openRouterMessage struct {
	Role    string        `json:"role"`
	Content []interface{} `json:"content"`
}

type textContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type imageUrlContent struct {
	Type     string   `json:"type"`
	ImageUrl imageUrl `json:"image_url"`
}

type imageUrl struct {
	URL string `json:"url"`
}

type fileContent struct {
	Type string   `json:"type"`
	File fileData `json:"file"`
}

type fileData struct {
	FileData string `json:"file_data"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type openRouterResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Extract envia o arquivo (imagem ou PDF) ao OpenRouter e retorna o Timesheet estruturado.
func (o *OpenRouterExtractor) Extract(fileBytes []byte, mimeType string) (*models.Timesheet, error) {
	b64Data := base64.StdEncoding.EncodeToString(fileBytes)

	var mediaContent interface{}
	if mimeType == "application/pdf" {
		mediaContent = fileContent{
			Type: "file",
			File: fileData{
				FileData: fmt.Sprintf("data:%s;base64,%s", mimeType, b64Data),
			},
		}
	} else {
		mediaContent = imageUrlContent{
			Type: "image_url",
			ImageUrl: imageUrl{
				URL: fmt.Sprintf("data:%s;base64,%s", mimeType, b64Data),
			},
		}
	}

	reqBody := openRouterRequest{
		Model: o.Model,
		Messages: []openRouterMessage{
			{
				Role: "user",
				Content: []interface{}{
					textContent{
						Type: "text",
						Text: ExtractionPrompt,
					},
					mediaContent,
				},
			},
		},
		Temperature: 0.1,
	}

	// Forçar resposta em formato JSON para modelos suportados
	reqBody.ResponseFormat = &responseFormat{Type: "json_object"}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar request: %w", err)
	}

	// Fazer requisição HTTP
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt*2) * time.Second)
		}

		req, err := http.NewRequest("POST", openRouterAPIBase, bytes.NewReader(jsonBody))
		if err != nil {
			lastErr = fmt.Errorf("erro ao criar requisição (tentativa %d): %w", attempt+1, err)
			continue
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+o.APIKey)
		req.Header.Set("HTTP-Referer", "https://github.com/fernangcortes/ponto-real-go")
		req.Header.Set("X-Title", "Ponto Real Go")

		resp, err := o.client.Do(req)
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

		var orResp openRouterResponse
		if err := json.Unmarshal(body, &orResp); err != nil {
			lastErr = fmt.Errorf("erro ao parsear resposta do OpenRouter (tentativa %d): %w. Resposta: %s", attempt+1, err, string(body))
			continue
		}

		if orResp.Error != nil {
			lastErr = fmt.Errorf("erro da API OpenRouter: %s", orResp.Error.Message)
			return nil, lastErr // erro não-retryable (geralmente chave inválida ou saldo insuficiente)
		}

		if len(orResp.Choices) == 0 || orResp.Choices[0].Message.Content == "" {
			lastErr = fmt.Errorf("resposta vazia da API (tentativa %d). Resposta: %s", attempt+1, string(body))
			continue
		}

		textResponse := orResp.Choices[0].Message.Content
		textResponse = cleanJSONResponse(textResponse)

		// Parse do JSON extraído
		var timesheet models.Timesheet
		if err := json.Unmarshal([]byte(textResponse), &timesheet); err != nil {
			lastErr = fmt.Errorf("resposta do OpenRouter não é JSON válido (tentativa %d): %w\nResposta: %s", attempt+1, err, truncate(textResponse, 500))
			continue
		}

		// Validação básica
		if len(timesheet.Dias) == 0 {
			lastErr = fmt.Errorf("OpenRouter retornou 0 dias (tentativa %d)", attempt+1)
			continue
		}

		return &timesheet, nil
	}

	return nil, fmt.Errorf("falha após 3 tentativas: %w", lastErr)
}
