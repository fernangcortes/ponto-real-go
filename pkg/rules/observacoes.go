package rules

import "strings"

// MsgRevisarCargaReduzida é o aviso de dia com jornada reduzida por decreto
// cuja carga exigida ainda não foi informada.
const MsgRevisarCargaReduzida = "Expediente reduzido: confira a carga horária do dia definida pelo decreto."

// MsgRevisarCargaDispensa é o aviso de dia de dispensa cuja jornada exigida
// ainda não foi informada.
//
// Cada dispensa tem a sua: o ato pode liberar o dia inteiro, meio período ou a
// partir de determinado horário. Sem essa informação não há contra o que apurar,
// então o dia entra neutro e pede conferência — em vez de o sistema arbitrar uma
// jornada e produzir um saldo que ninguém consegue justificar.
const MsgRevisarCargaDispensa = "Dispensa: informe a jornada exigida neste dia, conforme o ato que a concedeu."

// MsgRevisarSemHorario avisa que um dia justificado veio sem batimento algum.
// Na ficha do SFR esses dias costumam ter horário: quase sempre significa que a
// leitura perdeu os batimentos, e perder ponto real em silêncio é inaceitável.
const MsgRevisarSemHorario = "Nenhum horário foi lido para este dia. Confira a folha original: os batimentos podem ter se perdido na extração."

// MsgRevisarColunasNaoFecham avisa que os batimentos do dia existem mas não
// formam nenhum par de entrada e saída — logo o dia acusa a jornada inteira como
// débito, mesmo tendo sido cumprida.
//
// A causa típica é a extração ter lido um horário na coluna errada, e ela erra
// porque as janelas de plausibilidade foram calibradas para a jornada de 8h. No
// dia 03/07/2026, uma dispensa de 4h com 09:26 e 13:45, o 13:45 caiu no retorno
// do almoço: nenhum turno fechou, o dia acusou −04:00 em vez de +00:19 e a frase
// automática não saiu.
//
// O aviso só é possível DEPOIS de a jornada do ato ser informada, que é quando
// se sabe que 13:45 é o fim do expediente e não a volta do almoço. Por isso ele
// não cabe na extração: lá esse número ainda não existe.
const MsgRevisarColunasNaoFecham = "Nenhum turno fecha: os batimentos deste dia estão em colunas que não formam par de entrada e saída."

// ObsKind classifica o significado semântico de uma observação/ocorrência
// da ficha de frequência (coluna OBSERVAÇÕES/OCORRÊNCIAS do SFR).
type ObsKind string

const (
	ObsNenhuma            ObsKind = ""
	ObsFeriado            ObsKind = "feriado"
	ObsPontoFacultativo   ObsKind = "ponto_facultativo"
	ObsRecesso            ObsKind = "recesso"
	ObsFerias             ObsKind = "ferias"
	ObsDispensa           ObsKind = "dispensa"
	ObsExpedienteReduzido ObsKind = "expediente_reduzido"
	ObsCompensacao        ObsKind = "compensacao"
	ObsFimDeSemana        ObsKind = "fim_de_semana"
)

// normalizeObs deixa o texto em caixa alta e sem acentos, para que a
// comparação funcione tanto com "COMPENSAÇÃO" quanto "COMPENSACAO".
func normalizeObs(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))

	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case 'Á', 'À', 'Â', 'Ã', 'Ä':
			b.WriteRune('A')
		case 'É', 'È', 'Ê', 'Ë':
			b.WriteRune('E')
		case 'Í', 'Ì', 'Î', 'Ï':
			b.WriteRune('I')
		case 'Ó', 'Ò', 'Ô', 'Õ', 'Ö':
			b.WriteRune('O')
		case 'Ú', 'Ù', 'Û', 'Ü':
			b.WriteRune('U')
		case 'Ç':
			b.WriteRune('C')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// obsPattern associa um trecho de texto ao seu significado.
// A ordem importa: os padrões mais específicos vêm primeiro.
var obsPatterns = []struct {
	needle string
	kind   ObsKind
}{
	// Expediente reduzido por decreto (ex: "EXPEDIENTE REDUZIDO - COPA 2026 - DEC. 10.925/2026").
	{"EXPEDIENTE REDUZIDO", ObsExpedienteReduzido},
	{"HORARIO REDUZIDO", ObsExpedienteReduzido},
	{"JORNADA REDUZIDA", ObsExpedienteReduzido},

	// Férias homologadas: ausência autorizada, não é falta.
	//
	// Estava fora do vocabulário dos dois lados do sistema. O front-end acertava
	// por acidente (dia sem batimento e sem saldo caía em "folga"); o backend
	// aplicava "dia útil sem batimento = falta" e descontava a jornada inteira.
	// Num mês com 9 dias de férias isso somava 72 horas de débito inexistente.
	{"FERIAS", ObsFerias},

	{"DISPENSA", ObsDispensa},
	{"RECESSO", ObsRecesso},
	{"PONTO FACULTATIVO", ObsPontoFacultativo},
	{"FERIADO", ObsFeriado},

	// Puramente informativo: não altera cálculo nem preenchimento.
	{"COMPENSACAO", ObsCompensacao},

	{"SABADO", ObsFimDeSemana},
	{"DOMINGO", ObsFimDeSemana},
}

// ClassifyObservacao interpreta o texto livre de uma observação/ocorrência.
// Recebe quantos campos quiser (ocorrência, motivo) e devolve o primeiro
// significado reconhecido.
func ClassifyObservacao(textos ...string) ObsKind {
	for _, t := range textos {
		n := normalizeObs(t)
		if n == "" {
			continue
		}
		for _, p := range obsPatterns {
			if strings.Contains(n, p.needle) {
				return p.kind
			}
		}
	}
	return ObsNenhuma
}

// IsFimDeSemanaObs informa se o texto é um marcador de fim de semana
// (SÁBADO/DOMINGO), usado como âncora de calendário no realinhamento.
//
// Exige correspondência EXATA, não "contém": a extração às vezes funde duas
// observações numa só célula (ex: "DISPENSA PARA FREQUÊNCIA A CURSO DE
// DOUTORADO, MESTRADO, SÁBADO"). Tratar isso como âncora apontaria um sábado
// numa sexta-feira e acusaria desalinhamento onde não há.
func IsFimDeSemanaObs(texto string) (weekday string, ok bool) {
	switch normalizeObs(texto) {
	case "SABADO":
		return "Sáb", true
	case "DOMINGO":
		return "Dom", true
	}
	return "", false
}
