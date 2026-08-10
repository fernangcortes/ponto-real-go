package extraction

import (
	"testing"

	"github.com/fernangcortes/ponto-real-go/pkg/models"
	"github.com/fernangcortes/ponto-real-go/pkg/rules"
)

// junho2026 monta uma folha de 30 dias com as observações informadas.
func junho2026(obs map[int]string) *models.Timesheet {
	ts := &models.Timesheet{MesAno: "06/2026"}
	for dia := 1; dia <= 30; dia++ {
		ts.Dias = append(ts.Dias, models.DayRecord{Dia: dia, Motivo: obs[dia]})
	}
	return ts
}

func TestAlignCorrigeDiaDaSemanaPeloCalendario(t *testing.T) {
	ts := junho2026(nil)
	AlignObservacoes(ts)

	// Junho/2026 começa numa segunda-feira.
	casos := map[int]string{1: "Seg", 6: "Sáb", 7: "Dom", 28: "Dom", 30: "Ter"}
	for dia, esperado := range casos {
		if got := ts.Dias[dia-1].DiaSemana; got != esperado {
			t.Errorf("dia %d: dia da semana = %q, esperado %q", dia, got, esperado)
		}
	}
}

func TestAlignNaoMexeQuandoJaAlinhado(t *testing.T) {
	// Fins de semana reais de junho/2026.
	ts := junho2026(map[int]string{
		6: "SÁBADO", 7: "DOMINGO", 13: "SÁBADO", 14: "DOMINGO",
		20: "SÁBADO", 21: "DOMINGO", 27: "SÁBADO", 28: "DOMINGO",
		2: "FERIADO", 22: "EXPEDIENTE REDUZIDO - COPA 2026 - DEC. 10.925/2026",
	})
	res := AlignObservacoes(ts)

	if res.Desalinhado || res.Corrigido {
		t.Fatalf("folha alinhada não deveria ser alterada: %+v", res)
	}
	if ts.Dias[21].Motivo != "EXPEDIENTE REDUZIDO - COPA 2026 - DEC. 10.925/2026" {
		t.Errorf("observação do dia 22 foi movida indevidamente: %q", ts.Dias[21].Motivo)
	}
}

func TestAlignCorrigeDeslocamentoGlobal(t *testing.T) {
	// Toda a coluna lida 2 dias acima do lugar certo.
	ts := junho2026(map[int]string{
		4: "SÁBADO", 5: "DOMINGO", 11: "SÁBADO", 12: "DOMINGO",
		18: "SÁBADO", 19: "DOMINGO", 25: "SÁBADO", 26: "DOMINGO",
		20: "EXPEDIENTE REDUZIDO - COPA 2026",
	})
	res := AlignObservacoes(ts)

	if !res.Corrigido || res.DeslocamentoAplicado != 2 {
		t.Fatalf("esperado deslocamento +2 aplicado, veio %+v", res)
	}
	if ts.Dias[5].Motivo != "SÁBADO" {
		t.Errorf("dia 6 deveria ter SÁBADO, veio %q", ts.Dias[5].Motivo)
	}
	// A observação lida no dia 20 pertence ao dia 22.
	if ts.Dias[21].Motivo != "EXPEDIENTE REDUZIDO - COPA 2026" {
		t.Errorf("dia 22 deveria ter o expediente reduzido, veio %q", ts.Dias[21].Motivo)
	}
}

func TestAlignSinalizaDesalinhamentoIrregular(t *testing.T) {
	// Deslocamentos irregulares (+2,+2,+1,+1,...) como no PDF real do SFR:
	// nenhum shift global resolve, então nada deve ser movido.
	ts := junho2026(map[int]string{
		4: "SÁBADO", 5: "DOMINGO", 12: "SÁBADO", 14: "DOMINGO",
		18: "SÁBADO", 19: "DOMINGO", 25: "SÁBADO", 27: "DOMINGO",
		22: "EXPEDIENTE REDUZIDO - COPA 2026",
	})
	res := AlignObservacoes(ts)

	if !res.Desalinhado {
		t.Fatal("desalinhamento irregular deveria ser detectado")
	}
	if res.Corrigido {
		t.Fatal("não deveria ter corrigido um padrão irregular")
	}
	// Dados preservados, mas sinalizados.
	if ts.Dias[21].Motivo != "EXPEDIENTE REDUZIDO - COPA 2026" {
		t.Errorf("observação não deveria ter sido movida: %q", ts.Dias[21].Motivo)
	}
	// Só os marcadores em conflito são marcados: dia 4 diz SÁBADO mas é quinta.
	if !ts.Dias[3].Revisar {
		t.Error("marcador SÁBADO numa quinta-feira deveria ser sinalizado")
	}
	// Dia 14 diz DOMINGO e realmente é domingo: não deve ser marcado.
	if ts.Dias[13].Revisar {
		t.Error("marcador correto não deveria ser sinalizado")
	}
	// Dias sem marcador de fim de semana não são poluídos com aviso.
	if ts.Dias[21].Revisar {
		t.Error("dia sem marcador de calendário não deveria ser sinalizado")
	}
}

