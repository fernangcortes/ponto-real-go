package service

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/fernangcortes/ponto-real-go/pkg/apperr"
	"github.com/fernangcortes/ponto-real-go/pkg/extraction"
	"github.com/fernangcortes/ponto-real-go/pkg/models"
	"github.com/fernangcortes/ponto-real-go/pkg/repository"
	"github.com/fernangcortes/ponto-real-go/pkg/rules"
)

// TimesheetService orquestra os casos de uso de folha de ponto.
type TimesheetService struct {
	engine           *rules.Engine
	repo             repository.TimesheetRepository
	extractorFactory extraction.ExtractorFactory
}

// NewTimesheetService cria uma nova instância de TimesheetService com injeção de dependências.
func NewTimesheetService(
	engine *rules.Engine,
	repo repository.TimesheetRepository,
	extractorFactory extraction.ExtractorFactory,
) *TimesheetService {
	return &TimesheetService{
		engine:           engine,
		repo:             repo,
		extractorFactory: extractorFactory,
	}
}

// GetEngineConfig retorna a configuração atual do motor de regras.
func (s *TimesheetService) GetEngineConfig() models.RulesConfig {
	return s.engine.Config
}

// Process processa uma lista de dias, classificando-os e gerando o resumo de saldo.
func (s *TimesheetService) Process(req models.ProcessRequest) (*models.ProcessResponse, error) {
	if len(req.Dias) == 0 {
		return nil, fmt.Errorf("%w: nenhum dia fornecido", apperr.ErrRequisicaoInvalida)
	}

	ts := &models.Timesheet{
		Version: 1,
		MesAno:  req.MesAno,
		Dias:    req.Dias,
	}

	// Corrigir o dia da semana pelo calendário real quando o mês é conhecido.
	// Sem isso o motor confia no campo "w" lido do documento, que às vezes vem
	// deslocado — e é ele que decide se um dia sem batimento é falta ou folga.
	// Com MesAno vazio a função não faz nada, então continua válido chamar sem.
	extraction.AlignObservacoes(ts)

	for i := range ts.Dias {
		ts.Dias[i].Tipo = s.engine.ClassifyDay(&ts.Dias[i])
	}

	return &models.ProcessResponse{
		Timesheet: *ts,
		Summary:   s.engine.CalculateSummary(ts.Dias),
	}, nil
}

// Validate valida as regras de horários para um único dia.
func (s *TimesheetService) Validate(day models.DayRecord) models.ValidateResponse {
	errs := s.engine.ValidateDay(&day)
	worked := s.engine.CalculateDayWorked(&day)
	lunch := s.engine.CalculateLunchDuration(&day)

	return models.ValidateResponse{
		Valid:   len(errs) == 0,
		Errors:  errs,
		Worked:  rules.MinutesToTimeUnsigned(worked),
		Lunch:   rules.MinutesToTimeUnsigned(lunch),
		Balance: rules.MinutesToTime(worked - s.engine.Config.CargaHorariaDiaria),
	}
}

// ListMonths retorna todos os meses cadastrados no repositório.
func (s *TimesheetService) ListMonths() ([]models.MonthSummary, error) {
	return s.repo.List()
}

// LoadMonth carrega os dados de um mês específico.
//
// Os avisos de conferência são diagnósticos derivados dos dados, não fatos
// gravados: reavaliamos na leitura para que um aviso já resolvido não
// reapareça, e para que dados salvos por versões antigas sejam reclassificados
// com as regras atuais.
func (s *TimesheetService) LoadMonth(mesAno string) (*models.MonthData, error) {
	data, err := s.repo.Load(mesAno)
	if err != nil || data == nil {
		return data, err
	}

	ts := &models.Timesheet{MesAno: data.MesAno, Servidor: data.Servidor}
	ts.Dias = make([]models.DayRecord, len(data.Dias))
	for i, d := range data.Dias {
		ts.Dias[i] = d.DayRecord
	}

	extraction.AlignObservacoes(ts)
	for i := range ts.Dias {
		ts.Dias[i].Tipo = s.engine.ClassifyDay(&ts.Dias[i])
	}
	s.engine.CalculateSummary(ts.Dias)

	for i := range data.Dias {
		data.Dias[i].DayRecord = ts.Dias[i]
	}
	return data, nil
}

