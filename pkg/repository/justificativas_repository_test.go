package repository

import (
	"testing"

	"github.com/fernangcortes/ponto-real-go/pkg/models"
)

func TestNormalizarFrasesDescartaVazias(t *testing.T) {
	frases := NormalizarFrases([]models.Justificativa{
		{Texto: "Cumpri a jornada."},
		{Texto: "   "},
		{Texto: ""},
	})

	if len(frases) != 1 {
		t.Fatalf("esperava 1 frase, veio %d: %+v", len(frases), frases)
	}
	if frases[0].Texto != "Cumpri a jornada." {
		t.Errorf("texto = %q", frases[0].Texto)
	}
}

func TestNormalizarFrasesTiraEspacoDasPontas(t *testing.T) {
	frases := NormalizarFrases([]models.Justificativa{
		{Texto: "  Cumpri a jornada.  "},
	})

	if frases[0].Texto != "Cumpri a jornada." {
		t.Errorf("esperava o texto sem espaço nas pontas, veio %q", frases[0].Texto)
	}
}

// A mesma frase salva de meses diferentes chegaria duas vezes na lista e viraria
// um item morto no seletor.
func TestNormalizarFrasesFundeRepetidasSomandoUsos(t *testing.T) {
	frases := NormalizarFrases([]models.Justificativa{
		{Texto: "Cumpri a jornada.", Tipo: "dispensa", Usos: 3},
		{Texto: "cumpri a JORNADA.", Usos: 2},
		{Texto: "Outra frase.", Usos: 1},
	})

	if len(frases) != 2 {
		t.Fatalf("esperava 2 frases após a fusão, veio %d: %+v", len(frases), frases)
	}
	if frases[0].Usos != 5 {
		t.Errorf("usos somados = %d, esperava 5", frases[0].Usos)
	}
	if frases[0].Texto != "Cumpri a jornada." {
		t.Errorf("a primeira grafia deve vencer, veio %q", frases[0].Texto)
	}
	if frases[0].Tipo != "dispensa" {
		t.Errorf("tipo perdido na fusão: %q", frases[0].Tipo)
	}
}

// Uma frase salva sem tipo e depois salva de novo num dia de dispensa passa a
// ordenar junto às de dispensa.
func TestNormalizarFrasesHerdaTipoDaRepetida(t *testing.T) {
	frases := NormalizarFrases([]models.Justificativa{
		{Texto: "Cumpri a jornada."},
		{Texto: "Cumpri a jornada.", Tipo: "dispensa"},
	})

	if frases[0].Tipo != "dispensa" {
		t.Errorf("esperava herdar o tipo, veio %q", frases[0].Tipo)
	}
}

func TestNormalizarFrasesPreservaAOrdem(t *testing.T) {
	frases := NormalizarFrases([]models.Justificativa{
		{Texto: "A"}, {Texto: "B"}, {Texto: "C"},
	})

	for i, esperado := range []string{"A", "B", "C"} {
		if frases[i].Texto != esperado {
			t.Errorf("posição %d = %q, esperava %q", i, frases[i].Texto, esperado)
		}
	}
}

// Primeira execução: o arquivo ainda não existe, e isso não é erro.
func TestJustificativasArquivoAusenteDevolveVazio(t *testing.T) {
	repo := NewJSONJustificativasRepository(t.TempDir() + "/nao-existe.json")

	b, err := repo.Load()
	if err != nil {
		t.Fatalf("arquivo ausente não deveria dar erro: %v", err)
	}
	if len(b.Frases) != 0 {
		t.Errorf("esperava biblioteca vazia, veio %+v", b.Frases)
	}
}

func TestJustificativasRoundTrip(t *testing.T) {
	repo := NewJSONJustificativasRepository(t.TempDir() + "/justificativas.json")

	original := models.BibliotecaJustificativas{Frases: []models.Justificativa{
		{Texto: "Cumpri a jornada de {jornada} exigida pelo ato.", Tipo: "dispensa", Usos: 2},
	}}
	if err := repo.Save(original); err != nil {
		t.Fatalf("Save: %v", err)
	}

	lida, err := repo.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(lida.Frases) != 1 || lida.Frases[0].Texto != original.Frases[0].Texto {
		t.Fatalf("frases não sobreviveram ao round-trip: %+v", lida.Frases)
	}
	if lida.Frases[0].Usos != 2 || lida.Frases[0].Tipo != "dispensa" {
		t.Errorf("metadados perdidos: %+v", lida.Frases[0])
	}
	if lida.UpdatedAt.IsZero() {
		t.Error("UpdatedAt não foi carimbado na gravação")
	}
}

// Apagar uma frase é gravar a lista sem ela: não pode haver fusão com o que já
// estava no disco, senão nada é excluível.
func TestJustificativasSaveSubstituiEmVezDeFundir(t *testing.T) {
	repo := NewJSONJustificativasRepository(t.TempDir() + "/justificativas.json")

	if err := repo.Save(models.BibliotecaJustificativas{Frases: []models.Justificativa{
		{Texto: "A"}, {Texto: "B"},
	}}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := repo.Save(models.BibliotecaJustificativas{Frases: []models.Justificativa{
		{Texto: "A"},
	}}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	lida, err := repo.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(lida.Frases) != 1 || lida.Frases[0].Texto != "A" {
		t.Errorf("a frase excluída voltou: %+v", lida.Frases)
	}
}