func TestExpedienteReduzidoNaoGeraHorario(t *testing.T) {
	engine := rules.NewEngineWithDefaults()
	adjuster := NewRulesAdjuster(engine)

	// Dia 22/06 real: faltou apenas a saída da tarde.
	ts := &models.Timesheet{
		MesAno: "06/2026",
		Dias: []models.DayRecord{{
			Dia: 22, DiaSemana: "Seg",
			Entrada1: "09:42", Saida1: "11:40", Entrada2: "12:40", Saida2: "",
			Motivo: "EXPEDIENTE REDUZIDO - COPA 2026 - DEC. 10.925/2026",
		}},
	}

	out := adjuster.Adjust(ts)
	d := out.Dias[0]

	if d.Saida2 != "" {
		t.Errorf("não deveria inventar saída em expediente reduzido, gerou %q", d.Saida2)
	}
	if !d.Revisar {
		t.Error("dia de expediente reduzido deveria pedir conferência da carga")
	}
}

func TestExpedienteReduzidoNaoGeraDeficit(t *testing.T) {
	engine := rules.NewEngineWithDefaults()
	dias := []models.DayRecord{{
		Dia: 22, DiaSemana: "Seg",
		Entrada1: "09:42", Saida1: "11:40", Entrada2: "12:40", Saida2: "",
		Motivo: "EXPEDIENTE REDUZIDO - COPA 2026 - DEC. 10.925/2026",
	}}

	summary := engine.CalculateSummary(dias)

	if summary.SaldoTotalRealMinutos != 0 {
		t.Errorf("expediente reduzido sem carga definida não pode gerar déficit, veio %d min",
			summary.SaldoTotalRealMinutos)
	}
	if summary.TotalFaltas != 0 {
		t.Errorf("expediente reduzido não é falta, veio %d", summary.TotalFaltas)
	}
}

func TestExpedienteReduzidoComCargaDefinida(t *testing.T) {
	engine := rules.NewEngineWithDefaults()
	// Decreto define 4h nesse dia; servidor cumpriu 08:00-12:00.
	dias := []models.DayRecord{{
		Dia: 22, DiaSemana: "Seg",
		Entrada1: "08:00", Saida1: "10:00", Entrada2: "11:00", Saida2: "13:00",
		Motivo: "EXPEDIENTE REDUZIDO - COPA 2026", CargaEsperada: 240,
	}}

	summary := engine.CalculateSummary(dias)

	if summary.SaldoTotalRealMinutos != 0 {
		t.Errorf("4h trabalhadas contra carga de 4h deveria dar saldo 0, veio %d",
			summary.SaldoTotalRealMinutos)
	}
}

func TestFaltouEntradaDaManhaNaoInventaRetornoDeAlmoco(t *testing.T) {
	engine := rules.NewEngineWithDefaults()
	adjuster := NewRulesAdjuster(engine)

	// Dia 01/06 lido literalmente das colunas do PDF: 11:30, 12:33, 20:06.
	// 11:30 é tarde demais para ser entrada da manhã — o que faltou foi ela.
	ts := &models.Timesheet{
		MesAno: "06/2026",
		Dias: []models.DayRecord{{
			Dia: 1, DiaSemana: "Seg",
			Entrada1: "11:30", Saida1: "12:33", Entrada2: "20:06", Saida2: "",
		}},
	}

	d := adjuster.Adjust(ts).Dias[0]

	if d.Saida1 != "11:30" || d.Entrada2 != "12:33" || d.Saida2 != "20:06" {
		t.Errorf("horários reais deveriam virar saída-almoço/retorno/saída, veio s1=%q e2=%q s2=%q",
			d.Saida1, d.Entrada2, d.Saida2)
	}
	// A entrada da manhã é a gerada, e tem de ser de manhã.
	if d.Entrada1 == "" || d.Entrada1 >= "11:30" {
		t.Errorf("entrada da manhã gerada implausível: %q", d.Entrada1)
	}
	if d.Bloqueio[0] != 0 {
		t.Error("entrada da manhã deveria estar marcada como gerada")
	}
}

