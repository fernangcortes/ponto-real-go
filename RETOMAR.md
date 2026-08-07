# Onde paramos

Ponto de partida para uma nova sessão. O detalhe de tudo que foi feito está em
[REFATORACAO.md](REFATORACAO.md); aqui fica só o necessário para retomar.

## Estado

Branch **`refatoracao`**, 7 commits à frente da `master`, árvore limpa.
**Nada foi mergeado ainda.**

```
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
go build ./... && go vet ./... && go test ./...   # 180 testes
golangci-lint run ./...                            # 0 issues
npm run lint && npm test                           # 52 testes
```

## O que o usuário precisa fazer antes de qualquer coisa nova

1. **Conferir os dois meses no app.** Muita regra de cálculo mudou — férias,
   dispensa, saldo oficial. Ele é quem sabe se os números batem com a realidade.
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

### 2. Nada mais é urgente
- `web/styles.css`: 1985 linhas, 9 `!important` (item 3.5). Baixo retorno.
- Prettier no front-end. Baixo retorno.
- A troca da 4.8 (front-end consumir `/api/process`): **recomendação é NÃO
  fazer** — o teste compartilhado já entrega a garantia que ela entregaria, sem
  latência de rede a cada tecla nem falha quando o servidor não responde. O
  raciocínio completo está na 4.8 do plano.

## Armadilhas que custaram tempo nesta sessão

**Testar no navegador escreve em `data/`.** O app tem auto-save; qualquer
interação de teste (editar campo, trocar tipo de dia, mexer na jornada) grava no
arquivo do mês. Ao longo desta sessão isso corrompeu `data/06_2026.json` mais de
uma vez. Confira e limpe depois de testar:

```bash
python -c "
import json; d=json.load(open('data/06_2026.json',encoding='utf-8'))
print([(x['d'],k) for x in d['dias'] for k in ('carga','day_type_override','ocorrencia_manual','justificativa_manual','limpos') if k in x])"
```

**`gofmt -l` no Windows lista todos os arquivos.** É o CRLF da cópia de
trabalho, não erro de formatação. Para checar de verdade, converta para LF numa
pasta temporária e rode `gofmt -l` lá — ou confie no CI, que roda em Linux.
(Detalhe: só `index.html` e `styles.css` estavam em CRLF no repositório; os `.go`
sempre estiveram em LF. Eu errei esse diagnóstico duas vezes por amostrar um
arquivo e generalizar.)

**`-race` não roda aqui.** Exige `CGO_ENABLED=1` e um compilador C que não está
instalado. O teste de concorrência das configurações passa mesmo com o bug de
volta — o portão real é o CI.

**O `web/` é embutido no binário** via `go:embed`. Mudança em `web/**` só
aparece depois de reiniciar o servidor de preview.

## Como o projeto está organizado agora

```
pkg/app/        composição (main.go e api/index.go são cascas)
pkg/apperr/     erros de domínio; pkg/api os mapeia para status HTTP
pkg/rules/      motor de regras (rules.json embutido via go:embed)
pkg/service/    casos de uso; validações de upload
pkg/extraction/ provedores de IA, retry, ajuste de horários
web/js/         domain.js, util.js, config.js, justificativa.js — testáveis
web/app.js      só o que depende do DOM
testdata/regras.json  casos compartilhados entre os dois lados
```

**O ponto mais importante da arquitetura atual:** `testdata/regras.json` tem 13
casos que o motor Go e o `web/js/domain.js` são obrigados a reproduzir com os
mesmos números. Mudar uma regra exige mudar o valor esperado ali, uma vez, e os
dois lados junto. Foi isso que fechou a divergência de 72 horas entre as duas
implementações — e é o que impede que ela volte.
