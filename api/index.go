package handler

import (
	"fmt"
	"net/http"

	"github.com/fernangcortes/ponto-real-go/internal/api"
	"github.com/fernangcortes/ponto-real-go/internal/rules"
)

var finalHandler http.Handler

func init() {
	// Carregar regras
	engine := rules.NewEngineWithDefaults()

	// Configurar rotas da API
	mux := http.NewServeMux()
	handler := api.NewHandler(engine)
	handler.RegisterRoutes(mux)

	// Aplicar middlewares
	finalHandler = api.Chain(mux,
		api.RecoveryMiddleware,
		api.LoggingMiddleware,
		api.CORSMiddleware,
	)

	fmt.Println("[Vercel] Serverless backend inicializado com sucesso!")
}

// Handler é o entrypoint exigido pelo Vercel em Go.
func Handler(w http.ResponseWriter, r *http.Request) {
	finalHandler.ServeHTTP(w, r)
}
