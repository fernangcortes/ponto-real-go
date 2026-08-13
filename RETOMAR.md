# Onde paramos

Ponto de partida para uma nova sessão. As regras de operação estão no
[MANUAL.md](MANUAL.md); o detalhe do que foi feito, em
[REFATORACAO.md](REFATORACAO.md). Aqui fica só o necessário para retomar.

## Estado

**A reescrita do `adjustDay` está na `master`** desde 2026-08-13, pelo PR #2
(`cf51c8d`), com o CI verde nos três jobs — o `-race` incluído. Detalhe no item 1
abaixo. A branch `reescrita-adjuster` cumpriu seu papel.

**A refatoração anterior foi mergeada na `master`** em 2026-08-10, pelo PR #1
(`1d9f31b`). A branch `refatoracao` cumpriu seu papel e não é mais o lugar de
trabalhar.

**O CI está verde nos três jobs** — e essa é a novidade que mais importa,
porque significa que o `-race` finalmente rodou. Ele exige compilador C, não
roda na máquina de desenvolvimento e por isso era o único portão sem resposta
nos 13 commits da refatoração. Rodou, e não achou corrida.

O Lint quebrou na primeira execução, por versão de ferramenta e não por achado
no código: a action estava em `@v6`, que resolve `latest` dentro da linha v1 do
golangci-lint, e a v1 não roda contra um `go.mod` que pede Go 1.25 nem entende o
`.golangci.yml` no formato v2. Corrigido em `0476b17`, fixando a action em `@v9`
e a versão em `v2.12.2` — a mesma da máquina de desenvolvimento, de propósito:
lint que diverge entre local e CI só descobre problema depois do push.

Os 13 commits que entraram no merge:

```
42186f5 docs: registrar o mapa do RulesAdjuster e o que a suíte não segura
943afbb fix: não deixar o desvio do minuto redondo encurtar o almoço
88b3f19 fix: impedir que a entrada da manhã seja inventada de madrugada
2edb5a4 docs: registrar o hash do commit anterior no RETOMAR
ca0fd34 feat: dar voz ao dia de dispensa cumprida no documento SEI
40975df docs: adicionar RETOMAR.md com o estado e as armadilhas do projeto
995d552 fix: permitir apagar um horário e manter o aviso de jornada em dia
fc485ae docs: registrar o plano de refatoração e o que foi executado
25b049f build: adicionar CI, lint e portões de qualidade
7c54e2c test: travar as regras compartilhadas entre backend e front-end
b22bdcd refactor(frontend): modularizar, corrigir escape e tirar efeito colateral do render
db4f6db refactor(backend): reestruturar camadas, corrigir defeitos e cobrir com testes
e4e612e chore: normalizar fim de linha e fixar a regra no .gitattributes
```

Depois vieram, direto na `master`: `0476b17` (conserto do CI de lint) e o
registro da v1.1.1 com o [MANUAL.md](MANUAL.md).

Verificação local (tudo verde):

```bash
go build ./... && go vet ./... && go test ./...   # 227 testes
golangci-lint run ./...                            # 0 issues
npm run lint && npm test                           # 76 testes
```

O CI acrescenta a estes o `go test -race ./...`, que é o portão que só ele dá.

## O que o usuário precisa fazer antes de qualquer coisa nova

1. **Conferir os dois meses no app.** Muita regra de cálculo mudou — férias,
   dispensa, saldo oficial, e agora as duas seções do documento SEI. Ele é quem
   sabe se os números batem com a realidade. Já está mergeado, então a
   conferência é para achar erro, não para decidir se entra.

## O que falta, em ordem de valor

### 1. ~~Reescrever o `RulesAdjuster`~~ — feito em 2026-08-11

`pkg/extraction/rules_adjuster.go`, função `adjustDay`. É o código que **inventa
horários que vão para um documento assinado por um servidor público** — o último
ponto do sistema onde algo pode sair arbitrário. Eram 12 combinações de
batimentos nomeadas mais um fallback que registrava *"combinação não prevista"*,
cada uma com a sua própria cópia da aritmética.

