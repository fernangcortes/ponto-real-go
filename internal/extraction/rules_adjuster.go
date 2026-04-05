package extraction

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/fernangcortes/ponto-real-go/internal/models"
	"github.com/fernangcortes/ponto-real-go/internal/rules"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

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
			d.Saldo = "-" + formatMinsDuration(r.Engine.Config.CargaHorariaDiaria)
			continue
		}

		// Ajustar
		var o []int
		e1, s1, e2, s2, o = r.adjustDay(e1, s1, e2, s2)

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
		diff := total - r.Engine.Config.CargaHorariaDiaria
		if diff == 0 {
			d.Saldo = "00:00"
		} else if diff > 0 {
			d.Saldo = "+" + formatMinsDuration(diff)
		} else {
			d.Saldo = "-" + formatMinsDuration(-diff)
		}
	}

	return &result
}

// adjustDay gera horários faltantes baseado nos existentes e na Carga Horária.
func (r *RulesAdjuster) adjustDay(e1, s1, e2, s2 int) (int, int, int, int, []int) {
	carga := r.Engine.Config.CargaHorariaDiaria
	minAlmoco := r.Engine.Config.AlmocoMinimo

	// Regras explícitas de Posição Única (se só temos 1 ponto preenchido em qualquer var, encontrar qual é o real):
	filledList := []int{}
	if e1 > 0 {
		filledList = append(filledList, e1)
	}
	if s1 > 0 {
		filledList = append(filledList, s1)
	}
	if e2 > 0 {
		filledList = append(filledList, e2)
	}
	if s2 > 0 {
		filledList = append(filledList, s2)
	}

	if len(filledList) == 1 {
		val := filledList[0]
		e1, s1, e2, s2 = 0, 0, 0, 0
		if val <= 10*60+30 {
			e1 = val // antes de 10:30 = Entrada
		} else if val <= 12*60 {
			s1 = val // entre 10:31 e 12:00 = Saída do almoço
		} else if val <= 16*60 {
			e2 = val // após 12:00 até 16:00 = Retorno do almoço
		} else {
			s2 = val // após as 16:00, certamente é a Saída 2 (fim do expediente)
		}
	}

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
			s1 = e1 + 180
			s1 = avoidRoundMins(s1)
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
			s1 = e1 + 180
			s1 = avoidRoundMins(s1)
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
