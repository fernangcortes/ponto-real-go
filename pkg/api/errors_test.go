package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/fernangcortes/ponto-real-go/pkg/apperr"
)

// A tabela erro→status é o coração da Fase 2: antes toda falha virava 500 e o
// front-end não conseguia distinguir "corrija sua chave" de "o servidor caiu".
func TestStatusDoErro(t *testing.T) {
	casos := []struct {
		nome     string
		err      error
		esperado int
	}{
		{"chave ausente", apperr.ErrChaveAusente, http.StatusBadRequest},
		{"modelo inválido", apperr.ErrModeloInvalido, http.StatusBadRequest},
		{"arquivo inválido", apperr.ErrArquivoInvalido, http.StatusBadRequest},
		{"provedor inválido", apperr.ErrProvedorInvalido, http.StatusBadRequest},
		{"requisição inválida", apperr.ErrRequisicaoInvalida, http.StatusBadRequest},
		{"provedor recusou", apperr.ErrProvedorRecusou, http.StatusBadRequest},
		{"mês não encontrado", apperr.ErrMesNaoEncontrado, http.StatusNotFound},
		{"extração falhou", apperr.ErrExtracaoFalhou, http.StatusBadGateway},
		{"cliente desistiu", context.Canceled, 499},
		{"erro desconhecido", errors.New("algo inesperado"), http.StatusInternalServerError},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := statusDoErro(c.err); got != c.esperado {
				t.Errorf("statusDoErro(%v) = %d; esperado %d", c.err, got, c.esperado)
			}
		})
	}
}

// A classificação precisa sobreviver ao empacotamento: os erros atravessam três
// camadas até chegar ao handler, ganhando contexto no caminho.
func TestStatusDoErroAtravessaEnvelopamento(t *testing.T) {
	profundo := fmt.Errorf("no handler: %w",
		fmt.Errorf("no serviço: %w",
			fmt.Errorf("%w: image/gif", apperr.ErrArquivoInvalido)))

	if got := statusDoErro(profundo); got != http.StatusBadRequest {
		t.Errorf("statusDoErro = %d; esperado 400 mesmo com três camadas de envelope", got)
	}
}

func TestMascararChave(t *testing.T) {
	casos := map[string]string{
		"":                      "",
		"curta":                 "****",
		"12345678":              "****",
		"sk-or-v1-abcdefghijkl": "sk-o...ijkl",
	}

	for entrada, esperado := range casos {
		if got := mascarar(entrada); got != esperado {
			t.Errorf("mascarar(%q) = %q; esperado %q", entrada, got, esperado)
		}
	}
}