Hoje são 14 casos nomeados de duas a quatro linhas cada, e **cada coluna
inventada nasce num gerador só**. O que muda de um caso para outro é de qual
horário conhecido o gerador se ancora e em que ordem os quatro se resolvem.
`adjustDay` caiu de 232 para 99 linhas e o `slog.Warn` sumiu: as duas
combinações que caíam no fallback agora têm nome e comentário próprios.

**O que segurou a reescrita** foi uma comparação com a implementação anterior —
não os testes de invariante, que não olham número. Com um sorteio que não guarda
estado (sempre o mínimo da faixa, sempre o máximo, sempre o meio, e mais quatro
posições), `adjustDay` vira função pura dos batimentos, e as duas implementações
puderam ser postas lado a lado sobre **as 14 combinações × 24 horários de borda
= 29.544 dias, em 7 regimes de sorteio: 206.808 comparações, zero diferenças.**
As ferramentas dessa comparação ficaram em `rules_adjuster_grade_test.go`; a
cópia do código antigo foi apagada depois de servir.

E o snapshot das 14 linhas **não mudou nem um minuto**, o que diz mais do que a
comparação: a ordem em que os dados são jogados também foi preservada, então a
equivalência vale sob o sorteio de verdade e não só sob os determinísticos.

#### As divergências que sobraram, à vista e sem decisão

A reescrita **não unificou constante nenhuma**, de propósito: unificar muda
horário em dia de servidor, e isso é decisão de quem assina. Elas agora aparecem
como argumento na chamada, em vez de ficarem enterradas em cópias:

1. **Piso da tarde** — 3h em cinco casos, 2h no dia em que só o retorno do
   almoço foi batido, e **nenhum** nos dois dias sem âncora de manhã.
2. **Faixa da manhã sorteada solta** — 220–260 min num caso, 200–240 noutro,
   para a mesma pergunta.
3. **Desvio de −5 a +15 min** — entra antes de conferir o piso em todos os
   casos, menos no dia em que só a entrada foi batida, onde entra depois. Ali a
   tarde pode terminar alguns minutos abaixo do piso.
4. **Informação disponível e ignorada** — no dia com as duas saídas (`.X.X`), a
   entrada é sorteada solta embora a tarde seja conhecida e desse para calculá-la
   pela carga, que é o que os outros casos com tarde conhecida fazem.

Sobre o piso ausente do item 1: se a saída para o almoço fosse muito tarde, a
tarde ficaria negativa e a saída sairia ANTES do retorno. Não acontece — mas só
porque `janelaSlot` limita até que horas `reinterpretarPontos` aceita chamar um
ponto de "saída para o almoço". **É uma trava que mora em outra função**, e nada
no lugar avisava disso.

Desde 2026-08-13 avisa: `rules_adjuster_acoplamento_test.go` varre a ENTRADA de
`reinterpretarPontos` e quebra, explicando o motivo, se alguém alargar as
janelas. Confirmado por mutação nas duas bordas.

**A varredura corrigiu o que este documento dizia.** Não é verdade que a saída
para o almoço pare nas 13:30 nesses dois casos — isso vale só quando há um ponto
só. Com dois pontos, a reinterpretação decide pela penalidade **somada**, e um par
como 14:44/14:45 destoa menos como saída-almoço/retorno (74 min) do que como
retorno/saída (75 min). A saída para o almoço chega, então, às **14:44**, e a
menor tarde gerada é de **70 minutos**, não 144. A folga é metade do que se
supunha, e depende de duas bordas da janela — o teto da saída para o almoço e o
piso da saída da tarde —, não de uma.

#### O mapa que orientou a reescrita (medido em 2026-08-10)

**Os 13 caminhos são 4 decisões.** Um gerador por coluna, reescrito de 3 a 6
vezes, com constantes que divergiram entre as cópias por acidente:

| O que é inventado | Aparece em | Cópias idênticas | Divergência entre elas |
|---|---|---|---|
| saída da tarde | 6 ramos | 3 iguais caractere a caractere | piso de 3h, de 2h, ou nenhum |
| entrada da manhã | 6 ramos | 3 iguais | um estava sem a trava das 07:00 |
| retorno do almoço | 6 ramos | 3 iguais | só um é diferente de verdade |
| saída para o almoço | 8 ramos | 4 iguais | só um é diferente; a cópia do fallback é código morto |

Sobram **5 regras realmente distintas** em 13 caminhos.

**O fallback é alcançável** — roda 114 vezes na própria grade de testes. Caem
nele um único batimento entre 10:31 e 13:30, ou dois batimentos entre 10:31 e
16:00 (uns 48 mil pares de horários). Não é caso exótico: é o dia em que o
servidor bateu na saída e na volta do almoço e esqueceu as duas pontas. O log
diz *"combinação não prevista"*, o que faz parecer defeito o que é rotina.

**Nos dados reais do usuário, só 3 dos 13 ramos já rodaram**, em 6 dias de
junho e julho: "falta a entrada" (01/06), "falta a saída" (02, 17 e 22/06) e
"falta a saída para o almoço" (16/06 e 10/07). O risco está concentrado neles.

**Os testes não protegiam a reescrita** — e é por isso que o primeiro passo não
foi reescrever. Todos os 13 ramos eram executados, mas 11 só pela grade
exaustiva (`TestHorariosGeradosSempreValidos`), que confere três invariantes —
nada passa das 23:59, ordem cronológica, nenhum batimento real some — e **nunca
olhava o número gerado**.

> Prova por mutação, na época: somar 45 minutos a toda saída inventada pelo ramo
> "falta a saída" deixava `go test ./...` inteiramente verde. São 2h15 a mais em
> junho, no documento assinado, sem nenhum portão reclamar.

#### Feito em 2026-08-11: os valores estão travados

O sorteio deixou de depender do `math/rand` global. Agora vive no próprio
ajustador (`RulesAdjuster.rng`), semeado pelo relógio em produção — horário
inventado tem de variar — e por `newRulesAdjusterComSemente` no teste. Em troca,
um ajustador não pode ser compartilhado entre goroutines; o serviço já cria um
por folha, então nada mudou na prática.

Sobre isso, `pkg/extraction/rules_adjuster_snapshot_test.go`: uma tabela com o
dia que entra e o dia inteiro que sai, para **as 14 combinações possíveis de
batimentos** — não 13. A ficha tem 4 colunas, logo 16 combinações; o dia vazio e
o dia completo o `adjustDay` nunca vê. As outras 14 são os caminhos que ele tem,
e `TestSnapshotCobreTodaCombinacaoPossivel` impede que um fique de fora. Os dois
ramos que o `.md` anterior contava como "fallback" são, na verdade, duas
combinações distintas cair no mesmo lugar: só a saída para o almoço, ou a saída
e o retorno.

Onde havia dado real, ele foi usado: 01/06 (falta a entrada), 02/06 (falta a
saída) e 16/06 (falta a saída para o almoço) — os únicos ramos que
comprovadamente já rodaram na vida real.

A mesma mutação de antes agora quebra o teste, com o dia e os dois horários lado
a lado:

```
--- FAIL: TestSnapshotDosHorariosGerados/falta_a_saída_da_tarde_(02/06_real)
    esperado:  [09:11 12:36 13:37 18:19]
    veio:      [09:11 12:36 13:37 19:04]
```

Uma segunda mutação, na entrada arbitrada (das 08:00 para as 09:00), quebra os
dois casos sem âncora de manhã. Cobertura conferida por mutação, não por
`-coverprofile`.

**O snapshot não diz que os horários são os certos; diz que são os de hoje.**
Quando a mudança for proposital, o jeito de atualizar está escrito no cabeçalho
do arquivo: rodar, conferir caso a caso que o dia novo faz sentido (ordem,
almoço, piso das 07:00, carga), e só então trocar o valor.

