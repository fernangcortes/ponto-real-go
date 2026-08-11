package extraction

import (
	"sort"
	"testing"

	"github.com/fernangcortes/ponto-real-go/pkg/models"
	"github.com/fernangcortes/ponto-real-go/pkg/rules"
)

// Este arquivo trava os NÚMEROS que o gerador de horários produz hoje.
//
// A suíte que já existia confere invariantes: nada passa das 23:59, os pontos
// saem em ordem, nenhum batimento real desaparece, a entrada inventada não é de
// madrugada, o almoço inventado respeita o mínimo legal. Tudo isso continua
// valendo e continua sendo o que importa — mas nada disso olha o horário que
// saiu.
//
// A diferença é grande o bastante para ter sido medida: somar 45 minutos a toda
// saída inventada no ramo "falta a saída da tarde" deixava `go test ./...`
// inteiramente verde. Seriam 2h15 a mais em junho, num documento assinado por
// servidor público, sem nenhum portão reclamar.
//
// Daí este snapshot. Ele não afirma que os horários abaixo são os CERTOS: afirma
// que são os de hoje. Se um deles mudar, o teste quebra e alguém tem de decidir,
// de olho aberto, se a mudança é a pretendida — em vez de descobrir depois, na
// folha impressa.
//
// Como atualizar quando a mudança for proposital: rode o teste, confira caso a
// caso que o horário novo faz sentido (ordem, almoço, piso das 07:00, carga do
// dia) e só então troque o valor esperado.

// sementeDoSnapshot é arbitrária — qualquer número serve, desde que não mude.
// O que importa é que o sorteio pare de depender do relógio: sem semente fixa
// não há como repetir uma execução, e sem repetir não há como comparar.
const sementeDoSnapshot = 20260811

// casoDeSnapshot é um dia de entrada e o dia inteiro que sai dele.
type casoDeSnapshot struct {
	// ramo é o caminho de adjustDay que este caso exercita.
	ramo string
	// mascara são os batimentos preenchidos DEPOIS de reinterpretarPontos, na
	// ordem e1 s1 e2 s2. É ela que escolhe o ramo, e não a coluna em que o
	// horário chegou — por isso é afirmada aqui: se a reinterpretação mudar de
	// ideia sobre um dia, o caso deixa de cobrir o ramo que diz cobrir.
	mascara string
	// batidos é o que a extração entregou, coluna por coluna.
	batidos [4]string
	// esperado é o dia completo depois do ajuste, com a semente fixa.
	esperado [4]string
	// bloqueio marca quais horários são do servidor (1) e quais o sistema
	// inventou (0). É o que o front-end usa para dizer no que não se pode mexer.
	bloqueio [4]int
}

