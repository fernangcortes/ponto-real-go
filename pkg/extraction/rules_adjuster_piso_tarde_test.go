package extraction

import (
	"testing"

	"github.com/fernangcortes/ponto-real-go/pkg/rules"
)

// O piso da tarde é a única coisa que impede a saída do expediente de sair ANTES
// do retorno do almoço, e este arquivo é o que segura essa garantia.
//
// Vale a pena registrar de onde ela veio, porque já morou noutro lugar. Até
// 2026-08-13 dois casos geravam a tarde sem piso nenhum: o dia em que só a saída
// para o almoço foi batida, e aquele em que foram batidas a saída e o retorno.
// Neles a tarde era só o que sobrava da jornada depois de uma manhã que começava
// numa entrada ARBITRADA — logo, uma saída para o almoço tarde demais consumia a
// jornada inteira e não sobrava tarde.
//
// Não acontecia, mas por acidente: `janelaSlot` limitava até que horas
// `reinterpretarPontos` aceitava chamar um ponto de "saída para o almoço", e a
// folga inteira eram 70 minutos. Uma garantia de outra função, sem aviso nenhum
// no lugar onde era exercida.
//
// Com `tardeMinima` valendo em todos os casos, a garantia passou a ser
// aritmética e local. Os testes abaixo a exercitam DIRETAMENTE em `adjustDay`,
// de propósito: não passam por `reinterpretarPontos`, então visitam também os
// dias que as janelas hoje filtram. É o que faz o piso ser testado como piso, e
// não como coincidência.

// tardeMinimaEsperada é o piso escrito à mão, de propósito.
//
// Comparar contra `tardeMinima` — a constante que estes testes existem para
// guardar — não guardaria nada: zerá-la deixaria a suíte verde, porque toda
// tarde é maior ou igual a zero. Foi o que aconteceu na primeira versão deste
// arquivo, e só a prova por mutação mostrou.
//
// Mudar o piso de propósito passa, então, por mudar este número também — e é
// justamente aí que se quer alguém de olho aberto.
const tardeMinimaEsperada = 3 * 60

// horariosHostis são saídas para o almoço tarde o bastante para consumir a
// jornada inteira. Antes do piso universal, as três últimas produziam tarde
// negativa; a de 13:30 é o antigo teto da janela, e a de 14:44 o horário mais
// tardio que a reinterpretação realmente alcança.
var horariosHostis = []int{
	13*60 + 30, // o teto de janelaSlot para a saída para o almoço
	14*60 + 44, // o mais tarde que a reinterpretação alcança, com dois pontos
	15*60 + 54, // a partir daqui a tarde zerava, quando não havia piso
	17 * 60,
	20 * 60,
	23*60 + 59, // o último minuto do dia
}

