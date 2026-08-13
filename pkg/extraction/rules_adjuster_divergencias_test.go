package extraction

import (
	"fmt"
	"sort"
	"testing"

	"github.com/fernangcortes/ponto-real-go/pkg/rules"
)

// Ferramenta de MEDIÇÃO das quatro divergências herdadas, para que a decisão
// sobre unificá-las possa ser tomada com o número na frente em vez de no
// abstrato.
//
// ESTE ARQUIVO TEM PRAZO DE VALIDADE. Ele existe enquanto as quatro divergências
// estiverem em aberto. Decididas as quatro, `constantesDeHoje()` deixa de
// descrever o gerador, `TestVarianteReproduzOAjustadorDeHoje` quebra dizendo
// isso, e o arquivo inteiro pode ser apagado — não há nada aqui que proteja o
// código, só o que mede uma escolha pendente.
//
// Os relatórios só imprimem com `-v`, para não poluir o `go test ./...`:
//
//	go test ./pkg/extraction -run TestRelatorio -v
//	go test ./pkg/extraction -run TestSonda -v
//
// Ela não muda nada no gerador: `adjustDayCom` é uma cópia de `adjustDay` em que
// as quatro constantes viram parâmetro. Com `constantesDeHoje()` ela TEM de
// devolver exatamente o que `adjustDay` devolve — é o que
// TestVarianteReproduzOAjustadorDeHoje prova, sobre a grade inteira e sobre as
// 14 linhas do snapshot. Sem essa prova, qualquer diferença medida depois
// poderia ser erro da cópia, e não efeito da constante.

// constantes reúne as quatro divergências, indexadas pela máscara de batimentos
// do caso em que aparecem.
type constantes struct {
	// 1: piso da tarde — 3h, 2h ou nenhum, conforme o caso.
	piso map[string]int
	// 2: faixa da manhã sorteada solta — 220–260 num caso, 200–240 noutro.
	manhaSolta map[string][2]int
	// 3: se o desvio de −5 a +15 min entra antes de conferir o piso.
	desvioAntesDoPiso map[string]bool
	// 4: se o dia com as duas saídas (.X.X) calcula a entrada pela carga, como
	// fazem os outros casos de tarde conhecida, em vez de sorteá-la solta.
	entradaPelaCargaNasDuasSaidas bool
}

// mascarasComPisoDeTarde são os casos que chamam saidaDaTarde, e portanto os
// únicos onde o piso e a ordem do desvio têm efeito.
var mascarasComPisoDeTarde = []string{"XXX.", "XX..", "X.X.", "X...", "..X.", ".X..", ".XX."}

func constantesDeHoje() constantes {
	return constantes{
		piso: map[string]int{
			"XXX.": 180, "XX..": 180, "X.X.": 180, "X...": 180,
			"..X.": 120,
			".X..": semPiso, ".XX.": semPiso,
		},
		manhaSolta: map[string][2]int{
			"..X.": {220, 260},
			".X.X": {200, 240},
		},
		desvioAntesDoPiso: map[string]bool{
			"XXX.": true, "XX..": true, "X.X.": true,
			"X...": false,
			"..X.": true, ".X..": true, ".XX.": true,
		},
		entradaPelaCargaNasDuasSaidas: false,
	}
}

// com devolve uma cópia com as alterações aplicadas, sem tocar no original.
func (c constantes) com(f func(*constantes)) constantes {
	novo := constantes{
		piso:                          map[string]int{},
		manhaSolta:                    map[string][2]int{},
		desvioAntesDoPiso:             map[string]bool{},
		entradaPelaCargaNasDuasSaidas: c.entradaPelaCargaNasDuasSaidas,
	}
	for k, v := range c.piso {
		novo.piso[k] = v
	}
	for k, v := range c.manhaSolta {
		novo.manhaSolta[k] = v
	}
	for k, v := range c.desvioAntesDoPiso {
		novo.desvioAntesDoPiso[k] = v
	}
	f(&novo)
	return novo
}

