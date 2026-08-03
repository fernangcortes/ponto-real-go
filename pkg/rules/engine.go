package rules

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/fernangcortes/ponto-real-go/pkg/models"
)

// Engine é o motor de regras de ponto que valida e classifica dias.
type Engine struct {
	Config models.RulesConfig
}

// NewEngine cria um Engine com as regras do arquivo JSON.
func NewEngine(configPath string) (*Engine, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler regras: %w", err)
	}

	var config models.RulesConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("erro ao parsear regras: %w", err)
	}

	return &Engine{Config: config}, nil
}

// NewEngineWithDefaults cria um Engine com regras padrão UEG.
func NewEngineWithDefaults() *Engine {
	return &Engine{
		Config: models.RulesConfig{
			CargaHorariaDiaria: 480,
			AlmocoMinimo:       60,
			VariacaoMin:        475,
			VariacaoMax:        500,
			AlmocoGeradoMin:    60,
			AlmocoGeradoMax:    75,
			HorarioContratual:  "08:30-12:00/13:00-17:30",
			NomeInstituicao:    "UNIVERSIDADE ESTADUAL DE GOIÁS - UEG",
		},
	}
}

// TimeToMinutes converte "HH:MM" para minutos desde meia-noite.
// Retorna 0 se o formato for inválido ou vazio.
func TimeToMinutes(t string) int {
	t = strings.TrimSpace(t)
	if t == "" || t == "**:**" {
		return 0
	}
	parts := strings.Split(t, ":")
	if len(parts) != 2 {
		return 0
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return 0
	}
	return h*60 + m
}

// MinutesToTime converte minutos para string "HH:MM" com sinal.
func MinutesToTime(m int) string {
	sign := "+"
	if m < 0 {
		sign = "-"
		m = -m
	}
	return fmt.Sprintf("%s%02d:%02d", sign, m/60, m%60)
}

// MinutesToTimeUnsigned converte minutos para string "HH:MM" sem sinal.
func MinutesToTimeUnsigned(m int) string {
	if m < 0 {
		m = -m
	}
	return fmt.Sprintf("%02d:%02d", m/60, m%60)
}

// IsTimeValid verifica se uma string de horário é válida e não vazia.
func IsTimeValid(t string) bool {
	t = strings.TrimSpace(t)
	if t == "" || t == "**:**" {
		return false
	}
	parts := strings.Split(t, ":")
	if len(parts) != 2 {
		return false
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return false
	}
	return h >= 0 && h <= 23 && m >= 0 && m <= 59
}

// diasUteis contém os dias da semana que são úteis.
var diasUteis = map[string]bool{
	"Seg": true, "Ter": true, "Qua": true, "Qui": true, "Sex": true,
}

// CargaDoDia retorna a jornada exigida no dia, em minutos.
// Usa a carga específica do dia quando definida (ex: decreto de expediente
// reduzido); caso contrário, a carga global das regras.
func (e *Engine) CargaDoDia(d *models.DayRecord) int {
	if d.CargaEsperada > 0 {
		return d.CargaEsperada
	}
	return e.Config.CargaHorariaDiaria
}

// ClassifyDay classifica o tipo de um dia baseado nos seus dados.
func (e *Engine) ClassifyDay(d *models.DayRecord) models.DayType {
	// Interpretar a observação/ocorrência com o vocabulário do SFR.
	switch ClassifyObservacao(d.Motivo, d.Ocorrencia) {
	case ObsDispensa:
		return models.DayTypeDispensa
	case ObsRecesso:
		return models.DayTypeRecesso
	case ObsFeriado, ObsPontoFacultativo:
		return models.DayTypeFeriado
	case ObsExpedienteReduzido:
		// Jornada reduzida por decreto: o ponto batido já vale como cumprido.
		return models.DayTypeExpedienteReduzido
	}
	// ObsCompensacao é apenas informativo: segue a classificação normal.

	// Finais de semana
	if d.DiaSemana == "Sáb" || d.DiaSemana == "Sab" || d.DiaSemana == "Dom" {
		return models.DayTypeFolga
	}

	// Contar pontos preenchidos
	pontos := 0
	if IsTimeValid(d.Entrada1) {
		pontos++
	}
	if IsTimeValid(d.Saida1) {
		pontos++
	}
	if IsTimeValid(d.Entrada2) {
		pontos++
	}
	if IsTimeValid(d.Saida2) {
		pontos++
	}

	// 4 pontos = completo
	if pontos == 4 {
		return models.DayTypeCompleto
	}

	// 1-3 pontos = parcial (ponto faltante)
	if pontos > 0 {
		return models.DayTypeParcial
	}

	// 0 pontos em dia útil = falta (se não tem justificativa)
	if diasUteis[d.DiaSemana] {
		return models.DayTypeFalta
	}

	return models.DayTypeFolga
}

