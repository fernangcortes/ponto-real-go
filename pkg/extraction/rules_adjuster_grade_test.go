package extraction

import (
	"testing"

	"github.com/fernangcortes/ponto-real-go/pkg/models"
	"github.com/fernangcortes/ponto-real-go/pkg/rules"
)

// Ferramentas para exercitar o gerador de horários sem depender da sorte.
//
// Com um sorteio que não guarda estado — sempre o mínimo, sempre o máximo,
// sempre o meio da faixa pedida — adjustDay vira uma função pura dos
// batimentos. Duas coisas passam a ser possíveis: comparar duas implementações
// horário a horário, e visitar de propósito as pontas de cada faixa sorteada,
// que o sorteio de verdade só alcança por acaso.

// sorteioFixo devolve sempre a mesma posição dentro da faixa pedida.
type sorteioFixo func(n int) int

func (s sorteioFixo) Intn(n int) int {
	if n <= 0 {
		return 0
	}
	return s(n)
}

type sorteioNomeado struct {
	nome  string
	fonte sorteador
}

// sorteiosDeterministicos são os três extremos que interessam: o menor valor de
// cada faixa, o maior, e o do meio. O sinal do desvio do minuto redondo, que sai
// de um Intn(2), fica negativo no primeiro e positivo nos outros dois.
func sorteiosDeterministicos() []sorteioNomeado {
	return []sorteioNomeado{
		{"sempre o mínimo", sorteioFixo(func(int) int { return 0 })},
		{"sempre o máximo", sorteioFixo(func(n int) int { return n - 1 })},
		{"sempre o meio", sorteioFixo(func(n int) int { return (n - 1) / 2 })},
	}
}

// horariosDaGrade são horários escolhidos nas bordas — as fronteiras das
// janelas de janelaSlot, os horários contratuais, o piso das 07:00, o último
// minuto do dia — mais os que já causaram defeito de verdade.
var horariosDaGrade = []int{
	6 * 60,     // 06:00 — início da janela da entrada
	7 * 60,     // 07:00 — o piso da entrada inventada
	7*60 + 29,  // 07:29
	8*60 + 30,  // 08:30 — entrada contratual
	10*60 + 29, // 10:29
	10*60 + 30, // 10:30 — fim da janela da entrada
	10*60 + 31, // 10:31 — primeiro minuto que cai no fallback
	11*60 + 29, // 11:29
	11*60 + 30, // 11:30 — início da janela do retorno do almoço
	11*60 + 33, // 11:33 — o retorno que puxava a entrada para a madrugada
	12 * 60,    // 12:00 — saída contratual para o almoço
	13 * 60,    // 13:00 — retorno contratual
	13*60 + 29, // 13:29
	13*60 + 30, // 13:30 — fim da janela da saída para o almoço
	13*60 + 31, // 13:31
	14*60 + 30, // 14:30 — o retorno que encurtava o almoço abaixo do mínimo
	15*60 + 59, // 15:59
	16 * 60,    // 16:00 — fim da janela do retorno, início da janela da saída
	16*60 + 1,  // 16:01
	17*60 + 30, // 17:30 — saída contratual
	18*60 + 40, // 18:40
	20*60 + 6,  // 20:06 — a saída real do dia 01/06
	22 * 60,    // 22:00
	23*60 + 59, // 23:59 — o último minuto do dia
}

// paraCadaDiaDaGrade chama fn com toda combinação de batimentos que adjustDay
// pode receber: as 14 máscaras possíveis — 16 menos o dia vazio e o dia
// completo, que Adjust nem lhe entrega — com os horários em ordem crescente,
// que é como reinterpretarPontos sempre os entrega.
func paraCadaDiaDaGrade(fn func([4]int)) {
	for bits := 1; bits <= 14; bits++ {
		var slots []int
		for slot := 0; slot < 4; slot++ {
			if bits&(1<<slot) != 0 {
				slots = append(slots, slot)
			}
		}
		combinacoesCrescentes(len(slots), func(escolha []int) {
			var dia [4]int
			for i, slot := range slots {
				dia[slot] = horariosDaGrade[escolha[i]]
			}
			fn(dia)
		})
	}
}

// TestInvariantesNasPontasDoSorteio passa a grade inteira pelo caminho de
// produção com o sorteio preso nos extremos de cada faixa.
//
// As invariantes conferidas são as que a suíte já cobre — dentro do dia, em
// ordem, entrada inventada nunca de madrugada. O que muda é que ali o sorteio é
// de verdade e só alcança as pontas por acaso, e aqui elas são visitadas de
// propósito, em toda combinação de batimentos.
func TestInvariantesNasPontasDoSorteio(t *testing.T) {
	for _, s := range sorteiosDeterministicos() {
		t.Run(s.nome, func(t *testing.T) {
			paraCadaDiaDaGrade(func(dia [4]int) {
				adjuster := &RulesAdjuster{Engine: rules.NewEngineWithDefaults(), rng: s.fonte}
				out := adjuster.Adjust(&models.Timesheet{
					MesAno: "06/2026",
					Dias: []models.DayRecord{{
						Dia: 10, DiaSemana: "Qua",
						Entrada1: formatMins(dia[0]), Saida1: formatMins(dia[1]),
						Entrada2: formatMins(dia[2]), Saida2: formatMins(dia[3]),
					}},
				}).Dias[0]

				pontos := [4]int{
					parseMins(out.Entrada1), parseMins(out.Saida1),
					parseMins(out.Entrada2), parseMins(out.Saida2),
				}
				saiu := func() string {
					return quatroHorarios(dia) + " -> " + quatroHorarios(pontos)
				}

				anterior := 0
				for _, p := range pontos {
					if p == 0 {
						continue
					}
					if p > fimDoDia {
						t.Fatalf("horário fora do dia: %s", saiu())
					}
					if p < anterior {
						t.Fatalf("horários fora de ordem: %s", saiu())
					}
					anterior = p
				}

				if len(out.Bloqueio) == 4 && out.Bloqueio[0] == 0 &&
					pontos[0] > 0 && pontos[0] < 7*60 {
					t.Fatalf("entrada inventada de madrugada: %s", saiu())
				}
			})
		})
	}
}

func quatroHorarios(v [4]int) string {
	fora := "["
	for i, m := range v {
		if i > 0 {
			fora += " "
		}
		if m <= 0 {
			fora += "--:--"
		} else {
			fora += formatMins(m)
		}
	}
	return fora + "]"
}

// combinacoesCrescentes percorre as escolhas de k horários distintos da grade,
// sempre em ordem crescente.
func combinacoesCrescentes(k int, fn func([]int)) {
	escolha := make([]int, k)
	var caminha func(pos, inicio int)
	caminha = func(pos, inicio int) {
		if pos == k {
			fn(escolha)
			return
		}
		for i := inicio; i < len(horariosDaGrade); i++ {
			escolha[pos] = i
			caminha(pos+1, i+1)
		}
	}
	caminha(0, 0)
}