Dois defeitos dessa mesma família já foram corrigidos e têm teste. A reescrita
os preservou — `naoDeMadrugada` e `avoidRoundMinsEntre` são hoje o único lugar
onde cada um mora:

- **Entrada de madrugada** (`88b3f19`). O ramo do dia em que só o retorno do
  almoço foi batido era o único sem a trava das 07:00: um retorno às 11:30
  puxava a entrada para 05:56. A manhã tem de ser **recontada depois da trava**,
  senão a tarde compensa uma manhã que não aconteceu e o dia fecha com uma hora
  a mais do que o servidor trabalhou.
- **Almoço abaixo do mínimo legal** (`943afbb`). O desvio dos minutos redondos
  era aplicado como lei e comia o intervalo por cima, gerando almoço de 58
  minutos — abaixo do mínimo que `almocoGerado` promete no próprio comentário.
  Agora `avoidRoundMinsEntre` recebe a faixa em que pode andar e aceita ficar
  redondo quando nada cabe. **A regra é preferência, não lei**: o redondo
  aparece em 0,31% dos horários gerados.

**O que resta neste item** não é código, é decisão: as quatro divergências
listadas acima. Cada uma muda horário de servidor, e cada uma cabe numa linha da
chamada em `adjustDay`.

**As quatro já estão medidas** (2026-08-13), em
`pkg/extraction/rules_adjuster_divergencias_test.go`:

```bash
go test ./pkg/extraction -run TestRelatorio -v   # quais dias mudam e quantos minutos
go test ./pkg/extraction -run TestSonda -v       # quanto saldo o dia gerado inventa
```

A régua é uma cópia de `adjustDay` com as quatro constantes como parâmetro, e ela
se afere antes de medir: com as constantes de hoje tem de devolver o que
`adjustDay` devolve — 68.936 comparações para os valores, mais as 14 linhas do
snapshot para a ordem dos sorteios. A medição roda só sobre os dias que a
produção pode receber (os pontos fixos de `reinterpretarPontos`); sem esse filtro
o pior caso parecia ser de 660 minutos de saldo inventado, num dia que `Adjust`
nunca entregaria.

O resumo do que foi medido:

| Divergência | Se unificar | Dias que mudam | Dias reais do usuário |
|---|---|---|---|
| 1. piso da tarde | 3h em todos | 46 de 20.076 (0,2%), até 37 min | nenhum muda |
| | nenhum piso | 840 de 20.076 (4,2%), até 244 min | nenhum muda |
| 2. faixa da manhã | as duas em 220–260 | 364 (1,8%), até 27 min | nenhum muda |
| | as duas em 200–240 | 56 (0,3%), até 27 min | nenhum muda |
| 3. ordem do desvio | sempre antes do piso | **zero** | nenhum muda |
| 4. entrada em `.X.X` | pela carga | 419 (2,1%), até 186 min | nenhum muda |

**Nenhuma das quatro toca os seis dias reais de junho e julho**, em nenhum dos 7
regimes de sorteio.

O detalhe que decide a nº 1 não é "quantos minutos muda", é **quanto saldo o dia
gerado inventa** — porque é isso que vai para o documento assinado. O piso é o
que quebra o fechamento: com ele, os casos de tarde inventada fecham a jornada em
84%, 76% e 69% das vezes; **sem piso nenhum, em 95% a 96%**, e o excesso máximo
cai de +239 min para +22 (que é só o desvio de −5 a +15 mais a fuga do minuto
redondo).

**Mas tirar o piso não é opção.** Medido: sem piso nenhum, o caso `X.X.` (as duas
entradas reais, as duas saídas inventadas) produz tarde de **−64 minutos** — a
saída sai antes do retorno. E a tarde cai abaixo de uma hora em 1,8% dos dias de
`XXX.`, 3,3% de `XX..` e 11,4% de `X.X.`. O piso não é só inflador de saldo: onde
ele segura, está segurando de verdade.

#### Recomendação (2026-08-13), à espera da decisão

