package extraction

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Testes das duas implementações de Extractor contra um servidor local.
// Cobrem o que a Fase 1 corrigiu: retry sem vazar corpo de resposta,
// cancelamento por contexto e erro não-repetível parando na hora.

// folhaJSON é uma resposta de extração válida, com um dia.
const folhaJSON = `{"version":1,"mes_ano":"06/2026","servidor":{"nome":"FULANO"},` +
	`"dias":[{"d":1,"w":"Seg","e1":"08:00","s1":"12:00","e2":"13:00","s2":"17:00"}]}`

func respostaGemini(texto string) string {
	b, _ := json.Marshal(map[string]any{
		"candidates": []any{
			map[string]any{"content": map[string]any{"parts": []any{map[string]any{"text": texto}}}},
		},
	})
	return string(b)
}

func respostaOpenRouter(texto string) string {
	b, _ := json.Marshal(map[string]any{
		"choices": []any{map[string]any{"message": map[string]any{"content": texto}}},
	})
	return string(b)
}

// backoffRapido encurta as esperas entre tentativas para o teste não gastar os
// 6 segundos reais de backoff.
func backoffRapido(t *testing.T) {
	t.Helper()
	base, rate := backoffBase, esperaRateLimit
	backoffBase, esperaRateLimit = time.Millisecond, time.Millisecond
	t.Cleanup(func() { backoffBase, esperaRateLimit = base, rate })
}

func novoGemini(t *testing.T, srv *httptest.Server) *GeminiExtractor {
	t.Helper()
	backoffRapido(t)
	e := NewGeminiExtractor("chave-secreta", "gemini-2.5-flash")
	e.baseURL = srv.URL
	e.client = srv.Client()
	return e
}

func novoOpenRouter(t *testing.T, srv *httptest.Server) *OpenRouterExtractor {
	t.Helper()
	backoffRapido(t)
	e := NewOpenRouterExtractor("chave-secreta", "google/gemini-2.5-flash")
	e.endpoint = srv.URL
	e.client = srv.Client()
	return e
}

// --- Caminho feliz e credenciais ---

func TestGeminiMandaChaveNoCabecalhoNaoNaURL(t *testing.T) {
	var gotHeader, gotQuery string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("x-goog-api-key")
		gotQuery = r.URL.RawQuery
		fmt.Fprint(w, respostaGemini(folhaJSON))
	}))
	defer srv.Close()

	ts, err := novoGemini(t, srv).Extract(context.Background(), []byte("arquivo"), "image/png")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(ts.Dias) != 1 {
		t.Errorf("len(Dias) = %d; esperado 1", len(ts.Dias))
	}

	if gotHeader != "chave-secreta" {
		t.Errorf("x-goog-api-key = %q; esperado a chave", gotHeader)
	}
	// A chave costumava viajar como ?key=... — URL vaza em log de proxy e histórico.
	if strings.Contains(gotQuery, "chave-secreta") {
		t.Errorf("a chave apareceu na query string: %q", gotQuery)
	}
}

func TestOpenRouterMandaBearer(t *testing.T) {
	var gotAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		fmt.Fprint(w, respostaOpenRouter(folhaJSON))
	}))
	defer srv.Close()

	if _, err := novoOpenRouter(t, srv).Extract(context.Background(), []byte("arquivo"), "application/pdf"); err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if gotAuth != "Bearer chave-secreta" {
		t.Errorf("Authorization = %q", gotAuth)
	}
}

// A resposta pode vir embrulhada em bloco de código markdown.
func TestRespostaEmMarkdownEhDesembrulhada(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, respostaGemini("```json\n"+folhaJSON+"\n```"))
	}))
	defer srv.Close()

	ts, err := novoGemini(t, srv).Extract(context.Background(), []byte("arquivo"), "image/png")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if ts.MesAno != "06/2026" {
		t.Errorf("MesAno = %q", ts.MesAno)
	}
}

// --- Retry ---

func TestGeminiRepeteAteConseguir(t *testing.T) {
	var chamadas int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&chamadas, 1) < 3 {
			fmt.Fprint(w, respostaGemini("isso não é JSON"))
			return
		}
		fmt.Fprint(w, respostaGemini(folhaJSON))
	}))
	defer srv.Close()

	ts, err := novoGemini(t, srv).Extract(context.Background(), []byte("arquivo"), "image/png")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if ts == nil || len(ts.Dias) != 1 {
		t.Fatalf("resultado inesperado: %+v", ts)
	}
	if n := atomic.LoadInt32(&chamadas); n != 3 {
		t.Errorf("chamadas = %d; esperado 3", n)
	}
}

