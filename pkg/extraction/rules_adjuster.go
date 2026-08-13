package extraction

import (
	"fmt"
	"math/rand"
	"sort"
	"time"

	"github.com/fernangcortes/ponto-real-go/pkg/models"
	"github.com/fernangcortes/ponto-real-go/pkg/rules"
)

// fimDoDia é o último minuto válido do dia (23:59), usado para impedir que a
// geração de horários produza valores fora da faixa de 24 horas.
const fimDoDia = 23*60 + 59

// MsgRevisarMeiaNoite avisa que a saída gerada foi limitada ao fim do dia.
const MsgRevisarMeiaNoite = "Horário gerado ultrapassaria a meia-noite; confira os pontos originais deste dia."

// semPiso e semTeto marcam a ausência de limite em avoidRoundMinsEntre.
const (
	semPiso = -1 << 30
	semTeto = 1 << 30
)

// tardeMinima é o piso da tarde gerada: mesmo que a manhã já tenha consumido a
// jornada inteira, o expediente da tarde não sai com menos de 3 horas.
//
// Ele existe por duas razões. A óbvia é plausibilidade — servidor que volta do
// almoço e sai quinze minutos depois não é um dia crível. A que só apareceu ao
// medir é que sem ele a tarde chega a ficar NEGATIVA, com a saída antes do
// retorno, no dia em que as duas entradas são reais e as duas saídas inventadas.
//
// Custa saldo: quando o piso segura, o dia gerado fecha acima da jornada, e essa
// diferença é crédito que o sistema inventou. É um preço aceito de olho aberto,
// porque a alternativa medida era pior.
//
// Até 2026-08-13 este número era 180 em quatro casos, 120 num e inexistente em
// dois — divergência herdada de quando cada combinação tinha a sua própria cópia
// da aritmética. Unificado nos 180, que era o valor da maioria e o único que
// chegava a segurar alguma coisa.
const tardeMinima = 180

// manhaSoltaMin e manhaSoltaMax são a manhã sorteada quando NADA a determina —
// nem a carga do dia (porque a tarde ainda não existe), nem dois batimentos
// reais que a delimitem.
//
// A faixa contém a manhã contratual de 210 min (08:30–12:00), que é o que a
// torna plausível. Até 2026-08-13 um dos três lugares que fazem esta mesma
// pergunta usava 220–260, sem que ninguém tivesse registrado por quê.
const (
	manhaSoltaMin = 200
	manhaSoltaMax = 240
)

// RulesAdjuster ajusta horários faltantes usando lógica determinística.
// Substitui o LLM para ajuste — muito mais confiável e rápido.
//
// O sorteio vive no próprio ajustador, e não no rand global do pacote. É o que
// permite repetir uma execução inteira a partir de uma semente, e sem isso a
// suíte não consegue travar os horários que cada ramo produz: ela ficaria verde
// tanto para o número certo quanto para o errado.
//
// Em troca, um ajustador não pode ser usado por duas goroutines ao mesmo tempo.
// É um por folha processada, que é como o serviço já o cria.
type RulesAdjuster struct {
	Engine *rules.Engine
	rng    sorteador
}

// sorteador é a fonte de aleatoriedade do gerador. *rand.Rand a satisfaz, então
// a produção não vê diferença.
//
// Ela existe para o teste poder injetar um sorteio determinístico — sempre o
// mínimo, sempre o máximo, sempre o meio. Com um sorteio que não guarda estado,
// adjustDay vira uma função pura dos batimentos: o resultado deixa de depender
// da ORDEM em que os sorteios acontecem, e duas implementações podem ser
// comparadas horário a horário sobre milhares de dias.
type sorteador interface {
	Intn(n int) int
}

// NewRulesAdjuster cria o ajustador de produção, com o sorteio semeado pelo
// relógio: horário inventado tem de variar de uma execução para outra.
func NewRulesAdjuster(engine *rules.Engine) *RulesAdjuster {
	return newRulesAdjusterComSemente(engine, time.Now().UnixNano())
}

