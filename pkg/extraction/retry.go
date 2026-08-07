package extraction

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/fernangcortes/ponto-real-go/pkg/apperr"
	"github.com/fernangcortes/ponto-real-go/pkg/models"
)

// tentativasMax é o número de chamadas ao provedor antes de desistir.
const tentativasMax = 3

// backoffBase é o passo da espera entre tentativas: a n-ésima repetição espera
// n × backoffBase. É variável em vez de constante só para os testes poderem
// encurtá-la — em produção nunca é alterada.
var backoffBase = 2 * time.Second

// esperaRateLimit é a pausa extra quando o provedor devolve 429.
var esperaRateLimit = 5 * time.Second

// erroFinal marca um erro que não adianta repetir: chave inválida, saldo
// insuficiente, payload recusado. Repetir só faria o usuário esperar mais para
// receber a mesma resposta.
type erroFinal struct{ err error }

func (e erroFinal) Error() string { return e.err.Error() }
func (e erroFinal) Unwrap() error { return e.err }

// naoRepetir envolve um erro para que comTentativas pare imediatamente.
func naoRepetir(err error) error { return erroFinal{err} }

// comTentativas executa fn até tentativasMax vezes, com espera crescente entre
// as tentativas, abortando assim que o contexto for cancelado.
func comTentativas(
	ctx context.Context,
	fn func(ctx context.Context, tentativa int) (*models.Timesheet, error),
) (*models.Timesheet, error) {
	var ultimoErr error

	for i := 0; i < tentativasMax; i++ {
		if i > 0 {
			if err := esperar(ctx, time.Duration(i)*backoffBase); err != nil {
				return nil, err
			}
		}

		ts, err := fn(ctx, i+1)
		if err == nil {
			return ts, nil
		}

		var final erroFinal
		if errors.As(err, &final) {
			return nil, final.err
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		ultimoErr = err
	}

	// Esgotar as tentativas significa que o provedor respondeu, mas nunca de
	// forma utilizável. Não há o que o usuário configurar — daí ErrExtracaoFalhou
	// e não ErrProvedorRecusou.
	return nil, fmt.Errorf("%w após %d tentativas: %w", apperr.ErrExtracaoFalhou, tentativasMax, ultimoErr)
}

// esperar dorme pelo período pedido, mas retorna antes se o contexto for
// cancelado — um time.Sleep puro deixaria a requisição pendurada.
func esperar(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
