# Plano de refatoração — Ponto Real Go

Levantamento feito sobre o estado atual (`master`, commit `f795dff`). Build limpo,
`go vet` limpo, 25 testes passando. O código **funciona**; o que segue é sobre
sustentabilidade, não sobre consertar algo quebrado — com exceção dos itens
marcados 🔴, que são defeitos reais.

Números de referência:

| Área | Linhas | Observação |
|---|---:|---|
| `web/app.js` | 2554 | escopo global único, sem módulos |
| `web/styles.css` | 1985 | arquivo único |
| `pkg/extraction/rules_adjuster.go` | 536 | 13 ramos de `if` para gerar horários |
| `pkg/api/handler.go` | 425 | HTTP + regra de aplicação misturados |
| `web/index.html` | 411 | 16 handlers inline |
| `pkg/rules/engine.go` | 394 | motor canônico — bem testado |

---

## O problema estrutural nº 1: a regra de negócio existe duas vezes e diverge

Todo o cálculo de saldo e classificação de dia está implementado em `pkg/rules/engine.go`
**e** de novo em `web/app.js`. As duas implementações não concordam:

| Regra | Backend (`engine.go`) | Frontend (`app.js`) |
|---|---|---|
| Carga diária | `e.Config.CargaHorariaDiaria` (configurável) | `480` fixo em 4 lugares |
| Carga de dispensa | não existe | `CARGA_DISPENSA = 240` (app.js:585) |
| Nome do tipo | `expediente_reduzido` | `reduzido` |
| Feriado | `DayTypeFeriado` | mapeado para `'recesso'` (app.js:102) |
| Detecção de falta | `diasUteis[d.DiaSemana]` | `d.saldo === '-08:00'` (app.js:122) |
| Preenchimento automático | `RulesAdjuster.adjustDay` | `autoFillDay` — "espelho", reescrito |

E o agravante: **`/api/process` e `/api/validate` estão implementados, roteados e
nunca são chamados**. Nenhum `fetch` do front-end aponta para eles (verificado).
O motor canônico está pronto, exposto e testado — e o front-end o ignora para
recalcular tudo por conta própria. Toda regra nova hoje precisa ser escrita duas
vezes, em duas linguagens, com dois conjuntos de constantes.

**Esta é a refatoração que mais paga.** Está na Fase 4 porque exige as seams das
fases anteriores.

---

## Fase 0 — Rede de segurança e limpeza ✅ CONCLUÍDA

### 0.1 Remover código morto ✅
Removido (verificado sem nenhum chamador):

| Alvo | Linhas | Por quê |
|---|---:|---|
| `pkg/extraction/adjuster.go` (`GeminiAdjuster`) | 132 | substituído pelo `RulesAdjuster`; nunca instanciado |
| `pkg/extraction/prompt_adjust.go` (`AdjustmentPrompt`) | 63 | só era usado pelo `GeminiAdjuster` |
| `extraction.AvailableModels()` | 4 | "backward compatibility" sem consumidor |
| `TimesheetRepository.Delete` | 8 | no contrato e implementado, sem rota nem chamada |
| `models.Timesheet.Atrasos` / `.Faltas` | 2 | extraídos e nunca lidos (saíram também do schema do prompt) |
| `rand.Seed` em `init()` | 3 | deprecado desde Go 1.20 e **no-op** com `go 1.25` no go.mod |

**`AlignResult` foi mantido — o item original do plano estava errado.** Ele
parece morto olhando só o código de produção (as duas chamadas em
`timesheet_service.go` descartam o retorno), mas quatro testes de
`align_test.go` asseguram sobre `res.Corrigido`, `res.Desalinhado` e
`res.DeslocamentoAplicado`. É o resultado observável da função e o que torna o
realinhamento testável.

**Config fantasma — resolvido:**
- `AlmocoGeradoMin/Max` **ligados** ao `RulesAdjuster` via `almocoGerado()`.
  Comportamento idêntico: a config já continha 60/75 e o código fazia
  `minAlmoco + randBetween(0, 15)` = 60..75. O helper protege contra faixa
  mal configurada, nunca gerando almoço abaixo do mínimo legal.
- `VariacaoMin`, `VariacaoMax` e `HorarioContratual` **removidos** de
  `RulesConfig`, de `rules.json` e de `NewEngineWithDefaults` — descreviam um
  comportamento que o código não tem.
- Uma ocorrência ficou de fora de propósito: `s1 = e2 - minAlmoco - randBetween(0, 10)`
  (`rules_adjuster.go`, ramo de 3 pontos preenchidos) usa a faixa 60..70, não
  60..75. Unificar mudaria comportamento, então fica para a reescrita do
  adjuster na Fase 4.1.

*Resultado: −239 linhas de produção, build e `go vet` limpos.*

### 0.2 Testes de caracterização ✅
De 25 para **84 testes** (1 pulado no Windows), cobrindo os dois pacotes que
tinham zero:

- `pkg/api/handler_test.go` (37 casos) — todas as 11 rotas via `BuildHandler`,
  com dublês de extrator/factory/repositórios. Cobre mascaramento de chave,
  preservação de chave em payload vazio, MIME inferido e recusado, modelo
  inválido, defaults por provedor, auto-save do mês, `[]` vs `null`, conversão
  `06_2026` → `06/2026` e preflight CORS.
- `pkg/repository/json_repository_test.go` (14 casos) — round-trip, sobrescrita,
  ordenação, tolerância a arquivo corrompido, permissão 0600 das settings e o
  comportamento sob `VERCEL=1`.

### 0.3 Achados novos, descobertos ao escrever os testes

🔴 **`List()` ordena por string, não por data.** `json_repository.go` faz
`summaries[i].MesAno > summaries[j].MesAno` sobre `"MM/AAAA"`. Com o mês na
frente, `12/2025` ordena como mais recente que `01/2026` — o ano é ignorado. Todo
usuário que virar o ano vê a lista de meses fora de ordem. Está travado em
`TestListOrdenaPorStringNaoPorData`, que **documenta o comportamento errado** e
falha quando for corrigido. Correção: ordenar por `AAAA/MM`. Trivial, mas é
mudança de comportamento visível — por isso não entrou na Fase 0.

⚠️ **`-race` não roda nesta máquina.** Exige `CGO_ENABLED=1` e um compilador C;
não há gcc/mingw instalado. O data race da Fase 1.1 não é detectável localmente
— o portão tem que ser o CI Linux (Fase 5). Ao atacar a Fase 1.1, o primeiro
passo é escrever um teste com `SaveSettings` e `Health` concorrentes, que só vai
acusar sob `-race` no CI.

~~⚠️ **O repositório inteiro está em CRLF e `gofmt -l` reprova os 21 arquivos.**~~

**Este achado estava errado — corrigido na Fase 5.** Os blobs no repositório já
estão em LF; o CRLF existe só na cópia de trabalho, por causa do
`core.autocrlf=true`, que é a configuração correta no Windows. Não havia
nenhuma normalização a fazer.

O que acontece de fato: `gofmt -l` rodado no Windows enxerga o CRLF da cópia de
trabalho e lista todos os arquivos. É ruído local, não erro. Comparando o
conteúdo real (convertido para LF), só **4 arquivos** tinham problema de
formatação de verdade — linhas em branco sobrando e alinhamento de comentário —
e foram corrigidos na Fase 5.

Lição: `gofmt -l` não é confiável no Windows com `autocrlf`. A checagem que vale
é a do CI, em Linux.

---

## Fase 1 — Defeitos no backend ✅ CONCLUÍDA

### 1.1 Data race em `Handler.settings` ✅
As configurações eram um campo mutável do `Handler`, escrito por `SaveSettings`
enquanto `Upload`, `Health` e `GetModels` liam — cada requisição na sua goroutine.

Agora vivem em `pkg/api/settings_store.go`, um tipo com `sync.RWMutex` que só
expõe cópias (`Get`) e uma escrita atômica (`Atualizar`). `ChaveDoProvedorAtivo`
concentra a regra de "qual chave vale", que estava repetida em três handlers.

`TestSettingsConcorrentesNaoDaoRace` roda 8 escritores e 8 leitores em paralelo.
**Atenção: ele só acusa a falha sob `go test -race`**, que não roda na máquina
de desenvolvimento (sem compilador C). O portão de verdade é o CI — ver Fase 5.

### 1.2 `defer resp.Body.Close()` dentro do loop de retry ✅
Cada tentativa virou uma função própria (`tentar`), então o corpo fecha ao fim
dela em vez de acumular três conexões até o `Extract` inteiro terminar.

A lógica de repetição saiu duplicada dos dois extratores e virou
`pkg/extraction/retry.go`, com o conceito que faltava: `naoRepetir()` marca o
erro que não melhora repetindo (chave inválida, crédito insuficiente), e o laço
para na hora em vez de fazer o usuário esperar três vezes pela mesma recusa.

### 1.3 Nenhuma chamada externa é cancelável ✅
`Extractor.Extract` agora recebe `context.Context`, propagado de `r.Context()`
pelo handler e pelo serviço, até `http.NewRequestWithContext`. As esperas entre
tentativas também respeitam o cancelamento (`esperar()` em vez de `time.Sleep`).

`TestCancelamentoInterrompeAChamada` prova o efeito: o que levaria 120s × 3
agora aborta em milissegundos.

### 1.4 Backend diverge entre deploys ✅
`rules.json` passou a ser `//go:embed` em `pkg/rules`. `NewEngineWithDefaults`
lê o arquivo embutido em vez de um struct literal hardcoded, então servidor local
e serverless partem exatamente das mesmas regras — e a busca por caminho relativo
dependente do diretório de trabalho sumiu.

`NewEngine(path)` continua existindo para override consciente, exposto via
`PONTO_REAL_RULES`. Caminho ilegível avisa e cai nas embutidas.
`TestBuildAPIEBuildServerUsamAsMesmasRegras` trava a convergência.

### 1.5 Wiring de DI duplicado ✅
Nasceu `pkg/app`, com `BuildAPI` (serverless) e `BuildServer` (local + estáticos).
`main.go` foi de 93 para ~80 linhas — quase todas de banner — e `api/index.go`
de 37 para 23.