func TestPontosPlausiveisNaoSaoRemanejados(t *testing.T) {
	engine := rules.NewEngineWithDefaults()
	adjuster := NewRulesAdjuster(engine)

	// Dia 22/06: 09:42, 11:40, 12:40 encaixam nos 3 primeiros slots.
	// Só a saída falta — nada pode ser deslocado.
	ts := &models.Timesheet{
		MesAno: "06/2026",
		Dias: []models.DayRecord{{
			Dia: 22, DiaSemana: "Seg",
			Entrada1: "09:42", Saida1: "11:40", Entrada2: "12:40", Saida2: "",
		}},
	}

	d := adjuster.Adjust(ts).Dias[0]

	if d.Entrada1 != "09:42" || d.Saida1 != "11:40" || d.Entrada2 != "12:40" {
		t.Errorf("pontos plausíveis foram remanejados: e1=%q s1=%q e2=%q",
			d.Entrada1, d.Saida1, d.Entrada2)
	}
	if d.Saida2 == "" {
		t.Error("a saída deveria ter sido gerada")
	}
}

func TestNenhumHorarioRealEhDescartado(t *testing.T) {
	engine := rules.NewEngineWithDefaults()
	adjuster := NewRulesAdjuster(engine)

	// Nenhum horário original pode sumir, seja qual for o tipo do dia.
	casos := []models.DayRecord{
		{Dia: 12, DiaSemana: "Sex", Entrada1: "09:09", Saida1: "13:02",
			Motivo: "DISPENSA PARA FREQUÊNCIA A CURSO"},
		{Dia: 29, DiaSemana: "Seg", Entrada1: "09:31", Saida1: "12:26",
			Motivo: "EXPEDIENTE REDUZIDO - COPA 2026"},
		{Dia: 1, DiaSemana: "Seg", Entrada1: "11:30", Saida1: "12:33", Entrada2: "20:06"},
	}

	for _, caso := range casos {
		originais := []string{caso.Entrada1, caso.Saida1, caso.Entrada2, caso.Saida2}
		out := adjuster.Adjust(&models.Timesheet{
			MesAno: "06/2026", Dias: []models.DayRecord{caso},
		}).Dias[0]
		final := []string{out.Entrada1, out.Saida1, out.Entrada2, out.Saida2}

		for _, orig := range originais {
			if orig == "" {
				continue
			}
			achou := false
			for _, f := range final {
				if f == orig {
					achou = true
					break
				}
			}
			if !achou {
				t.Errorf("dia %d: horário real %q desapareceu (resultado: %v)", caso.Dia, orig, final)
			}
		}
	}
}

func TestDiaJustificadoSemHorarioEhSinalizado(t *testing.T) {
	engine := rules.NewEngineWithDefaults()

	// Caso real do dia 12: dispensa, mas a extração não trouxe batimento algum.
	dias := []models.DayRecord{{
		Dia: 12, DiaSemana: "Sex",
		Motivo: "DISPENSA PARA FREQUÊNCIA A CURSO DE DOUTORADO, MESTRADO",
	}}

	engine.CalculateSummary(dias)

	if !dias[0].Revisar {
		t.Fatal("dia justificado sem nenhum horário deveria ser sinalizado")
	}
	if dias[0].RevisarMotivo != rules.MsgRevisarSemHorario {
		t.Errorf("aviso errado: %q", dias[0].RevisarMotivo)
	}
}