// CalculateDayWorked calcula os minutos trabalhados em um dia com 4 pontos.
func (e *Engine) CalculateDayWorked(d *models.DayRecord) int {
	e1 := TimeToMinutes(d.Entrada1)
	s1 := TimeToMinutes(d.Saida1)
	e2 := TimeToMinutes(d.Entrada2)
	s2 := TimeToMinutes(d.Saida2)

	if e1 == 0 || s1 == 0 || e2 == 0 || s2 == 0 {
		return 0
	}

	return (s1 - e1) + (s2 - e2)
}

// CalculateLunchDuration calcula a duração do almoço em minutos.
func (e *Engine) CalculateLunchDuration(d *models.DayRecord) int {
	s1 := TimeToMinutes(d.Saida1)
	e2 := TimeToMinutes(d.Entrada2)
	if s1 == 0 || e2 == 0 {
		return 0
	}
	return e2 - s1
}

// ValidateDay verifica se os horários de um dia são válidos.
// Retorna lista de erros encontrados (vazia se tudo OK).
func (e *Engine) ValidateDay(d *models.DayRecord) []string {
	var errs []string

	// Só valida se temos 4 horários preenchidos
	if !IsTimeValid(d.Entrada1) || !IsTimeValid(d.Saida1) ||
		!IsTimeValid(d.Entrada2) || !IsTimeValid(d.Saida2) {
		return errs // sem validação para dias incompletos
	}

	e1 := TimeToMinutes(d.Entrada1)
	s1 := TimeToMinutes(d.Saida1)
	e2 := TimeToMinutes(d.Entrada2)
	s2 := TimeToMinutes(d.Saida2)

	// Ordem cronológica
	if e1 >= s1 {
		errs = append(errs, "entrada 1 deve ser antes da saída 1")
	}
	if s1 >= e2 {
		errs = append(errs, "saída 1 (almoço) deve ser antes da entrada 2")
	}
	if e2 >= s2 {
		errs = append(errs, "entrada 2 deve ser antes da saída 2")
	}

	// Duração do almoço
	lunch := e2 - s1
	if lunch < e.Config.AlmocoMinimo {
		errs = append(errs, fmt.Sprintf("almoço de %d min é menor que o mínimo de %d min", lunch, e.Config.AlmocoMinimo))
	}

	// Carga horária (respeita jornada reduzida do dia, quando houver)
	carga := e.CargaDoDia(d)
	worked := (s1 - e1) + (s2 - e2)
	if worked < carga {
		errs = append(errs, fmt.Sprintf("carga de %s é menor que %s",
			MinutesToTimeUnsigned(worked), MinutesToTimeUnsigned(carga)))
	}

	return errs
}

// ParseSaldo converte uma string de saldo ("+01:20", "-08:00", "00:00") para minutos.
func ParseSaldo(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	isNeg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	s = strings.TrimPrefix(s, "+")
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return 0
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return 0
	}
	total := h*60 + m
	if isNeg {
		return -total
	}
	return total
}