**Defeito extra encontrado aqui:** o `main.go` montava o handler da API já
encadeado com os middlewares e depois encadeava o mux inteiro de novo por fora.
Toda requisição `/api` passava **duas vezes** por Recovery, Logging e CORS, e
aparecia duplicada no log. `BuildServer` agora usa um mux só e uma passagem só.
Verificado no boot: três chamadas de API, três linhas de log.

**Também corrigido:** o banner fatiava `NomeInstituicao[:30]` em bytes — panic no
boot com nome curto, acento cortado no meio com nome acentuado. Agora conta runas
e trunca com reticências.

### 1.6 `List()` ordena por string em vez de por data ✅
Passou a comparar `(ano, mês)`. `TestListOrdenaPorDataNaViradaDoAno` cobre a
virada e `TestListMesComFormatoInvalidoVaiParaOFim` garante que um nome de
arquivo estranho não derrube a listagem.

### 1.7 Versão unificada (antecipado da Fase 2.5) ✅
Como a Fase 1 já mexia no `Health` e no banner, o número saiu dos três lugares
onde estava solto e foi para `pkg/version`. O front-end agora lê a versão de
`/api/health` em vez de trazê-la escrita no HTML.

Valor adotado: **1.1.0**, que é o que o README e o histórico de commits já
anunciavam — o código é que estava atrasado em 1.0.0.

### 1.8 Chave do Gemini fora da query string (achado da Fase 1) ✅
A chave da API viajava como `?key=...` na URL. URL é o pior lugar para segredo:
vai parar em log de proxy, em histórico e em referer. Passou para o cabeçalho
`x-goog-api-key`, que a API do Gemini aceita. `TestGeminiMandaChaveNoCabecalhoNaoNaURL`
falha se alguém reverter.

---

## Fase 2 — Camadas: tirar regra de negócio do HTTP ✅ CONCLUÍDA

### 2.1 `handler.go` fazia coisas que não são HTTP ✅
O `Upload` tinha 107 linhas decidindo provedor, chave, limite, detecção e
validação de MIME, modelo padrão e validação de modelo — só então chamava o
serviço. Agora tem **40**, e o `handler.go` inteiro caiu de 425 para 284 linhas.

Tudo isso virou `service.UploadRequest` + `normalizar()` em
[upload.go](pkg/service/upload.go). O ganho não é só de arrumação: essas regras
passaram a ser testáveis sem montar requisição multipart — são 17 casos novos em
`upload_test.go`.

O catálogo de modelos também mudou de casa. `ModeloPadrao`, `ModeloValido` e
`ModelosDoProvedor` foram para `pkg/extraction`, que já era dono das listas.
O padrão deixou de ser string repetida no handler e passou a ser "o primeiro do
catálogo", que por convenção é o recomendado.

### 2.2 Erros de domínio e mapeamento para status ✅
Nasceu [pkg/apperr](pkg/apperr/apperr.go), com oito sentinelas, e
`statusDoErro` em [errors.go](pkg/api/errors.go) — uma tabela só, com
`errors.Is`, que sobrevive a três camadas de envelopamento.

O que o usuário vê mudou de fato:

| Situação | Antes | Agora |
|---|---|---|
| Chave de API inválida | 500 | **400** com a mensagem do provedor |
| Crédito insuficiente | 500 | **400** |
| Arquivo/modelo inválido | 400 | 400 |
| Extração nunca deu folha utilizável | 500 | **502** (o problema é do serviço externo) |
| Cliente fechou a aba | 500 | 499 |
| Falha inesperada | 500 | 500 |

Só 5xx vai para o log de erro: 4xx é o usuário sendo avisado do que corrigir, e
encheria o log sem informar nada.

### 2.3 `VERCEL` vazando para dentro do repositório ✅
A variável agora é lida em **um único ponto do código de produção**:
`ConfigFromEnv`, na composição. Eram seis (cinco no repositório JSON, um no
handler).

`NoopTimesheetRepository` e `NoopSettingsRepository` ([noop.go](pkg/repository/noop.go))
tornaram "não guardar nada" uma implementação explícita em vez de um `if` escondido.
O repositório JSON voltou a ser só um repositório JSON.

De quebra sumiu um caminho morto: o `filepath()` das settings tinha um ramo
`/tmp/` para o Vercel que nunca era usado, porque o handler já pulava a gravação
naquele ambiente.

### 2.4 Logging ✅
Todo `fmt.Printf`/`println` de log virou `log/slog` estruturado. O banner de boot
continua em `fmt` — é saída para o usuário, não registro.

`ConfigurarLog` fica em `pkg/app`; o código usa as funções de pacote do `slog`.
Optei por não carregar um `*slog.Logger` por todas as camadas: para um binário
só, o plumbing custa mais do que rende. Testes silenciam com `TestMain`
(`PONTO_REAL_TEST_LOG=1` religa).

O `LoggingMiddleware` passou a registrar **status e duração** via um
`statusRecorder`. Antes só dizia método e caminho — não dava para saber se a
requisição deu certo nem se estava lenta, que são as duas perguntas que levam
alguém a olhar o log.