// adjustDayCom é adjustDay com as quatro divergências como parâmetro. Toda linha
// que não envolve uma delas é idêntica ao original, de propósito.
func (r *RulesAdjuster) adjustDayCom(e1, s1, e2, s2 int, c constantes) (int, int, int, int) {
	switch {
	case e1 == 0 && s1 > 0 && e2 > 0 && s2 > 0: // .XXX
		e1 = r.entradaPelaCarga(s1, s2-e2)

	case e1 > 0 && s1 == 0 && e2 > 0 && s2 > 0: // X.XX
		s1 = naoAntesDaEntrada(r.saidaParaAlmocoEspremida(e1, e2), e1, e2)

	case e1 > 0 && s1 > 0 && e2 == 0 && s2 > 0: // XX.X
		e2 = r.retornoDoAlmoco(s1, r.almocoGerado(), s2)

	case e1 > 0 && s1 > 0 && e2 > 0 && s2 == 0: // XXX.
		s2 = r.saidaDaTarde(e2, s1-e1, c.piso["XXX."], c.desvioAntesDoPiso["XXX."])

	case e1 > 0 && s1 == 0 && e2 == 0 && s2 > 0: // X..X
		almoco := r.almocoGerado()
		s1 = r.saidaParaAlmocoEntreAsPontas(e1, s2, almoco)
		e2 = r.retornoDoAlmoco(s1, almoco, s2)

	case e1 > 0 && s1 > 0 && e2 == 0 && s2 == 0: // XX..
		e2 = r.retornoDoAlmoco(s1, r.almocoGerado(), 0)
		s2 = r.saidaDaTarde(e2, s1-e1, c.piso["XX.."], c.desvioAntesDoPiso["XX.."])

	case e1 == 0 && s1 == 0 && e2 > 0 && s2 > 0: // ..XX
		s1 = r.saidaParaAlmocoAntesDoRetorno(e2, r.almocoGerado())
		e1 = r.entradaPelaCarga(s1, s2-e2)

	case e1 > 0 && s1 == 0 && e2 > 0 && s2 == 0: // X.X.
		s1 = naoAntesDaEntrada(r.saidaParaAlmocoAntesDoRetorno(e2, r.almocoGerado()), e1, e2)
		s2 = r.saidaDaTarde(e2, s1-e1, c.piso["X.X."], c.desvioAntesDoPiso["X.X."])

	case e1 == 0 && s1 > 0 && e2 == 0 && s2 > 0: // .X.X
		if c.entradaPelaCargaNasDuasSaidas {
			e2 = r.retornoDoAlmoco(s1, r.almocoGerado(), s2)
			e1 = r.entradaPelaCarga(s1, s2-e2)
		} else {
			faixa := c.manhaSolta[".X.X"]
			e1, _ = r.entradaAntesDaSaidaParaOAlmoco(s1, faixa[0], faixa[1])
			e2 = r.retornoDoAlmoco(s1, r.almocoGerado(), s2)
		}

	case e1 > 0 && s1 == 0 && e2 == 0 && s2 == 0: // X...
		s1 = r.saidaParaAlmocoDepoisDaEntrada(e1)
		e2 = r.retornoDoAlmoco(s1, r.almocoGerado(), 0)
		s2 = r.saidaDaTarde(e2, s1-e1, c.piso["X..."], c.desvioAntesDoPiso["X..."])

	case e1 == 0 && s1 == 0 && e2 == 0 && s2 > 0: // ...X
		almoco := r.almocoGerado()
		var tarde int
		e2, tarde = r.retornoAPartirDaSaida(s2)
		s1 = r.saidaParaAlmocoAntesDoRetorno(e2, almoco)
		e1 = r.entradaPelaCarga(s1, tarde)

	case e1 == 0 && s1 == 0 && e2 > 0 && s2 == 0: // ..X.
		var manha int
		s1 = r.saidaParaAlmocoAntesDoRetorno(e2, r.almocoGerado())
		faixa := c.manhaSolta["..X."]
		e1, manha = r.entradaAntesDaSaidaParaOAlmoco(s1, faixa[0], faixa[1])
		s2 = r.saidaDaTarde(e2, manha, c.piso["..X."], c.desvioAntesDoPiso["..X."])

	case e1 == 0 && s1 > 0 && e2 == 0 && s2 == 0: // .X..
		e1 = r.entradaArbitrada()
		e2 = r.retornoDoAlmoco(s1, r.almocoGerado(), 0)
		s2 = r.saidaDaTarde(e2, s1-e1, c.piso[".X.."], c.desvioAntesDoPiso[".X.."])

	case e1 == 0 && s1 > 0 && e2 > 0 && s2 == 0: // .XX.
		e1 = r.entradaArbitrada()
		s2 = r.saidaDaTarde(e2, s1-e1, c.piso[".XX."], c.desvioAntesDoPiso[".XX."])

	default:
	}

	return e1, s1, e2, s2
}