// casosDoSnapshot cobre as 14 combinações possíveis de batimentos — todas, como
// TestSnapshotCobreTodaCombinacaoPossivel garante.
//
// Onde havia dado real do usuário, ele foi usado: são os únicos ramos que
// comprovadamente já rodaram na vida real, e portanto onde um erro custa caro.
var casosDoSnapshot = []casoDeSnapshot{
	{
		ramo:     "só as pontas: o almoço inteiro é inventado",
		mascara:  "X..X",
		batidos:  [4]string{"08:34", "", "", "17:52"},
		esperado: [4]string{"08:34", "11:35", "12:46", "17:52"},
		bloqueio: [4]int{1, 0, 0, 1},
	},
	{
		ramo:     "só a entrada da manhã",
		mascara:  "X...",
		batidos:  [4]string{"08:34", "", "", ""},
		esperado: [4]string{"08:34", "12:14", "13:19", "17:53"},
		bloqueio: [4]int{1, 0, 0, 0},
	},
	{
		ramo:     "só a saída da tarde",
		mascara:  "...X",
		batidos:  [4]string{"", "", "", "17:52"},
		esperado: [4]string{"08:27", "12:26", "13:37", "17:52"},
		bloqueio: [4]int{0, 0, 0, 1},
	},
	{
		// O caso que revelou a entrada de madrugada (88b3f19): aqui o piso das
		// 07:00 entra em ação e a manhã é RECONTADA a partir dele. Sem isso a
		// tarde compensaria uma manhã que não aconteceu.
		ramo:     "só o retorno do almoço",
		mascara:  "..X.",
		batidos:  [4]string{"", "", "11:33", ""},
		esperado: [4]string{"07:27", "10:22", "11:33", "16:50"},
		bloqueio: [4]int{0, 0, 1, 0},
	},
	{
		ramo:     "manhã completa, falta a tarde",
		mascara:  "XX..",
		batidos:  [4]string{"08:34", "12:07", "", ""},
		esperado: [4]string{"08:34", "12:07", "13:18", "17:41"},
		bloqueio: [4]int{1, 1, 0, 0},
	},
	{
		ramo:     "tarde completa, falta a manhã",
		mascara:  "..XX",
		batidos:  [4]string{"", "", "13:41", "17:52"},
		esperado: [4]string{"08:31", "12:32", "13:41", "17:52"},
		bloqueio: [4]int{0, 0, 1, 1},
	},
	{
		// Dia 01/06/2026 do usuário, lido literalmente das colunas do PDF:
		// 11:30 é tarde demais para ser entrada da manhã, e a reinterpretação
		// empurra os três para saída-almoço/retorno/saída.
		ramo:     "falta a entrada da manhã (01/06 real)",
		mascara:  ".XXX",
		batidos:  [4]string{"11:30", "12:33", "20:06", ""},
		esperado: [4]string{"09:32", "11:30", "12:33", "20:06"},
		bloqueio: [4]int{0, 1, 1, 1},
	},
	{
		// Dia 16/06/2026 do usuário. O intervalo real entre entrada e retorno
		// não deixa espaço para uma manhã cheia mais o almoço, então a saída
		// para o almoço é colada no mínimo legal antes do retorno.
		ramo:     "falta a saída para o almoço (16/06 real)",
		mascara:  "X.XX",
		batidos:  [4]string{"08:58", "", "13:09", "19:13"},
		esperado: [4]string{"08:58", "12:01", "13:09", "19:13"},
		bloqueio: [4]int{1, 0, 1, 1},
	},
	{
		ramo:     "falta o retorno do almoço",
		mascara:  "XX.X",
		batidos:  [4]string{"08:34", "12:07", "", "17:52"},
		esperado: [4]string{"08:34", "12:07", "13:18", "17:52"},
		bloqueio: [4]int{1, 1, 0, 1},
	},
	{
		// Dia 02/06/2026 do usuário — o ramo que mais rodou na vida real
		// (02, 17 e 22 de junho).
		ramo:     "falta a saída da tarde (02/06 real)",
		mascara:  "XXX.",
		batidos:  [4]string{"09:11", "12:36", "13:37", ""},
		esperado: [4]string{"09:11", "12:36", "13:37", "18:19"},
		bloqueio: [4]int{1, 1, 1, 0},
	},
	{
		ramo:     "as duas entradas, nenhuma saída",
		mascara:  "X.X.",
		batidos:  [4]string{"08:34", "", "13:41", ""},
		esperado: [4]string{"08:34", "12:32", "13:41", "17:55"},
		bloqueio: [4]int{1, 0, 1, 0},
	},
	{
		ramo:     "as duas saídas, nenhuma entrada",
		mascara:  ".X.X",
		batidos:  [4]string{"", "10:47", "", "17:52"},
		esperado: [4]string{"07:07", "10:47", "11:52", "17:52"},
		bloqueio: [4]int{0, 1, 0, 1},
	},
	{
		// Cai no fallback, que registra "combinação não prevista" no log — e não
		// é combinação exótica nenhuma: é o dia em que só um batimento do meio
		// do dia foi lido.
		ramo:     "fallback: só a saída para o almoço",
		mascara:  ".X..",
		batidos:  [4]string{"10:47", "", "", ""},
		esperado: [4]string{"08:44", "10:47", "11:52", "18:03"},
		bloqueio: [4]int{0, 1, 0, 0},
	},
	{
		// O outro fallback: o servidor bateu na saída e na volta do almoço e
		// esqueceu as duas pontas.
		ramo:     "fallback: saída e retorno do almoço",
		mascara:  ".XX.",
		batidos:  [4]string{"11:30", "13:41", "", ""},
		esperado: [4]string{"08:44", "11:30", "13:41", "18:51"},
		bloqueio: [4]int{0, 1, 1, 0},
	},
}

// ajustaUmDia roda o ajuste sobre um único dia comum, com semente fixa.
func ajustaUmDia(semente int64, batidos [4]string) models.DayRecord {
	adjuster := newRulesAdjusterComSemente(rules.NewEngineWithDefaults(), semente)
	return adjuster.Adjust(&models.Timesheet{
		MesAno: "06/2026",
		Dias: []models.DayRecord{{
			Dia: 10, DiaSemana: "Qua",
			Entrada1: batidos[0], Saida1: batidos[1],
			Entrada2: batidos[2], Saida2: batidos[3],
		}},
	}).Dias[0]
}

// mascaraDeSlots desenha quais dos 4 batimentos estão preenchidos.
func mascaraDeSlots(pontos [4]int) string {
	m := make([]byte, 4)
	for i, p := range pontos {
		m[i] = '.'
		if p > 0 {
			m[i] = 'X'
		}
	}
	return string(m)
}