// TestHorariosGeradosSempreValidos varre uma grade ampla de combinações de
// batimentos e exige que o resultado seja sempre um dia coerente: dentro das
// 24h e em ordem cronológica. É o que impede regressões como o "27:12".
func TestHorariosGeradosSempreValidos(t *testing.T) {
	engine := rules.NewEngineWithDefaults()
	adjuster := NewRulesAdjuster(engine)

	horarios := []string{"", "06:15", "08:30", "10:29", "11:30", "12:33",
		"13:02", "15:59", "16:00", "18:40", "20:06", "23:40"}

	for _, a := range horarios {
		for _, b := range horarios {
			for _, c := range horarios {
				for _, e := range horarios {
					entrada := []string{a, b, c, e}
					out := adjuster.Adjust(&models.Timesheet{
						MesAno: "06/2026",
						Dias: []models.DayRecord{{
							Dia: 10, DiaSemana: "Qua",
							Entrada1: a, Saida1: b, Entrada2: c, Saida2: e,
						}},
					}).Dias[0]

					pontos := []string{out.Entrada1, out.Saida1, out.Entrada2, out.Saida2}

					preenchidos := 0
					for _, v := range entrada {
						if v != "" {
							preenchidos++
						}
					}

					// Dia já completo é intocável: o adjuster não gera nada e não
					// tem o direito de reordenar batimentos reais do servidor.
					if preenchidos == 4 {
						for i := range entrada {
							if pontos[i] != entrada[i] {
								t.Fatalf("dia completo foi alterado: %v -> %v", entrada, pontos)
							}
						}
						continue
					}

					anterior := ""
					for _, p := range pontos {
						if p == "" {
							continue
						}
						if p > "23:59" {
							t.Fatalf("entrada %v gerou horário fora do dia: %v", entrada, pontos)
						}
						if anterior != "" && p < anterior {
							t.Fatalf("entrada %v gerou horários fora de ordem: %v", entrada, pontos)
						}
						anterior = p
					}

					// Nenhum batimento real pode desaparecer.
					for _, orig := range entrada {
						if orig == "" {
							continue
						}
						achou := false
						for _, p := range pontos {
							if p == orig {
								achou = true
								break
							}
						}
						if !achou {
							t.Fatalf("entrada %v perdeu o horário real %q: %v", entrada, orig, pontos)
						}
					}
				}
			}
		}
	}
}

// TestEntradaGeradaNuncaEhDeMadrugada trava o piso das 07:00 para toda entrada
// da manhã INVENTADA — batimento real do servidor é intocável, seja qual for a
// hora.
//
// O piso existia em todos os geradores de entrada menos num: o do dia em que só
// o retorno do almoço foi batido. Ali um único ponto às 11:33 produzia entrada
// às 06:01, horário que seguiria para um documento assinado sem que nada
// reclamasse: a ordem cronológica está correta e nenhum ponto real sumiu, que
// era tudo o que a suíte conferia.
func TestEntradaGeradaNuncaEhDeMadrugada(t *testing.T) {
	adjuster := NewRulesAdjuster(rules.NewEngineWithDefaults())

	const pisoManha = 7 * 60

	verifica := func(t *testing.T, campos [4]string) {
		t.Helper()
		out := adjuster.Adjust(&models.Timesheet{
			MesAno: "06/2026",
			Dias: []models.DayRecord{{
				Dia: 10, DiaSemana: "Qua",
				Entrada1: campos[0], Saida1: campos[1],
				Entrada2: campos[2], Saida2: campos[3],
			}},
		}).Dias[0]

		if len(out.Bloqueio) != 4 || out.Bloqueio[0] != 0 {
			return // entrada real, ou dia que o adjuster não toca
		}
		if e1 := parseMins(out.Entrada1); e1 > 0 && e1 < pisoManha {
			t.Fatalf("batimentos %v geraram início de expediente de madrugada: %s %s %s %s",
				campos, out.Entrada1, out.Saida1, out.Entrada2, out.Saida2)
		}
	}

	// O caso que revelou o defeito.
	t.Run("um ponto às 11:33 no retorno do almoço", func(t *testing.T) {
		for i := 0; i < 200; i++ {
			verifica(t, [4]string{"", "", "11:33", ""})
		}
	})

	// Um único batimento, em qualquer coluna, em qualquer minuto do dia.
	t.Run("um ponto em qualquer minuto", func(t *testing.T) {
		for col := 0; col < 4; col++ {
			for m := 1; m <= fimDoDia; m++ {
				var campos [4]string
				campos[col] = formatMins(m)
				for i := 0; i < 4; i++ {
					verifica(t, campos)
				}
			}
		}
	})

	// As demais combinações, na mesma grade do teste exaustivo.
	t.Run("dois e três batimentos", func(t *testing.T) {
		horarios := []string{"", "06:15", "08:30", "10:29", "11:30", "12:33",
			"13:02", "15:59", "16:00", "18:40", "20:06", "23:40"}
		for _, a := range horarios {
			for _, b := range horarios {
				for _, c := range horarios {
					for _, e := range horarios {
						verifica(t, [4]string{a, b, c, e})
					}
				}
			}
		}
	})
}