func TestGeminiDesisteDepoisDeTresTentativas(t *testing.T) {
	var chamadas int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&chamadas, 1)
		fmt.Fprint(w, respostaGemini("isso nunca vai virar JSON"))
	}))
	defer srv.Close()

	_, err := novoGemini(t, srv).Extract(context.Background(), []byte("arquivo"), "image/png")
	if err == nil {
		t.Fatal("esperado erro depois de esgotar as tentativas")
	}
	if n := atomic.LoadInt32(&chamadas); n != tentativasMax {
		t.Errorf("chamadas = %d; esperado %d", n, tentativasMax)
	}
}

// Chave inválida não melhora repetindo: tem que falhar na primeira.
func TestGeminiErroDeCredencialNaoRepete(t *testing.T) {
	var chamadas int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&chamadas, 1)
		fmt.Fprint(w, `{"error":{"code":401,"message":"API key not valid","status":"UNAUTHENTICATED"}}`)
	}))
	defer srv.Close()

	_, err := novoGemini(t, srv).Extract(context.Background(), []byte("arquivo"), "image/png")
	if err == nil {
		t.Fatal("esperado erro")
	}
	if !strings.Contains(err.Error(), "API key not valid") {
		t.Errorf("erro = %v; esperado a mensagem do provedor", err)
	}
	if n := atomic.LoadInt32(&chamadas); n != 1 {
		t.Errorf("chamadas = %d; erro de credencial não pode ser repetido", n)
	}
}

func TestOpenRouterErroDaAPINaoRepete(t *testing.T) {
	var chamadas int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&chamadas, 1)
		fmt.Fprint(w, `{"error":{"message":"Insufficient credits"}}`)
	}))
	defer srv.Close()

	_, err := novoOpenRouter(t, srv).Extract(context.Background(), []byte("arquivo"), "image/png")
	if err == nil {
		t.Fatal("esperado erro")
	}
	if n := atomic.LoadInt32(&chamadas); n != 1 {
		t.Errorf("chamadas = %d; esperado 1", n)
	}
}

// Extração que devolve zero dias é falha de leitura, e vale repetir.
func TestZeroDiasEhRepetido(t *testing.T) {
	var chamadas int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&chamadas, 1) == 1 {
			fmt.Fprint(w, respostaGemini(`{"version":1,"mes_ano":"06/2026","dias":[]}`))
			return
		}
		fmt.Fprint(w, respostaGemini(folhaJSON))
	}))
	defer srv.Close()

	if _, err := novoGemini(t, srv).Extract(context.Background(), []byte("arquivo"), "image/png"); err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if n := atomic.LoadInt32(&chamadas); n != 2 {
		t.Errorf("chamadas = %d; esperado 2", n)
	}
}

// --- Cancelamento ---

// Antes não havia contexto: fechar a aba deixava o servidor esperando o
// provedor por até 120s, três vezes seguidas.
func TestCancelamentoInterrompeAChamada(t *testing.T) {
	liberado := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-liberado
	}))
	defer srv.Close()
	defer close(liberado)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	inicio := time.Now()
	_, err := novoGemini(t, srv).Extract(ctx, []byte("arquivo"), "image/png")
	decorrido := time.Since(inicio)

	if err == nil {
		t.Fatal("esperado erro de cancelamento")
	}
	// Sem cancelamento isso levaria os 120s do timeout do client, três vezes.
	if decorrido > 5*time.Second {
		t.Errorf("levou %v; o cancelamento deveria abortar quase imediatamente", decorrido)
	}
}

func TestContextoJaCanceladoNaoChamaOProvedor(t *testing.T) {
	var chamadas int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&chamadas, 1)
		fmt.Fprint(w, respostaGemini(folhaJSON))
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := novoGemini(t, srv).Extract(ctx, []byte("arquivo"), "image/png"); err == nil {
		t.Fatal("esperado erro com contexto já cancelado")
	}
	if n := atomic.LoadInt32(&chamadas); n != 0 {
		t.Errorf("chamadas = %d; contexto cancelado não deveria chegar ao provedor", n)
	}
}