| # | Recomendação | Por quê |
|---|---|---|
| 1 | **piso de 3h em todos** | é o valor da maioria (4 dos 7 usos) e o único que faz trabalho real; estendê-lo aos dois casos sem piso custa 46 dias em 20.076 (0,2%), no máximo 37 min, e no máximo +32 min de saldo inventado. Em troca, **acaba o acoplamento com `janelaSlot`** — a garantia passa a morar em `adjustDay`, onde deveria, e `rules_adjuster_acoplamento_test.go` pode ser apagado (ele próprio diz isso ao passar a falhar) |
| 2 | **as duas em 200–240** | é o que os outros dois geradores da mesma pergunta já usam (`saidaParaAlmocoDepoisDaEntrada` e `saidaParaAlmocoEspremida`), e a faixa contém a manhã contratual de 210 min (08:30–12:00); 220–260 é a exceção solitária. É também a mudança mais barata: 56 dias contra 364 |
| 3 | **sempre antes do piso** | **custo zero**: nenhum dia muda. Na carga de 480 min o piso desse caso nunca chega a segurar (0 de 42 dias; a menor tarde é de 255 min contra um piso de 180). A assimetria não distingue nada — só parece distinguir. E numa carga menor, que é configurável, ela vira defeito: a 360 min o piso passa a segurar sempre e a tarde termina em 174 min, **abaixo do piso de 180** que a chamada diz respeitar |
| 4 | **calculada pela carga** | usa informação que existe e está sendo jogada fora, que é o defeito de verdade. Hoje esse caso fecha a jornada em **8%** dos dias, com desvio médio de 2h40; pela carga vai a **42%**, com 1h28 — exatamente a faixa dos irmãos que já fazem isso (`.XXX` fecha 42%, `..XX` 51%). Atenção: é a única das quatro que muda a ORDEM em que os sorteios são consumidos, porque o retorno precisa ser calculado antes da entrada |

Nenhuma das quatro toca os seis dias reais. Se a 1 for aceita, a linha do
MANUAL.md que promete *"o dia fecha na jornada exigida, 8h, com variação de alguns
minutos"* continua tão aproximada quanto hoje — é o piso que a enfraquece, e ele
fica. Vale saber que essa linha é a única do MANUAL que a medição não sustenta ao
pé da letra.

### 2. Batimento na coluna errada em dia de dispensa
Levantado pelo usuário em 2026-08-10 e **conscientemente deixado manual** — mas
o custo apareceu na mesma sessão.

Em 03/07/2026 o usuário bateu 09:26 e 13:45 num dia de dispensa de 4h. A
extração leu o 13:45 como retorno do almoço (E2), então nenhum turno fecha, o
dia acusa −04:00 em vez de +00:19, e a frase automática não sai. A correção é
arrastar o 13:45 para S1 — **e ela não sobrevive ao reprocessamento do mês**,
como se viu duas vezes na sessão.

Por que a extração erra: `Adjust` sai fora em dia de dispensa — o early-return
de `DayTypeDispensa`, na própria `Adjust` — sem chamar `reinterpretarPontos`; e
mesmo se chamasse não adiantaria. Conferido: com 09:26 e 13:45, pôr 13:45 em E2
custa penalidade **0** (a janela do retorno vai de 11:30 às 16:00) e pô-lo em S1
custa **15** (a janela da saída para o almoço fecha às 13:30). A reinterpretação
escolheria E2 também. As janelas foram calibradas para a jornada de 8h; numa
dispensa de 4h, 13:45 é o fim do expediente. A jornada do ato, porém, só é
informada **depois**, no navegador: nenhuma correção na extração tem esse número.

#### Recomendação (2026-08-13): sugerir na tela, não preservar a correção

Das duas opções, **a sugestão na tela**, e o motivo decisivo é que ela dispensa
resolver a persistência em vez de tentar resolvê-la.