// paraCadaDiaAlcancavel percorre só os dias que adjustDay pode REALMENTE receber
// em produção.
//
// A grade crua chama adjustDay direto, e por isso inclui dias que Adjust nunca
// entregaria: uma manhã de 06:00 às 22:00, por exemplo. Adjust sempre passa por
// reinterpretarPontos antes, e a saída dela é o único que adjustDay vê — logo os
// dias alcançáveis são exatamente os pontos fixos da reinterpretação.
//
// A diferença não é acadêmica: sem o filtro, o pior caso medido para o piso da
// tarde era de 660 minutos de saldo inventado, num dia que não existe.
func paraCadaDiaAlcancavel(fn func([4]int)) {
	paraCadaDiaDaGrade(func(dia [4]int) {
		e1, s1, e2, s2 := reinterpretarPontos(dia[0], dia[1], dia[2], dia[3])
		if [4]int{e1, s1, e2, s2} == dia {
			fn(dia)
		}
	})
}

// TestReinterpretacaoEIdempotente sustenta o filtro acima: se a reinterpretação
// mudasse de ideia ao ser aplicada duas vezes, "ponto fixo" não descreveria o
// que adjustDay recebe.
func TestReinterpretacaoEIdempotente(t *testing.T) {
	paraCadaDiaDaGrade(func(dia [4]int) {
		a1, b1, c1, d1 := reinterpretarPontos(dia[0], dia[1], dia[2], dia[3])
		a2, b2, c2, d2 := reinterpretarPontos(a1, b1, c1, d1)
		if [4]int{a1, b1, c1, d1} != [4]int{a2, b2, c2, d2} {
			t.Fatalf("reinterpretação não é idempotente: %s -> %s -> %s",
				quatroHorarios(dia), quatroHorarios([4]int{a1, b1, c1, d1}),
				quatroHorarios([4]int{a2, b2, c2, d2}))
		}
	})
}

// sorteiosDaMedicao são os sete regimes determinísticos: com um sorteio que não
// guarda estado, adjustDay vira função pura dos batimentos e duas versões podem
// ser postas lado a lado sem que a ORDEM dos sorteios interfira.
func sorteiosDaMedicao() []sorteioNomeado {
	return []sorteioNomeado{
		{"mínimo", sorteioFixo(func(int) int { return 0 })},
		{"máximo", sorteioFixo(func(n int) int { return n - 1 })},
		{"meio", sorteioFixo(func(n int) int { return (n - 1) / 2 })},
		{"1/4", sorteioFixo(func(n int) int { return (n - 1) / 4 })},
		{"3/4", sorteioFixo(func(n int) int { return 3 * (n - 1) / 4 })},
		{"1/3", sorteioFixo(func(n int) int { return (n - 1) / 3 })},
		{"2/3", sorteioFixo(func(n int) int { return 2 * (n - 1) / 3 })},
	}
}

// rodaComVariante roda um dia por adjustDayCom com o sorteio dado.
func rodaComVariante(dia [4]int, s sorteador, c constantes) [4]int {
	r := &RulesAdjuster{Engine: rules.NewEngineWithDefaults(), rng: s}
	e1, s1, e2, s2 := r.adjustDayCom(dia[0], dia[1], dia[2], dia[3], c)
	return [4]int{e1, s1, e2, s2}
}

// rodaComOAtual roda o mesmo dia pelo adjustDay de produção.
func rodaComOAtual(dia [4]int, s sorteador) [4]int {
	r := &RulesAdjuster{Engine: rules.NewEngineWithDefaults(), rng: s}
	e1, s1, e2, s2, _ := r.adjustDay(dia[0], dia[1], dia[2], dia[3])
	return [4]int{e1, s1, e2, s2}
}