### 2.5 Versão em três lugares ✅
Feito na Fase 1.7.

### 2.6 Achado da Fase 2: `node_modules` entrou no `./...`
O ESLint traz dependências com arquivos `.go` de exemplo dentro, e o
`go build/vet/test ./...` passou a varrê-los — efeito colateral do `package.json`
que a Fase 5 introduziu. Resolvido com a diretiva `ignore` do Go 1.25 no
`go.mod`.

---

## Fase 3 — Front-end: quebrar o monólito ✅ (menos o CSS)

Feito: **3.1**, **3.2**, **3.3**, **3.4** e a conversão de todos os handlers
inline para delegação. Pendente só **3.5** (CSS) e o Prettier.

### 3.1 Os dois defeitos ✅

**Escape de HTML** — resolvido com um `esc()` único, aplicado em todo ponto de
interpolação (16 pontos). Verificado com uma observação hostil
(`DISPENSA P/ CURSO "AVANÇADO" <b>X</b>`): a linha continua íntegra com 11
células e a tag não vira elemento. Antes, a aspa fechava o atributo `title` e
quebrava a linha.

Os `onclick="copyCell('${valor}')"` foram a raiz do problema — ali o dado
atravessava uma string JS **dentro** de um atributo HTML, o que exige dois
níveis de escape diferentes. Substituídos por `data-copy` + delegação de clique,
que deixa um nível só.

**Render com efeito colateral** — a auto-detecção virou `detectarTipoDia()`,
função pura, e o render apenas a lê. Verificado: após renderizar,
`dayTypeOverride` continua `null`.

Isso destravou um bug que o usuário sentia e que não estava no plano: **escolher
"Útil" num dia com DISPENSA no motivo não funcionava**. O `changeDayType` zerava
o override, o render seguinte redetectava dispensa e reescrevia por cima. Agora
"util" é guardado como escolha explícita e sobrevive ao re-render.

**Achado ao unificar:** a antiga auto-detecção do render e o antigo `classifyDay`
usavam ordens DIFERENTES — o render checava fim de semana antes da observação, o
`classifyDay` depois. Num sábado com "DISPENSA" o seletor mostrava "FDS" enquanto
o cálculo tratava como dispensa. Mantive a ordem do `classifyDay`, que é a que
sempre determinou os saldos.

### 3.2 Dividir em módulos ES ✅

`<script type="module">`, sem bundler — o projeto segue sem etapa de build.

| Módulo | Linhas | Conteúdo |
|---|---:|---|
| `js/config.js` | 55 | CONFIG, constantes, `resolverApiBase` |
| `js/util.js` | 76 | tempo, `esc`, `copiavel` — nada de DOM |
| `js/domain.js` | 260 | classificação, saldos, `deWire`/`paraWire` |
| `js/justificativa.js` | 33 | frase padrão (isola o `localStorage`) |
| `web/app.js` | 2386 | só o que depende do DOM |

**O ganho que importa: `npm test` agora roda 28 testes do domínio do front-end,
sem navegador.** Era o item que estava pendente desde a Fase 5, e cobre a mesma
aritmética que decide o saldo que o servidor público vai assinar — incluindo os
casos que só existiam na cabeça de quem escreveu: dia neutro que mostra 00:00 mas
não entra no total, expediente reduzido sem carga informada, dispensa sem
horário, e o `false` de `ocorrencia_manual` que não pode virar `undefined`.

O escopo de módulo também eliminou o vazamento de globais: das ~40 funções e
variáveis que viviam no escopo global, restam em `window` apenas as 8 que o
`index.html` chama de `onclick` — verificado no navegador.

**Nota:** `domain.test.js` fica ao lado do código e portanto é embutido no
binário pelo `go:embed all:web` (~9 KB). Preferi manter o teste junto do módulo
a separá-lo só para poupar isso.

### ~~3.2~~ (descrição original abaixo)
Sem bundler, só `<script type="module">` — o projeto não tem build e não precisa ter:

```
web/js/
  config.js       CONFIG, constantes, versão
  domain.js       t2m, m2t, classifyDay, isOccurrenceDay, autoFillDay
  api.js          os 7 fetch, centralizados
  render/
    table.js      renderTables      (~145 linhas hoje)
    summary.js    updateAll         (~155 linhas hoje)
    sei.js        generateSeiHtml   (~160 linhas hoje)
  viewer.js       visualizador de documento (~220 linhas hoje)
  tour.js         Tour              (~310 linhas hoje)
  main.js         wiring + listeners
```

**A parte difícil já está feita.** Todos os 20 handlers inline do `app.js` viraram
`data-*` + delegação (`click`, `focusin`, `focusout`, `change`), e as 8 funções
que só existiam em `window` para serem alcançáveis do HTML gerado voltaram a ser
`const` de módulo. Restam em `window` apenas as 9 que o `index.html` chama
diretamente — e essas continuam funcionando sob `type="module"`, porque a
atribuição a `window` é explícita.

O que falta é o recorte dos arquivos em si, agora sem obstáculo estrutural.

