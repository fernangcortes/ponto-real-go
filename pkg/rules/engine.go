package rules

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/fernangcortes/ponto-real-go/pkg/models"
)

// regrasPadrao são as regras embutidas no binário.
//
// Embutir em vez de ler do disco elimina duas armadilhas que existiam antes:
// o servidor local procurava rules.json por caminho relativo (e caía num padrão
// diferente se o executável fosse chamado de outro diretório), enquanto o deploy
// serverless usava um conjunto de regras hardcoded em Go. O mesmo cálculo
// produzia resultados diferentes conforme onde rodava.
//
//go:embed rules.json
var regrasPadrao []byte

// Engine é o motor de regras de ponto que valida e classifica dias.
type Engine struct {
	Config models.RulesConfig
}

// NewEngine cria um Engine com as regras de um arquivo JSON externo.
// Use apenas para sobrescrever conscientemente as regras embutidas.
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

// NewEngineWithDefaults cria um Engine com as regras embutidas no binário.
//
// Entra em pânico se o JSON embutido for inválido: isso é erro de build, não
// condição de execução, e TestRegrasEmbutidasSaoValidas o pega antes do deploy.
func NewEngineWithDefaults() *Engine {
	var config models.RulesConfig
	if err := json.Unmarshal(regrasPadrao, &config); err != nil {
		panic(fmt.Sprintf("rules.json embutido é inválido: %v", err))
	}
	return &Engine{Config: config}
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

// tipoPorEscolhaManual traduz a escolha do usuário no seletor da linha.
//
// "util" está ausente de propósito: escolhê-lo derruba a detecção e devolve o
// dia à contagem normal de batimentos, que é como o usuário desfaz uma detecção
// equivocada.
var tipoPorEscolhaManual = map[string]models.DayType{
	"fds":        models.DayTypeFolga,
	"folga":      models.DayTypeFolga,
	"feriado":    models.DayTypeFeriado,
	"convocacao": models.DayTypeRecesso,
	"dispensa":   models.DayTypeDispensa,
	"reduzido":   models.DayTypeExpedienteReduzido,
	"ferias":     models.DayTypeFerias,
}

// ClassifyDay classifica o tipo de um dia baseado nos seus dados.
func (e *Engine) ClassifyDay(d *models.DayRecord) models.DayType {
	// A escolha manual do usuário prevalece sobre a leitura da observação.
	if d.DayTypeOverride != "" {
		if t, ok := tipoPorEscolhaManual[d.DayTypeOverride]; ok {
			return t
		}
		// "util": o usuário afirmou que é dia de trabalho apesar da observação.
		// Pula a detecção e vai direto para a contagem de batimentos.
		return e.classificarPorBatimentos(d)
	}

	// Interpretar a observação/ocorrência com o vocabulário do SFR.
	switch ClassifyObservacao(d.Motivo, d.Ocorrencia) {
	case ObsDispensa:
		return models.DayTypeDispensa
	case ObsFerias:
		return models.DayTypeFerias
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

	return e.classificarPorBatimentos(d)
}

// classificarPorBatimentos classifica pelo que foi registrado no ponto, sem
// olhar a observação. É o caminho normal dos dias úteis e também aonde vai
// parar o dia que o usuário marcou explicitamente como "útil".
func (e *Engine) classificarPorBatimentos(d *models.DayRecord) models.DayType {
	pontos := 0
	for _, t := range []string{d.Entrada1, d.Saida1, d.Entrada2, d.Saida2} {
		if IsTimeValid(t) {
			pontos++
		}
	}

	if pontos == 4 {
		return models.DayTypeCompleto
	}
	if pontos > 0 {
		return models.DayTypeParcial // ponto faltante
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

// CalculateTurnosTrabalhados soma os turnos que estiverem completos, mesmo que
// o dia não tenha os quatro batimentos.
//
// Diferente de CalculateDayWorked, que exige os quatro e devolve 0 sem eles.
// Num dia de dispensa ou expediente reduzido é normal haver só o turno da
// manhã — exigir os quatro fazia a jornada do ato ser ignorada e o dia entrar
// como zero.
func (e *Engine) CalculateTurnosTrabalhados(d *models.DayRecord) int {
	total := 0
	if IsTimeValid(d.Entrada1) && IsTimeValid(d.Saida1) {
		total += TimeToMinutes(d.Saida1) - TimeToMinutes(d.Entrada1)
	}
	if IsTimeValid(d.Entrada2) && IsTimeValid(d.Saida2) {
		total += TimeToMinutes(d.Saida2) - TimeToMinutes(d.Entrada2)
	}
	return total
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

		// Saldo Oficial: a soma FIEL do que a ficha imprimiu, sem recálculo.
		//
		// Antes este total recalculava os dias em que o sistema gerou horário —
		// o dia 1 de junho, impresso como -06:47 porque faltou a entrada,
		// entrava como +01:52 depois do horário gerado. Somados os dias
		// ajustados, o mês virava +04:32 em vez de -26:15.
		//
		// O número existe para ser o retrato do problema: "a ficha acusa X, a
		// matemática diz Y, e a diferença é o que as justificativas explicam".
		// Recalculá-lo o aproximava do saldo real e o esvaziava de função, além
		// de contradizer o rótulo que a interface mostra ("soma dos saldos
		// originais da imagem").
		//
		// Fica antes de qualquer `continue`: todo dia entra, inclusive os neutros.
		summary.SaldoTotalMinutos += ParseSaldo(d.Saldo)

		// Dia justificado que veio sem nenhum batimento: sinal de que a leitura
		// perdeu os horários da ficha. É o aviso mais grave — perder ponto real
		// em silêncio é pior que uma carga por conferir — então tem prioridade.
		semHorario := (d.Tipo == models.DayTypeDispensa ||
			d.Tipo == models.DayTypeExpedienteReduzido) && nenhumHorarioValido(d)
		if semHorario {
			d.Revisar = true
			d.RevisarMotivo = MsgRevisarSemHorario
		}

		// Dias cuja jornada exigida vem de um ato (decreto de expediente
		// reduzido, ato de dispensa) e portanto varia caso a caso.
		//
		// Sem a carga informada não há contra o que apurar: o dia entra neutro e
		// pede conferência. A alternativa — arbitrar uma jornada — produziria um
		// saldo que ninguém consegue justificar diante do ato.
		if cargaPorAto(d.Tipo) {
			if d.CargaEsperada == 0 {
				d.SaldoReal = "00:00"
				if !semHorario {
					d.Revisar = true
					d.RevisarMotivo = msgCargaPendente(d.Tipo)
				}
				summary.DiasAjustados++
				continue
			}
			// Carga informada: o dia foi conferido, não precisa mais de revisão.
			if d.RevisarMotivo == MsgRevisarCargaReduzida || d.RevisarMotivo == MsgRevisarCargaDispensa {
				d.Revisar = false
				d.RevisarMotivo = ""
			}
		}

		// 1. Saldo Real, pela matemática dos batimentos.
		switch {
		case neutroPorEscolhaManual(d):
			// O usuário marcou o dia como feriado/folga/FDS/convocação. Ele está
			// dizendo que o dia não conta — nem a favor nem contra — mesmo que
			// haja ponto batido.
			d.SaldoReal = "00:00"

		case cargaPorAto(d.Tipo):
			// Chega aqui só com a jornada do ato informada. Soma os turnos
			// completos: nesses dias é normal existir só o da manhã.
			diff := e.CalculateTurnosTrabalhados(d) - e.CargaDoDia(d)
			d.SaldoReal = formatarSaldo(diff)
			summary.SaldoTotalRealMinutos += diff

		case allTimesValid(d):
			diff := e.CalculateDayWorked(d) - e.CargaDoDia(d)
			d.SaldoReal = formatarSaldo(diff)
			summary.SaldoTotalRealMinutos += diff

		case d.Tipo == models.DayTypeFalta:
			summary.SaldoTotalRealMinutos -= e.CargaDoDia(d)
			d.SaldoReal = "-" + MinutesToTimeUnsigned(e.CargaDoDia(d))

		default:
			// Feriado, folga, recesso, férias ou parcial não ajustado: herda o
			// saldo lido da ficha, para não negativar injustamente.
			summary.SaldoTotalRealMinutos += ParseSaldo(d.Saldo)
			d.SaldoReal = d.Saldo
		}

		// 2. Contadores exibidos na barra de status.
		switch d.Tipo {
		case models.DayTypeCompleto:
			summary.DiasCompletos++
			// Completo só conta como ajustado se algum horário foi gerado.
			if containsZero(d.Bloqueio) {
				summary.DiasAjustados++
			}

		case models.DayTypeParcial,
			models.DayTypeExpedienteReduzido,
			models.DayTypeDispensa:
			// Reduzido e dispensa chegam aqui apenas com a jornada informada;
			// sem ela o laço já os contou e seguiu adiante.
			summary.DiasAjustados++

		case models.DayTypeFalta:
			summary.TotalFaltas++
		}
	}

	summary.SaldoTotalFmt = MinutesToTime(summary.SaldoTotalMinutos)
	summary.SaldoTotalRealFmt = MinutesToTime(summary.SaldoTotalRealMinutos)
	return summary
}

// cargaPorAto informa se a jornada exigida no dia é definida por um ato
// específico (decreto, portaria de dispensa) em vez da carga global.
func cargaPorAto(t models.DayType) bool {
	return t == models.DayTypeExpedienteReduzido || t == models.DayTypeDispensa
}

// tiposNeutrosPorEscolha são as marcações com que o usuário declara que o dia
// não entra na conta, nem a favor nem contra.
var tiposNeutrosPorEscolha = map[string]bool{
	"feriado": true, "folga": true, "fds": true, "convocacao": true,
}

func neutroPorEscolhaManual(d *models.DayRecord) bool {
	return tiposNeutrosPorEscolha[d.DayTypeOverride]
}

// formatarSaldo monta "+HH:MM", "-HH:MM" ou "00:00".
func formatarSaldo(minutos int) string {
	switch {
	case minutos == 0:
		return "00:00"
	case minutos > 0:
		return "+" + MinutesToTimeUnsigned(minutos)
	default:
		return "-" + MinutesToTimeUnsigned(-minutos)
	}
}

func msgCargaPendente(t models.DayType) string {
	if t == models.DayTypeDispensa {
		return MsgRevisarCargaDispensa
	}
	return MsgRevisarCargaReduzida
}

// containsZero informa se algum batimento do dia foi gerado pelo sistema.
// Slice nil (dia sem marcação) significa que nada foi gerado.
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