// TestHorarioRedondoEhPreferenciaNaoLei trava o comportamento de
// avoidRoundMinsEntre: fugir do minuto redondo é o que se quer na maioria das
// vezes, mas cede quando o deslocamento não cabe.
func TestHorarioRedondoEhPreferenciaNaoLei(t *testing.T) {
	const meioDia = 12 * 60 // minuto redondo

	t.Run("sem limite, foge do redondo", func(t *testing.T) {
		for i := 0; i < 500; i++ {
			if got := avoidRoundMins(meioDia); minutoRedondo(got) {
				t.Fatalf("horário sem restrição continuou redondo: %s", formatMins(got))
			}
		}
	})

	t.Run("respeita o limite recebido", func(t *testing.T) {
		// Só pode andar para trás, no máximo 3 minutos.
		for i := 0; i < 500; i++ {
			got := avoidRoundMinsEntre(meioDia, meioDia-3, meioDia)
			if got < meioDia-3 || got > meioDia {
				t.Fatalf("saiu da faixa: %s", formatMins(got))
			}
			if minutoRedondo(got) {
				t.Fatalf("havia espaço para fugir do redondo e não fugiu: %s", formatMins(got))
			}
		}
	})

	t.Run("cede quando nada cabe", func(t *testing.T) {
		// Faixa que não admite nenhum deslocamento: o redondo tem de ficar.
		for i := 0; i < 100; i++ {
			if got := avoidRoundMinsEntre(meioDia, meioDia, meioDia); got != meioDia {
				t.Fatalf("sem espaço para andar, devia ter ficado em %s; veio %s",
					formatMins(meioDia), formatMins(got))
			}
		}
	})

	t.Run("não mexe em horário que já não é redondo", func(t *testing.T) {
		if got := avoidRoundMinsEntre(meioDia+7, semPiso, semTeto); got != meioDia+7 {
			t.Fatalf("horário não redondo foi alterado: %s", formatMins(got))
		}
	})
}

// TestAlmocoGeradoRespeitaOMinimoLegal trava a promessa que almocoGerado faz no
// próprio comentário: um almoço gerado nunca fica abaixo do mínimo legal, senão
// produz um dia que a própria ValidateDay recusa.
//
// O sorteio do almoço sempre respeitou o mínimo; quem o encurtava era o passo
// seguinte, que afasta o horário dos minutos redondos e podia comer o intervalo
// por cima — 14:30 de retorno real saía com almoço de 58 minutos.
func TestAlmocoGeradoRespeitaOMinimoLegal(t *testing.T) {
	engine := rules.NewEngineWithDefaults()
	adjuster := NewRulesAdjuster(engine)
	minAlmoco := engine.Config.AlmocoMinimo

	horarios := []string{"", "06:15", "08:30", "10:29", "11:30", "12:33",
		"13:02", "14:30", "15:59", "16:00", "18:40", "20:06", "23:40"}

	for _, a := range horarios {
		for _, b := range horarios {
			for _, c := range horarios {
				for _, e := range horarios {
					for rep := 0; rep < 3; rep++ {
						out := adjuster.Adjust(&models.Timesheet{
							MesAno: "06/2026",
							Dias: []models.DayRecord{{
								Dia: 10, DiaSemana: "Qua",
								Entrada1: a, Saida1: b, Entrada2: c, Saida2: e,
							}},
						}).Dias[0]
						if len(out.Bloqueio) != 4 {
							continue
						}

						e1 := parseMins(out.Entrada1)
						s1 := parseMins(out.Saida1)
						e2 := parseMins(out.Entrada2)
						if s1 <= 0 || e2 <= 0 {
							continue
						}

						// Almoço formado só por batimentos reais é do servidor,
						// não da geração: se ele almoçou em 40 minutos, azar.
						if out.Bloqueio[1] == 1 && out.Bloqueio[2] == 1 {
							continue
						}

						// Entrada e retorno reais colados um no outro não deixam
						// almoço legal nenhum caber — nesse caso a saída para o
						// almoço vai para o meio dos dois, como manda o código.
						// Exigir o mínimo aqui seria exigir o impossível.
						if out.Bloqueio[0] == 1 && out.Bloqueio[2] == 1 && e2-e1 < 2*minAlmoco {
							continue
						}

						if e2-s1 < minAlmoco {
							t.Fatalf("batimentos [%s %s %s %s] geraram almoço de %d min (mínimo é %d): %s %s %s %s",
								a, b, c, e, e2-s1, minAlmoco,
								out.Entrada1, out.Saida1, out.Entrada2, out.Saida2)
						}
					}
				}
			}
		}
	}
}

