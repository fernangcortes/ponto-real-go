# Onde paramos

Ponto de partida para uma nova sessão. O detalhe de tudo que foi feito está em
[REFATORACAO.md](REFATORACAO.md); aqui fica só o necessário para retomar.

## Estado

Branch **`refatoracao`**, 8 commits à frente da `master`, árvore limpa.
**Nada foi mergeado ainda.**

```
<novo>  feat: dar voz ao dia de dispensa cumprida no documento SEI
40975df docs: adicionar RETOMAR.md com o estado e as armadilhas do projeto
995d552 fix: permitir apagar um horário e manter o aviso de jornada em dia
fc485ae docs: registrar o plano de refatoração e o que foi executado
25b049f build: adicionar CI, lint e portões de qualidade
7c54e2c test: travar as regras compartilhadas entre backend e front-end
b22bdcd refactor(frontend): modularizar, corrigir escape e tirar efeito colateral do render
db4f6db refactor(backend): reestruturar camadas, corrigir defeitos e cobrir com testes
e4e612e chore: normalizar fim de linha e fixar a regra no .gitattributes
```

Verificação (tudo verde no último commit):

```bash
go build ./... && go vet ./... && go test ./...   # 194 testes
golangci-lint run ./...                            # 0 issues
npm run lint && npm test                           # 76 testes
```

## O que o usuário precisa fazer antes de qualquer coisa nova

1. **Conferir os dois meses no app.** Muita regra de cálculo mudou — férias,
   dispensa, saldo oficial, e agora as duas seções do documento SEI. Ele é quem
   sabe se os números batem com a realidade.
2. **Mergear**, se estiver bom: `git checkout master && git merge refatoracao`
3. **Dar push** — o `-race` nunca rodou nesta máquina (falta compilador C) e só
   é exercitado de verdade no CI.

## O que falta, em ordem de valor

### 1. Reescrever o `RulesAdjuster` (item 4.5 do plano)
`pkg/extraction/rules_adjuster.go`, função `adjustDay`: cascata de 13 `if`
cobrindo combinações de batimentos, terminando num fallback que registra
*"combinação não prevista"*. É o código que **inventa horários que vão para um
documento assinado por um servidor público** — o último ponto do sistema onde
algo pode sair arbitrário.

`janelaSlot` e `reinterpretarPontos`, no mesmo arquivo, já mostram o caminho
declarativo. Os testes de `pkg/extraction` protegem a reescrita.

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