// TestPisoDaTardeSeguraATardeMesmoComManhaAbsurda é o teste que quebra se o piso
// for afrouxado em qualquer um dos casos que inventam a tarde.
func TestPisoDaTardeSeguraATardeMesmoComManhaAbsurda(t *testing.T) {
	// Os dois casos que dependiam de janelaSlot, e mais um em que a manhã real
	// pode ser longa o bastante para engolir a jornada.
	casos := []struct {
		nome string
		dia  func(s1 int) [4]int
	}{
		{"só a saída para o almoço (.X..)", func(s1 int) [4]int { return [4]int{0, s1, 0, 0} }},
		{"a saída e o retorno do almoço (.XX.)", func(s1 int) [4]int {
			e2 := s1 + 90
			if e2 > fimDoDia {
				e2 = fimDoDia
			}
			return [4]int{0, s1, e2, 0}
		}},
		{"entrada de madrugada e manhã inteira (XXX.)", func(s1 int) [4]int {
			return [4]int{1, s1, s1 + 61, 0}
		}},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			for _, s := range sorteiosDeterministicos() {
				for _, s1 := range horariosHostis {
					dia := caso.dia(s1)
					r := &RulesAdjuster{Engine: rules.NewEngineWithDefaults(), rng: s.fonte}
					e1, s1g, e2, s2, _ := r.adjustDay(dia[0], dia[1], dia[2], dia[3])

					tarde := s2 - e2

					// A garantia dura: a saída do expediente vem DEPOIS do
					// retorno do almoço. É a que morava em janelaSlot.
					if tarde <= 0 {
						t.Errorf(
							"a saída do expediente (%s) veio ANTES do retorno do almoço (%s).\n"+
								"\n"+
								"  dia recebido: %s   (sorteio %q)\n"+
								"  dia gerado:   %s\n"+
								"\n"+
								"A tarde gerada é o que falta para fechar a jornada depois da manhã.\n"+
								"Quando a manhã já consumiu a jornada inteira, sobra zero ou menos. O\n"+
								"piso da tarde é o que impede isso, e ele vale em TODOS os casos\n"+
								"justamente para que a garantia não dependa de nenhuma outra função.\n"+
								"\n"+
								"Antes de 2026-08-13 dois casos saíam sem piso, e o que os salvava era\n"+
								"o teto de janelaSlot para a saída para o almoço — 70 minutos de folga,\n"+
								"numa função que ninguém associa a esta. Se o piso voltar a ser\n"+
								"opcional, a garantia volta para lá.",
							formatMins(s2), formatMins(e2),
							quatroHorarios(dia), s.nome,
							quatroHorarios([4]int{e1, s1g, e2, s2}),
						)
						continue
					}

					// A política: 3 horas, e nem o desvio do minuto redondo pode
					// comê-la por cima.
					if tarde < tardeMinimaEsperada {
						t.Errorf(
							"a tarde saiu com %d min, abaixo do piso de %d.\n"+
								"\n"+
								"  dia recebido: %s   (sorteio %q)\n"+
								"  dia gerado:   %s\n"+
								"\n"+
								"Se o piso foi alterado de propósito, mude tardeMinimaEsperada junto —\n"+
								"e confira antes que os dias gerados continuam fazendo sentido.\n"+
								"Se não foi, o suspeito é a fuga do minuto redondo comendo o piso por\n"+
								"cima: ela precisa receber e2+tardeMinima como limite inferior, senão\n"+
								"desloca a saída para baixo dele. Foi o que já aconteceu com o almoço.",
							tarde, tardeMinimaEsperada,
							quatroHorarios(dia), s.nome,
							quatroHorarios([4]int{e1, s1g, e2, s2}),
						)
					}
				}
			}
		})
	}
}

// TestPisoDaTardeValeEmTodoCasoQueInventaATarde confere que nenhum caminho de
// adjustDay ficou de fora — inclusive os que hoje nunca encostam no piso.
//
// Um piso que vale em seis dos sete casos é exatamente a situação de onde se
// veio: parece uniforme, e o sétimo é o que quebra.
func TestPisoDaTardeValeEmTodoCasoQueInventaATarde(t *testing.T) {
	for _, s := range sorteiosDeterministicos() {
		paraCadaDiaDaGrade(func(dia [4]int) {
			// Só interessam os dias em que a saída da tarde é inventada e o
			// retorno do almoço já existe ou também é inventado.
			if dia[3] > 0 {
				return
			}

			r := &RulesAdjuster{Engine: rules.NewEngineWithDefaults(), rng: s.fonte}
			e1, s1, e2, s2, _ := r.adjustDay(dia[0], dia[1], dia[2], dia[3])
			if e2 == 0 || s2 == 0 {
				return
			}

			if s2-e2 < tardeMinimaEsperada {
				t.Fatalf("tarde de %d min, abaixo do piso de %d\n  recebido: %s   (sorteio %q)\n  gerado:   %s",
					s2-e2, tardeMinimaEsperada, quatroHorarios(dia), s.nome,
					quatroHorarios([4]int{e1, s1, e2, s2}))
			}
		})
	}
}
