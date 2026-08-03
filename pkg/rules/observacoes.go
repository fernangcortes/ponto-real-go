package rules

import "strings"

// MsgRevisarCargaReduzida é o aviso de dia com jornada reduzida por decreto
// cuja carga exigida ainda não foi informada.
const MsgRevisarCargaReduzida = "Expediente reduzido: confira a carga horária do dia definida pelo decreto."

// MsgRevisarSemHorario avisa que um dia justificado veio sem batimento algum.
// Na ficha do SFR esses dias costumam ter horário: quase sempre significa que a
// leitura perdeu os batimentos, e perder ponto real em silêncio é inaceitável.
const MsgRevisarSemHorario = "Nenhum horário foi lido para este dia. Confira a folha original: os batimentos podem ter se perdido na extração."

// ObsKind classifica o significado semântico de uma observação/ocorrência
// da ficha de frequência (coluna OBSERVAÇÕES/OCORRÊNCIAS do SFR).
type ObsKind string

const (
	ObsNenhuma            ObsKind = ""
	ObsFeriado            ObsKind = "feriado"
	ObsPontoFacultativo   ObsKind = "ponto_facultativo"
	ObsRecesso            ObsKind = "recesso"
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
