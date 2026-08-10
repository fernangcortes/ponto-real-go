package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"unicode/utf8"

	"github.com/fernangcortes/ponto-real-go/pkg/app"
	"github.com/fernangcortes/ponto-real-go/pkg/version"
)

//go:embed all:web
var webFS embed.FS

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	app.ConfigurarLog(os.Getenv("PONTO_REAL_DEBUG") == "1")

	webContent, err := fs.Sub(webFS, "web")
	if err != nil {
		slog.Error("não foi possível montar o front-end embutido", "erro", err)
		os.Exit(1)
	}

	servidor := app.BuildServer(app.ConfigFromEnv(), webContent)

	imprimirBanner(port, servidor.Regras.NomeInstituicao)

	if err := http.ListenAndServe(":"+port, servidor.Handler); err != nil {
		slog.Error("servidor encerrado", "erro", err)
		os.Exit(1)
	}
}

// larguraCaixa é o espaço útil entre as bordas do banner.
const larguraCaixa = 38

func imprimirBanner(port, instituicao string) {
	fmt.Printf("\n")
	fmt.Printf("  ╔%s╗\n", repetir("═", larguraCaixa))
	fmt.Printf("  ║%s║\n", centralizar("🕐 Ponto Real Go v"+version.Atual, larguraCaixa))
	fmt.Printf("  ╠%s╣\n", repetir("═", larguraCaixa))
	fmt.Printf("  ║%s║\n", alinhar("  Server:  http://localhost:"+port, larguraCaixa))
	fmt.Printf("  ║%s║\n", alinhar("  API:     http://localhost:"+port+"/api", larguraCaixa))
	fmt.Printf("  ║%s║\n", alinhar("  Regras:  "+instituicao, larguraCaixa))
	fmt.Printf("  ╚%s╝\n", repetir("═", larguraCaixa))
	fmt.Printf("\n")
}

// alinhar corta ou completa o texto para caber exatamente na largura da caixa.
//
// Conta runas, não bytes: o nome da instituição vem das regras e costuma ter
// acentos. A versão anterior fatiava com s[:30], o que estourava com nome curto
// e cortava acento no meio com nome acentuado.
func alinhar(texto string, largura int) string {
	if utf8.RuneCountInString(texto) > largura {
		return string([]rune(texto)[:largura-1]) + "…"
	}
	return texto + repetir(" ", largura-utf8.RuneCountInString(texto))
}

func centralizar(texto string, largura int) string {
	n := utf8.RuneCountInString(texto)
	if n >= largura {
		return alinhar(texto, largura)
	}
	esquerda := (largura - n) / 2
	return repetir(" ", esquerda) + texto + repetir(" ", largura-n-esquerda)
}

func repetir(s string, n int) string {
	if n <= 0 {
		return ""
	}
	out := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}
