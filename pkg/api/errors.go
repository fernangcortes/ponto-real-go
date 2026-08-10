package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/fernangcortes/ponto-real-go/pkg/apperr"
)

// statusDoErro traduz erro de domínio em status HTTP.
//
// Esta tabela é o motivo de os erros existirem como sentinelas: antes toda
// falha de extração virava 500, inclusive chave inválida e crédito
// insuficiente — coisas que o usuário resolve sozinho, mas que o front-end não
// tinha como distinguir de "o servidor quebrou".
func statusDoErro(err error) int {
	switch {
	case errors.Is(err, apperr.ErrChaveAusente),
		errors.Is(err, apperr.ErrModeloInvalido),
		errors.Is(err, apperr.ErrArquivoInvalido),
		errors.Is(err, apperr.ErrProvedorInvalido),
		errors.Is(err, apperr.ErrRequisicaoInvalida):
		return http.StatusBadRequest

	case errors.Is(err, apperr.ErrMesNaoEncontrado):
		return http.StatusNotFound

	// A chave é do usuário e está nas Configurações dele: 400, não 500.
	case errors.Is(err, apperr.ErrProvedorRecusou):
		return http.StatusBadRequest

	// O provedor respondeu, mas nunca de forma utilizável. Nada que o usuário
	// configure resolve; 502 diz que o problema é do serviço lá fora.
	case errors.Is(err, apperr.ErrExtracaoFalhou):
		return http.StatusBadGateway

	case errors.Is(err, context.Canceled):
		// Cliente desistiu. Status não chega a lugar nenhum, mas 499 mantém o
		// log honesto sobre o que aconteceu.
		return 499

	default:
		return http.StatusInternalServerError
	}
}

// writeError responde com o erro em JSON, no status correspondente.
func writeError(w http.ResponseWriter, err error) {
	status := statusDoErro(err)

	// 5xx é problema nosso e merece registro; 4xx é o usuário sendo avisado do
	// que precisa corrigir, e encheria o log sem informar nada.
	if status >= http.StatusInternalServerError {
		slog.Error("falha ao atender requisição", "status", status, "erro", err)
	}

	writeJSON(w, status, map[string]string{"error": err.Error()})
}

// writeJSON escreve uma resposta JSON.
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	// O status já foi enviado, então não há como transformar a falha em 500: o
	// cliente vai receber um corpo truncado de qualquer jeito. O que dá para
	// fazer é não engolir o erro em silêncio.
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("erro ao serializar resposta", "erro", err)
	}
}
