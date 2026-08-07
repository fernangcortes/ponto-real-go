package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/fernangcortes/ponto-real-go/pkg/apperr"
	"github.com/fernangcortes/ponto-real-go/pkg/extraction"
)

// Estas validações moravam nas 107 linhas do handler HTTP. Aqui elas são
// testáveis sem montar requisição multipart.

var conteudoPNG = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

func TestNormalizarExigeChave(t *testing.T) {
	req := UploadRequest{
		Arquivo:  conteudoPNG,
		Filename: "ponto.png",
		Provider: extraction.ProviderOpenRouter,
	}

	err := req.normalizar()
	if !errors.Is(err, apperr.ErrChaveAusente) {
		t.Fatalf("err = %v; esperado ErrChaveAusente", err)
	}
	// A mensagem precisa dizer QUAL provedor configurar.
	if got := err.Error(); !strings.Contains(got, "OpenRouter") {
		t.Errorf("mensagem = %q; deveria nomear o provedor ativo", got)
	}
}

func TestNormalizarRecusaArquivoVazio(t *testing.T) {
	req := UploadRequest{Filename: "ponto.png", Provider: extraction.ProviderGemini, APIKey: "k"}

	if err := req.normalizar(); !errors.Is(err, apperr.ErrArquivoInvalido) {
		t.Errorf("err = %v; esperado ErrArquivoInvalido", err)
	}
}

func TestNormalizarInfereMimeQuandoGenerico(t *testing.T) {
	casos := []struct {
		nome     string
		mime     string
		filename string
		esperado string
	}{
		{"octet-stream cai nos magic bytes", "application/octet-stream", "sem-extensao", "image/png"},
		{"vazio usa a extensão", "", "ponto.PNG", "image/png"},
		{"informado é respeitado", "image/webp", "ponto.bin", "image/webp"},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			req := UploadRequest{
				Arquivo:  conteudoPNG,
				MimeType: c.mime,
				Filename: c.filename,
				Provider: extraction.ProviderGemini,
				APIKey:   "k",
			}
			if err := req.normalizar(); err != nil {
				t.Fatalf("normalizar: %v", err)
			}
			if req.MimeType != c.esperado {
				t.Errorf("MimeType = %q; esperado %q", req.MimeType, c.esperado)
			}
		})
	}
}

func TestNormalizarRecusaTipoNaoSuportado(t *testing.T) {
	req := UploadRequest{
		Arquivo:  []byte("PK\x03\x04conteudo"),
		MimeType: "application/vnd.ms-excel",
		Filename: "planilha.xlsx",
		Provider: extraction.ProviderGemini,
		APIKey:   "k",
	}

	err := req.normalizar()
	if !errors.Is(err, apperr.ErrArquivoInvalido) {
		t.Fatalf("err = %v; esperado ErrArquivoInvalido", err)
	}
	if got := err.Error(); !strings.Contains(got, "PNG") {
		t.Errorf("mensagem = %q; deveria dizer quais formatos servem", got)
	}
}

func TestNormalizarAplicaModeloPadraoDoProvedor(t *testing.T) {
	casos := map[string]string{
		extraction.ProviderGemini:     "gemini-2.5-flash",
		extraction.ProviderOpenRouter: "google/gemini-2.5-flash",
	}

	for provider, esperado := range casos {
		t.Run(provider, func(t *testing.T) {
			req := UploadRequest{
				Arquivo:  conteudoPNG,
				MimeType: "image/png",
				Filename: "ponto.png",
				Provider: provider,
				APIKey:   "k",
			}
			if err := req.normalizar(); err != nil {
				t.Fatalf("normalizar: %v", err)
			}
			if req.Modelo != esperado {
				t.Errorf("Modelo = %q; esperado %q", req.Modelo, esperado)
			}
		})
	}
}

func TestNormalizarRecusaModeloDeOutroProvedor(t *testing.T) {
	req := UploadRequest{
		Arquivo:  conteudoPNG,
		MimeType: "image/png",
		Filename: "ponto.png",
		// Modelo válido no catálogo do OpenRouter, não no do Gemini.
		Modelo:   "google/gemini-2.5-flash",
		Provider: extraction.ProviderGemini,
		APIKey:   "k",
	}

	if err := req.normalizar(); !errors.Is(err, apperr.ErrModeloInvalido) {
		t.Errorf("err = %v; esperado ErrModeloInvalido", err)
	}
}

func TestDetectMimeType(t *testing.T) {
	casos := []struct {
		filename string
		dados    []byte
		esperado string
	}{
		{"ponto.png", nil, "image/png"},
		{"ponto.JPEG", nil, "image/jpeg"},
		{"ponto.jpg", nil, "image/jpeg"},
		{"ponto.webp", nil, "image/webp"},
		{"ficha.pdf", nil, "application/pdf"},
		{"sem-extensao", conteudoPNG, "image/png"},
		{"sem-extensao", []byte{0xFF, 0xD8, 0xFF, 0xE0}, "image/jpeg"},
		{"sem-extensao", []byte("%PDF-1.7"), "application/pdf"},
		{"desconhecido", []byte("qualquer coisa"), "application/octet-stream"},
		{"vazio", nil, "application/octet-stream"},
	}

	for _, c := range casos {
		if got := DetectMimeType(c.filename, c.dados); got != c.esperado {
			t.Errorf("DetectMimeType(%q, %d bytes) = %q; esperado %q",
				c.filename, len(c.dados), got, c.esperado)
		}
	}
}