### 3.3 Quebrar `updateAll` ✅
De uma função de 155 linhas para seis peças, separando cálculo de renderização:

| Peça | Papel |
|---|---|
| `minutosTrabalhados`, `saldoDoDia` | puras: nenhum DOM |
| `calcularTotais` | percorre o mês uma vez, devolve totais + resultado por dia |
| `atualizarLinhaPrincipal`, `renderOcorrencias`, `renderStatusBar` | só desenham |
| `updateAll` | 4 linhas: calcula e distribui |

`saldoDoDia` devolve `{ diff, contribui, trabalhado }` — a distinção que estava
implícita no emaranhado de `if`: dia neutro **mostra** 00:00 mas não **entra** no
total, e dia sem cálculo possível herda o saldo lido da ficha.

Constantes que estavam soltas viraram nomes: `CARGA_DIARIA` (480, estava
repetida em 4 lugares) e `CARGA_DISPENSA` (240).

**Validação:** reimplementei o `updateAll`/`classifyDay` antigos no navegador e
comparei contra o novo sobre os dois meses salvos reais. Totais idênticos
(oficial, real, faltas, ajustados, completos) e **zero divergências de
classificação** nos 30 + 31 dias.

### 3.4 Mapeamento snake_case ↔ camelCase ✅
Virou o par `deWire` / `paraWire`, num lugar só. Os três pontos que mapeavam
campo a campo (`loadFromAPI`, `loadMonthData`, `saveCurrentMonth`) agora chamam
essas funções. Round-trip verificado campo a campo contra a resposta real do
backend.

A armadilha do `||` em `ocorrencia_manual` (onde `false` é valor válido) agora
está documentada e resolvida em um ponto, não em três.

### 3.5 `styles.css` — ⏳ PENDENTE
1985 linhas, 45 seções por comentário, 9 `!important`. Continua sendo o item de
menor risco e menor retorno; não foi tocado. **Prettier** idem.

### 3.6 Achado da Fase 3: o auto-save apagava a identificação do servidor 🔴
Descoberto ao verificar o resultado do recorte em módulos.

`saveCurrentMonth` montava o payload como
`servidor: { nome: <texto raspado do DOM> }`. Só o nome, e mesmo assim lido de
volta da tela em vez do estado. Resultado: **toda gravação apagava do arquivo o
CPF, a matrícula, o horário contratual, a unidade e o órgão** — exatamente os
campos que o documento SEI preenche. Bastava editar um horário para perdê-los, e
não havia como recuperá-los sem reenviar o documento original.

O app tinha os dados o tempo todo em `activeServidor`; o payload é que os
descartava. Corrigido para gravar o servidor inteiro, e verificado: depois de uma
edição com auto-save, os seis campos sobrevivem no arquivo.

---

## Fase 4 — Fonte única de verdade ✅ (menos a troca final)

**Direção escolhida: backend canônico.** O backend já é capaz de ser canônico —
sabe tudo que o front-end sabe — e as duas implementações estão travadas uma
contra a outra por um teste compartilhado. Falta só o front-end passar a
consumir o `/api/process` em vez de calcular (ver 4.8).

### 4.0 O diagnóstico que mudou a ordem do trabalho 🔴

Antes de ligar o front-end ao `/api/process`, medi o que cada lado calculava
sobre os dois meses reais salvos em `data/`. As implementações **não** produziam
o mesmo número:

| Mês | Divergência no saldo real |
|---|---:|
| 07/2026 | **4.320 min = 72 horas** |
| 06/2026 | 233 min |

**Julho: 9 dias de "Férias - Estatutário".** O front-end os tratava como neutros;
o backend aplicava "dia útil sem batimento = falta" e descontava 8h de cada um.
Ligar o `/api/process` naquele estado teria feito o sistema afirmar que o servidor
devia 72 horas depois de férias homologadas.

A causa real: **nenhum dos dois conhecia "Férias"**. O front-end chegava ao
resultado benigno por acidente — não reconhecia o motivo, o dia ficava sem
batimento e caía em "folga".

**Junho: dias de dispensa.** O front-end exigia 4h (`CARGA_DISPENSA = 240`), o
backend exigia 8h. Dois números arbitrários, nenhum vindo de norma alguma.

### 4.1 Férias entraram no vocabulário ✅
`ObsFerias` / `DayTypeFerias` no motor Go e `'ferias'` no `domain.js`, com
cuidado para "FERIADO" e "FÉRIAS" não se confundirem — há teste dos dois lados.
Férias homologadas são ausência autorizada: dia neutro, nunca falta.

### 4.2 A jornada da dispensa passou a vir do ato ✅
Removidos os dois números arbitrários. Dispensa agora usa o mesmo mecanismo que o
expediente reduzido já tinha: **a jornada exigida é informada pelo usuário,
conforme o ato que concedeu o dia**. Sem ela, o dia é neutro e fica marcado para
conferência — em vez de o sistema arbitrar uma jornada e produzir um saldo
indefensável diante do documento.

No backend isso virou `cargaPorAto()`, cobrindo os dois tipos com a mesma regra.

