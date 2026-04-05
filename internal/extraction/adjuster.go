package extraction

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ponto-real-go/internal/models"
)

// GeminiAdjuster ajusta horários faltantes usando a API Gemini (text-only).
type GeminiAdjuster struct {
	APIKey string
	Model  string
	client *http.Client
}

// NewGeminiAdjuster cria um novo adjuster.
func NewGeminiAdjuster(apiKey, model string) *GeminiAdjuster {
	if model == "" {
		model = "gemini-3.1-flash-lite-preview"
	}
	return &GeminiAdjuster{
		APIKey: apiKey,
		Model:  model,
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// Adjust recebe um Timesheet com dados crus e retorna com horários ajustados.
func (a *GeminiAdjuster) Adjust(ts *models.Timesheet) (*models.Timesheet, error) {
	// Serializar timesheet para JSON
	tsJSON, err := json.MarshalIndent(ts, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar timesheet: %w", err)
	}

	prompt := AdjustmentPrompt + "\n\nDADOS DA FOLHA DE PONTO:\n```json\n" + string(tsJSON) + "\n```"

	reqBody := geminiRequest{
		Contents: []geminiContent{
			{
				Parts: []geminiPart{
					{Text: prompt},
				},
			},
		},
		GenerationConfig: map[string]interface{}{
			"temperature":      0.7, // Mais variação para parecer humano
			"topP":             0.95,
			"maxOutputTokens":  65536,
			"responseMimeType": "application/json",
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar request: %w", err)
	}

	url := fmt.Sprintf("%s/%s:generateContent?key=%s", geminiAPIBase, a.Model, a.APIKey)

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt*2) * time.Second)
		}

		resp, err := a.client.Post(url, "application/json", bytes.NewReader(jsonBody))
		if err != nil {
			lastErr = fmt.Errorf("erro HTTP (tentativa %d): %w", attempt+1, err)
			continue
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = fmt.Errorf("erro leitura (tentativa %d): %w", attempt+1, err)
			continue
		}

		var gemResp geminiResponse
		if err := json.Unmarshal(body, &gemResp); err != nil {
			lastErr = fmt.Errorf("erro parse (tentativa %d): %w", attempt+1, err)
			continue
		}

		if gemResp.Error != nil {
			lastErr = fmt.Errorf("erro Gemini [%d]: %s", gemResp.Error.Code, gemResp.Error.Message)
			if gemResp.Error.Code == 429 {
				time.Sleep(5 * time.Second)
				continue
			}
			return nil, lastErr
		}

		if len(gemResp.Candidates) == 0 || len(gemResp.Candidates[0].Content.Parts) == 0 {
			lastErr = fmt.Errorf("resposta vazia (tentativa %d)", attempt+1)
			continue
		}

		text := cleanJSONResponse(gemResp.Candidates[0].Content.Parts[0].Text)

		var result models.Timesheet
		if err := json.Unmarshal([]byte(text), &result); err != nil {
			lastErr = fmt.Errorf("JSON inválido (tentativa %d): %w\nResposta: %s", attempt+1, err, truncate(text, 500))
			continue
		}

		if len(result.Dias) == 0 {
			lastErr = fmt.Errorf("0 dias retornados (tentativa %d)", attempt+1)
			continue
		}

		// Preservar dados do servidor original
		if result.Servidor.Nome == "" {
			result.Servidor = ts.Servidor
		}
		if result.MesAno == "" {
			result.MesAno = ts.MesAno
		}

		return &result, nil
	}

	return nil, fmt.Errorf("ajuste falhou após 3 tentativas: %w", lastErr)
}
