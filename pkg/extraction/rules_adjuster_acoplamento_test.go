package extraction

import (
	"sync"
	"testing"

	"github.com/fernangcortes/ponto-real-go/pkg/rules"
)

// Este arquivo trava um acoplamento que não está escrito em lugar nenhum do
// código onde ele importa.
//
// Nos dois casos sem piso de tarde — ".X.." (só a saída para o almoço foi
// batida) e ".XX." (a saída e o retorno) —, a tarde é apenas o que falta para
// fechar a jornada depois da manhã, e a manhã vai de uma entrada ARBITRADA
// (08:00–09:00) até a saída para o almoço. Quanto mais tarde essa saída, maior a
// manhã e menor a tarde. Nenhum piso a segura: passado certo ponto a tarde fica
// negativa e o expediente termina ANTES do retorno do almoço.
//
// Não acontece — e o motivo não está em adjustDay. Está em janelaSlot, que
// limita até que horas reinterpretarPontos aceita chamar um ponto de "saída para
// o almoço". É uma trava que mora em outra função, e nada no lugar avisa disso:
// quem alargar aquelas janelas amanhã quebra estes dois casos sem entender por
// quê.
//
// Um detalhe medido aqui e que o resto da documentação errava: o limite NÃO é
// simplesmente o teto de janelaSlot[1] (13:30).
//
//   - Com um ponto só (".X.."), é: a saída para o almoço nunca passa das 13:30.
//   - Com dois pontos (".XX."), reinterpretarPontos escolhe pela penalidade
//     SOMADA dos dois, e um par como 14:44/14:45 destoa menos como
//     saída-almoço/retorno (74 min de penalidade) do que como retorno/saída
//     (75 min). A saída para o almoço chega, então, às 14:44 — 74 minutos além
//     do teto da janela.
//
// O limite real é o empate entre o teto de janelaSlot[1] e o piso de
// janelaSlot[3]; alargar QUALQUER um dos dois empurra a saída para o almoço para
// mais tarde e come a folga. Por isso os testes abaixo são dirigidos pela própria
// janelaSlot, e não por horários copiados para cá.

var (
	umaVezDiasSemPiso sync.Once
	cacheDiasSemPiso  [][4]int
)

// diasSemPisoDeTarde devolve todo dia que reinterpretarPontos pode entregar a
// adjustDay nas máscaras ".X.." e ".XX.".
//
// A varredura é sobre a ENTRADA de reinterpretarPontos, não sobre a saída: é
// assim que janelaSlot entra na conta sozinha, sem ser copiada para cá. Mexer
// nas janelas muda esta lista.
func diasSemPisoDeTarde() [][4]int {
	umaVezDiasSemPiso.Do(func() {
		vistos := map[[4]int]bool{}

		guarda := func(bruto [4]int) {
			e1, s1, e2, s2 := reinterpretarPontos(bruto[0], bruto[1], bruto[2], bruto[3])
			dia := [4]int{e1, s1, e2, s2}
			if m := mascaraDeSlots(dia); m != ".X.." && m != ".XX." {
				return
			}
			if !vistos[dia] {
				vistos[dia] = true
				cacheDiasSemPiso = append(cacheDiasSemPiso, dia)
			}
		}

		// Um ponto só, em cada uma das 4 colunas em que a extração pode tê-lo posto.
		for t := 1; t <= fimDoDia; t++ {
			for col := 0; col < 4; col++ {
				var bruto [4]int
				bruto[col] = t
				guarda(bruto)
			}
		}

		// Dois pontos, em cada um dos 6 pares de colunas.
		for a := 1; a <= fimDoDia; a++ {
			for b := a + 1; b <= fimDoDia; b++ {
				for c1 := 0; c1 < 4; c1++ {
					for c2 := c1 + 1; c2 < 4; c2++ {
						var bruto [4]int
						bruto[c1], bruto[c2] = a, b
						guarda(bruto)
					}
				}
			}
		}
	})
	return cacheDiasSemPiso
}