// TestVarianteReproduzOAjustadorDeHoje é o alicerce de toda medição abaixo: com
// as constantes de hoje, a cópia parametrizada tem de devolver exatamente o que
// o gerador de produção devolve.
//
// Duas provas, porque provam coisas diferentes:
//
//   - a grade inteira × 7 regimes determinísticos prova que os VALORES batem;
//   - as 14 linhas do snapshot com a semente de verdade provam que a ORDEM em
//     que os sorteios são consumidos também bate — sem isso a equivalência
//     valeria só sob sorteio sem estado.
func TestVarianteReproduzOAjustadorDeHoje(t *testing.T) {
	hoje := constantesDeHoje()

	comparacoes := 0
	for _, s := range sorteiosDaMedicao() {
		paraCadaDiaDaGrade(func(dia [4]int) {
			atual := rodaComOAtual(dia, s.fonte)
			variante := rodaComVariante(dia, s.fonte, hoje)
			if atual != variante {
				t.Fatalf("a cópia divergiu do gerador de hoje (%s)\n  entrada:  %s\n  produção: %s\n  cópia:    %s",
					s.nome, quatroHorarios(dia), quatroHorarios(atual), quatroHorarios(variante))
			}
			comparacoes++
		})
	}
	t.Logf("valores idênticos em %d comparações (grade × 7 regimes)", comparacoes)

	for _, caso := range casosDoSnapshot {
		got := ajustaComConstantes(sementeDoSnapshot, caso.batidos, hoje)
		if got != caso.esperado {
			t.Fatalf("%s: a ordem dos sorteios divergiu\n  esperado: %v\n  veio:     %v",
				caso.ramo, caso.esperado, got)
		}
	}
	t.Logf("ordem dos sorteios idêntica nas %d linhas do snapshot", len(casosDoSnapshot))
}

// ajustaComConstantes reproduz o caminho de Adjust para um dia, mas passando
// pela cópia parametrizada.
func ajustaComConstantes(semente int64, batidos [4]string, c constantes) [4]string {
	r := newRulesAdjusterComSemente(rules.NewEngineWithDefaults(), semente)
	e1, s1, e2, s2 := reinterpretarPontos(
		parseMins(batidos[0]), parseMins(batidos[1]),
		parseMins(batidos[2]), parseMins(batidos[3]))
	e1, s1, e2, s2 = r.adjustDayCom(e1, s1, e2, s2, c)
	if s2 > fimDoDia {
		s2 = fimDoDia
	}
	return [4]string{formatMins(e1), formatMins(s1), formatMins(e2), formatMins(s2)}
}

// --- Os dias reais do usuário que passaram pelo gerador ---

// diasReais são os seis dias de junho e julho em que o sistema inventou horário.
// Os valores são os batimentos DE VERDADE, já nos slots em que o gerador os viu
// (a coluna inventada vai vazia); os outros dias dos dois meses ou vieram
// completos, ou são folga/férias/recesso, ou são dispensa e expediente reduzido,
// onde Adjust sai fora antes de inventar coisa alguma.
var diasReais = []struct {
	dia      string
	mascara  string
	batidos  [4]int
	oQueFoi  string
	geradoEm int // slot que o sistema inventou
}{
	{"01/06", ".XXX", [4]int{0, 11*60 + 30, 12*60 + 33, 20*60 + 6}, "faltou a entrada da manhã", 0},
	{"02/06", "XXX.", [4]int{9*60 + 11, 12*60 + 36, 13*60 + 37, 0}, "faltou a saída da tarde", 3},
	{"16/06", "X.XX", [4]int{8*60 + 58, 0, 13*60 + 9, 19*60 + 13}, "faltou a saída para o almoço", 1},
	{"17/06", "XXX.", [4]int{9*60 + 35, 12*60 + 4, 13*60 + 6, 0}, "faltou a saída da tarde", 3},
	{"22/06", "XXX.", [4]int{9*60 + 42, 11*60 + 40, 12*60 + 40, 0}, "faltou a saída da tarde", 3},
	{"10/07", "X.XX", [4]int{9*60 + 40, 0, 13*60 + 23, 19*60 + 30}, "faltou a saída para o almoço", 1},
}

func TestDiasReaisEstaoNasMascarasQueDizemEstar(t *testing.T) {
	for _, d := range diasReais {
		if got := mascaraDeSlots(d.batidos); got != d.mascara {
			t.Errorf("%s: máscara %q, esperada %q", d.dia, got, d.mascara)
		}
	}
}

// --- O relatório ---

type variante struct {
	nome string
	c    constantes
}

