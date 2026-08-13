# Manual do Ponto Real Go

Como o programa funciona por dentro e como operá-lo com segurança.

O [README](README.md) apresenta o projeto e lista funcionalidades. Este manual
explica **as regras**: o que o sistema decide sozinho, o que ele nunca decide, e
onde a conferência humana é obrigatória.

Toda regra descrita aqui foi conferida no código. Quando o texto cita um
arquivo, é lá que a regra mora.

---

## Índice

1. [O que o programa faz — e o que não faz](#1-o-que-o-programa-faz--e-o-que-não-faz)
2. [Instalação e configuração](#2-instalação-e-configuração)
3. [O fluxo completo](#3-o-fluxo-completo)
4. [O que o sistema inventa e o que ele nunca toca](#4-o-que-o-sistema-inventa-e-o-que-ele-nunca-toca)
5. [Tipos de dia](#5-tipos-de-dia)
6. [Os dois saldos](#6-os-dois-saldos)
7. [Jornada exigida: quando você precisa informar](#7-jornada-exigida-quando-você-precisa-informar)
8. [Os avisos ⚠️](#8-os-avisos-)
9. [O documento SEI](#9-o-documento-sei)
10. [A biblioteca de frases](#10-a-biblioteca-de-frases)
11. [Corrigindo o que o sistema errou](#11-corrigindo-o-que-o-sistema-errou)
12. [Armadilhas do uso diário](#12-armadilhas-do-uso-diário)
13. [Configuração das regras](#13-configuração-das-regras)
14. [Referência da API](#14-referência-da-api)
15. [Problemas comuns](#15-problemas-comuns)

---

## 1. O que o programa faz — e o que não faz

**Faz:** lê a folha de frequência do SFR (imagem ou PDF) com um modelo de IA,
transcreve os batimentos para uma tabela editável, completa os horários que
faltam, calcula o saldo do mês e monta o texto de ocorrências e justificativas
pronto para colar no SEI.

**Não faz:** não decide se você trabalhou. Não sabe o que aconteceu no dia que
você esqueceu de bater o ponto. Não conhece o ato que concedeu a sua dispensa.

Essa fronteira é o ponto mais importante do manual. O programa **inventa
horários plausíveis** para preencher lacunas, e esses horários vão para um
documento que você assina como servidor público. A responsabilidade pelo que
está escrito é sua, não do programa — e ele é construído para deixar sempre
visível o que foi lido e o que foi inventado.

---

## 2. Instalação e configuração

### Requisitos

- Go 1.25 ou mais novo
- Uma chave de API do **Google Gemini** ou do **OpenRouter**

### Rodar

```bash
go build ./...
go run .
```

O servidor sobe em `http://localhost:8080`. Para trocar a porta, defina a
variável de ambiente `PORT`.

O front-end inteiro (`web/`) é embutido no binário por `go:embed`.
**Consequência prática: mudança em `web/**` só aparece depois de reiniciar o
servidor.** Recarregar a página não basta.

### A chave de API

Configure pela engrenagem (⚙️) na interface. A chave é guardada localmente e
nunca vai para o repositório.

Sem chave, o upload é recusado com uma mensagem explícita — o sistema não tenta
processar e falhar depois.

### Formatos aceitos

PNG, JPEG, WebP e PDF. Qualquer outro tipo é recusado na entrada.

Quando o navegador não reconhece o arquivo e manda `application/octet-stream`, o
tipo é inferido pelo nome e pelos primeiros bytes do arquivo.

> `pkg/service/upload.go` — `tiposAceitos`, `normalizar()`

---

## 3. O fluxo completo

```
   ┌─────────────┐
   │ 1. Upload   │  PDF ou imagem da folha do SFR
   └──────┬──────┘
          ▼
   ┌─────────────┐
   │ 2. IA lê    │  Gemini ou OpenRouter transcrevem a tabela
   └──────┬──────┘
          ▼
   ┌─────────────┐
   │ 3. Alinha   │  Corrige o dia da semana pelo calendário real
   └──────┬──────┘
          ▼
   ┌─────────────┐
   │ 4. Ajusta   │  Reinterpreta colunas e inventa o que falta
   └──────┬──────┘
          ▼
   ┌─────────────┐
   │ 5. Calcula  │  Classifica cada dia e apura os dois saldos
   └──────┬──────┘
          ▼
   ┌─────────────┐
   │ 6. VOCÊ     │  ⚠️ conferência humana — insubstituível
   └──────┬──────┘
          ▼
   ┌─────────────┐
   │ 7. Gera SEI │  Tabela de ocorrências + justificativas
   └─────────────┘
```

### Passo 3 — o realinhamento

A extração às vezes lê a coluna de observações deslocada em relação aos dias. O
sistema detecta isso usando os marcadores `SÁBADO` e `DOMINGO` como âncoras de
calendário.

- Se **todo** o deslocamento for igual (ex.: tudo 2 dias acima), ele corrige.
- Se o deslocamento for **irregular**, ele **não move nada** e sinaliza os dias
  em conflito para você conferir. Mover por adivinhação seria pior que não
  mover.

O reconhecimento de `SÁBADO`/`DOMINGO` como âncora exige correspondência
**exata**. A extração às vezes funde duas observações numa célula só — por
exemplo `DISPENSA PARA FREQUÊNCIA A CURSO DE DOUTORADO, MESTRADO, SÁBADO`. Se
isso valesse como âncora, apontaria um sábado numa sexta e acusaria
desalinhamento onde não há.

> `pkg/extraction/align.go`, `pkg/rules/observacoes.go` — `IsFimDeSemanaObs`

---

## 4. O que o sistema inventa e o que ele nunca toca

### A regra de ouro

> **Batimento que veio da folha nunca é alterado, apagado ou movido de valor.**

O que o sistema pode fazer com um batimento real é **reinterpretar a qual
coluna ele pertence** — nunca mudar o horário em si.

### O array `Bloqueio` (`"o"` no JSON)

Cada dia carrega quatro marcas, uma por coluna, na ordem
**[Entrada 1, Saída 1, Entrada 2, Saída 2]**:

| Valor | Significado | Na tela |
|---|---|---|
| `1` | veio da folha — **original** | bloqueado |
| `0` | foi **gerado** pelo sistema | editável |

É esse array que o documento SEI usa para saber quais dias têm horário
inventado e portanto precisam de justificativa.

### Quando o sistema NÃO gera nada

| Situação | Por quê |
|---|---|
| Sábado, domingo, feriado, recesso | não há jornada a cumprir |
| Dia com os **4 batimentos** | não falta nada |
| Dia com **nenhum** batimento | não há âncora para inventar em cima |
| **Expediente reduzido** | o ponto batido já basta; a jornada exigida é menor |
| **Dispensa** | idem — a jornada vem do ato, não da regra geral |

Nos dois últimos casos o sistema ainda marca o `Bloqueio` (para a tela saber o
que é editável), mas **não inventa horário nenhum**.

### Reinterpretar a coluna

A ficha tem quatro colunas fixas. Quando um batimento falta, os demais escorregam
de posição, e ler a coluna literalmente produz absurdo.

Exemplo real, dia 01/06/2026 — a folha trouxe `11:30`, `12:33`, `20:06`:

- **Leitura literal:** entrada às 11:30, saída para o almoço às 12:33, retorno
  às 20:06. Um almoço de sete horas e meia.
- **Leitura correta:** faltou a *entrada da manhã*. Os três são saída para o
  almoço, retorno e saída do expediente.

O sistema resolve isso pontuando plausibilidade: testa todas as formas de
encaixar os horários (em ordem cronológica) nas quatro colunas e fica com a que
menos destoa das janelas típicas de cada batimento.

| Coluna | Janela plausível |
|---|---|
| Entrada da manhã | 06:00 – 10:30 |
| Saída para o almoço | 10:30 – 13:30 |
| Retorno do almoço | 11:30 – 16:00 |
| Saída do expediente | 16:00 – 23:59 |

Empate preserva a posição original. Dia com os quatro batimentos nunca é mexido.

> `pkg/extraction/rules_adjuster.go` — `janelaSlot`, `reinterpretarPontos`

### Como o horário inventado é escolhido

Depois de decidir a que coluna cada horário pertence, o sistema gera o que
falta. As garantias que ele respeita:

| Garantia | Valor |
|---|---|
| Entrada da manhã gerada nunca é de madrugada | **≥ 07:00** |
| Almoço gerado respeita o mínimo legal | **≥ 60 min** |
| Almoço gerado é sorteado na faixa | 60 – 75 min |
| Tarde gerada nunca é curta demais | **≥ 3h** |
| O dia fecha na jornada exigida | 8h, com variação de alguns minutos — **salvo quando o piso da tarde segura** |
| Nenhum horário passa da meia-noite | ≤ 23:59 |
| Horários não caem sempre em minuto redondo | `:00 :15 :30 :45` são evitados |

**Sobre o minuto redondo:** é uma preferência, não uma lei. Horário inventado
que cai sempre em `:00` ou `:30` denuncia que foi inventado. Mas quando fugir do
redondo quebraria outra regra — encurtar o almoço abaixo do mínimo, por
exemplo —, o sistema aceita o redondo. Na prática ele aparece em cerca de **0,3%**
dos horários gerados.

**Sobre o piso da tarde e o que ele custa:** a tarde gerada é, em princípio, o
que falta para fechar a jornada depois da manhã. Quando a manhã batida foi muito
longa, isso daria uma tarde de poucos minutos — servidor que volta do almoço e
sai em seguida —, e em casos extremos daria uma tarde negativa, com a saída antes
do retorno. Por isso ela nunca sai com menos de 3 horas.

O preço é este, e vale saber: **quando o piso segura, o dia gerado fecha acima da
jornada**, e a diferença é crédito que o sistema acrescentou. Acontece nos dias de
manhã longa — cerca de 13% dos casos em que só falta a saída da tarde. **Confira o
saldo desses dias**: o horário é plausível, mas o crédito não veio da folha.

**Quando o horário estourar a meia-noite:** se um batimento real muito tarde
fizer a geração passar das 23:59, o valor é limitado ao fim do dia e a linha
pede conferência — em vez de gravar algo impossível como "27:12".

### Um limite conhecido

Quando entrada e retorno do almoço são **os dois batimentos reais** e estão
colados (menos de ~75 minutos entre eles), não existe almoço legal que caiba
ali. Nesse caso o sistema põe a saída para o almoço no meio dos dois, e o
intervalo sai abaixo de 60 minutos.

Não é defeito: é a folha dizendo algo que nenhuma regra conserta. **Confira o
dia.**

---

## 5. Tipos de dia

### Como o tipo é decidido

A ordem importa — a primeira regra que responder, vence:

```
1. Você escolheu o tipo no seletor da linha?   → vale a sua escolha
2. A observação da folha diz alguma coisa?     → vale o que ela diz
3. É sábado ou domingo?                        → folga
4. Senão: conta os batimentos
```

### 1. Sua escolha manual

| Escolha no seletor | Vira |
|---|---|
| FDS / Folga | folga |
| Feriado | feriado |
| Convocação | recesso |
| Dispensa | dispensa |
| Reduzido | expediente reduzido |
| Férias | férias |
| **Útil** | *nenhum* — derruba a detecção e volta à contagem de batimentos |

**"Útil" é como você desfaz uma detecção equivocada.** Se a folha diz
`DISPENSA` numa linha em que você trabalhou normal, marque "Útil" e o dia volta
a ser tratado pelos batimentos.

### 2. O vocabulário da observação

O texto é comparado em caixa alta e sem acento, então `COMPENSAÇÃO` e
`COMPENSACAO` dão no mesmo. Basta **conter** o trecho. A ordem abaixo é a ordem
de precedência:

| Trecho reconhecido | Vira |
|---|---|
| `EXPEDIENTE REDUZIDO`, `HORARIO REDUZIDO`, `JORNADA REDUZIDA` | expediente reduzido |
| `FERIAS` | férias |
| `DISPENSA` | dispensa |
| `RECESSO` | recesso |
| `PONTO FACULTATIVO` | feriado |
| `FERIADO` | feriado |
| `COMPENSACAO` | **nada** — puramente informativo |
| `SABADO`, `DOMINGO` | fim de semana |

> `FERIAS` está no topo por um motivo caro: enquanto faltava, o backend aplicava
> "dia útil sem batimento = falta" e descontava a jornada inteira. Num mês com 9
> dias de férias, **72 horas de débito inexistente**.

### 3. Contagem de batimentos

| Batimentos | Tipo |
|---|---|
| 4 | completo |
| 1 a 3 | parcial |
| 0, em dia útil | **falta** |
| 0, em fim de semana | folga |

### A tabela completa

| Tipo | Gera horário? | Entra no saldo real | Precisa da jornada informada? |
|---|---|---|---|
| completo | não (já tem 4) | sim | não |
| parcial | **sim** | sim | não |
| falta | não | sim, **negativo** (−8h) | não |
| dispensa | **não** | sim, se informada | **sim** |
| expediente reduzido | **não** | sim, se informada | **sim** |
| férias | não | neutro | não |
| feriado | não | neutro | não |
| folga | não | neutro | não |
| recesso | não | neutro | não |

> `pkg/rules/engine.go` — `ClassifyDay`; `pkg/rules/observacoes.go` — `obsPatterns`

---

## 6. Os dois saldos

A tela mostra dois totais. Eles medem coisas diferentes e **é proposital que
não batam**.

### Saldo Oficial

A **soma fiel do que a ficha imprimiu**. Nunca é recalculado.

Todo dia entra, inclusive os neutros. É o retrato do problema: *"a ficha acusa
isto"*.

> Este número já foi recalculado no passado, e o resultado mostrou por que não
> deve ser: o dia 01/06, impresso como `-06:47` porque faltou a entrada, virava
> `+01:52` depois do horário gerado. Somados os dias ajustados, o mês passava de
> `-26:15` para `+04:32` — e o rótulo da tela, "soma dos saldos originais da
> imagem", virava mentira.

### Saldo Real

A **matemática dos batimentos**, depois de tudo ajustado. É o que você
efetivamente cumpriu.

Como cada dia entra:

| Situação | Saldo real do dia |
|---|---|
| Você marcou feriado/folga/FDS/convocação | `00:00` — neutro por decisão sua |
| Dispensa ou reduzido **com jornada informada** | turnos completos − jornada do ato |
| Dia com os 4 batimentos | (manhã + tarde) − jornada |
| Falta | **− jornada inteira** |
| Feriado, folga, recesso, férias, parcial não ajustado | herda o saldo da ficha |

Em dispensa e expediente reduzido, o sistema soma os **turnos completos** em vez
de exigir os quatro batimentos: nesses dias é normal existir só o da manhã.
Exigir os quatro fazia a jornada do ato ser ignorada e o dia entrar como zero.

### A diferença entre os dois

É exatamente **o que as justificativas precisam explicar**. Se o Oficial diz
−26:15 e o Real diz −00:20, a diferença é o que aconteceu por falha de
registro, não por falta de trabalho — e é isso que o documento SEI narra.

> `pkg/rules/engine.go` — `CalculateSummary`

---

## 7. Jornada exigida: quando você precisa informar

Na maioria dos dias a jornada é a global: **8 horas**.

Em **dispensa** e **expediente reduzido**, não. Cada ato define a sua — o
decreto pode reduzir para 4h, a portaria de dispensa pode liberar o dia inteiro,
meio período, ou a partir de determinado horário.

**O sistema não arbitra esse número.** Enquanto ele não for informado:

- o dia entra com saldo `00:00` (neutro, não penaliza);
- a linha mostra ⚠️ pedindo a informação;
- a frase automática de justificativa **não é escrita**.

Assim que você informa a jornada na linha, o ⚠️ some, o saldo é apurado contra
ela, e a frase automática passa a sair.

> Arbitrar uma jornada produziria um saldo que ninguém consegue justificar
> diante do ato. Melhor pedir.

---

## 8. Os avisos ⚠️

| Aviso | O que significa | O que fazer |
|---|---|---|
| *"Dispensa: informe a jornada exigida neste dia, conforme o ato que a concedeu."* | dia de dispensa sem jornada informada | preencha a carga na linha |
| *"Expediente reduzido: confira a carga horária do dia definida pelo decreto."* | idem, para decreto | preencha a carga na linha |
| *"Nenhum horário foi lido para este dia. Confira a folha original: os batimentos podem ter se perdido na extração."* | dia justificado que veio **sem batimento algum** | **abra a folha original** |
| *"Horário gerado ultrapassaria a meia-noite; confira os pontos originais deste dia."* | um batimento real muito tarde estourou o dia | confira se o horário da folha está certo |
| *"Nenhum turno fecha: os batimentos deste dia estão em colunas que não formam par de entrada e saída."* | dia de jornada por ato, com a jornada já informada, em que nenhuma entrada tem a sua saída | veja o botão de proposta na mesma linha |

**Sobre o último:** ele só é possível depois de a jornada do ato ser informada, e
é aí que está a razão de existir. Num dia de dispensa de 4h, um batimento às
13:45 é o fim do expediente; num dia comum de 8h, é a volta do almoço. A leitura
automática não tem como saber qual dos dois é, porque a jornada do ato só é
informada por você, depois — então ela às vezes põe o horário na coluna errada,
nenhum turno fecha, e o dia acusa a jornada inteira como débito.

Quando isso acontece e há **dois** batimentos, aparece ao lado do seletor de tipo
um botão como **"Ler como 09:26 → 13:45"**. Ele move os dois para entrada e saída
do expediente — é o que você faria arrastando. **Nenhum horário é alterado**, só a
coluna em que cada um é lido, e nada acontece sem você clicar: o batimento é seu.

Com um batimento só o aviso aparece sem botão, porque não há par a formar e
adivinhar o horário que falta seria inventar ponto.

A correção não é guardada, e não precisa ser: se você reprocessar o mês, a
leitura volta a errar a coluna, o aviso volta a aparecer e o botão volta a estar
lá.

O terceiro é o mais grave e por isso tem prioridade sobre os outros. Na ficha do
SFR, dias de dispensa quase sempre têm horário; vir sem nenhum costuma
significar que a **leitura perdeu batimentos reais**. Perder ponto real em
silêncio é pior que uma carga por conferir.

---

## 9. O documento SEI

O botão "Gerar SEI" monta duas seções: a **tabela de ocorrências** e as
**justificativas**.

### Quem entra no documento

**Um critério só**, e é exatamente o que o checkbox da linha mostra:

```
Você marcou o checkbox manualmente?  → vale a sua marcação
Senão: o dia tem algum horário gerado?  → entra
```

Marcou, o dia sai **nas duas seções**, sem exceção — nem para dispensa, nem
para dia sem horário nenhum.

> Antes existiam dois predicados para essa mesma pergunta, e eles discordavam: o
> checkbox aparecia marcado e o dia não saía em lugar nenhum. Pior, não havia
> como forçar — desmarcar e remarcar devolvia o dia ao estado "automático", que
> era justamente o que a tabela ignorava.

### A frase automática

Em dias de dispensa e expediente reduzido, o sistema escreve sozinho algo como:

> *Dia de dispensa: cumpri integralmente a jornada de 4h exigida pelo ato, com
> expediente das 09:26 às 13:45.*

**Ela só sai quando o cálculo a sustenta.** Fica calada quando:

- a jornada do ato **não foi informada** — não há contra o que comparar;
- a jornada **não foi cumprida** — o sistema não inventa desculpa para um
  déficit.

No segundo caso o campo fica para você escrever o que de fato aconteceu.

### Por que o campo está vazio

Campo em branco é indistinguível de defeito. Por isso, quando a frase automática
se cala, o próprio campo explica o motivo:

| Situação | O que o campo diz |
|---|---|
| jornada não informada | *"Informe a jornada da dispensa nesta linha e o sistema escreve a justificativa sozinho."* |
| jornada não cumprida | *"A jornada exigida não fecha neste dia — descreva o que aconteceu."* |

### O que você escreve vence

Texto seu sempre substitui a frase automática. **Apagar o texto devolve a frase
automática** — não é jeito de deixar o dia mudo no documento.

> `web/js/domain.js` — `isOccurrenceDay`; `web/js/justificativa.js` —
> `justificativaDeAto`, `motivoDoSilencio`

---

## 10. A biblioteca de frases

Frases que você escreve podem ser guardadas e reaproveitadas em qualquer mês.

### A frase é guardada sem a data

`10/07/2026 - ` é reposto na hora de exibir. Uma frase com a data grudada não
serve para agosto — que é justamente o motivo de existir uma biblioteca.

### As lacunas

Uma frase guardada pode citar números que mudam de dia para dia. *"Cumpri a
jornada de 4h exigida pelo ato"* fica errada no dia em que o ato exigiu 2h. As
lacunas resolvem:

| Marcador | Vira |
|---|---|
| `{jornada}` | a jornada exigida no dia (ex.: `4h`) |
| `{trabalhado}` | o total efetivamente trabalhado (ex.: `4h19`) |
| `{data}` | a data do dia (ex.: `03/07/2026`) |

Marcador sem valor correspondente é **apagado** junto com o espaço antes dele —
para a frase não sair com um `{jornada}` cru dentro de um documento assinado.

A duração é formatada como se fala (`4h`, `4h19`), não como se lê num relógio de
ponto. A frase é lida por uma pessoa.

### A ordem da lista

A biblioteca **não filtra nada** — nenhuma regra impede usar num dia útil o que
você escreveu numa dispensa. O que muda é a ordem: as frases do mesmo tipo de
dia sobem, e entre elas vence a mais usada.

A sugestão em cinza no campo vazio é mais restrita: só propõe o que já foi
escrito **naquele mesmo tipo de dia**.

### Onde fica

O servidor é a verdade; o `localStorage` é cache de leitura e fila de escrita.
O arquivo `justificativas.json` fica **ao lado do executável, fora de `data/`** —
porque o listador de meses trata todo `.json` daquela pasta como um mês salvo.

---

## 11. Corrigindo o que o sistema errou

### Editar um horário

Campos com fundo editável (`Bloqueio == 0`) são gerados e podem ser alterados
livremente. Campos bloqueados vieram da folha.

### Apagar um horário e deixá-lo em branco

Apagar um campo gerado não bastava: o auto-preencher o repreenchia assim que ele
perdia o foco. O sistema agora registra os campos que você apagou **de
propósito** e os deixa em paz.

### Corrigir o tipo do dia

Use o seletor da linha. Sua escolha vence a detecção automática, sempre.

Para desfazer uma detecção equivocada e voltar ao comportamento normal, escolha
**"Útil"**.

### Forçar um dia para dentro ou para fora do SEI

Use o checkbox da linha. Ele é o critério único — o que ele mostra é o que sai.

### Um batimento que caiu na coluna errada

Acontece principalmente em **dia de dispensa**, porque nesses dias o sistema não
reinterpreta colunas (ele sai antes, já que não vai gerar nada). E mesmo que
reinterpretasse, as janelas de plausibilidade são calibradas para a jornada de
8h — num dia de dispensa de 4h, um batimento às 13:45 é o fim do expediente, mas
cai dentro da janela de "retorno do almoço".

**Arraste o horário para a coluna certa.** ⚠️ Essa correção **não sobrevive ao
reprocessamento do mês**: se você subir a folha de novo, ela se perde.

---

## 12. Armadilhas do uso diário

### O auto-save grava nos seus dados reais

**Qualquer interação de edição — mudar um campo, trocar o tipo do dia, mexer na
jornada — grava no arquivo do mês em `data/`.**

Carregar um mês é seguro. **Editar não é.**

Para testar sem risco: copie `data/` para uma pasta temporária, faça o teste, e
compare com `diff` no final.

Para conferir o que foi gravado:

```bash
python -c "
import json; d=json.load(open('data/06_2026.json',encoding='utf-8'))
print([(x['d'],k) for x in d['dias'] for k in ('carga','day_type_override','ocorrencia_manual','justificativa_manual','limpos') if k in x])"
```

### Reprocessar o mês perde as correções manuais

Subir a folha de novo refaz a extração do zero. Correções de coluna se perdem.

### Os horários gerados mudam a cada extração

O preenchimento é sorteado. Reprocessar o mesmo mês produz horários diferentes
(todos plausíveis, todos fechando a jornada). Os horários já **salvos** não
mudam sozinhos — só se você reprocessar.

### Mudança em `web/` exige reiniciar o servidor

O front-end é embutido no binário por `go:embed`.

### No Windows, `gofmt -l` lista todos os arquivos

É o CRLF da cópia de trabalho, não erro de formatação. A checagem que vale roda
no CI, em Linux. O repositório guarda tudo em LF; a cópia local em CRLF; isso
está certo e não é problema a consertar.

### `-race` não roda em máquina sem compilador C

Exige `CGO_ENABLED=1`. O portão real é o CI.

---

## 13. Configuração das regras

`pkg/rules/rules.json` — **embutido no binário** por `go:embed`.

```json
{
  "carga_horaria_diaria_min": 480,
  "almoco_minimo_min": 60,
  "almoco_gerado_min_min": 60,
  "almoco_gerado_max_min": 75,
  "nome_instituicao": "UNIVERSIDADE ESTADUAL DE GOIÁS - UEG"
}
```

| Campo | O que faz |
|---|---|
| `carga_horaria_diaria_min` | jornada diária padrão, em minutos (480 = 8h) |
| `almoco_minimo_min` | mínimo legal do intervalo (60 = 1h) |
| `almoco_gerado_min_min` | piso do almoço sorteado |
| `almoco_gerado_max_min` | teto do almoço sorteado |
| `nome_instituicao` | cabeçalho do documento SEI |

`almoco_gerado_min_min` **nunca pode ser menor** que `almoco_minimo_min`: um
almoço gerado curto demais produziria um dia que a própria validação do sistema
recusa. Há teste travando isso.

> Embutir em vez de ler do disco eliminou duas armadilhas: o servidor local
> procurava o arquivo por caminho relativo (e caía num padrão diferente se
> chamado de outro diretório), enquanto o deploy usava regras fixas em Go. **O
> mesmo cálculo produzia resultados diferentes conforme onde rodava.**

### Mudar uma regra

`testdata/regras.json` tem casos que o motor Go e o `web/js/domain.js` são
**obrigados** a reproduzir com os mesmos números.

**Mudar uma regra exige mudar o valor esperado ali, uma vez, e os dois lados
junto.** Foi esse teste compartilhado que fechou uma divergência de 72 horas
entre as duas implementações — e é o que impede que ela volte.

---

## 14. Referência da API

| Método | Rota | O que faz |
|---|---|---|
| `GET` | `/api/health` | verificação de vida |
| `GET` | `/api/rules` | devolve as regras em vigor |
| `GET` | `/api/models` | modelos de IA disponíveis por provedor |
| `GET` | `/api/settings` | configurações salvas |
| `POST` | `/api/settings` | grava provedor e chaves |
| `POST` | `/api/upload` | envia a folha e extrai (multipart) |
| `POST` | `/api/process` | recalcula dias já extraídos |
| `POST` | `/api/validate` | valida um dia isolado |
| `GET` | `/api/months` | lista os meses salvos |
| `GET` | `/api/month/{mesAno}` | carrega um mês |
| `POST` | `/api/month/{mesAno}` | salva um mês |
| `GET` | `/api/justificativas` | biblioteca de frases |
| `POST` | `/api/justificativas` | grava a biblioteca inteira |

A biblioteca é sempre gravada **por inteiro**, nunca item a item. Isso torna a
exclusão trivial (a frase simplesmente não está na lista que chegou) e dispensa
identificador por frase — **o estado é a lista**.

### O que `/api/validate` confere

Só age em dias com os **quatro** batimentos preenchidos. Verifica:

- ordem cronológica das quatro marcações;
- almoço maior ou igual ao mínimo legal;
- total trabalhado maior ou igual à jornada do dia.

---

## 15. Problemas comuns

**A IA leu um horário errado.**
Corrija na tela. Se o erro for sistemático, tente outro modelo nas Configurações.

**Um dia de férias entrou como falta.**
Confira se a observação da folha contém `FERIAS`. Se não contiver, marque
"Férias" no seletor da linha.

**O saldo do mês não bate com a ficha.**
Não deve bater mesmo — veja [Os dois saldos](#6-os-dois-saldos). O Oficial copia
a ficha; o Real refaz a matemática. A diferença é o que as justificativas
explicam.

**A frase automática não aparece num dia de dispensa.**
Leia o texto cinza no próprio campo: ou falta informar a jornada do ato, ou a
jornada não fecha naquele dia.

**Um dia de dispensa está com −04:00 em vez do saldo certo.**
Provavelmente um batimento caiu na coluna errada. Veja
[Corrigindo o que o sistema errou](#11-corrigindo-o-que-o-sistema-errou).

**Mudei o CSS e nada mudou.**
Reinicie o servidor: `web/` é embutido no binário.

**O CI acusa erro de lint e localmente passa.**
Confira se as versões batem. O CI fixa `golangci-lint v2.12.2`; rode a mesma
aqui.

---

## Portões de qualidade

Antes de dar qualquer mudança por concluída:

```bash
go build ./... && go vet ./... && go test ./...
golangci-lint run ./...
npm run lint && npm test
```

O CI ainda roda `go test -race ./...`, que exige compilador C e por isso não
roda em toda máquina de desenvolvimento. **É o portão que só o CI dá.**

---

## Onde continuar

- [README.md](README.md) — apresentação e changelog
- [RETOMAR.md](RETOMAR.md) — estado atual e o que falta fazer
- [REFATORACAO.md](REFATORACAO.md) — histórico detalhado da refatoração
