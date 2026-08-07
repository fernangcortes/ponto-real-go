package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/fernangcortes/ponto-real-go/pkg/models"
	"github.com/fernangcortes/ponto-real-go/pkg/repository"
	"github.com/fernangcortes/ponto-real-go/pkg/rules"
)

// Este arquivo e web/js/regras-compartilhadas.test.js rodam os MESMOS casos
// (testdata/regras.json) contra as duas implementações de regra que o projeto
// tem — o motor Go e o domínio do front-end — e conferem contra os MESMOS
// valores esperados.
//
// É o que impede a duplicação de divergir de novo. Antes de existir, as duas
// discordavam em 72 horas num único mês e ninguém percebia: férias homologadas
// eram dia neutro na tela e falta no servidor.
//
// Fica em pkg/service, e não em pkg/rules, porque o realinhamento pelo
// calendário real acontece no serviço — e é ele que decide se um dia sem
// batimento é falta ou folga. Testar só o motor deixaria essa parte de fora.

type casoCompartilhado struct {
	Nome      string           `json:"nome"`
	Dia       models.DayRecord `json:"dia"`
	Tipo      string           `json:"tipo"`
	Contribui int              `json:"contribui"`
}

type fixtureRegras struct {
	MesAno string              `json:"mes_ano"`
	Casos  []casoCompartilhado `json:"casos"`
}

// tipoNaTela traduz o vocabulário do Go para o do front-end, que é o usado no
// arquivo de casos. Os dois nomes existem por história: unificá-los mudaria o
// formato dos meses já salvos em disco.
var tipoNaTela = map[models.DayType]string{
	models.DayTypeCompleto:           "completo",
	models.DayTypeParcial:            "parcial",
	models.DayTypeFalta:              "falta",
	models.DayTypeFolga:              "folga",
	models.DayTypeFeriado:            "recesso",
	models.DayTypeRecesso:            "recesso",
	models.DayTypeFerias:             "ferias",
	models.DayTypeDispensa:           "dispensa",
	models.DayTypeExpedienteReduzido: "reduzido",
}

func carregarFixture(t *testing.T) fixtureRegras {
	t.Helper()

	bytes, err := os.ReadFile(filepath.Join("..", "..", "testdata", "regras.json"))
	if err != nil {
		t.Fatalf("lendo testdata/regras.json: %v", err)
	}

	var f fixtureRegras
	if err := json.Unmarshal(bytes, &f); err != nil {
		t.Fatalf("parseando testdata/regras.json: %v", err)
	}
	if len(f.Casos) == 0 {
		t.Fatal("nenhum caso no arquivo compartilhado")
	}
	return f
}

func TestRegrasCompartilhadas(t *testing.T) {
	svc := NewTimesheetService(
		rules.NewEngineWithDefaults(),
		repository.NoopTimesheetRepository{},
		nil,
	)
	fixture := carregarFixture(t)

	for _, caso := range fixture.Casos {
		t.Run(caso.Nome, func(t *testing.T) {
			// Um dia por vez: o saldo do mês vira a contribuição daquele dia.
			resp, err := svc.Process(models.ProcessRequest{
				MesAno: fixture.MesAno,
				Dias:   []models.DayRecord{caso.Dia},
			})
			if err != nil {
				t.Fatalf("Process: %v", err)
			}

			dia := resp.Timesheet.Dias[0]
			if got := tipoNaTela[dia.Tipo]; got != caso.Tipo {
				t.Errorf("tipo = %q (Go: %q); esperado %q", got, dia.Tipo, caso.Tipo)
			}
			if got := resp.Summary.SaldoTotalRealMinutos; got != caso.Contribui {
				t.Errorf("contribuição ao saldo real = %d; esperado %d", got, caso.Contribui)
			}
		})
	}
}

// O fixture só serve ao seu propósito se o front-end também o consumir. Este
// teste falha se alguém mover ou renomear o teste espelho do lado JS.
func TestFrontEndConsomeOMesmoFixture(t *testing.T) {
	caminho := filepath.Join("..", "..", "web", "js", "regras-compartilhadas.test.js")
	if _, err := os.Stat(caminho); err != nil {
		t.Errorf("o teste espelho do front-end não foi encontrado em %s: %v", caminho, err)
	}
}
