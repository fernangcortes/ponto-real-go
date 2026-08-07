package api

import (
	"io"
	"log/slog"
	"os"
	"testing"
)

// TestMain silencia o log da aplicação durante os testes: o que interessa na
// saída é a falha do teste, não o rastro de execução do código sob teste.
// Rode com PONTO_REAL_TEST_LOG=1 para ver o log ao depurar.
func TestMain(m *testing.M) {
	if os.Getenv("PONTO_REAL_TEST_LOG") != "1" {
		slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	}
	os.Exit(m.Run())
}
