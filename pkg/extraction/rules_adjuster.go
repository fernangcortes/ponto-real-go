package extraction

import (
	"fmt"
	"math/rand"
	"sort"
	"time"

	"github.com/fernangcortes/ponto-real-go/pkg/models"
	"github.com/fernangcortes/ponto-real-go/pkg/rules"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

// fimDoDia é o último minuto válido do dia (23:59), usado para impedir que a
// geração de horários produza valores fora da faixa de 24 horas.
const fimDoDia = 23*60 + 59

// MsgRevisarMeiaNoite avisa que a saída gerada foi limitada ao fim do dia.
const MsgRevisarMeiaNoite = "Horário gerado ultrapassaria a meia-noite; confira os pontos originais deste dia."

// RulesAdjuster ajusta horários faltantes usando lógica determinística.
// Substitui o LLM para ajuste — muito mais confiável e rápido.
type RulesAdjuster struct {
	Engine *rules.Engine
}

func NewRulesAdjuster(engine *rules.Engine) *RulesAdjuster {
	return &RulesAdjuster{Engine: engine}
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

// adjustDay gera horários faltantes baseado nos existentes e na Carga Horária.
func (r *RulesAdjuster) adjustDay(e1, s1, e2, s2 int) (int, int, int, int, []int) {
	carga := r.Engine.Config.CargaHorariaDiaria
	minAlmoco := r.Engine.Config.AlmocoMinimo

	// A que batimento cada horário corresponde já foi resolvido em
	// reinterpretarPontos, que trata 1, 2 ou 3 pontos pela mesma régua de
	// plausibilidade. Aqui só resta gerar o que falta.
	o := []int{boolToInt(e1 > 0), boolToInt(s1 > 0), boolToInt(e2 > 0), boolToInt(s2 > 0)}

	// e1 e s2 preenchidos → gerar almoço no meio (CASO MAIS COMUM)
	if e1 > 0 && s1 == 0 && e2 == 0 && s2 > 0 {
		lunch := minAlmoco + randBetween(0, 15)
		minS1 := maxInt(e1+180, 11*60+30)
		maxS1 := minInt(s2-lunch-120, 13*60)
		if maxS1 < minS1 {
			maxS1 = minS1 + 30
		}
		s1 = randBetween(minS1, maxS1)
		s1 = avoidRoundMins(s1)
		e2 = s1 + lunch
		e2 = avoidRoundMins(e2)
		if e2 >= s2 {
			e2 = s2 - 30
		}
		return e1, s1, e2, s2, o
	}

	// apenas e1
	if e1 > 0 && s1 == 0 && e2 == 0 && s2 == 0 {
		s1 = e1 + randBetween(200, 240)
		s1 = avoidRoundMins(s1)
		lunch := minAlmoco + randBetween(0, 15)
		e2 = s1 + lunch
		e2 = avoidRoundMins(e2)
		morningWork := s1 - e1
		afternoonNeeded := carga - morningWork
		if afternoonNeeded < 180 {
			afternoonNeeded = 180
		}
		s2 = e2 + afternoonNeeded + randBetween(-5, 15)
		s2 = avoidRoundMins(s2)
		return e1, s1, e2, s2, o
	}

	// apenas s2 (o código de posição única garante que se tem só 1 valor tarde, ele o joga pro s2 se for MT tarde, mas fallback aqui normal)
	if e1 == 0 && s1 == 0 && e2 == 0 && s2 > 0 {
		lunch := minAlmoco + randBetween(0, 15)
		afternoonWork := randBetween(200, 270)
		e2 = s2 - afternoonWork
		e2 = avoidRoundMins(e2)
		s1 = e2 - lunch
		s1 = avoidRoundMins(s1)
		morningWork := carga - afternoonWork + randBetween(-5, 15)
		if morningWork < 120 {
			morningWork = 120
		}
		e1 = s1 - morningWork
		e1 = avoidRoundMins(e1)
		if e1 < 7*60 {
			e1 = 7*60 + randBetween(2, 28)
		}
		return e1, s1, e2, s2, o
	}

	// apenas e2 (Se ele retornou do almoço mas não bateu mais nada)
	if e1 == 0 && s1 == 0 && e2 > 0 && s2 == 0 {
		lunch := minAlmoco + randBetween(0, 15)
		s1 = e2 - lunch
		s1 = avoidRoundMins(s1)
		morningWork := randBetween(220, 260)
		e1 = s1 - morningWork
		e1 = avoidRoundMins(e1)

		afternoonNeeded := carga - morningWork + randBetween(-5, 15)
		if afternoonNeeded < 120 {
			afternoonNeeded = 120
		}
		s2 = e2 + afternoonNeeded
		s2 = avoidRoundMins(s2)
		return e1, s1, e2, s2, o
	}

	// e1 e s1 (manhã completa, falta tarde)
	if e1 > 0 && s1 > 0 && e2 == 0 && s2 == 0 {
		lunch := minAlmoco + randBetween(0, 15)
		e2 = s1 + lunch
		e2 = avoidRoundMins(e2)
		morningWork := s1 - e1
		afternoonNeeded := carga - morningWork + randBetween(-5, 15)
		if afternoonNeeded < 180 {
			afternoonNeeded = 180
		}
		s2 = e2 + afternoonNeeded
		s2 = avoidRoundMins(s2)
		return e1, s1, e2, s2, o
	}

	// e2 e s2 (tarde completa, falta manhã)
	if e1 == 0 && s1 == 0 && e2 > 0 && s2 > 0 {
		lunch := minAlmoco + randBetween(0, 15)
		s1 = e2 - lunch
		s1 = avoidRoundMins(s1)
		afternoonWork := s2 - e2
		morningNeeded := carga - afternoonWork + randBetween(-5, 15)
		if morningNeeded < 120 {
			morningNeeded = 120
		}
		e1 = s1 - morningNeeded
		e1 = avoidRoundMins(e1)
		if e1 < 7*60 {
			e1 = 7*60 + randBetween(2, 28)
		}
		return e1, s1, e2, s2, o
	}

	// 3 de 4 preenchidos — gerar o faltante
	if e1 == 0 && s1 > 0 && e2 > 0 && s2 > 0 {
		afternoonWork := s2 - e2
		morningNeeded := carga - afternoonWork + randBetween(-5, 15)
		if morningNeeded < 120 {
			morningNeeded = 120
		}
		e1 = s1 - morningNeeded
		e1 = avoidRoundMins(e1)
		if e1 < 7*60 {
			e1 = 7*60 + randBetween(2, 28)
		}
		return e1, s1, e2, s2, o
	}
	if e1 > 0 && s1 == 0 && e2 > 0 && s2 > 0 {
		lunch := e2 - e1 - randBetween(200, 240)
		if lunch < minAlmoco {
			s1 = e2 - minAlmoco - randBetween(0, 10)
		} else {
			s1 = e2 - lunch
		}
		s1 = avoidRoundMins(s1)
		if s1 <= e1 {
			// A saída do almoço tem de ficar entre a entrada e o retorno reais.
			s1 = meioEntre(e1, e2)
		}
		return e1, s1, e2, s2, o
	}
	if e1 > 0 && s1 > 0 && e2 == 0 && s2 > 0 {
		lunch := minAlmoco + randBetween(0, 15)
		e2 = s1 + lunch
		e2 = avoidRoundMins(e2)
		if e2 >= s2 {
			e2 = s2 - 30
		}
		return e1, s1, e2, s2, o
	}
	if e1 > 0 && s1 > 0 && e2 > 0 && s2 == 0 {
		morningWork := s1 - e1
		afternoonNeeded := carga - morningWork + randBetween(-5, 15)
		if afternoonNeeded < 180 {
			afternoonNeeded = 180
		}
		s2 = e2 + afternoonNeeded
		s2 = avoidRoundMins(s2)
		return e1, s1, e2, s2, o
	}

	// e1 e e2 (sem saídas)
	if e1 > 0 && s1 == 0 && e2 > 0 && s2 == 0 {
		s1 = e2 - minAlmoco - randBetween(0, 15)
		s1 = avoidRoundMins(s1)
		if s1 <= e1 {
			// Entrada e retorno perto demais para o almoço mínimo: a saída tem
			// de caber ENTRE os dois pontos reais, nunca depois do retorno.
			s1 = meioEntre(e1, e2)
		}
		morningWork := s1 - e1
		afternoonNeeded := carga - morningWork + randBetween(-5, 15)
		if afternoonNeeded < 180 {
			afternoonNeeded = 180
		}
		s2 = e2 + afternoonNeeded
		s2 = avoidRoundMins(s2)
		return e1, s1, e2, s2, o
	}

	// s1 e s2 (sem entradas)
	if e1 == 0 && s1 > 0 && e2 == 0 && s2 > 0 {
		morningWork := randBetween(200, 240)
		e1 = s1 - morningWork
		e1 = avoidRoundMins(e1)
		if e1 < 7*60 {
			e1 = 7*60 + randBetween(2, 28)
		}
		lunch := minAlmoco + randBetween(0, 15)
		e2 = s1 + lunch
		e2 = avoidRoundMins(e2)
		if e2 >= s2 {
			e2 = s2 - 30
		}
		return e1, s1, e2, s2, o
	}

	// Fallback
	fmt.Printf("[RulesAdjuster] Caso não previsto: e1=%d s1=%d e2=%d s2=%d\n", e1, s1, e2, s2)
	if e1 == 0 {
		e1 = 8*60 + randBetween(0, 60)
		e1 = avoidRoundMins(e1)
	}
	if s1 == 0 {
		s1 = e1 + randBetween(200, 240)
		s1 = avoidRoundMins(s1)
	}
	if e2 == 0 {
		e2 = s1 + minAlmoco + randBetween(0, 15)
		e2 = avoidRoundMins(e2)
	}
	if s2 == 0 {
		morningWork := s1 - e1
		s2 = e2 + (carga - morningWork) + randBetween(-5, 15)
		s2 = avoidRoundMins(s2)
	}
	return e1, s1, e2, s2, o
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

func randBetween(min, max int) int {
	if min >= max {
		return min
	}
	return min + rand.Intn(max-min+1)
}

func avoidRoundMins(m int) int {
	mins := m % 60
	if mins == 0 || mins == 15 || mins == 30 || mins == 45 {
		offset := randBetween(1, 7)
		if rand.Intn(2) == 0 {
			offset = -offset
		}
		m += offset
		if m%60 < 0 {
			m += 60
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