// newRulesAdjusterComSemente cria um ajustador reproduzível: a mesma semente
// com os mesmos batimentos devolve sempre os mesmos horários. Existe para o
// teste conseguir comparar números — é a única forma de uma mudança de valor
// ser percebida.
func newRulesAdjusterComSemente(engine *rules.Engine, semente int64) *RulesAdjuster {
	return &RulesAdjuster{Engine: engine, rng: rand.New(rand.NewSource(semente))}
}

// almocoGerado sorteia a duração do almoço a ser inventado num dia, dentro da
// faixa configurada em rules.json.
//
// A faixa nunca pode começar abaixo do almoço mínimo legal: um almoço gerado
// curto demais produziria um dia que a própria ValidateDay recusa.
func (r *RulesAdjuster) almocoGerado() int {
	min := r.Engine.Config.AlmocoGeradoMin
	if min < r.Engine.Config.AlmocoMinimo {
		min = r.Engine.Config.AlmocoMinimo
	}
	max := r.Engine.Config.AlmocoGeradoMax
	if max < min {
		max = min + 15
	}
	return r.randBetween(min, max)
}

// Adjust recebe um Timesheet e preenche horários faltantes com valores realistas.
func (r *RulesAdjuster) Adjust(ts *models.Timesheet) *models.Timesheet {
	result := *ts
	result.Dias = make([]models.DayRecord, len(ts.Dias))
	copy(result.Dias, ts.Dias)

	for i := range result.Dias {
		d := &result.Dias[i]

		diaTipo := r.Engine.ClassifyDay(d)
		if diaTipo == models.DayTypeFolga || diaTipo == models.DayTypeFeriado || diaTipo == models.DayTypeRecesso {
			continue
		}

		// Expediente reduzido por decreto: a jornada exigida é menor que a padrão,
		// então o ponto batido já basta. Nunca gerar horário aqui — apenas marcar
		// os pontos existentes como originais e pedir conferência da carga do dia.
		if diaTipo == models.DayTypeExpedienteReduzido {
			e1 := parseMins(d.Entrada1)
			s1 := parseMins(d.Saida1)
			e2 := parseMins(d.Entrada2)
			s2 := parseMins(d.Saida2)
			if countFilled(e1, s1, e2, s2) > 0 {
				d.Bloqueio = []int{boolToInt(e1 > 0), boolToInt(s1 > 0), boolToInt(e2 > 0), boolToInt(s2 > 0)}
			}
			if d.CargaEsperada == 0 {
				d.Revisar = true
				d.RevisarMotivo = rules.MsgRevisarCargaReduzida
			}
			continue
		}

		// Dispensa: não gera horários automaticamente, mas marca o array de bloqueio
		// para que o frontend saiba quais campos são editáveis
		if diaTipo == models.DayTypeDispensa {
			e1 := parseMins(d.Entrada1)
			s1 := parseMins(d.Saida1)
			e2 := parseMins(d.Entrada2)
			s2 := parseMins(d.Saida2)
			filled := countFilled(e1, s1, e2, s2)
			if filled > 0 {
				d.Bloqueio = []int{boolToInt(e1 > 0), boolToInt(s1 > 0), boolToInt(e2 > 0), boolToInt(s2 > 0)}
			}
			continue
		}

		e1 := parseMins(d.Entrada1)
		s1 := parseMins(d.Saida1)
		e2 := parseMins(d.Entrada2)
		s2 := parseMins(d.Saida2)

		filled := countFilled(e1, s1, e2, s2)

		if filled == 4 {
			continue
		}

		if filled == 0 {
			continue
		}

		// Reinterpretar pontos cuja posição na tabela não faz sentido no relógio
		// antes de gerar qualquer coisa.
		e1, s1, e2, s2 = reinterpretarPontos(e1, s1, e2, s2)

		// Ajustar
		var o []int
		e1, s1, e2, s2, o = r.adjustDay(e1, s1, e2, s2)

		// Um ponto original muito tarde (ex: retorno de almoço às 20:06) faz a
		// geração estourar a meia-noite e produzir horário impossível como
		// "27:12". Nesse caso limitamos ao fim do dia e pedimos conferência,
		// em vez de gravar um horário inválido na folha.
		if s2 > fimDoDia {
			s2 = fimDoDia
			d.Revisar = true
			d.RevisarMotivo = MsgRevisarMeiaNoite
		}

		d.Entrada1 = formatMins(e1)
		d.Saida1 = formatMins(s1)
		d.Entrada2 = formatMins(e2)
		d.Saida2 = formatMins(s2)
		d.Bloqueio = o

		// Recalcular
		morning := s1 - e1
		afternoon := s2 - e2
		total := morning + afternoon
		d.ExpSaldo = formatMinsDuration(total)
	}

	return &result
}