// TestTardeNuncaFicaNegativaSemPisoDeTarde é o teste que quebra quando alguém
// alarga a janela da saída para o almoço — ou abaixa a da saída da tarde.
func TestTardeNuncaFicaNegativaSemPisoDeTarde(t *testing.T) {
	dias := diasSemPisoDeTarde()
	if len(dias) == 0 {
		t.Fatal("a varredura não achou nenhum dia sem piso de tarde; o teste deixou de testar")
	}

	menorTarde, diaDaMenor, regimeDaMenor := 1<<30, [4]int{}, ""

	for _, s := range sorteiosDeterministicos() {
		for _, dia := range dias {
			r := &RulesAdjuster{Engine: rules.NewEngineWithDefaults(), rng: s.fonte}
			e1, s1, e2, s2, _ := r.adjustDay(dia[0], dia[1], dia[2], dia[3])

			if tarde := s2 - e2; tarde < menorTarde {
				menorTarde, diaDaMenor, regimeDaMenor = tarde, dia, s.nome
			}

			if s2-e2 > 0 {
				continue
			}

			t.Fatalf(
				"a tarde ficou negativa: a saída (%s) veio ANTES do retorno do almoço (%s).\n"+
					"\n"+
					"  dia recebido: %s   (sorteio %q)\n"+
					"  dia gerado:   %s\n"+
					"\n"+
					"POR QUE ISTO QUEBROU\n"+
					"\n"+
					"Nos casos \".X..\" e \".XX.\" a tarde não tem piso: ela é só o que falta para\n"+
					"fechar a jornada depois da manhã, e a manhã começa numa entrada ARBITRADA\n"+
					"entre 08:00 e 09:00. Uma saída para o almoço tarde demais consome a jornada\n"+
					"inteira, e não sobra tarde.\n"+
					"\n"+
					"O que impedia isso não está em adjustDay: está em janelaSlot, que decide até\n"+
					"que horas reinterpretarPontos aceita chamar um ponto de saída para o almoço.\n"+
					"Hoje o teto da saída para o almoço está em %s e o piso da saída da tarde em\n"+
					"%s — e é o empate entre os dois que fixa o limite. Alargar qualquer um dos\n"+
					"dois deixa a saída para o almoço chegar mais tarde e come a folga.\n"+
					"\n"+
					"Para consertar, escolha uma:\n"+
					"  - devolver as janelas ao que eram; ou\n"+
					"  - dar um piso de tarde a estes dois casos em adjustDay, como as outras\n"+
					"    cinco combinações já têm (e então este arquivo pode ser apagado).",
				formatMins(s2), formatMins(e2),
				quatroHorarios(dia), s.nome, quatroHorarios([4]int{e1, s1, e2, s2}),
				formatMins(janelaSlot[1].max), formatMins(janelaSlot[3].min),
			)
		}
	}

	t.Logf("%d dias sem piso de tarde; a menor tarde é de %d min, no dia %s (sorteio %q)",
		len(dias), menorTarde, quatroHorarios(diaDaMenor), regimeDaMenor)
}

// TestAFolgaDaTardeSemPisoEstreitaEMedida põe número na margem, para que ela
// deixe de ser invisível.
//
// A folga é a distância entre a saída para o almoço mais tardia que o gerador
// pode receber e a primeira que zeraria a tarde. Hoje são 70 minutos — metade do
// que se supunha, porque com dois pontos a saída para o almoço chega às 14:44, e
// não às 13:30.
//
// O teste não fixa 70: fixa que a folga existe, e a imprime. Quem alargar as
// janelas vê o número encolher no log antes de ele virar zero no outro teste.
func TestAFolgaDaTardeSemPisoEstreitaEMedida(t *testing.T) {
	maiorS1Alcancavel := 0
	for _, dia := range diasSemPisoDeTarde() {
		if dia[1] > maiorS1Alcancavel {
			maiorS1Alcancavel = dia[1]
		}
	}

	primeiroQueZera := 0
	for s1 := janelaSlot[1].min; s1 <= fimDoDia && primeiroQueZera == 0; s1++ {
		for _, s := range sorteiosDeterministicos() {
			r := &RulesAdjuster{Engine: rules.NewEngineWithDefaults(), rng: s.fonte}
			_, _, e2, s2, _ := r.adjustDay(0, s1, 0, 0)
			if s2-e2 <= 0 {
				primeiroQueZera = s1
				break
			}
		}
	}

	if primeiroQueZera == 0 {
		t.Fatal("a tarde não zera nem com a saída para o almoço às 23:59.\n" +
			"Se um piso de tarde foi dado aos casos \".X..\" e \".XX.\", o acoplamento com\n" +
			"janelaSlot acabou e este arquivo pode ser apagado: a garantia passou a estar\n" +
			"no piso, que é onde ela deveria morar.")
	}

	folga := primeiroQueZera - maiorS1Alcancavel
	if folga <= 0 {
		t.Fatalf("acabou a folga: a saída para o almoço já alcança %s, e a tarde zera a\n"+
			"partir de %s. Os casos sem piso de tarde podem gerar saída antes do retorno.",
			formatMins(maiorS1Alcancavel), formatMins(primeiroQueZera))
	}

	t.Logf("a saída para o almoço chega no máximo às %s (o teto de janelaSlot é %s);\n"+
		"a tarde zeraria a partir das %s. A folga inteira são %d minutos, e ela mora\n"+
		"em janelaSlot — não em adjustDay.",
		formatMins(maiorS1Alcancavel), formatMins(janelaSlot[1].max),
		formatMins(primeiroQueZera), folga)
}