// CalculateSummary calcula os totais do mês a partir dos dias.
func (e *Engine) CalculateSummary(dias []models.DayRecord) models.TimesheetSummary {
	var summary models.TimesheetSummary

	for i := range dias {
		d := &dias[i]

		// Classificar o dia se não classificado
		if d.Tipo == "" {
			d.Tipo = e.ClassifyDay(d)
		}

		// Dia justificado que veio sem nenhum batimento: sinal de que a leitura
		// perdeu os horários da ficha. É o aviso mais grave — perder ponto real
		// em silêncio é pior que uma carga por conferir — então tem prioridade.
		semHorario := (d.Tipo == models.DayTypeDispensa ||
			d.Tipo == models.DayTypeExpedienteReduzido) && nenhumHorarioValido(d)
		if semHorario {
			d.Revisar = true
			d.RevisarMotivo = MsgRevisarSemHorario
		}

		// Expediente reduzido por decreto: o ponto batido já vale como cumprido.
		// Sem carga definida para o dia não há como apurar déficit, então o dia
		// entra como neutro e fica sinalizado para conferência manual.
		if d.Tipo == models.DayTypeExpedienteReduzido && d.CargaEsperada == 0 {
			d.SaldoReal = "00:00"
			if !semHorario {
				d.Revisar = true
				d.RevisarMotivo = MsgRevisarCargaReduzida
			}
			summary.DiasAjustados++
			continue
		}
		// Carga informada: o dia foi conferido, não precisa mais de revisão.
		if d.Tipo == models.DayTypeExpedienteReduzido && d.RevisarMotivo == MsgRevisarCargaReduzida {
			d.Revisar = false
			d.RevisarMotivo = ""
		}

		// 1. Calcular Saldo Real baseando-se em Matemática Irrefutável
		if allTimesValid(d) {
			worked := e.CalculateDayWorked(d)
			diff := worked - e.CargaDoDia(d)
			if diff == 0 {
				d.SaldoReal = "00:00"
			} else if diff > 0 {
				d.SaldoReal = "+" + MinutesToTimeUnsigned(diff)
			} else {
				d.SaldoReal = "-" + MinutesToTimeUnsigned(-diff)
			}
			summary.SaldoTotalRealMinutos += diff
		} else {
			// Se o dia não tem 4 pontos válidos, saldo real depende do tipo
			if d.Tipo == models.DayTypeFalta {
				summary.SaldoTotalRealMinutos -= e.CargaDoDia(d)
				d.SaldoReal = "-" + MinutesToTimeUnsigned(e.CargaDoDia(d))
			} else {
				// Feriado, Folga, Recesso ou Parcial que não foi ajustado -> Saldo real = Saldo Oficial (para não negativar injustamente)
				summary.SaldoTotalRealMinutos += ParseSaldo(d.Saldo)
				d.SaldoReal = d.Saldo
			}
		}

		// 2. Calcular Saldo Oficial/Extraído baseando-se no original (d.Saldo)
		switch d.Tipo {
		case models.DayTypeCompleto:
			summary.DiasCompletos++
			if d.Bloqueio != nil && containsZero(d.Bloqueio) {
				summary.DiasAjustados++
				// Se nós geramos horários (Adjuster rodou), o saldo extraído não existe mais e usamos o novo
				worked := e.CalculateDayWorked(d)
				summary.SaldoTotalMinutos += worked - e.CargaDoDia(d)
			} else {
				summary.SaldoTotalMinutos += ParseSaldo(d.Saldo)
			}

		case models.DayTypeParcial:
			summary.DiasAjustados++
			if allTimesValid(d) {
				// Se o adjuster reescreveu, calcular na mão
				worked := e.CalculateDayWorked(d)
				summary.SaldoTotalMinutos += worked - e.CargaDoDia(d)
			} else {
				summary.SaldoTotalMinutos += ParseSaldo(d.Saldo)
			}

		case models.DayTypeExpedienteReduzido:
			// Chega aqui apenas com carga do dia definida pelo usuário.
			summary.DiasAjustados++
			if allTimesValid(d) {
				worked := e.CalculateDayWorked(d)
				summary.SaldoTotalMinutos += worked - e.CargaDoDia(d)
			}

		case models.DayTypeFalta:
			summary.TotalFaltas++
			summary.SaldoTotalMinutos += ParseSaldo(d.Saldo)

		default:
			// Feriado, folga, dispensa, recesso: usa saldo original se existir
			summary.SaldoTotalMinutos += ParseSaldo(d.Saldo)
		}
	}

	summary.SaldoTotalFmt = MinutesToTime(summary.SaldoTotalMinutos)
	summary.SaldoTotalRealFmt = MinutesToTime(summary.SaldoTotalRealMinutos)
	return summary
}

func containsZero(arr []int) bool {
	for _, v := range arr {
		if v == 0 {
			return true
		}
	}
	return false
}

func allTimesValid(d *models.DayRecord) bool {
	return IsTimeValid(d.Entrada1) && IsTimeValid(d.Saida1) &&
		IsTimeValid(d.Entrada2) && IsTimeValid(d.Saida2)
}

// nenhumHorarioValido informa que o dia não tem batimento algum.
func nenhumHorarioValido(d *models.DayRecord) bool {
	return !IsTimeValid(d.Entrada1) && !IsTimeValid(d.Saida1) &&
		!IsTimeValid(d.Entrada2) && !IsTimeValid(d.Saida2)
}
