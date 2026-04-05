package main

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"os"

	"github.com/fernangcortes/ponto-real-go/internal/api"
	"github.com/fernangcortes/ponto-real-go/internal/rules"
)

//go:embed all:web
var webFS embed.FS

func main() {
	// Determinar porta
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Carregar regras
	engine, err := loadRules()
	if err != nil {
		fmt.Printf("[WARN] Usando regras padrão: %v\n", err)
		engine = rules.NewEngineWithDefaults()
	}

	// Configurar rotas
	mux := http.NewServeMux()

	// Registrar endpoints da API
	handler := api.NewHandler(engine)
	handler.RegisterRoutes(mux)

	// Servir arquivos estáticos do front-end (embeds)
	webContent, err := fs.Sub(webFS, "web")
	if err != nil {
		fmt.Printf("[FATAL] Erro ao montar front-end: %v\n", err)
		os.Exit(1)
	}
	mux.Handle("GET /", http.FileServer(http.FS(webContent)))

	// Aplicar middlewares
	finalHandler := api.Chain(mux,
		api.RecoveryMiddleware,
		api.LoggingMiddleware,
		api.CORSMiddleware,
	)

	// Iniciar servidor
	fmt.Printf("\n")
	fmt.Printf("  ╔══════════════════════════════════════╗\n")
	fmt.Printf("  ║       🕐 Ponto Real Go v0.3.0       ║\n")
	fmt.Printf("  ╠══════════════════════════════════════╣\n")
	fmt.Printf("  ║  Server:  http://localhost:%s      ║\n", port)
	fmt.Printf("  ║  API:     http://localhost:%s/api  ║\n", port)
	fmt.Printf("  ║  Regras:  %s  ║\n", engine.Config.NomeInstituicao[:30])
	fmt.Printf("  ╚══════════════════════════════════════╝\n")
	fmt.Printf("\n")

	if err := http.ListenAndServe(":"+port, finalHandler); err != nil {
		fmt.Printf("[FATAL] %v\n", err)
		os.Exit(1)
	}
}

func loadRules() (*rules.Engine, error) {
	// Tentar carregar de arquivo local primeiro
	paths := []string{
		"rules.json",
		"internal/rules/rules.json",
	}
	for _, p := range paths {
		if e, err := rules.NewEngine(p); err == nil {
			return e, nil
		}
	}
	return nil, fmt.Errorf("nenhum arquivo de regras encontrado")
}
