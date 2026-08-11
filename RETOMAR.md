# Onde paramos

Ponto de partida para uma nova sessão. As regras de operação estão no
[MANUAL.md](MANUAL.md); o detalhe do que foi feito, em
[REFATORACAO.md](REFATORACAO.md). Aqui fica só o necessário para retomar.

## Estado

**A refatoração foi mergeada na `master`** em 2026-08-10, pelo PR #1
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
go build ./... && go vet ./... && go test ./...   # 204 testes
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

### 1. Reescrever o `RulesAdjuster` (item 4.5 do plano)
`pkg/extraction/rules_adjuster.go`, função `adjustDay`: 12 combinações de
batimentos nomeadas mais um fallback que registra *"combinação não prevista"*.
É o código que **inventa horários que vão para um documento assinado por um
servidor público** — o último ponto do sistema onde algo pode sair arbitrário.

`janelaSlot` e `reinterpretarPontos`, no mesmo arquivo, já mostram o caminho
declarativo.

Em 2026-08-10 o terreno foi mapeado antes de qualquer reescrita. O que se sabe
agora foi **medido, não deduzido**:

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

**Os testes NÃO protegem a reescrita.** Esta é a descoberta que muda o plano.
Todos os 13 ramos são executados, mas 11 só pela grade exaustiva
(`TestHorariosGeradosSempreValidos`), que confere três invariantes — nada passa
das 23:59, ordem cronológica, nenhum batimento real some — e **nunca olha o
número gerado**.

> Prova por mutação: somar 45 minutos a toda saída inventada pelo ramo "falta a
> saída" deixa `go test ./...` inteiramente verde. São 2h15 a mais em junho, no
> documento assinado, sem nenhum portão reclamar.

**Portanto o primeiro passo da reescrita não é reescrever.** É travar os
valores de hoje: semente fixa no sorteio (`randBetween` usa o `math/rand`
global, então não há como reproduzir uma execução) e snapshot dos 13 ramos. Sem
isso a suíte fica verde tanto para o certo quanto para o errado, e a reescrita
é inverificável.

Dois defeitos dessa mesma família já foram corrigidos e têm teste. A reescrita
não pode perdê-los:

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

**Merecia uma sessão dedicada, com contexto limpo.**

### 2. Batimento na coluna errada em dia de dispensa
Levantado pelo usuário em 2026-08-10 e **conscientemente deixado manual** — mas
o custo apareceu na mesma sessão.

Em 03/07/2026 o usuário bateu 09:26 e 13:45 num dia de dispensa de 4h. A
extração leu o 13:45 como retorno do almoço (E2), então nenhum turno fecha, o
dia acusa −04:00 em vez de +00:19, e a frase automática não sai. A correção é
arrastar o 13:45 para S1 — **e ela não sobrevive ao reprocessamento do mês**,
como se viu duas vezes na sessão.

Por que a extração erra: `Adjust` sai fora em dia de dispensa
(`rules_adjuster.go:81`) sem chamar `reinterpretarPontos`; e mesmo se chamasse,
as janelas de `janelaSlot` foram calibradas para a jornada de 8h — 13:45 cai
dentro de E2 com penalidade 0. Numa dispensa de 4h, 13:45 é o fim do
expediente. A jornada do ato, porém, só é informada **depois**, no navegador:
uma correção na extração não tem esse número.

Opções, se ele quiser retomar: sugerir a correção na tela (⚠️ com botão) quando
tipo é dispensa, a jornada está informada e os pontos não fecham; ou preservar
a correção manual através do reprocessamento.

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
