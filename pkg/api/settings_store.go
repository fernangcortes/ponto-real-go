package api

import (
	"os"
	"strings"
	"sync"

	"github.com/fernangcortes/ponto-real-go/pkg/extraction"
	"github.com/fernangcortes/ponto-real-go/pkg/models"
	"github.com/fernangcortes/ponto-real-go/pkg/repository"
)

// settingsStore guarda as configurações do aplicativo com acesso seguro entre
// goroutines.
//
// O servidor HTTP atende cada requisição em sua própria goroutine, então
// POST /api/settings escreve enquanto /api/upload, /api/health e /api/models
// leem. Antes isso era um campo comum no Handler, sem sincronização: um data
// race concreto, não teórico.
type settingsStore struct {
	mu    sync.RWMutex
	repo  repository.SettingsRepository
	atual models.AppSettings
}

// newSettingsStore carrega as configurações salvas e completa o que faltar com
// as variáveis de ambiente.
func newSettingsStore(repo repository.SettingsRepository) *settingsStore {
	s := &settingsStore{repo: repo}

	// Falha ao ler é esperada na primeira execução: seguimos com o zero value e
	// deixamos o ambiente preencher.
	if carregado, err := repo.Load(); err == nil {
		s.atual = carregado
	}

	if s.atual.Provider == "" {
		s.atual.Provider = extraction.ProviderGemini
	}
	if s.atual.GeminiAPIKey == "" {
		s.atual.GeminiAPIKey = os.Getenv("GEMINI_API_KEY")
	}
	if s.atual.OpenRouterAPIKey == "" {
		s.atual.OpenRouterAPIKey = os.Getenv("OPENROUTER_API_KEY")
	}

	return s
}

// Get devolve uma cópia do estado atual, segura para uso fora do lock.
func (s *settingsStore) Get() models.AppSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.atual
}

// Atualizar aplica as mudanças recebidas e grava no repositório.
//
// Campo vazio significa "não mexi neste": o front-end devolve o placeholder
// mascarado, e sobrescrever com ele apagaria a chave real.
//
// Não há mais decisão de "gravar ou não" aqui: em ambiente sem persistência
// quem monta o grafo injeta um NoopSettingsRepository, e a gravação vira um
// no-op de graça.
func (s *settingsStore) Atualizar(provider, geminiKey, openRouterKey string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if p := strings.TrimSpace(provider); p != "" {
		s.atual.Provider = p
	} else {
		s.atual.Provider = extraction.ProviderGemini
	}
	if k := strings.TrimSpace(geminiKey); k != "" {
		s.atual.GeminiAPIKey = k
	}
	if k := strings.TrimSpace(openRouterKey); k != "" {
		s.atual.OpenRouterAPIKey = k
	}

	return s.repo.Save(s.atual)
}

// ChaveDoProvedorAtivo devolve a chave do provedor selecionado. A chave do
// outro provedor não serve como substituta.
func (s *settingsStore) ChaveDoProvedorAtivo() (provider, apiKey string) {
	atual := s.Get()
	if atual.Provider == extraction.ProviderOpenRouter {
		return extraction.ProviderOpenRouter, atual.OpenRouterAPIKey
	}
	return atual.Provider, atual.GeminiAPIKey
}