func TestAvisoAntigoNaoPersisteQuandoResolvido(t *testing.T) {
	// Folha alinhada, mas carregando um aviso gravado por uma versão anterior.
	ts := junho2026(map[int]string{
		6: "SÁBADO", 7: "DOMINGO", 13: "SÁBADO", 14: "DOMINGO",
		20: "SÁBADO", 21: "DOMINGO", 27: "SÁBADO", 28: "DOMINGO",
	})
	ts.Dias[2].Revisar = true
	ts.Dias[2].RevisarMotivo = "Observação possivelmente deslocada: os marcadores SÁBADO/DOMINGO não batem com o calendário do mês."

	AlignObservacoes(ts)

	if ts.Dias[2].Revisar {
		t.Error("aviso de alinhamento já resolvido deveria ter sido limpo")
	}
}

func TestAvisoDeOutraEtapaSobrevive(t *testing.T) {
	// O aviso de carga do decreto não pertence ao alinhamento e deve persistir.
	ts := junho2026(map[int]string{
		6: "SÁBADO", 7: "DOMINGO", 13: "SÁBADO", 14: "DOMINGO",
		20: "SÁBADO", 21: "DOMINGO", 27: "SÁBADO", 28: "DOMINGO",
	})
	ts.Dias[21].Revisar = true
	ts.Dias[21].RevisarMotivo = rules.MsgRevisarCargaReduzida

	AlignObservacoes(ts)

	if !ts.Dias[21].Revisar {
		t.Error("aviso de carga do decreto não deveria ser limpo pelo alinhamento")
	}
}

func TestObservacaoFundidaNaoViraAncora(t *testing.T) {
	// Caso real do dia 12: duas observações fundidas numa célula. O "SÁBADO"
	// no fim do texto não pode ser lido como âncora numa sexta-feira.
	ts := junho2026(map[int]string{
		6: "SÁBADO", 7: "DOMINGO", 13: "SÁBADO", 14: "DOMINGO",
		20: "SÁBADO", 21: "DOMINGO", 27: "SÁBADO", 28: "DOMINGO",
	})
	ts.Dias[11].Ocorrencia = "DISPENSA PARA FREQUÊNCIA A CURSO DE DOUTORADO, MESTRADO, SÁBADO"

	res := AlignObservacoes(ts)

	if res.Desalinhado {
		t.Error("observação fundida não deveria acusar desalinhamento")
	}
	if ts.Dias[11].Revisar {
		t.Error("dia 12 não deveria ser marcado para revisão por falso positivo")
	}
}

func TestCompensacaoEhApenasInformativa(t *testing.T) {
	engine := rules.NewEngineWithDefaults()
	adjuster := NewRulesAdjuster(engine)

	// COMPENSAÇÃO não deve bloquear o preenchimento normal.
	ts := &models.Timesheet{
		MesAno: "06/2026",
		Dias: []models.DayRecord{{
			Dia: 17, DiaSemana: "Qua",
			Entrada1: "09:35", Saida1: "12:04", Entrada2: "13:06", Saida2: "",
			Motivo: "COMPENSAÇÃO DEDUZIDA",
		}},
	}

	out := adjuster.Adjust(ts)
	if out.Dias[0].Saida2 == "" {
		t.Error("dia com COMPENSAÇÃO deveria seguir o preenchimento normal")
	}

	if got := engine.ClassifyDay(&out.Dias[0]); got != models.DayTypeCompleto {
		t.Errorf("COMPENSAÇÃO não deveria mudar a classificação, veio %q", got)
	}
}
