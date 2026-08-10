package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func cfgDeTeste(t *testing.T) Config {
	t.Helper()
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("PONTO_REAL_RULES", "")
	return Config{
		DataDir:      t.TempDir(),
		SettingsFile: "settings_app_test.json",
		Persistencia: true,
	}
}

// A variável de ambiente do Vercel é lida em UM lugar: aqui, na composição.
// Antes o repositório a consultava sozinho, em cinco pontos.
func TestConfigFromEnvDecidePersistencia(t *testing.T) {
	t.Run("ambiente normal", func(t *testing.T) {
		t.Setenv("VERCEL", "")
		if !ConfigFromEnv().Persistencia {
			t.Error("Persistencia = false; esperado true fora do Vercel")
		}
	})

	t.Run("vercel", func(t *testing.T) {
		t.Setenv("VERCEL", "1")
		if ConfigFromEnv().Persistencia {
			t.Error("Persistencia = true; no Vercel o disco é efêmero")
		}
	})
}

// Sem persistência a composição injeta os repositórios no-op, e a API continua
// respondendo — só não guarda nada.
func TestSemPersistenciaMontaRepositoriosNoop(t *testing.T) {
	cfg := cfgDeTeste(t)
	cfg.Persistencia = false

	srv := BuildServer(cfg, fstest.MapFS{})

	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, httptest.NewRequest("GET", "/api/months", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; corpo: %s", rec.Code, rec.Body.String())
	}
	if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
		t.Errorf("corpo = %q; esperado lista vazia", got)
	}

	// Mês inexistente continua sendo 404, não 500.
	rec = httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, httptest.NewRequest("GET", "/api/month/06_2026", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d; esperado 404", rec.Code)
	}
}

// Regressão da divergência entre deploys: o entrypoint serverless montava o
// motor com regras hardcoded em Go enquanto o servidor local lia rules.json, e
// o mesmo cálculo dava resultado diferente conforme onde rodava.
func TestBuildAPIEBuildServerUsamAsMesmasRegras(t *testing.T) {
	cfg := cfgDeTeste(t)

	api := BuildAPI(cfg)
	server := BuildServer(cfg, fstest.MapFS{})

	if api.Regras != server.Regras {
		t.Errorf("as duas composições divergiram:\n  BuildAPI:    %+v\n  BuildServer: %+v",
			api.Regras, server.Regras)
	}
	if api.Regras.CargaHorariaDiaria == 0 {
		t.Error("CargaHorariaDiaria zerada; as regras embutidas não foram carregadas")
	}
}

// O file server registrado em "/" não pode engolir as rotas da API.
func TestBuildServerServeAPIEEstaticosNoMesmoHandler(t *testing.T) {
	cfg := cfgDeTeste(t)
	estaticos := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<h1>Ponto Real</h1>")},
	}

	srv := BuildServer(cfg, estaticos)

	t.Run("rota da API", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, httptest.NewRequest("GET", "/api/health", nil))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d; corpo: %s", rec.Code, rec.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("resposta não é JSON: %s", rec.Body.String())
		}
		if body["status"] != "ok" {
			t.Errorf("status = %v", body["status"])
		}
	})

	t.Run("arquivo estático", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		if got := rec.Body.String(); got != "<h1>Ponto Real</h1>" {
			t.Errorf("corpo = %q", got)
		}
	})
}

func TestBuildServerAplicaMiddlewares(t *testing.T) {
	srv := BuildServer(cfgDeTeste(t), fstest.MapFS{})

	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, httptest.NewRequest("OPTIONS", "/api/health", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("preflight: status = %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q", got)
	}
}

// Caminho de regras ilegível avisa e segue com as embutidas, em vez de derrubar
// o servidor ou rodar com um conjunto de regras diferente em silêncio.
func TestRulesPathInvalidoCaiNasEmbutidas(t *testing.T) {
	cfg := cfgDeTeste(t)
	cfg.RulesPath = "caminho/que/nao/existe/rules.json"

	srv := BuildServer(cfg, fstest.MapFS{})

	if srv.Regras.CargaHorariaDiaria == 0 {
		t.Error("deveria ter caído nas regras embutidas")
	}
}