// SaveMonth salva o estado completo de um mês no repositório.
func (s *TimesheetService) SaveMonth(data models.MonthData) error {
	return s.repo.Save(data)
}

// IsValidMesAno verifica se o formato MesAno é válido (ex: 04/2026).
func (s *TimesheetService) IsValidMesAno(mesAno string) bool {
	if mesAno == "" {
		return false
	}
	if strings.Contains(mesAno, "?") {
		return false
	}
	parts := strings.Split(mesAno, "/")
	if len(parts) != 2 {
		return false
	}
	if len(parts[0]) != 2 || len(parts[1]) != 4 {
		return false
	}
	return true
}

// ExtractDateFromFilename tenta inferir a data (mês/ano) pelo nome do arquivo.
func (s *TimesheetService) ExtractDateFromFilename(filename string) string {
	filename = strings.ToLower(filename)

	type monthSearch struct {
		name string
		code string
	}
	monthsSearchList := []monthSearch{
		{"fevereiro", "02"},
		{"setembro", "09"},
		{"novembro", "11"},
		{"dezembro", "12"},
		{"janeiro", "01"},
		{"outubro", "10"},
		{"agosto", "08"},
		{"abril", "04"},
		{"março", "03"},
		{"marco", "03"},
		{"junho", "06"},
		{"julho", "07"},
		{"maio", "05"},
		{"fev", "02"},
		{"set", "09"},
		{"nov", "11"},
		{"dez", "12"},
		{"jan", "01"},
		{"out", "10"},
		{"ago", "08"},
		{"abr", "04"},
		{"mar", "03"},
		{"jun", "06"},
		{"jul", "07"},
		{"mai", "05"},
	}

	detectedMonth := ""
	for _, m := range monthsSearchList {
		if strings.Contains(filename, m.name) {
			detectedMonth = m.code
			break
		}
	}

	detectedYear := ""
	// Tentar detectar ano de 4 dígitos (2020-2039)
	reYear4 := regexp.MustCompile(`20[2-3]\d`)
	detectedYear = reYear4.FindString(filename)

	// Se não achou ano de 4 dígitos, procurar 2 dígitos após o nome do mês
	if detectedYear == "" && detectedMonth != "" {
		reYear2 := regexp.MustCompile(`\d{2}`)
		allNums := reYear2.FindAllString(filename, -1)
		for _, num := range allNums {
			if num != detectedMonth {
				detectedYear = "20" + num
				break
			}
		}
	}

	// Se ainda não achou ano, usar o ano atual como fallback
	if detectedYear == "" && detectedMonth != "" {
		detectedYear = fmt.Sprintf("%d", time.Now().Year())
	}

	// Se não achou nome de mês, tentar padrão numérico como "04-2026" ou "04_2026" ou "2026_04"
	if detectedMonth == "" {
		reNumDate := regexp.MustCompile(`(?:0[1-9]|1[0-2])[_\-\/](20[2-3]\d)`)
		match := reNumDate.FindStringSubmatch(filename)
		if len(match) > 0 {
			// A string pode ser algo como "04-2026"
			detectedYear = match[1]
			// O mês é os primeiros 2 dígitos
			parts := strings.Split(regexp.MustCompile(`[_\-\/]`).ReplaceAllString(match[0], " "), " ")
			if len(parts) > 0 {
				detectedMonth = parts[0]
			}
		} else {
			reNumDateRev := regexp.MustCompile(`(20[2-3]\d)[_\-\/](0[1-9]|1[0-2])`)
			matchRev := reNumDateRev.FindStringSubmatch(filename)
			if len(matchRev) == 3 {
				detectedYear = matchRev[1]
				detectedMonth = matchRev[2]
			}
		}
	}

	if detectedMonth != "" && detectedYear != "" {
		return fmt.Sprintf("%s/%s", detectedMonth, detectedYear)
	}

	return ""
}