A informação que falta — a jornada do ato — não existe no servidor no momento da
extração; existe no navegador, digitada pelo usuário. Decidir onde a informação
está é decidir onde a correção mora. E como o aviso é **derivado** do estado
atual da edição, ele reaparece sozinho depois de um reprocessamento: a extração
volta a pôr 13:45 em E2, a tela volta a apontar, e o conserto continua a um
clique. O problema "a correção não sobrevive ao reprocessamento" deixa de existir
sem que ninguém precise fazer merge de edição manual com extração nova — que é o
que a outra opção exigiria, e ela mudaria o significado de reprocessar para
**todos** os dias, não só os de dispensa.

**Quando aparece.** As quatro condições, todas já calculáveis com o que existe
em `web/js/domain.js`:

1. o tipo do dia exige jornada por ato (`CARGA_POR_ATO`: dispensa ou expediente
   reduzido);
2. a jornada **está informada** (`d.carga > 0`) — sem ela já sai o aviso de hoje,
   `MSG_CARGA_DISPENSA`, e dois avisos na mesma linha seriam ruído;
3. há batimento no dia;
4. **nada fecha**: `minutosTrabalhados(d) === 0`, que é exatamente a função que
   já soma só os turnos completos.

No 03/07 as quatro batem: dispensa, carga 240, dois batimentos, nenhum turno
fechado — e é por isso que o dia acusa −04:00.

**O que aparece.** O ⚠️ da linha, pelo mesmo caminho de `avisoDeRevisao`, que já
existe para juntar o que o backend apurou com o que depende da edição em curso.
O texto diz o que está errado e o que se propõe, com os horários do dia:

> Nenhum turno fecha: 09:26 e 13:45 estão em colunas que não formam par. Numa
> jornada de 04:00, o mais provável é que sejam entrada e saída do expediente.
> **[Ler como 09:26 → 13:45]**

**O que o botão faz.** Move os dois batimentos para E1 e S1 — o mais cedo na
entrada, o mais tarde na saída —, sem alterar nenhum horário. É o que o usuário
faz hoje arrastando, e vale para os quatro pares que não fecham
(`{E1,E2}`, `{E1,S2}`, `{S1,E2}`, `{S1,S2}`); os pares que fecham não disparam o
aviso. No 03/07 o resultado é 09:26–13:45 = 4h19 contra 4h de jornada: **+00:19**,
que é o número que o usuário esperava.

**O que ele nunca faz:** aplicar-se sozinho. O batimento é do servidor; o sistema
pode apontar que a leitura da coluna não fecha, não decidir por ele. E como só
move colunas, nenhum horário é inventado — o que o mantém fora do território do
`adjustDay`.

**Limite honesto:** a regra só cobre o dia com **dois** batimentos e nenhum turno
fechado. Com três, algum turno fecha e o aviso não sai; com um só, não há o que
propor. São os casos em que a proposta seria adivinhação.

### 3. Nada mais é urgente
- `web/styles.css`: ~2100 linhas, 9 `!important` (item 3.5). Baixo retorno.
- Prettier no front-end. Baixo retorno.
- A troca da 4.8 (front-end consumir `/api/process`): **recomendação é NÃO
  fazer** — o teste compartilhado já entrega a garantia que ela entregaria, sem
  latência de rede a cada tecla nem falha quando o servidor não responde. O
  raciocínio completo está na 4.8 do plano.

## Armadilhas que custaram tempo

**Testar no navegador escreve em `data/`.** O app tem auto-save; qualquer
interação de teste (editar campo, trocar tipo de dia, mexer na jornada) grava no
arquivo do mês. Confira e limpe depois de testar:

```bash
python -c "
import json; d=json.load(open('data/06_2026.json',encoding='utf-8'))
print([(x['d'],k) for x in d['dias'] for k in ('carga','day_type_override','ocorrencia_manual','justificativa_manual','limpos') if k in x])"
```

O jeito seguro de verificar sem tocar em nada: copiar `data/` para uma pasta
temporária, subir o preview, **só carregar e ler** (nunca editar), e comparar
com `diff` ao final. Carregar um mês não dispara o auto-save; interagir dispara.