func TestSnapshotDosHorariosGerados(t *testing.T) {
	for _, caso := range casosDoSnapshot {
		t.Run(caso.ramo, func(t *testing.T) {
			e1, s1, e2, s2 := reinterpretarPontos(
				parseMins(caso.batidos[0]), parseMins(caso.batidos[1]),
				parseMins(caso.batidos[2]), parseMins(caso.batidos[3]))
			if got := mascaraDeSlots([4]int{e1, s1, e2, s2}); got != caso.mascara {
				t.Fatalf("a reinterpretação mudou de ideia sobre este dia: máscara %q, esperada %q\n"+
					"o caso deixou de exercitar o ramo que diz exercitar", got, caso.mascara)
			}

			d := ajustaUmDia(sementeDoSnapshot, caso.batidos)
			gerado := [4]string{d.Entrada1, d.Saida1, d.Entrada2, d.Saida2}

			if gerado != caso.esperado {
				t.Errorf("horários mudaram\nbatidos:   %v\nesperado:  %v\nveio:      %v\n"+
					"se a mudança é proposital, confira que o dia novo faz sentido antes de atualizar o valor",
					caso.batidos, caso.esperado, gerado)
			}

			if len(d.Bloqueio) != 4 || [4]int(d.Bloqueio) != caso.bloqueio {
				t.Errorf("mudou quem é batimento real e quem é inventado: esperado %v, veio %v",
					caso.bloqueio, d.Bloqueio)
			}
		})
	}
}

// TestSnapshotCobreTodaCombinacaoPossivel impede que um ramo fique de fora.
//
// A ficha tem 4 colunas, logo 16 combinações de preenchimento. Duas o adjustDay
// nunca vê — o dia vazio e o dia completo, que Adjust devolve intocados. As
// outras 14 são exatamente os caminhos que ele tem, e todas precisam estar na
// tabela: um ramo sem snapshot é um ramo onde a reescrita pode mudar horário
// sem que nada perceba.
func TestSnapshotCobreTodaCombinacaoPossivel(t *testing.T) {
	falta := map[string]bool{}
	for bits := 0; bits < 16; bits++ {
		var pontos [4]int
		for slot := 0; slot < 4; slot++ {
			if bits&(1<<slot) != 0 {
				pontos[slot] = 1
			}
		}
		m := mascaraDeSlots(pontos)
		if m == "...." || m == "XXXX" {
			continue
		}
		falta[m] = true
	}

	for _, caso := range casosDoSnapshot {
		if !falta[caso.mascara] {
			t.Errorf("máscara %q repetida ou impossível, no caso %q", caso.mascara, caso.ramo)
		}
		delete(falta, caso.mascara)
	}

	if len(falta) > 0 {
		var descobertas []string
		for m := range falta {
			descobertas = append(descobertas, m)
		}
		sort.Strings(descobertas)
		t.Errorf("combinações de batimentos sem snapshot: %v", descobertas)
	}
}

// TestMesmaSementeRepeteOResultado é o alicerce de tudo o que está acima: sem
// isso os valores esperados seriam sorte, não medida.
func TestMesmaSementeRepeteOResultado(t *testing.T) {
	for _, caso := range casosDoSnapshot {
		primeira := ajustaUmDia(sementeDoSnapshot, caso.batidos)
		segunda := ajustaUmDia(sementeDoSnapshot, caso.batidos)

		if primeira.Entrada1 != segunda.Entrada1 || primeira.Saida1 != segunda.Saida1 ||
			primeira.Entrada2 != segunda.Entrada2 || primeira.Saida2 != segunda.Saida2 {
			t.Errorf("%s: a mesma semente deu resultados diferentes: %s %s %s %s / %s %s %s %s",
				caso.ramo,
				primeira.Entrada1, primeira.Saida1, primeira.Entrada2, primeira.Saida2,
				segunda.Entrada1, segunda.Saida1, segunda.Entrada2, segunda.Saida2)
		}
	}
}

// TestSementesDiferentesGeramHorariosDiferentes protege o outro lado: em
// produção o horário inventado TEM de variar. Um horário sempre igual denuncia
// de longe que foi a máquina que o escreveu — e o dia 10 de todo mês sairia com
// a mesma saída da tarde.
func TestSementesDiferentesGeramHorariosDiferentes(t *testing.T) {
	iguais := 0
	for _, caso := range casosDoSnapshot {
		outra := ajustaUmDia(sementeDoSnapshot+1, caso.batidos)
		gerado := [4]string{outra.Entrada1, outra.Saida1, outra.Entrada2, outra.Saida2}
		if gerado == caso.esperado {
			iguais++
		}
	}
	if iguais == len(casosDoSnapshot) {
		t.Fatal("trocar a semente não mudou nenhum horário: o sorteio parou de sortear")
	}
}
