package handler

import (
	"fmt"
	"net/http"

	"github.com/fernangcortes/ponto-real-go/pkg/api"
	"github.com/fernangcortes/ponto-real-go/pkg/extraction"
	"github.com/fernangcortes/ponto-real-go/pkg/repository"
	"github.com/fernangcortes/ponto-real-go/pkg/rules"
	"github.com/fernangcortes/ponto-real-go/pkg/service"
)

var finalHandler http.Handler

func init() {
	// Carregar regras com padrões
	engine := rules.NewEngineWithDefaults()

	// Inicializar dependências
	timesheetRepo := repository.NewJSONTimesheetRepository("data")
	settingsRepo := repository.NewJSONSettingsRepository("settings.json")
	extractorFactory := extraction.NewRegistryExtractorFactory()

	timesheetService := service.NewTimesheetService(engine, timesheetRepo, extractorFactory)
	handler := api.NewHandler(timesheetService, settingsRepo)

	// Configurar rotas e middlewares
	finalHandler = api.BuildHandler(handler)

	fmt.Println("[Vercel] Serverless backend inicializado com sucesso!")
}

// Handler é o entrypoint exigido pelo Vercel em Go.
func Handler(w http.ResponseWriter, r *http.Request) {
	finalHandler.ServeHTTP(w, r)
}