// efeitoNaGrade mede, sobre a grade inteira e os 7 regimes, quantos dias mudam
// e de quanto.
type efeitoNaGrade struct {
	dias, mudam int
	maiorMin    int
	somaMin     int
	porMascara  map[string]int
	piorDia     string
}

func mediEfeitoNaGrade(hoje, nova constantes) efeitoNaGrade {
	e := efeitoNaGrade{porMascara: map[string]int{}}
	for _, s := range sorteiosDaMedicao() {
		paraCadaDiaAlcancavel(func(dia [4]int) {
			e.dias++
			a := rodaComVariante(dia, s.fonte, hoje)
			b := rodaComVariante(dia, s.fonte, nova)
			if a == b {
				return
			}
			e.mudam++
			e.porMascara[mascaraDeSlots(dia)]++

			maior := 0
			for i := range a {
				if d := abs(a[i] - b[i]); d > maior {
					maior = d
				}
			}
			e.somaMin += maior
			if maior > e.maiorMin {
				e.maiorMin = maior
				e.piorDia = fmt.Sprintf("%s (%s) %s -> %s  em vez de %s",
					quatroHorarios(dia), s.nome, quatroHorarios(a), quatroHorarios(b), quatroHorarios(a))
			}
		})
	}
	return e
}

// TestRelatorioDasDivergencias imprime o efeito de unificar cada uma das quatro
// divergências. Não afirma nada: mede, para que a decisão seja tomada com o
// número na frente. Rode com:
//
//	go test ./pkg/extraction -run TestRelatorioDasDivergencias -v
func TestRelatorioDasDivergencias(t *testing.T) {
	if !testing.Verbose() {
		t.Skip("relatório de apoio à decisão; rode com -v para ver os números")
	}
	hoje := constantesDeHoje()

	grupos := []struct {
		titulo    string
		pergunta  string
		variantes []variante
	}{
		{
			titulo:   "DIVERGÊNCIA 1 — piso da tarde",
			pergunta: "hoje: 3h em quatro casos, 2h em ..X., nenhum em .X.. e .XX.",
			variantes: []variante{
				{"todos com piso de 3h", hoje.com(func(c *constantes) {
					for _, m := range mascarasComPisoDeTarde {
						c.piso[m] = 180
					}
				})},
				{"todos com piso de 2h", hoje.com(func(c *constantes) {
					for _, m := range mascarasComPisoDeTarde {
						c.piso[m] = 120
					}
				})},
				{"nenhum com piso", hoje.com(func(c *constantes) {
					for _, m := range mascarasComPisoDeTarde {
						c.piso[m] = semPiso
					}
				})},
			},
		},
		{
			titulo:   "DIVERGÊNCIA 2 — faixa da manhã sorteada solta",
			pergunta: "hoje: 220–260 min em ..X., 200–240 min em .X.X",
			variantes: []variante{
				{"as duas em 220–260", hoje.com(func(c *constantes) {
					c.manhaSolta[".X.X"] = [2]int{220, 260}
				})},
				{"as duas em 200–240", hoje.com(func(c *constantes) {
					c.manhaSolta["..X."] = [2]int{200, 240}
				})},
			},
		},
		{
			titulo:   "DIVERGÊNCIA 3 — onde entra o desvio de −5 a +15 min",
			pergunta: "hoje: antes do piso em todos, menos em X..., onde entra depois",
			variantes: []variante{
				{"sempre antes do piso", hoje.com(func(c *constantes) {
					c.desvioAntesDoPiso["X..."] = true
				})},
			},
		},
		{
			titulo:   "DIVERGÊNCIA 4 — a entrada do dia com as duas saídas (.X.X)",
			pergunta: "hoje: sorteada solta, embora a tarde seja conhecida",
			variantes: []variante{
				{"calculada pela carga, como nos outros casos", hoje.com(func(c *constantes) {
					c.entradaPelaCargaNasDuasSaidas = true
				})},
			},
		},
	}

	for _, g := range grupos {
		fmt.Printf("\n\n========================================================================\n")
		fmt.Printf("%s\n%s\n", g.titulo, g.pergunta)
		fmt.Printf("========================================================================\n")

		for _, v := range g.variantes {
			fmt.Printf("\n--- se unificar: %s ---\n", v.nome)

			// (a) as 14 combinações, com a semente do snapshot
			fmt.Printf("\n  [a] as 14 combinações (semente do snapshot)\n")
			mudou := 0
			for _, caso := range casosDoSnapshot {
				antes := ajustaComConstantes(sementeDoSnapshot, caso.batidos, hoje)
				depois := ajustaComConstantes(sementeDoSnapshot, caso.batidos, v.c)
				if antes == depois {
					continue
				}
				mudou++
				fmt.Printf("      %-8s %-46s\n", caso.mascara, caso.ramo)
				fmt.Printf("               hoje %v\n               nova %v   %s\n",
					antes, depois, deltaEmMinutos(antes, depois))
			}
			if mudou == 0 {
				fmt.Printf("      nenhuma das 14 muda um minuto\n")
			}

			// (b) a grade inteira
			e := mediEfeitoNaGrade(hoje, v.c)
			fmt.Printf("\n  [b] dias que a produção pode receber: %d × 7 regimes de sorteio\n", e.dias/7)
			if e.mudam == 0 {
				fmt.Printf("      nenhum dia muda\n")
			} else {
				fmt.Printf("      mudam %d de %d (%.1f%%), maior diferença %d min, média %.1f min\n",
					e.mudam, e.dias, 100*float64(e.mudam)/float64(e.dias),
					e.maiorMin, float64(e.somaMin)/float64(e.mudam))
				var ms []string
				for m := range e.porMascara {
					ms = append(ms, m)
				}
				sort.Strings(ms)
				fmt.Printf("      por combinação: ")
				for _, m := range ms {
					fmt.Printf("%s=%d  ", m, e.porMascara[m])
				}
				fmt.Printf("\n")
			}

			// (c) os dias reais do usuário
			fmt.Printf("\n  [c] os seis dias reais de junho e julho\n")
			mudouReal := 0
			for _, d := range diasReais {
				diferentes := []string{}
				for _, s := range sorteiosDaMedicao() {
					a := rodaComVariante(d.batidos, s.fonte, hoje)
					b := rodaComVariante(d.batidos, s.fonte, v.c)
					if a != b {
						diferentes = append(diferentes,
							fmt.Sprintf("%s: %s -> %s", s.nome, quatroHorarios(a), quatroHorarios(b)))
					}
				}
				if len(diferentes) > 0 {
					mudouReal++
					fmt.Printf("      %s (%s, %s) MUDA:\n", d.dia, d.mascara, d.oQueFoi)
					for _, l := range diferentes {
						fmt.Printf("          %s\n", l)
					}
				}
			}
			if mudouReal == 0 {
				fmt.Printf("      nenhum dos seis muda, em nenhum dos 7 regimes de sorteio\n")
			}
		}
	}
	fmt.Printf("\n")
}

