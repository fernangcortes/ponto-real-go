package extraction

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/fernangcortes/ponto-real-go/pkg/models"
	"github.com/fernangcortes/ponto-real-go/pkg/rules"
)

// msgConflitoCalendario é o aviso de marcador de fim de semana que não bate com
// o calendário. Tem prefixo próprio para que a reavaliação saiba distinguir os
// avisos que ela mesma emitiu dos que vieram de outra etapa (ex: carga do decreto).
const msgConflitoCalendario = "Observação diz %q, mas o dia %d é %s. A coluna de observações pode estar deslocada."

// AlignResult descreve o que o realinhamento fez com a folha.
type AlignResult struct {
	DeslocamentoAplicado int  // em dias; 0 = nada foi movido
	Desalinhado          bool // marcadores de fim de semana não batem com o calendário
	Corrigido            bool // conseguimos corrigir com segurança
}

// AlignObservacoes confere e corrige o alinhamento da coluna OBSERVAÇÕES da ficha SFR.
//
// Nessa ficha a coluna de observações é um bloco de texto corrido, não uma célula
// por linha: observações longas quebram em duas linhas e empurram as seguintes
// para baixo. O resultado é que a observação lida para o dia N frequentemente
// pertence a outro dia.
//
// Os marcadores SÁBADO e DOMINGO são a âncora: pelo calendário real sabemos
// exatamente em que dias eles deveriam cair.
//
//   - Se um único deslocamento global faz todos os marcadores baterem, aplicamos.
//   - Se o desalinhamento é irregular, NÃO adivinhamos: os dias com observação
//     ficam marcados para conferência manual.
//   - Se já está tudo certo, a função não faz nada.
//
// Também corrige o dia da semana ("w") a partir do calendário real, que é
// informação objetiva e não depende de leitura.
func AlignObservacoes(ts *models.Timesheet) AlignResult {
	var res AlignResult

	ano, mes, ok := parseMesAno(ts.MesAno)
	if !ok {
		return res
	}

	// Limpar avisos de uma avaliação anterior. São diagnósticos derivados dos
	// dados atuais, não fatos permanentes — sem isso um aviso já corrigido
	// (ou emitido por uma versão antiga) ficaria colado no mês para sempre.
	// Preservamos apenas os avisos que pertencem a outras etapas do pipeline.
	for i := range ts.Dias {
		switch ts.Dias[i].RevisarMotivo {
		case rules.MsgRevisarCargaReduzida, rules.MsgRevisarSemHorario, MsgRevisarMeiaNoite:
			continue // dono é o adjuster / o motor de regras
		}
		ts.Dias[i].Revisar = false
		ts.Dias[i].RevisarMotivo = ""
	}

	diasNoMes := time.Date(ano, time.Month(mes)+1, 0, 0, 0, 0, 0, time.UTC).Day()

	// Dia da semana real por número do dia — informação objetiva do calendário.
	weekdayDoDia := make(map[int]string, diasNoMes)
	fimDeSemanaReal := map[int]bool{}
	for dia := 1; dia <= diasNoMes; dia++ {
		wd := time.Date(ano, time.Month(mes), dia, 0, 0, 0, 0, time.UTC).Weekday()
		weekdayDoDia[dia] = abreviacaoWeekday(wd)
		if wd == time.Saturday || wd == time.Sunday {
			fimDeSemanaReal[dia] = true
		}
	}

	// Corrigir o dia da semana de cada registro.
	for i := range ts.Dias {
		if w, ok := weekdayDoDia[ts.Dias[i].Dia]; ok {
			ts.Dias[i].DiaSemana = w
		}
	}

	// Marcadores de fim de semana lidos, na ordem em que aparecem.
	type marcador struct {
		idx     int // posição no slice de dias
		dia     int // número do dia onde foi lido
		weekday string
	}
	var lidos []marcador
	for i := range ts.Dias {
		d := &ts.Dias[i]
		w, isFDS := rules.IsFimDeSemanaObs(d.Motivo)
		if !isFDS {
			w, isFDS = rules.IsFimDeSemanaObs(d.Ocorrencia)
		}
		if isFDS {
			dia := d.Dia
			if dia == 0 {
				dia = i + 1
			}
			lidos = append(lidos, marcador{idx: i, dia: dia, weekday: w})
		}
	}

	if len(lidos) == 0 {
		return res
	}

	// Já está alinhado? Todo marcador cai num fim de semana real e com o dia certo.
	alinhado := true
	for _, m := range lidos {
		if !fimDeSemanaReal[m.dia] || weekdayDoDia[m.dia] != m.weekday {
			alinhado = false
			break
		}
	}
	if alinhado {
		return res
	}

	res.Desalinhado = true

	// Procurar um deslocamento global único que faça todos os marcadores baterem.
	melhorShift := 0
	achou := false
	for shift := -5; shift <= 5; shift++ {
		if shift == 0 {
			continue
		}
		todosBatem := true
		for _, m := range lidos {
			destino := m.dia + shift
			if destino < 1 || destino > diasNoMes ||
				!fimDeSemanaReal[destino] || weekdayDoDia[destino] != m.weekday {
				todosBatem = false
				break
			}
		}
		if todosBatem {
			melhorShift = shift
			achou = true
			break
		}
	}

	if !achou {
		// Desalinhamento irregular. Não há mapeamento seguro, então marcamos
		// apenas os dias com evidência concreta — o marcador de fim de semana
		// que contradiz o calendário — em vez de poluir o mês inteiro.
		marcados := 0
		for _, m := range lidos {
			if fimDeSemanaReal[m.dia] && weekdayDoDia[m.dia] == m.weekday {
				continue // esse marcador está certo
			}
			idx := m.idx
			ts.Dias[idx].Revisar = true
			ts.Dias[idx].RevisarMotivo = fmt.Sprintf(msgConflitoCalendario,
				m.weekday, m.dia, weekdayDoDia[m.dia])
			marcados++
		}
		fmt.Printf("[Align] Desalinhamento irregular; %d marcador(es) de fim de semana em conflito\n", marcados)
		return res
	}

	// Aplicar o deslocamento global às observações.
	type obs struct{ ocorrencia, motivo string }
	novos := make([]obs, len(ts.Dias))
	for i := range ts.Dias {
		d := &ts.Dias[i]
		if d.Motivo == "" && d.Ocorrencia == "" {
			continue
		}
		destino := i + melhorShift
		if destino < 0 || destino >= len(novos) {
			continue // observação sairia do mês; descartada junto com o deslocamento
		}
		novos[destino] = obs{ocorrencia: d.Ocorrencia, motivo: d.Motivo}
	}
	for i := range ts.Dias {
		ts.Dias[i].Ocorrencia = novos[i].ocorrencia
		ts.Dias[i].Motivo = novos[i].motivo
	}

	res.DeslocamentoAplicado = melhorShift
	res.Corrigido = true
	fmt.Printf("[Align] Observações realinhadas em %+d dia(s) via âncora de fim de semana\n", melhorShift)
	return res
}

func abreviacaoWeekday(w time.Weekday) string {
	switch w {
	case time.Monday:
		return "Seg"
	case time.Tuesday:
		return "Ter"
	case time.Wednesday:
		return "Qua"
	case time.Thursday:
		return "Qui"
	case time.Friday:
		return "Sex"
	case time.Saturday:
		return "Sáb"
	default:
		return "Dom"
	}
}

// parseMesAno converte "06/2026" em (2026, 6, true).
func parseMesAno(s string) (ano, mes int, ok bool) {
	parts := strings.Split(strings.TrimSpace(s), "/")
	if len(parts) != 2 {
		return 0, 0, false
	}
	m, err1 := strconv.Atoi(parts[0])
	a, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || m < 1 || m > 12 || a < 1900 {
		return 0, 0, false
	}
	return a, m, true
}
