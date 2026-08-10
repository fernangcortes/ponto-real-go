// Package app monta o grafo de dependências do aplicativo.
//
// Existe para que o servidor local (main.go) e o deploy serverless
// (api/index.go) partam da mesma composição. Antes cada um montava o seu, e os
// dois já tinham divergido: o serverless usava regras hardcoded em Go enquanto o
// local lia rules.json do disco, então o mesmo cálculo dava resultados
// diferentes conforme onde rodava.
package app

import (
	"io/fs"
	"log/slog"
	"net/http"
	"os"

	"github.com/fernangcortes/ponto-real-go/pkg/api"
	"github.com/fernangcortes/ponto-real-go/pkg/extraction"
	"github.com/fernangcortes/ponto-real-go/pkg/models"
	"github.com/fernangcortes/ponto-real-go/pkg/repository"
	"github.com/fernangcortes/ponto-real-go/pkg/rules"
	"github.com/fernangcortes/ponto-real-go/pkg/service"
)

// Config reúne as decisões de ambiente da composição.
type Config struct {
	// DataDir é onde os meses processados são gravados.
	DataDir string
	// SettingsFile é o nome do arquivo de configurações (resolvido ao lado do executável).
	SettingsFile string
	// JustificativasFile é o nome do arquivo da biblioteca de frases, também
	// resolvido ao lado do executável. Fora de DataDir de propósito: lá todo
	// .json é lido como um mês salvo.
	JustificativasFile string
	// RulesPath, se preenchido, sobrescreve conscientemente as regras embutidas
	// no binário. Vazio — o caso normal — usa as embutidas.
	RulesPath string

	// Persistencia diz se há disco durável. Falso monta repositórios no-op.
	//
	// Esta é a decisão que antes vivia dentro do repositório JSON, que
	// consultava os.Getenv("VERCEL") em cinco pontos e mudava de comportamento
	// sozinho. Ambiente é assunto de composição, não de persistência.
	Persistencia bool
}

// ConfigFromEnv monta a configuração a partir do ambiente, com os padrões do projeto.
func ConfigFromEnv() Config {
	return Config{
		DataDir:            "data",
		SettingsFile:       "settings.json",
		JustificativasFile: "justificativas.json",
		RulesPath:          os.Getenv("PONTO_REAL_RULES"),
		// No Vercel o disco é efêmero: gravar não dá erro, mas o arquivo some
		// no próximo request.
		Persistencia: os.Getenv("VERCEL") != "1",
	}
}

// Servidor é o resultado da composição.
type Servidor struct {
	Handler http.Handler
	// Regras é a configuração efetivamente carregada, para exibição no boot.
	Regras models.RulesConfig
}

// BuildAPI devolve apenas as rotas /api, com os middlewares aplicados.
// É o que o entrypoint serverless precisa.
func BuildAPI(cfg Config) Servidor {
	h, regras := novoHandler(cfg)
	return Servidor{Handler: api.BuildHandler(h), Regras: regras}
}

// BuildServer monta a API e os arquivos estáticos sob um ÚNICO mux e uma única
// passagem de middlewares.
//
// Antes o main montava o handler da API já encadeado e depois encadeava tudo de
// novo por fora: toda requisição /api passava duas vezes pelos middlewares e
// aparecia duplicada no log.
func BuildServer(cfg Config, estaticos fs.FS) Servidor {
	h, regras := novoHandler(cfg)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	mux.Handle("/", http.FileServer(http.FS(estaticos)))

	handler := api.Chain(mux,
		api.RecoveryMiddleware,
		api.LoggingMiddleware,
		api.CORSMiddleware,
	)
	return Servidor{Handler: handler, Regras: regras}
}

func novoHandler(cfg Config) (*api.Handler, models.RulesConfig) {
	engine := carregarRegras(cfg.RulesPath)
	timesheetRepo, settingsRepo, justificativasRepo := repositorios(cfg)

	svc := service.NewTimesheetService(engine, timesheetRepo, extraction.NewRegistryExtractorFactory())
	return api.NewHandler(svc, settingsRepo, justificativasRepo), engine.Config
}

// repositorios escolhe entre gravar em disco e descartar, conforme o ambiente.
func repositorios(cfg Config) (repository.TimesheetRepository, repository.SettingsRepository,
	repository.JustificativasRepository) {
	if !cfg.Persistencia {
		slog.Info("persistência desativada: meses, configurações e justificativas ficam apenas em memória")
		return repository.NoopTimesheetRepository{}, repository.NoopSettingsRepository{},
			repository.NoopJustificativasRepository{}
	}
	return repository.NewJSONTimesheetRepository(cfg.DataDir),
		repository.NewJSONSettingsRepository(cfg.SettingsFile),
		repository.NewJSONJustificativasRepository(cfg.JustificativasFile)
}

// carregarRegras usa as regras embutidas, a menos que um caminho externo seja
// pedido explicitamente. Um caminho ilegível não derruba o servidor: avisa e
// segue com as embutidas, que sempre existem.
func carregarRegras(path string) *rules.Engine {
	if path == "" {
		return rules.NewEngineWithDefaults()
	}

	engine, err := rules.NewEngine(path)
	if err != nil {
		slog.Warn("não foi possível ler as regras externas; usando as embutidas",
			"caminho", path, "erro", err)
		return rules.NewEngineWithDefaults()
	}

	slog.Info("regras carregadas de arquivo externo", "caminho", path)
	return engine
}

// ConfigurarLog instala o logger padrão do processo.
//
// O código usa as funções de pacote do slog em vez de carregar um *slog.Logger
// por todas as camadas: para um binário só, o ganho da injeção não paga o custo
// de plumbing. Testes silenciam o ruído com slog.SetDefault.
func ConfigurarLog(debug bool) {
	nivel := slog.LevelInfo
	if debug {
		nivel = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: nivel,
	})))
}
