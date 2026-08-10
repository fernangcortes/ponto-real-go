package api

import (
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// CORSMiddleware adiciona headers CORS para permitir requests do front-end.
func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// statusRecorder guarda o status escrito, que o ResponseWriter não expõe.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Write cobre o handler que escreve o corpo sem chamar WriteHeader; nesse caso
// o net/http assume 200 e o registro precisa refletir isso.
func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	return s.ResponseWriter.Write(b)
}

// LoggingMiddleware registra cada requisição da API.
//
// Passou a registrar status e duração: só método e caminho não diziam se a
// requisição deu certo nem se estava lenta, que são as duas perguntas que
// levam alguém a olhar o log.
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Arquivo estático não interessa: encheria o log a cada página.
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		inicio := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)

		slog.Info("requisição",
			"metodo", r.Method,
			"caminho", r.URL.Path,
			"status", rec.status,
			"duracao", time.Since(inicio).Round(time.Millisecond),
		)
	})
}

// RecoveryMiddleware recupera de panics e retorna 500.
func RecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				slog.Error("panic ao atender requisição",
					"caminho", r.URL.Path,
					"erro", err,
				)
				http.Error(w, "Erro interno do servidor", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// Chain aplica uma sequência de middlewares a um handler.
func Chain(h http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}

// BuildHandler cria um multiplexer para as rotas da API e aplica os middlewares padrão.
func BuildHandler(h *Handler) http.Handler {
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return Chain(mux,
		RecoveryMiddleware,
		LoggingMiddleware,
		CORSMiddleware,
	)
}
