package extraction

import (
	"fmt"

	"github.com/fernangcortes/ponto-real-go/pkg/apperr"
)

// ExtractorFactory define a interface para criação de extratores de folha de ponto.
type ExtractorFactory interface {
	Create(provider string, apiKey string, model string) (Extractor, error)
}

// RegistryExtractorFactory implementa ExtractorFactory permitindo registrar novos provedores dinamicamente.
type RegistryExtractorFactory struct {
	registry map[string]func(apiKey string, model string) Extractor
}

// NewRegistryExtractorFactory cria uma fábrica com os extratores padrão registrados (Gemini e OpenRouter).
func NewRegistryExtractorFactory() *RegistryExtractorFactory {
	factory := &RegistryExtractorFactory{
		registry: make(map[string]func(apiKey string, model string) Extractor),
	}

	// Registrar provedores nativos
	factory.Register(ProviderGemini, func(apiKey string, model string) Extractor {
		return NewGeminiExtractor(apiKey, model)
	})
	factory.Register(ProviderOpenRouter, func(apiKey string, model string) Extractor {
		return NewOpenRouterExtractor(apiKey, model)
	})

	return factory
}

// Register registra uma função builder para um novo provedor.
func (f *RegistryExtractorFactory) Register(provider string, builder func(apiKey string, model string) Extractor) {
	f.registry[provider] = builder
}

// Create constrói uma instância de extrator para o provedor e modelo solicitados.
func (f *RegistryExtractorFactory) Create(provider string, apiKey string, model string) (Extractor, error) {
	builder, ok := f.registry[provider]
	if !ok {
		return nil, fmt.Errorf("%w: %s", apperr.ErrProvedorInvalido, provider)
	}
	return builder(apiKey, model), nil
}