// TestSondaOPisoDaTarde mede, para cada caso que chama saidaDaTarde, quantas
// vezes o piso de fato SEGURA a tarde. Um piso que nunca segura é decoração — e
// dois dos três valores de hoje são exatamente isso, na carga de 480 min.
//
// A carga é configurável (`carga_horaria_diaria_min`), então a sonda roda também
// numa carga menor: é lá que os pisos hoje inertes acordam.
func TestSondaOPisoDaTarde(t *testing.T) {
	if !testing.Verbose() {
		t.Skip("sondagem de apoio à decisão; rode com -v")
	}

	for _, carga := range []int{480, 360} {
		fmt.Printf("\n=== carga diária de %d min ===\n", carga)
		fmt.Printf("%-8s %-8s %8s %8s %12s %12s\n",
			"caso", "piso", "dias", "segura", "menor tarde", "maior tarde")

		for _, mascara := range mascarasComPisoDeTarde {
			var dias, segura, maior int
			menor := 1 << 30
			semPisoNenhum := constantesDeHoje().com(func(c *constantes) {
				c.piso[mascara] = semPiso
			})

			for _, s := range sorteiosDaMedicao() {
				paraCadaDiaAlcancavel(func(dia [4]int) {
					if mascaraDeSlots(dia) != mascara {
						return
					}
					dias++

					r := &RulesAdjuster{Engine: engineComCarga(carga), rng: s.fonte}
					_, _, e2, s2 := r.adjustDayCom(dia[0], dia[1], dia[2], dia[3], constantesDeHoje())
					if tarde := s2 - e2; tarde < menor {
						menor = tarde
					} else if tarde > maior {
						maior = tarde
					}

					r2 := &RulesAdjuster{Engine: engineComCarga(carga), rng: s.fonte}
					_, _, e2b, s2b := r2.adjustDayCom(dia[0], dia[1], dia[2], dia[3], semPisoNenhum)
					if s2b-e2b < s2-e2 {
						segura++
					}
				})
			}

			nome := fmt.Sprintf("%d", constantesDeHoje().piso[mascara])
			if constantesDeHoje().piso[mascara] == semPiso {
				nome = "nenhum"
			}
			fmt.Printf("%-8s %-8s %8d %8d %12d %12d\n", mascara, nome, dias, segura, menor, maior)
		}
	}
}

