package handler

import (
	"log/slog"
	"net/http"

	"github.com/fernangcortes/ponto-real-go/pkg/app"
)

var finalHandler http.Handler

func init() {
	app.ConfigurarLog(false)

	// Mesma composição do servidor local — inclusive as regras embutidas no
	// binário, que antes eram substituídas aqui por um conjunto hardcoded
	// diferente.
	finalHandler = app.BuildAPI(app.ConfigFromEnv()).Handler

	slog.Info("backend serverless inicializado")
}

// Handler é o entrypoint exigido pelo Vercel em Go.
func Handler(w http.ResponseWriter, r *http.Request) {
	finalHandler.ServeHTTP(w, r)
}