**Interface:** o campo de minutos cru deu lugar a um seletor com atalhos —
*A conferir*, *Dia inteiro (8h)*, *Meio período (4h)*, *2 horas*, *Outra…* — com
tooltip explicando que o valor vem do ato. O seletor de tipo de dia também ganhou
tooltip por opção. Escolher *Outra…* abre um campo livre em minutos.

### 4.3 Resultado: as duas implementações convergiram ✅

Rodando o mesmo diagnóstico depois das correções, sobre os mesmos dados reais:

```
=== 06/2026 ===  soma das diferenças: 0
=== 07/2026 ===  soma das diferenças: 0
```

Zero divergência de saldo por dia e **zero divergência de classificação** em
61 dias. É o pré-requisito para a troca: agora ela não muda número nenhum.

### 4.4 O backend aprendeu o que só o front-end sabia ✅

1. **`day_type_override` mudou de casa** — saiu de `MonthDayRecord` e foi para
   `DayRecord`, onde sempre pertenceu: a escolha manual afeta a
   **classificação**, e portanto o cálculo. Enquanto ficou só no registro do
   mês, o `/api/process` não enxergava as escolhas do usuário e reclassificava
   por conta própria. `MonthDayRecord` ficou com o que de fato é só do documento
   SEI (ocorrência e justificativa manuais).

2. **Calendário real** — `ProcessRequest` ganhou `mes_ano`, e o `Process`
   passou a chamar `AlignObservacoes` como o resto do pipeline já fazia. O motor
   deixou de confiar no campo `w` lido do documento, que às vezes vem deslocado
   — e é ele que decide se um dia sem batimento é falta ou folga.

3. ~~**Definição de "Saldo Oficial"**~~ ✅ — ver 4.6.

### 4.7 O teste compartilhado, e os 4 defeitos que ele achou na primeira execução ✅

[`testdata/regras.json`](testdata/regras.json) tem 13 casos com os valores
esperados. **As duas implementações rodam os mesmos casos e conferem contra os
mesmos números**: [`pkg/service/regras_compartilhadas_test.go`](pkg/service/regras_compartilhadas_test.go)
e [`web/js/regras-compartilhadas.test.js`](web/js/regras-compartilhadas.test.js).
Mudar uma regra passa a exigir mudar o valor esperado uma vez — e os dois lados
junto.

Na primeira execução ele reprovou 4 casos. Dois eram defeitos reais do motor Go:

- **Dispensa e expediente reduzido com jornada informada davam saldo zero.** O
  motor só aplicava a jornada do ato quando o dia tinha os **quatro**
  batimentos. Nesses dias é normal haver só o turno da manhã — e aí a jornada do
  decreto era simplesmente ignorada. Corrigido com
  `CalculateTurnosTrabalhados`, que soma os turnos completos.
- **Dia marcado manualmente como feriado/folga ainda somava saldo.** Se havia
  ponto batido, o motor calculava a diferença mesmo assim. O usuário que marca
  "Feriado" está dizendo que o dia não conta; agora não conta.

Os outros 2 eram do próprio teste, que rodava contra o motor isolado — sem o
realinhamento de calendário, que acontece no serviço. Movido para `pkg/service`,
onde o pipeline é o mesmo que a aplicação usa.

**Nenhum desses quatro apareceu nos dois meses reais** — as combinações
necessárias não ocorriam neles. Só apareceram porque o fixture cobre as regras,
não os dados que por acaso existem.

### 4.6 O Saldo Oficial voltou a ser fiel à ficha ✅

Era a última divergência, e não era decisão de produto: era defeito. O motor
recalculava os dias em que o sistema gerou horário, contradizendo o rótulo que a
própria interface mostra — *"soma dos saldos originais da imagem"*.

Concretamente, o dia 1 de junho:

| | |
|---|---|
| Impresso na ficha | **−06:47** (faltou a entrada) |
| Depois do horário gerado (09:11) | 09:52 trabalhadas → +01:52 |
| O que entrava no Saldo Oficial | +01:52 ❌ |
| O que entra agora | −06:47 ✅ |

Somados os 8 dias ajustados, o mês de junho aparecia como **+04:32** em vez de
**−26:15**.

O número existe para ser o retrato do problema: *"a ficha me acusa de −26:15, a
matemática real diz +10:13, e a diferença é o que as justificativas explicam"*.
Recalculá-lo o aproximava do saldo real e o esvaziava de função.

A acumulação passou para o topo do laço, antes de qualquer `continue`, para que
todo dia entre — inclusive os neutros, que a ficha também mediu. O `switch` que
sobrou só conta dias.

**Resultado: convergência completa.** Sobre os dois meses reais, front-end e
backend produzem agora o mesmo saldo real, o mesmo saldo oficial, os mesmos
contadores e a mesma classificação em todos os 61 dias.

### 4.8 O que resta: o front-end consumir o `/api/process` ⏳

Tudo que essa troca precisava está pronto. O que sobra é decidir se ela vale, e
o balanço mudou durante a fase:

- **A favor:** apaga ~120 linhas de regra do JS. Uma implementação em vez de duas.
- **Contra:** `updateAll` vira assíncrono e cada edição de campo passa a
  depender da rede. No binário local é ~1ms; num deploy remoto, 100–300ms por
  tecla. E o app deixa de funcionar se o servidor não responder.