// TestSondaOSaldoInventado mede o que de fato vai para o documento: quanto o dia
// gerado se afasta da carga diária.
//
// Um piso de tarde não é de graça. Quando ele segura, a tarde deixa de ser "o
// que falta para fechar as 8h" e vira "3h no mínimo" — e a diferença é saldo que
// o sistema inventou a favor do servidor, num papel que ele assina. É este o
// número que decide a divergência 1, não o "quantos minutos muda".
func TestSondaOSaldoInventado(t *testing.T) {
	if !testing.Verbose() {
		t.Skip("sondagem de apoio à decisão; rode com -v")
	}

	hoje := constantesDeHoje()
	variantes := []variante{
		{"hoje", hoje},
		{"piso de 3h em todos", hoje.com(func(c *constantes) {
			for _, m := range mascarasComPisoDeTarde {
				c.piso[m] = 180
			}
		})},
		{"piso nenhum em todos", hoje.com(func(c *constantes) {
			for _, m := range mascarasComPisoDeTarde {
				c.piso[m] = semPiso
			}
		})},
		{"entrada pela carga em .X.X", hoje.com(func(c *constantes) {
			c.entradaPelaCargaNasDuasSaidas = true
		})},
	}

	fmt.Printf("\nsaldo inventado = (manhã + tarde do dia gerado) − 480 min\n")
	fmt.Printf("%-30s %-8s %8s %11s %10s %10s\n",
		"variante", "caso", "dias", "fecha em 0", "média |Δ|", "maior Δ")

	casos := append(append([]string{}, mascarasComPisoDeTarde...), ".X.X")
	for _, v := range variantes {
		for _, mascara := range casos {
			var dias, fecham, soma, maior int
			for _, s := range sorteiosDaMedicao() {
				paraCadaDiaAlcancavel(func(dia [4]int) {
					if mascaraDeSlots(dia) != mascara {
						return
					}
					r := &RulesAdjuster{Engine: rules.NewEngineWithDefaults(), rng: s.fonte}
					e1, s1, e2, s2 := r.adjustDayCom(dia[0], dia[1], dia[2], dia[3], v.c)

					saldo := (s1 - e1) + (s2 - e2) - 480
					dias++
					if abs(saldo) <= 20 { // o desvio de −5 a +15 e o fuga-do-redondo
						fecham++
					}
					soma += abs(saldo)
					if abs(saldo) > abs(maior) {
						maior = saldo
					}
				})
			}
			if dias == 0 {
				continue
			}
			fmt.Printf("%-30s %-8s %8d %10.0f%% %10.0f %+10d\n",
				v.nome, mascara, dias, 100*float64(fecham)/float64(dias),
				float64(soma)/float64(dias), maior)
		}
		fmt.Printf("\n")
	}
}

func engineComCarga(min int) *rules.Engine {
	e := rules.NewEngineWithDefaults()
	e.Config.CargaHorariaDiaria = min
	return e
}

func deltaEmMinutos(antes, depois [4]string) string {
	nomes := [4]string{"entrada", "saída-almoço", "retorno", "saída"}
	var partes []string
	for i := range antes {
		a, b := parseMins(antes[i]), parseMins(depois[i])
		if a == b {
			continue
		}
		sinal := "+"
		if b < a {
			sinal = "−"
		}
		partes = append(partes, fmt.Sprintf("%s %s%d min", nomes[i], sinal, abs(b-a)))
	}
	if len(partes) == 0 {
		return ""
	}
	out := "("
	for i, p := range partes {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out + ")"
}