**O usuário mexe no app durante a sessão.** Em 2026-08-10 o `data/07_2026.json`
mudou três vezes no meio da conversa — ele estava com o app aberto na 8080. Não
confie na leitura do começo da sessão: releia o arquivo e olhe o `updated_at`
antes de afirmar qualquer coisa sobre um dia.

**Verificar regra sem navegador nenhum:** um `.mjs` que importa `web/js/domain.js`
direto e roda o dia real por `deWire` → `classifyDay` → `saldoDoDia`. Prova com
os dados dele, sem escrever nada. Atenção: `getJustTemplate()` usa
`localStorage`, então passe o `template` explicitamente às funções de
`justificativa.js` fora do navegador.

**Cobertura de linha engana no `rules_adjuster.go`.** No `-coverprofile`, o
bloco que começa na linha do `if` conta as **avaliações da condição**, não as
execuções do corpo — pegue o bloco cujo fim é maior que o início, senão todo
ramo parece rodar milhares de vezes. E cobertura ali não é proteção: para saber
se a suíte segura mesmo, **mude um número de propósito e veja se ela percebe**.
Foi assim que se descobriu que não segurava.

**`gofmt -l` no Windows lista todos os arquivos.** É o CRLF da cópia de
trabalho, não erro de formatação. Para checar de verdade, converta para LF numa
pasta temporária e rode `gofmt -l` lá — ou confie no CI, que roda em Linux.
(Detalhe: só `index.html` e `styles.css` estavam em CRLF no repositório; os `.go`
sempre estiveram em LF.)

**`-race` não roda aqui.** Exige `CGO_ENABLED=1` e um compilador C que não está
instalado. O portão real é o CI.

**O `web/` é embutido no binário** via `go:embed`. Mudança em `web/**` só
aparece depois de reiniciar o servidor.

## Como o projeto está organizado agora

```
pkg/app/        composição (main.go e api/index.go são cascas)
pkg/apperr/     erros de domínio; pkg/api os mapeia para status HTTP
pkg/rules/      motor de regras (rules.json embutido via go:embed)
pkg/service/    casos de uso; validações de upload
pkg/extraction/ provedores de IA, retry, ajuste de horários
web/js/         domain.js, util.js, config.js, justificativa.js, biblioteca.js
web/app.js      só o que depende do DOM
testdata/regras.json  casos compartilhados entre backend e front-end
```

**O ponto mais importante da arquitetura atual:** `testdata/regras.json` tem 13
casos que o motor Go e o `web/js/domain.js` são obrigados a reproduzir com os
mesmos números. Mudar uma regra exige mudar o valor esperado ali, uma vez, e os
dois lados junto. Foi isso que fechou a divergência de 72 horas entre as duas
implementações — e é o que impede que ela volte.

## Decisões do documento SEI (2026-08-10)

Todas escolhidas pelo usuário, e o código as documenta no ponto em que valem:

- **Um critério só decide quem entra no documento**: `isOccurrenceDay`, que é
  exatamente o que o checkbox da linha mostra. Marcou, o dia sai nas duas
  seções — tabela de ocorrência e justificativa —, sem exceção para dispensa
  nem para dia sem horário nenhum. Dois predicados para a mesma pergunta foi o
  defeito que essa unificação corrigiu.
- **A frase automática só afirma o que o cálculo sustenta.** `justificativaDeAto`
  cala-se quando a jornada do ato não foi informada e quando não foi cumprida;
  `motivoDoSilencio` explica o silêncio no placeholder, porque campo em branco
  é indistinguível de defeito.
- **A frase é guardada sem a data** e com lacunas `{jornada}` `{trabalhado}`
  `{data}`, resolvidas na exibição — é o que a torna reaproveitável em qualquer
  mês.
- **Servidor é a verdade** na biblioteca de frases; `localStorage` é cache de
  leitura e fila de escrita. Fundir os dois lados tornaria a exclusão
  impossível.
- `justificativas.json` fica **ao lado do executável, fora de `data/`**: o
  `List()` do `JSONTimesheetRepository` trata todo `.json` daquela pasta como um
  mês salvo.