- **E o motivo original sumiu:** a Fase 4 nasceu porque as duas discordavam.
  Não discordam mais, e o teste compartilhado impede que voltem a discordar —
  que era o objetivo real.

Há também um detalhe de contrato: o `/api/process` devolve o tipo **calculado**
(`completo`, `dispensa`), mas o seletor da linha usa outro vocabulário
(`util`, `fds`, `dispensa`) com a detecção automática por trás. A troca exigiria
a API expor esse segundo vocabulário.

**Recomendação: parar aqui.** O teste compartilhado entrega a garantia que a
troca entregaria, sem latência e sem modo de falha offline. Retomar se e quando
aparecer um segundo cliente do cálculo — aí a duplicação passa a custar de novo.

### 4.5 De quebra: simplificar o `RulesAdjuster` ⏳ MAPEADO, NÃO REESCRITO

*(Esta seção aparecia duplicada, como "4.5" e como "4.1 De quebra". Unificada
aqui em 2026-08-11, com o que o levantamento apurou.)*

`adjustDay` é uma cascata de 12 combinações de pontos preenchidos mais um
fallback que loga *"combinação não prevista"*. `janelaSlot` e
`reinterpretarPontos`, no mesmo arquivo, já mostram o caminho declarativo.

**A frase "os testes de `extraction` protegem a reescrita" estava errada** — e
essa é a descoberta que reordenou o trabalho. O que foi medido em 2026-08-10:

**Os 13 caminhos são 4 decisões.** Um gerador por coluna, reescrito de 3 a 6
vezes, com constantes que divergiram entre as cópias por acidente:

| O que é inventado | Aparece em | Cópias idênticas | Divergência entre elas |
|---|---|---|---|
| saída da tarde | 6 ramos | 3 iguais caractere a caractere | piso de 3h, de 2h, ou nenhum |
| entrada da manhã | 6 ramos | 3 iguais | um estava sem a trava das 07:00 |
| retorno do almoço | 6 ramos | 3 iguais | só um é diferente de verdade |
| saída para o almoço | 8 ramos | 4 iguais | só um é diferente; a cópia do fallback é código morto |

Sobram **5 regras realmente distintas** em 13 caminhos.

**O fallback é alcançável**, e por batimentos banais: um único ponto entre 10:31
e 13:30, ou dois pontos entre 10:31 e 16:00 (uns 48 mil pares de horários). Roda
114 vezes na própria grade de testes. O log diz "combinação não prevista", o que
faz parecer defeito o que é rotina.

**A suíte executa todos os 13 ramos e não trava nenhum valor.** Onze deles só
são alcançados pela grade exaustiva (`TestHorariosGeradosSempreValidos`), que
confere três invariantes — nada passa das 23:59, ordem cronológica, nenhum
batimento real some — e nunca olha o número gerado.

> **Prova por mutação:** somar 45 minutos a toda saída inventada pelo ramo
> "falta a saída" deixa `go test ./...` inteiramente verde. São 2h15 a mais em
> junho, num documento assinado, sem nenhum portão reclamar.

**Consequência para a reescrita:** o primeiro passo não é reescrever, é **travar
os valores de hoje** — semente fixa no sorteio (`randBetween` usa o `math/rand`
global, então não há como reproduzir uma execução) e snapshot dos 13 ramos. Sem
isso a suíte fica verde tanto para o certo quanto para o errado.

**Dois defeitos dessa família já foram corrigidos** (v1.1.1), com teste que
falha antes e passa depois:

- entrada da manhã inventada às 05:56, por falta da trava das 07:00 num ramo;
- almoço gerado de 58 minutos, porque o desvio do minuto redondo comia o
  intervalo por cima.

O detalhe operacional dessas regras está no [MANUAL.md](MANUAL.md).

---

## Fase 5 — Ferramentas e portões ✅ CONCLUÍDA

*(Antecipada: veio antes da Fase 2 porque a Fase 1 introduziu código concorrente
cuja rede de proteção só funciona no CI.)*

### 5.1 CI ✅
[`.github/workflows/ci.yml`](.github/workflows/ci.yml), três jobs:

| Job | O que roda |
|---|---|
| **Go** | `gofmt -l`, `go build`, `go vet`, `go test -race ./...` |
| **Lint** | `golangci-lint` (conjunto padrão v2: errcheck, govet, ineffassign, staticcheck, unused) |
| **Front-end** | `npm ci` + `eslint web/` |

O `-race` é a razão principal deste CI existir: exige compilador C e não roda na
máquina de desenvolvimento, então sem ele `TestSettingsConcorrentesNaoDaoRace`
passaria mesmo com o data race de volta.

Todos os três foram executados localmente antes de entrar no arquivo. Nenhum é
um `.yml` escrito no escuro.

### 5.2 `.gitattributes` ✅
Não era preciso normalizar nada (ver correção na Fase 0.3), mas as regras
passam a valer independentemente do `core.autocrlf` de cada máquina: um clone
com `autocrlf=false` gravaria CRLF no repositório e sujaria o diff de todos.

