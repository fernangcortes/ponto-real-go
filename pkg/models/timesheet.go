package models

// ServerInfo contém os dados do servidor público extraídos da folha de frequência.
type ServerInfo struct {
	Nome      string `json:"nome"`
	CPF       string `json:"cpf"`
	Matricula string `json:"matricula"`
	Horario   string `json:"horario"` // ex: "1630 - UEG 8H (08:30H ÀS 12:00H / 13:00H ÀS 17:30H)"
	Unidade   string `json:"unidade"` // ex: "UNIDADE UNIVERSITÁRIA DE GOIÂNIA - LARANJEIRAS"
	Orgao     string `json:"orgao"`   // ex: "UNIVERSIDADE ESTADUAL DE GOIÁS - UEG"
}

// DayType classifica o tipo de dia na folha de ponto.
type DayType string

const (
	DayTypeFeriado  DayType = "feriado"
	DayTypeFolga    DayType = "folga"    // sáb/dom
	DayTypeFalta    DayType = "falta"    // dia útil sem registro e sem justificativa
	DayTypeParcial  DayType = "parcial"  // dia com ponto faltante (1-3 registros)
	DayTypeCompleto DayType = "completo" // dia com 4 pontos
	DayTypeDispensa DayType = "dispensa" // dispensa para curso, etc.
	DayTypeRecesso  DayType = "recesso"  // recesso institucional
)

// DayRecord representa um dia na folha de frequência.
type DayRecord struct {
	Dia        int     `json:"d"`
	DiaSemana  string  `json:"w"`
	Entrada1   string  `json:"e1"`
	Saida1     string  `json:"s1"`
	Entrada2   string  `json:"e2"`
	Saida2     string  `json:"s2"`
	ExpSaldo   string  `json:"es"`                   // Expediente/Saldo (E/S na tabela)
	Saldo      string  `json:"saldo"`                // Saldo Original/Extraído
	SaldoReal  string  `json:"saldo_real,omitempty"` // Saldo Calculado Matemático
	Ocorrencia string  `json:"ocor"`
	Motivo     string  `json:"mot"`
	Bloqueio   []int   `json:"o,omitempty"`    // [E1, S1, E2, S2]: 1=original(bloqueado), 0=gerado(editável)
	Tipo       DayType `json:"tipo,omitempty"` // classificação do dia
}

// Timesheet representa uma folha de frequência mensal completa.
type Timesheet struct {
	Version  int         `json:"version"`
	MesAno   string      `json:"mes_ano"` // "01/2026"
	Servidor ServerInfo  `json:"servidor"`
	Dias     []DayRecord `json:"dias"`
	Atrasos  string      `json:"atrasos"` // total de atrasos: "-09:28"
	Faltas   int         `json:"faltas"`  // total de faltas: 8
}

// TimesheetSummary contém os totais calculados do mês.
type TimesheetSummary struct {
	SaldoTotalMinutos     int    `json:"saldo_total_minutos"`
	SaldoTotalFmt         string `json:"saldo_total_fmt"` // "+02:30" ou "-09:28" (Oficial)
	SaldoTotalRealMinutos int    `json:"saldo_total_real_minutos"`
	SaldoTotalRealFmt     string `json:"saldo_total_real_fmt"` // Matemático
	TotalFaltas           int    `json:"total_faltas"`
	DiasAjustados         int    `json:"dias_ajustados"` // qtd de dias com horário gerado
	DiasCompletos         int    `json:"dias_completos"`
}

// ProcessRequest é o payload enviado pelo front-end para processamento.
type ProcessRequest struct {
	Dias []DayRecord `json:"dias"`
}

// ProcessResponse é a resposta do backend ao front-end.
type ProcessResponse struct {
	Timesheet Timesheet        `json:"timesheet"`
	Summary   TimesheetSummary `json:"summary"`
}

// RulesConfig contém as regras parametrizáveis de cálculo.
type RulesConfig struct {
	CargaHorariaDiaria int    `json:"carga_horaria_diaria_min"` // em minutos (480 = 8h)
	AlmocoMinimo       int    `json:"almoco_minimo_min"`        // em minutos (60 = 1h)
	VariacaoMin        int    `json:"variacao_min_min"`         // total mín gerado (475 = 7h55m)
	VariacaoMax        int    `json:"variacao_max_min"`         // total máx gerado (500 = 8h20m)
	AlmocoGeradoMin    int    `json:"almoco_gerado_min_min"`    // almoço gerado mín (60 = 1h)
	AlmocoGeradoMax    int    `json:"almoco_gerado_max_min"`    // almoço gerado máx (75 = 1h15m)
	HorarioContratual  string `json:"horario_contratual"`       // "08:30-12:00/13:00-17:30"
	NomeInstituicao    string `json:"nome_instituicao"`
}