// janelaSlot é a faixa de horário plausível para cada um dos 4 batimentos,
// em minutos desde a meia-noite. Baseadas na jornada contratual 08:30-12:00 /
// 13:00-17:30, com folga generosa para atrasos e horas extras.
var janelaSlot = [4]struct{ min, max int }{
	{6 * 60, 10*60 + 30},     // e1 — entrada da manhã
	{10*60 + 30, 13*60 + 30}, // s1 — saída para o almoço
	{11*60 + 30, 16 * 60},    // e2 — retorno do almoço
	{16 * 60, 23*60 + 59},    // s2 — saída do expediente
}

// penalidadeSlot mede o quanto um horário destoa da janela plausível do slot.
// Zero significa encaixe perfeito.
func penalidadeSlot(t, slot int) int {
	j := janelaSlot[slot]
	if t < j.min {
		return j.min - t
	}
	if t > j.max {
		return t - j.max
	}
	return 0
}

// reinterpretarPontos decide a que batimento cada horário realmente corresponde.
//
// A ficha tem 4 colunas fixas, mas quando um batimento falta os demais
// escorregam de coluna. Ler a posição literalmente produz absurdos: no dia com
// 11:30, 12:33 e 20:06 a leitura literal diz que 11:30 é a entrada da manhã e
// 20:06 é o retorno do almoço — quando na verdade faltou a entrada da manhã, e
// esses três são saída-almoço, retorno e saída.
//
// Em vez de regras pontuais, testamos todas as formas de encaixar os horários
// (em ordem cronológica) nos 4 slots e ficamos com a que menos destoa das
// janelas plausíveis. Empates preservam a posição original, e dias com os 4
// batimentos nunca são alterados.
func reinterpretarPontos(e1, s1, e2, s2 int) (int, int, int, int) {
	atual := [4]int{e1, s1, e2, s2}

	// Horários preenchidos, em ordem cronológica, com o slot em que vieram.
	var tempos, slotOriginal []int
	for slot, t := range atual {
		if t > 0 {
			tempos = append(tempos, t)
			slotOriginal = append(slotOriginal, slot)
		}
	}

	k := len(tempos)
	if k == 0 || k == 4 {
		return e1, s1, e2, s2 // nada a decidir
	}

	sort.Ints(tempos)

	melhorPenalidade, melhorDeslocamento := -1, 0
	var melhorCombo []int

	// Todas as escolhas de k slots entre os 4, mantendo a ordem crescente.
	for mask := 0; mask < 16; mask++ {
		var combo []int
		for slot := 0; slot < 4; slot++ {
			if mask&(1<<slot) != 0 {
				combo = append(combo, slot)
			}
		}
		if len(combo) != k {
			continue
		}

		penalidade, deslocamento := 0, 0
		for i, slot := range combo {
			penalidade += penalidadeSlot(tempos[i], slot)
			deslocamento += abs(slot - slotOriginal[i])
		}

		// Menor penalidade vence; empate fica com quem mexeu menos.
		if melhorPenalidade == -1 || penalidade < melhorPenalidade ||
			(penalidade == melhorPenalidade && deslocamento < melhorDeslocamento) {
			melhorPenalidade, melhorDeslocamento, melhorCombo = penalidade, deslocamento, combo
		}
	}

	var novo [4]int
	for i, slot := range melhorCombo {
		novo[slot] = tempos[i]
	}
	return novo[0], novo[1], novo[2], novo[3]
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// meioEntre devolve um horário estritamente entre dois pontos reais, usado
// quando o intervalo disponível é menor que o almoço mínimo.
func meioEntre(inicio, fim int) int {
	if fim-inicio < 2 {
		return inicio // sem espaço: não inventa ponto intermediário
	}
	return inicio + (fim-inicio)/2
}

// adjustDay gera os horários que faltam a partir dos que existem.
//
// São 14 combinações possíveis de batimentos — a ficha tem 4 colunas, logo 16,
// menos o dia vazio e o dia completo, que Adjust nem entrega aqui. Mas 14
// combinações não são 14 regras: cada coluna inventada nasce num gerador só,
// logo abaixo, e o que muda de um caso para outro é de qual horário conhecido
// ele se ancora e em que ordem os quatro se resolvem.
//
// A que batimento cada horário corresponde já foi resolvido em
// reinterpretarPontos, que trata 1, 2 ou 3 pontos pela mesma régua de
// plausibilidade. Aqui só resta gerar o que falta.
//
// Até 2026-08-13 quatro constantes divergiam entre casos que sabem exatamente a
// mesma coisa — herança de quando cada combinação tinha a sua própria cópia do
// cálculo. As quatro foram medidas e unificadas, e nenhuma delas mudou um dia
// real: o piso da tarde nos 180 min (tardeMinima), a manhã sorteada solta em
// 200–240 (manhaSoltaMin/Max), o desvio dos minutos sempre antes de conferir o
// piso, e a entrada do dia com as duas saídas calculada pela carga, como fazem
// os outros casos de tarde conhecida.
func (r *RulesAdjuster) adjustDay(e1, s1, e2, s2 int) (int, int, int, int, []int) {
	o := []int{boolToInt(e1 > 0), boolToInt(s1 > 0), boolToInt(e2 > 0), boolToInt(s2 > 0)}

	switch {
	// --- Falta um batimento só ---

	case e1 == 0 && s1 > 0 && e2 > 0 && s2 > 0:
		e1 = r.entradaPelaCarga(s1, s2-e2)

	case e1 > 0 && s1 == 0 && e2 > 0 && s2 > 0:
		s1 = naoAntesDaEntrada(r.saidaParaAlmocoEspremida(e1, e2), e1, e2)

	case e1 > 0 && s1 > 0 && e2 == 0 && s2 > 0:
		e2 = r.retornoDoAlmoco(s1, r.almocoGerado(), s2)

	case e1 > 0 && s1 > 0 && e2 > 0 && s2 == 0:
		s2 = r.saidaDaTarde(e2, s1-e1)

	// --- Faltam dois ---

	// As duas pontas do dia são reais e o almoço inteiro é inventado. É o caso
	// mais comum, e o único em que a saída para o almoço tem de caber entre dois
	// horários que o servidor realmente bateu.
	case e1 > 0 && s1 == 0 && e2 == 0 && s2 > 0:
		almoco := r.almocoGerado()
		s1 = r.saidaParaAlmocoEntreAsPontas(e1, s2, almoco)
		e2 = r.retornoDoAlmoco(s1, almoco, s2)

	case e1 > 0 && s1 > 0 && e2 == 0 && s2 == 0:
		e2 = r.retornoDoAlmoco(s1, r.almocoGerado(), 0)
		s2 = r.saidaDaTarde(e2, s1-e1)

	case e1 == 0 && s1 == 0 && e2 > 0 && s2 > 0:
		s1 = r.saidaParaAlmocoAntesDoRetorno(e2, r.almocoGerado())
		e1 = r.entradaPelaCarga(s1, s2-e2)

	case e1 > 0 && s1 == 0 && e2 > 0 && s2 == 0:
		s1 = naoAntesDaEntrada(r.saidaParaAlmocoAntesDoRetorno(e2, r.almocoGerado()), e1, e2)
		s2 = r.saidaDaTarde(e2, s1-e1)

	// As duas saídas são reais. O retorno vem primeiro porque é ele que fecha a
	// tarde; com a tarde conhecida, a entrada sai da carga do dia, como em todo
	// caso que sabe quanto durou a tarde. Até 2026-08-13 ela era sorteada solta
	// aqui, ignorando informação que estava à mão: o dia fechava na jornada em
	// 8% das vezes, contra 42% agora.
	case e1 == 0 && s1 > 0 && e2 == 0 && s2 > 0:
		e2 = r.retornoDoAlmoco(s1, r.almocoGerado(), s2)
		e1 = r.entradaPelaCarga(s1, s2-e2)

	// --- Falta o dia quase inteiro ---

	case e1 > 0 && s1 == 0 && e2 == 0 && s2 == 0:
		s1 = r.saidaParaAlmocoDepoisDaEntrada(e1)
		e2 = r.retornoDoAlmoco(s1, r.almocoGerado(), 0)
		s2 = r.saidaDaTarde(e2, s1-e1)

	// Só a saída da tarde: o dia é montado de trás para frente.
	case e1 == 0 && s1 == 0 && e2 == 0 && s2 > 0:
		almoco := r.almocoGerado()
		var tarde int
		e2, tarde = r.retornoAPartirDaSaida(s2)
		s1 = r.saidaParaAlmocoAntesDoRetorno(e2, almoco)
		e1 = r.entradaPelaCarga(s1, tarde)

	case e1 == 0 && s1 == 0 && e2 > 0 && s2 == 0:
		var manha int
		s1 = r.saidaParaAlmocoAntesDoRetorno(e2, r.almocoGerado())
		e1, manha = r.entradaAntesDaSaidaParaOAlmoco(s1)
		s2 = r.saidaDaTarde(e2, manha)

	// --- Os dois casos sem nenhuma âncora de manhã ---
	//
	// Só horários do meio do dia foram lidos: a saída para o almoço sozinha, ou
	// ela e o retorno. Não têm de onde deduzir a entrada, então ela é arbitrada
	// entre 08:00 e 09:00.
	//
	// Estes dois eram o "combinação não prevista" que ia para o log como se
	// fosse defeito. Não são: são o dia em que o servidor bateu na saída e na
	// volta do almoço e esqueceu as duas pontas.
	//
	// Eram também os dois únicos casos em que a tarde saía sem piso, e por isso
	// os únicos cuja garantia de não ficar negativa morava em janelaSlot, noutra
	// função. Com tardeMinima valendo aqui também, a garantia passou a morar
	// onde é exercida.

	case e1 == 0 && s1 > 0 && e2 == 0 && s2 == 0:
		e1 = r.entradaArbitrada()
		e2 = r.retornoDoAlmoco(s1, r.almocoGerado(), 0)
		s2 = r.saidaDaTarde(e2, s1-e1)

	case e1 == 0 && s1 > 0 && e2 > 0 && s2 == 0:
		e1 = r.entradaArbitrada()
		s2 = r.saidaDaTarde(e2, s1-e1)

	// Dia vazio ou completo: Adjust não chega a chamar aqui, e não haveria o que
	// inventar de todo modo.
	default:
	}

	return e1, s1, e2, s2, o
}

// --- Os geradores: cada coluna inventada nasce num lugar só ---

// saidaParaAlmocoEntreAsPontas sorteia a saída para o almoço quando a entrada e
// a saída do dia são reais e não há nada entre elas. É a única que precisa caber
// dentro de dois horários batidos pelo servidor, então reserva três horas de
// manhã, o almoço inteiro e duas horas de tarde antes de sortear o que sobra.
func (r *RulesAdjuster) saidaParaAlmocoEntreAsPontas(e1, s2, almoco int) int {
	minS1 := maxInt(e1+180, 11*60+30)
	maxS1 := minInt(s2-almoco-120, 13*60)
	if maxS1 < minS1 {
		maxS1 = minS1 + 30
	}
	return r.avoidRoundMins(r.randBetween(minS1, maxS1))
}

// saidaParaAlmocoDepoisDaEntrada: só a entrada é conhecida, então a manhã é
// sorteada solta a partir dela.
func (r *RulesAdjuster) saidaParaAlmocoDepoisDaEntrada(e1 int) int {
	return r.avoidRoundMins(e1 + r.randBetween(manhaSoltaMin, manhaSoltaMax))
}

// saidaParaAlmocoAntesDoRetorno: o retorno é conhecido, então a saída é ele
// menos o almoço — nunca perto o bastante para encurtá-lo abaixo do mínimo.
func (r *RulesAdjuster) saidaParaAlmocoAntesDoRetorno(e2, almoco int) int {
	return r.avoidRoundMinsEntre(e2-almoco, semPiso, e2-r.Engine.Config.AlmocoMinimo)
}

// saidaParaAlmocoEspremida: a entrada E o retorno são reais. Tenta reservar uma
// manhã inteira antes do almoço; quando o intervalo entre os dois não comporta
// nem o almoço mínimo, cola a saída no mínimo antes do retorno.
func (r *RulesAdjuster) saidaParaAlmocoEspremida(e1, e2 int) int {
	minAlmoco := r.Engine.Config.AlmocoMinimo

	almoco := e2 - e1 - r.randBetween(manhaSoltaMin, manhaSoltaMax)
	var s1 int
	if almoco < minAlmoco {
		s1 = e2 - minAlmoco - r.randBetween(0, 10)
	} else {
		s1 = e2 - almoco
	}
	return r.avoidRoundMinsEntre(s1, semPiso, e2-minAlmoco)
}

// naoAntesDaEntrada garante que a saída para o almoço caia ENTRE a entrada e o
// retorno reais. Quando os dois estão perto demais para caber qualquer almoço,
// ela vai para o meio dos dois: exigir o mínimo legal seria exigir o impossível.
func naoAntesDaEntrada(s1, e1, e2 int) int {
	if s1 <= e1 {
		return meioEntre(e1, e2)
	}
	return s1
}

// retornoDoAlmoco é a saída para o almoço mais o almoço, nunca antes do mínimo
// legal e nunca em cima da saída da tarde. Passe s2 = 0 quando a saída ainda não
// for conhecida.
func (r *RulesAdjuster) retornoDoAlmoco(s1, almoco, s2 int) int {
	e2 := r.avoidRoundMinsEntre(s1+almoco, s1+r.Engine.Config.AlmocoMinimo, semTeto)
	if s2 > 0 && e2 >= s2 {
		e2 = s2 - 30
	}
	return e2
}

// retornoAPartirDaSaida conta o retorno de trás para frente, a partir da saída
// da tarde, quando não há nada antes dele em que se apoiar. Devolve também a
// tarde sorteada: quem calcula a manhã depois precisa dela antes do desvio de
// minutos, não da diferença já arredondada.
func (r *RulesAdjuster) retornoAPartirDaSaida(s2 int) (int, int) {
	tarde := r.randBetween(200, 270)
	return r.avoidRoundMins(s2 - tarde), tarde
}

// entradaPelaCarga: a tarde já é conhecida, então a manhã é o que falta para
// fechar a jornada do dia — com um mínimo de duas horas.
func (r *RulesAdjuster) entradaPelaCarga(s1, tardeTrabalhada int) int {
	manha := r.Engine.Config.CargaHorariaDiaria - tardeTrabalhada + r.randBetween(-5, 15)
	if manha < 120 {
		manha = 120
	}
	return r.naoDeMadrugada(r.avoidRoundMins(s1 - manha))
}

// entradaAntesDaSaidaParaOAlmoco sorteia a manhã solta, para o caso em que a
// tarde ainda não existe e não há carga de onde deduzi-la.
//
// Devolve também a manhã efetivamente trabalhada, porque a trava das 07:00 pode
// encurtá-la: sem recontar, a tarde compensaria uma manhã que não aconteceu e o
// dia fecharia com mais horas do que o servidor cumpriu.
func (r *RulesAdjuster) entradaAntesDaSaidaParaOAlmoco(s1 int) (int, int) {
	manha := r.randBetween(manhaSoltaMin, manhaSoltaMax)
	e1 := r.avoidRoundMins(s1 - manha)
	if travada := r.naoDeMadrugada(e1); travada != e1 {
		return travada, s1 - travada
	}
	return e1, manha
}

// entradaArbitrada é a entrada dos dias sem nenhuma âncora de manhã: não há de
// onde deduzi-la, então fica entre 08:00 e 09:00.
func (r *RulesAdjuster) entradaArbitrada() int {
	return r.avoidRoundMins(8*60 + r.randBetween(0, 60))
}

// naoDeMadrugada trava o início do expediente INVENTADO nas 07:00 — batimento
// real do servidor é intocável, seja qual for a hora. Sem isso um retorno de
// almoço às 11:30 puxava a entrada para 05:56.
func (r *RulesAdjuster) naoDeMadrugada(e1 int) int {
	if e1 < 7*60 {
		return 7*60 + r.randBetween(2, 28)
	}
	return e1
}

// saidaDaTarde é o que falta para fechar a jornada depois da manhã que
// realmente aconteceu, nunca abaixo de tardeMinima.
//
// O desvio de −5 a +15 minutos entra ANTES de o piso ser conferido, para que o
// piso seja de fato um piso: aplicá-lo depois deixa a tarde terminar alguns
// minutos abaixo dele, que era o que acontecia num dos casos até 2026-08-13. Na
// carga de 480 min aquele caso nunca chegava a encostar no piso, então a
// diferença nunca aparecia — mas a carga é configurável, e numa de 360 min a
// tarde saía com 174 min contra um piso de 180.
// A fuga do minuto redondo recebe o piso como limite, e não é por preciosismo:
// sem ele o deslocamento come a tarde por cima e ela sai com 179 minutos contra
// um piso de 180 — o mesmo defeito que o almoço gerado já teve, quando o desvio
// era aplicado como lei em vez de preferência.
func (r *RulesAdjuster) saidaDaTarde(e2, manhaTrabalhada int) int {
	tarde := r.Engine.Config.CargaHorariaDiaria - manhaTrabalhada + r.randBetween(-5, 15)
	if tarde < tardeMinima {
		tarde = tardeMinima
	}
	return r.avoidRoundMinsEntre(e2+tarde, e2+tardeMinima, semTeto)
}

// --- Helpers ---

func parseMins(t string) int {
	if t == "" || t == "**:**" {
		return 0
	}
	var h, m int
	_, err := fmt.Sscanf(t, "%d:%d", &h, &m)
	if err != nil {
		return 0
	}
	return h*60 + m
}

func formatMins(m int) string {
	if m <= 0 {
		return ""
	}
	return fmt.Sprintf("%02d:%02d", m/60, m%60)
}

func formatMinsDuration(m int) string {
	if m < 0 {
		m = -m
	}
	return fmt.Sprintf("%02d:%02d", m/60, m%60)
}

func countFilled(vals ...int) int {
	n := 0
	for _, v := range vals {
		if v > 0 {
			n++
		}
	}
	return n
}

func (r *RulesAdjuster) randBetween(min, max int) int {
	if min >= max {
		return min
	}
	return min + r.rng.Intn(max-min+1)
}

// minutoRedondo diz se o horário cai num minuto "certinho" — :00, :15, :30 ou
// :45 —, que denuncia de longe um horário inventado.
func minutoRedondo(m int) bool {
	mins := m % 60
	return mins == 0 || mins == 15 || mins == 30 || mins == 45
}

// avoidRoundMins afasta um horário gerado dos minutos redondos, quando não há
// nada limitando para que lado ele pode andar.
func (r *RulesAdjuster) avoidRoundMins(m int) int {
	return r.avoidRoundMinsEntre(m, semPiso, semTeto)
}

// avoidRoundMinsEntre afasta o horário dos minutos redondos sem sair da faixa
// [piso, teto].
//
// Fugir do minuto redondo é uma preferência, não uma lei. Quando nenhum
// deslocamento cabe na faixa, o horário fica redondo mesmo: um :30 de vez em
// quando é melhor que um horário que quebra uma regra. Era o que acontecia com
// o almoço — o deslocamento comia o intervalo por cima e produzia almoço de 58
// minutos, abaixo do mínimo legal que a geração promete respeitar.
//
// O sorteio de sempre é tentado primeiro, então nas chamadas sem limite o
// resultado é o de antes. A faixa só entra em cena quando ele não cabe, e aí o
// mesmo deslocamento para o outro lado quase sempre resolve — ficar redondo é
// o último recurso, não o segundo.
//
// Deslocar até 7 minutos nunca alcança o minuto redondo seguinte, que está a
// 15 de distância; por isso basta conferir a faixa.
func (r *RulesAdjuster) avoidRoundMinsEntre(m, piso, teto int) int {
	if !minutoRedondo(m) {
		return m
	}

	passo := r.randBetween(1, 7)
	sinal := 1
	if r.rng.Intn(2) == 0 {
		sinal = -1
	}

	for volta := 0; volta < 7; volta++ {
		d := (passo+volta-1)%7 + 1
		for _, s := range []int{sinal, -sinal} {
			if cand := m + d*s; cand >= piso && cand <= teto {
				return cand
			}
		}
	}
	return m
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