### 5.3 Formatação ✅
4 arquivos estavam fora do `gofmt` (linha em branco sobrando no fim,
alinhamento de comentário). Corrigidos. `golangci-lint` apontou 2 problemas
reais, também corrigidos:

- `writeJSON` ignorava o erro de `json.Encoder.Encode` — falha na serialização
  mandava corpo truncado em silêncio. Agora ao menos registra.
- String de erro capitalizada (ST1005).

### 5.4 ESLint no front-end ✅
[`eslint.config.mjs`](eslint.config.mjs) com quatro regras, só defeito real:
`no-undef`, `no-unused-vars`, `no-redeclare`, `eqeqeq`. Sem opinião de estilo.

Achou 13 problemas no `app.js`, todos resolvidos:

- **`emptyClass` calculado e nunca usado** em `mkMainInput`. A intenção era
  renderizar campo original vazio como `readonly`/apagado, mas o valor nunca
  chegou ao HTML — o `if (!v)` acima devolve um input editável comum. **Removi
  só a variável morta, sem mexer no comportamento:** decidir se campo original
  vazio deve ser editável é questão de produto, e fica para a Fase 3.
- `origUpdateAll` e `_origRenderTables`: restos de uma tentativa de
  monkey-patching abandonada — o próprio comentário dizia "we'll check via a
  simpler mechanism" e o código seguia por `setInterval`.
- `const [mm, yyyy]` desestruturado e nunca usado em `loadMonthData`.
- Dois `catch (e)` com parâmetro não usado → `catch { }`.
- `copyBtn` e `copyRow` pareciam mortas por só serem chamadas de `onclick`
  embutido. Passaram para `window.`, seguindo a convenção que o próprio arquivo
  já usa (`window.copyCell`) — o que de quebra eliminou um `const copyBtn` local
  que sombreava a função global.
- `updateSeiPreview()`/`closeSeiModal()` eram atribuídas a `window.` e chamadas
  sem prefixo. Chamadas explicitadas.

### 5.5 O que ficou de fora, de propósito
- **Prettier.** Reformatar 4.500 linhas de JS/CSS logo antes de a Fase 3
  reescrevê-las é churn puro que ainda enterraria o diff real. Entra na Fase 3.
- **Testes de front-end.** `node --test` sobre `domain.js` só faz sentido depois
  que a Fase 3 criar o `domain.js`.

---

## Ordem sugerida

| Fase | Escopo | Risco | Impacto |
|---|---|---|---|
| ~~0~~ | ~~Código morto + testes de caracterização~~ | — | ✅ concluída |
| ~~1~~ | ~~Defeitos backend (race, body leak, ctx, embed, DI, ordenação)~~ | — | ✅ concluída |
| ~~5~~ | ~~CI e lint~~ (antecipada) | — | ✅ concluída |
| ~~2~~ | ~~Camadas, erros, logging~~ | — | ✅ concluída |
| ~~3~~ | ~~Modularizar front-end~~ | — | ✅ (falta só 3.5, o CSS) |
| ~~4~~ | ~~Fonte única de verdade~~ | — | ✅ (troca final: ver 4.8) |

**Sobrou do plano inteiro:** o CSS (3.5), o Prettier, a reescrita do
`RulesAdjuster` (4.5) e a troca da 4.8 — sobre a qual a recomendação agora é
*não fazer*, porque o teste compartilhado já entrega a garantia que ela
entregaria.

A lição da Fase 4 vale para o resto: *unificar duas implementações começa por
medir onde elas discordam, não por escolher uma*. Se eu tivesse ligado o
`/api/process` primeiro, o sistema passaria a acusar 72 horas de débito
inexistente — e o erro só apareceria quando alguém conferisse a folha assinada.

A lição da Fase 4 vale para o resto: *unificar duas implementações começa por
medir onde elas discordam, não por escolher uma*. Se eu tivesse ligado o
`/api/process` primeiro, o sistema passaria a acusar 72 horas de débito
inexistente — e o erro só apareceria quando alguém conferisse a folha assinada.

Estado atual: **180 testes Go + 45 de front-end** (eram 25 e 0 no início);
`gofmt`/`vet`/`golangci-lint`/`eslint` sem apontamentos; convergência **total**
entre front-end e backend verificada sobre os dois meses reais — saldo real,
saldo oficial, contadores e classificação dia a dia — e travada por 13 casos
compartilhados que os dois lados são obrigados a reproduzir.

Fase 5 vem antes da 4 de propósito: a Fase 4 é a mais arriscada e é a que mais
se beneficia de ter os portões automáticos já rodando.

## Se der para fazer só três coisas

1. **Fase 1.1 e 1.2** — o data race em `Handler.settings` e o vazamento de corpo
   de resposta são defeitos reais, e a correção é pequena.
2. **Fase 1.4** — backend calculando diferente entre local e Vercel é uma
   armadilha silenciosa; `go:embed` resolve numa tarde.
3. **Fase 3.1** — os dois bugs de front-end (escape e render com efeito colateral)
   afetam o usuário diretamente, hoje.
