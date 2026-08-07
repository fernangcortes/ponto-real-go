package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/fernangcortes/ponto-real-go/pkg/extraction"
	"github.com/fernangcortes/ponto-real-go/pkg/models"
	"github.com/fernangcortes/ponto-real-go/pkg/repository"
)

// Regressão do data race: as configurações eram um campo comum do Handler, lido
// por /api/health, /api/models e /api/upload enquanto POST /api/settings
// escrevia — cada requisição na sua goroutine, sem sincronização.
//
// Este teste só ACUSA a falha sob `go test -race`. Sem o detector ele passa
// mesmo com o bug, então o portão de verdade é o CI (a máquina de
// desenvolvimento não tem compilador C para o -race).
func TestSettingsConcorrentesNaoDaoRace(t *testing.T) {
	h := newHarness(t, models.AppSettings{Provider: extraction.ProviderGemini, GeminiAPIKey: "inicial"}, nil)

	const goroutines = 8
	const iteracoes = 25

	var wg sync.WaitGroup

	chamar := func(metodo, caminho, corpo string) {
		var req *http.Request
		if corpo == "" {
			req = httptest.NewRequest(metodo, caminho, nil)
		} else {
			req = httptest.NewRequest(metodo, caminho, strings.NewReader(corpo))
			req.Header.Set("Content-Type", "application/json")
		}
		h.srv.ServeHTTP(httptest.NewRecorder(), req)
	}

	// Escritores
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iteracoes; j++ {
				chamar("POST", "/api/settings", `{"provider":"openrouter","open_router_api_key":"chave"}`)
			}
		}()
	}

	// Leitores
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iteracoes; j++ {
				chamar("GET", "/api/health", "")
				chamar("GET", "/api/models", "")
				chamar("GET", "/api/settings", "")
			}
		}()
	}

	wg.Wait()

	// Estado final coerente: o provedor mudou e a chave do gemini sobreviveu,
	// já que nenhuma escrita a sobrescreveu.
	final := h.settings.ultimo()
	if final.Provider != extraction.ProviderOpenRouter {
		t.Errorf("Provider = %q; esperado openrouter", final.Provider)
	}
	if final.GeminiAPIKey != "inicial" {
		t.Errorf("GeminiAPIKey = %q; nenhuma escrita deveria tê-la tocado", final.GeminiAPIKey)
	}
}

// A leitura devolve uma cópia: quem chama não consegue alterar o estado interno
// sem passar pelo lock.
func TestGetDevolveCopia(t *testing.T) {
	store := newSettingsStore(&fakeSettingsRepo{
		settings: models.AppSettings{Provider: extraction.ProviderGemini, GeminiAPIKey: "original"},
	})

	copia := store.Get()
	copia.GeminiAPIKey = "adulterada"

	if store.Get().GeminiAPIKey != "original" {
		t.Error("mexer na cópia alterou o estado interno do store")
	}
}

func TestChaveDoProvedorAtivoIgnoraChaveDoOutro(t *testing.T) {
	casos := []struct {
		nome     string
		settings models.AppSettings
		provider string
		chave    string
	}{
		{"gemini", models.AppSettings{Provider: extraction.ProviderGemini, GeminiAPIKey: "g", OpenRouterAPIKey: "o"}, extraction.ProviderGemini, "g"},
		{"openrouter", models.AppSettings{Provider: extraction.ProviderOpenRouter, GeminiAPIKey: "g", OpenRouterAPIKey: "o"}, extraction.ProviderOpenRouter, "o"},
		{"openrouter sem chave própria", models.AppSettings{Provider: extraction.ProviderOpenRouter, GeminiAPIKey: "g"}, extraction.ProviderOpenRouter, ""},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			t.Setenv("GEMINI_API_KEY", "")
			t.Setenv("OPENROUTER_API_KEY", "")

			store := newSettingsStore(&fakeSettingsRepo{settings: c.settings})
			provider, chave := store.ChaveDoProvedorAtivo()

			if provider != c.provider || chave != c.chave {
				t.Errorf("= (%q, %q); esperado (%q, %q)", provider, chave, c.provider, c.chave)
			}
		})
	}
}

// Em ambiente sem disco durável, quem monta o grafo injeta o repositório no-op
// e o store nem precisa saber: a mudança vale em memória e a gravação some
// sozinha.
//
// Antes isso era um `if os.Getenv("VERCEL") != "1"` dentro do handler.
func TestSemPersistenciaMudancaValeEmMemoria(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("OPENROUTER_API_KEY", "")

	store := newSettingsStore(repository.NoopSettingsRepository{})

	if err := store.Atualizar(extraction.ProviderOpenRouter, "", "chave"); err != nil {
		t.Fatalf("Atualizar com repositório no-op não deveria falhar: %v", err)
	}

	atual := store.Get()
	if atual.OpenRouterAPIKey != "chave" || atual.Provider != extraction.ProviderOpenRouter {
		t.Errorf("estado em memória = %+v; a mudança deveria valer mesmo sem persistência", atual)
	}
}

// Repositório de settings que falha na leitura (primeira execução, ou ambiente
// sem disco) não impede o servidor de subir: o store cai no padrão e completa
// pelas variáveis de ambiente.
func TestFalhaAoCarregarCaiNoPadraoEnoAmbiente(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "chave-do-ambiente")
	t.Setenv("OPENROUTER_API_KEY", "")

	store := newSettingsStore(repository.NoopSettingsRepository{})

	atual := store.Get()
	if atual.Provider != extraction.ProviderGemini {
		t.Errorf("Provider = %q; esperado o padrão gemini", atual.Provider)
	}
	if atual.GeminiAPIKey != "chave-do-ambiente" {
		t.Errorf("GeminiAPIKey = %q; deveria vir do ambiente", atual.GeminiAPIKey)
	}
}

func TestAtualizarPropagaErroDeGravacao(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("OPENROUTER_API_KEY", "")

	repo := &fakeSettingsRepo{saveErr: errGravacao}
	store := newSettingsStore(repo)

	if err := store.Atualizar(extraction.ProviderGemini, "chave", ""); err == nil {
		t.Error("esperado erro vindo do repositório")
	}
}

var errGravacao = errors.New("disco cheio")
