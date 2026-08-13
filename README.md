# 🕐 Ponto Real Go

**Folha de Frequência Inteligente** — Sistema que lê automaticamente folhas de ponto por IA, calcula saldos, gera ocorrências e justificativas para servidores públicos.

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white)](https://golang.org)
[![Gemini AI](https://img.shields.io/badge/Gemini_AI-Google-4285F4?logo=google&logoColor=white)](https://ai.google.dev)
[![OpenRouter](https://img.shields.io/badge/OpenRouter-Multi--modelo-6E56CF)](https://openrouter.ai)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

> 📖 **[Leia o MANUAL](MANUAL.md)** — as regras em detalhe: o que o sistema
> inventa, o que ele nunca toca, como cada saldo é apurado e onde a conferência
> humana é obrigatória. Este README apresenta; o manual explica.

---

## 📋 Sobre

O **Ponto Real Go** é uma ferramenta web que automatiza o processo de análise de folhas de frequência (ponto eletrônico) para servidores públicos, com regras padrão calibradas para a **UEG (Universidade Estadual de Goiás)** mas configuráveis para outras instituições. O sistema usa IA para extrair os dados de imagens/PDFs da folha de ponto — à escolha do usuário, via **Google Gemini** diretamente ou via **OpenRouter** (com acesso a modelos de diversos provedores, como GPT-4o mini, Qwen e Llama) — concilia a leitura com o calendário e um motor de regras próprio, calcula automaticamente os saldos de horas e gera textos de ocorrência e justificativa prontos para uso no SEI (Sistema Eletrônico de Informações) do estado.

### 🎯 Para quem é?

Servidores públicos que precisam:
- Conferir e ajustar suas folhas de ponto mensais
- Detectar e corrigir inconsistências de leitura (observações deslocadas, horários implausíveis, expediente reduzido)
- Calcular saldos de horas trabalhadas
- Gerar ocorrências e justificativas padronizadas prontas para colar no SEI
- Manter histórico organizado de todos os meses

---

## ✨ Funcionalidades

### 🤖 Leitura Inteligente por IA (Multi-provedor)
- Upload de imagem (PNG, JPEG, WebP) ou PDF da folha de ponto.
- Extração automática via **Google Gemini** (Gemini 2.5 Flash, Gemini 3.1 Flash Lite/Pro).
- Suporte nativo ao **OpenRouter** para modelos variados (Gemini 2.5/2.0 Flash, GPT-4o mini, Qwen 2.5 VL, Qwen 3.7 Flash, Llama 3.2, Nemotron 3 Nano Omni).
- **Extração por Nome de Arquivo**: se o Vision da IA falhar em achar o mês/ano no papel, o sistema analisa o nome do arquivo enviado (e.g. `abril2026.png`) e deduz o período.
- **Prompt orientado ao layout do SFR**: instruções dedicadas ao vocabulário e à disposição real da ficha de frequência para reduzir erros de leitura.

### 🧭 Realinhamento e Conferência Automática
- **Realinhamento de observações deslocadas**: usa o calendário como âncora para detectar e corrigir colunas de observação fora de posição.
- **Reatribuição de horários por plausibilidade**: evita gerar horário indevido em dias que não deveriam ter batimento e reatribui pontos lidos incorretamente.
- **Vocabulário centralizado de observações/ocorrências**: motor de regras único que classifica os avisos da ficha SFR (feriado, expediente reduzido, sem horário lido, etc.).
- **Avisos de conferência recalculados** automaticamente ao processar ou reabrir um mês salvo.
- **Expediente reduzido por decreto**: suporte a carga horária diferenciada por dia, com aviso quando a carga ainda não foi informada.

### 🖼️ Visualizador Lateral do Documento
- Painel lateral redimensionável para conferir a imagem/PDF original enquanto revisa a tabela extraída.
- Zoom por scroll centrado no cursor, arraste (pan) e botões de zoom/reset.
- Paginação para documentos PDF com múltiplas páginas.

### 💼 Exportação Direta para o SEI (Rich Text)
- Geração do formulário de frequência completo nos padrões e tabelas do **SEI** (Sistema Eletrônico de Informações).
- Copia toda a estrutura formatada com bordas e cores como Rich Text (HTML) para que o usuário possa colar direto no editor do SEI (CKEditor).
- Exibe somente os dias ajustados e suas respectivas justificativas em formato padronizado.
- Lembra os dados da sua Chefia Imediata localmente para facilitar futuros preenchimentos.

### 📊 Cálculo Automático
- Cálculo de saldo diário (horas trabalhadas vs. jornada configurada).
- Saldo total do mês (extraído vs. calculado).
- Contagem de faltas, dias ajustados e completos.
- Detecção automática de fins de semana via calendário real.

### 🏷️ Tipos de Dia
| Tipo | Descrição |
|------|-----------|
| **Útil** | Dia normal de trabalho (jornada configurada) |
| **FDS** | Fim de semana (auto-detectado pelo calendário) |
| **Dispensa** | Meio período — calcula horas trabalhadas parcialmente |
| **Feriado** | Feriado — sem jornada obrigatória |
| **Folga** | Folga — sem jornada obrigatória |
| **Convocação** | Dia de convocação especial |
| **Reduzido** | Expediente reduzido por decreto — carga horária diferenciada, configurável por dia |

### ✏️ Edição Inteligente
- Horários editáveis em azul (gerados automaticamente).
- Horários originais preservados em preto.
- Auto-complemento inteligente de horários faltantes.
- Modo Dispensa: edição parcial sem auto-completar.

### 🗂️ Ocorrência Manual para o SEI
- Toggle por dia na tabela principal para adicionar ou remover uma ocorrência manualmente, sobrepondo a detecção automática baseada em horário gerado.
- Quando não há horário gerado para inferir a justificativa, o usuário digita o motivo livremente.
- A sobreposição é persistida por dia e respeitada tanto na tela quanto no documento gerado pelo Gerar SEI.

### 📋 Sistema de Cópia e Saldos
- **Saldo Original Preservado**: O saldo da imagem original nunca é apagado. Se houver divergência com o calculado, a tabela exibe ambos (o original riscado e o calculado em destaque).
- **Cópia Unificada**: Clique em qualquer célula de horário ou saldo para copiar. No caso de divergência de saldos, você pode clicar em cima de cada um individualmente para copiar apenas o que desejar.
- **Copiar Linha** — dia + horários + saldo original.
- **Copiar Tabela** — dados tabulados (TAB-separated, Excel-compatível).
- **Copiar Ocorrência** / **Copiar Justificativa** separadamente.
- **Copiar Tudo** — relatório completo de uma vez.

### 💾 Persistência Flexível
- Salvamento automático de cada mês analisado localmente (`data/`).
- **Deploy Serverless**: Modo stateless inteligente para deploys rápidos em ambientes como o Vercel, onde a persistência de disco local é desabilitada mantendo o funcionamento estático perfeito.

### 🎓 Tutorial Interativo
- Tour guiado para novos usuários (pré e pós-upload).
- Spotlight em cada elemento com explicação.
- Navegação por teclado (← → Enter Esc).
- Botão "?" para reiniciar a qualquer momento.

### 🎨 Interface
- Tema claro e escuro (toggle no header).
- Design responsivo e moderno.
- Feedback visual em todas as ações (toasts).
- Favicon personalizado.

---

## 🚀 Como Usar

### Requisitos
- **Go 1.21+** instalado
- Uma chave de API de **pelo menos um** provedor de IA:
  - **Google Gemini** (gratuita em [aistudio.google.com](https://aistudio.google.com)), ou
  - **OpenRouter** (em [openrouter.ai](https://openrouter.ai)), que dá acesso a modelos de vários provedores com uma única chave.

### Instalação

```bash
# Clone o repositório
git clone https://github.com/fernangcortes/ponto-real-go.git
cd ponto-real-go

# Execute o servidor
go run .
```

### Configuração

1. Acesse `http://localhost:8080`
2. Clique no ⚙️ (Configurações) no header
3. Escolha o provedor de IA (Google Gemini ou OpenRouter) e o modelo desejado
4. Insira a chave de API correspondente (também pode ser definida via variável de ambiente, veja abaixo)
5. Pronto! Faça upload da sua folha de ponto

As chaves informadas pela interface têm prioridade sobre as variáveis de ambiente e, em execução local, ficam salvas em `settings.json` para não precisar reinformá-las a cada acesso.

---

## ☁️ Deployment (Render)

Para hospedar o **Ponto Real Go** no [Render](https://render.com):

1. Conecte seu repositório GitHub ao Render.
2. O Render usará o arquivo `render.yaml` (Blueprint) automaticamente.
3. Se for configurar manualmente, use os seguintes parâmetros:
   - **Runtime**: `Go`
   - **Build Command**: `go build -o app .`
   - **Start Command**: `./app`
   - **Environment Variables** (opcionais — também podem ser definidas depois pela interface):
     - `PORT`: `10000` (ou sua preferência)
     - `GEMINI_API_KEY`: Sua chave do Google Gemini
     - `OPENROUTER_API_KEY`: Sua chave do OpenRouter

O mesmo binário também roda como função serverless na **Vercel** (`vercel.json` + `api/index.go`). Nesse modo (`VERCEL=1`), a persistência de configurações/meses em disco é desabilitada automaticamente — as chaves de API devem ser fornecidas via variáveis de ambiente ou reinformadas pela interface a cada sessão.

---

## 🏗️ Arquitetura

```
ponto-real-go/
├── main.go            # Servidor HTTP e bootstrap (inicializa e injeta dependências)
├── vercel.json        # Configuração de deploys no Vercel
├── api/
│   └── index.go       # Ponto de entrada Serverless para Vercel
├── web/               # Frontend (embed no binário ou CDN no Vercel)
│   ├── index.html     # Interface principal
│   ├── app.js         # Lógica do frontend
│   ├── styles.css     # Estilos (com ocultação de barra de rolagem global)
│   └── favicon.*      # Ícones
├── pkg/               # Pacotes Go
│   ├── api/
│   │   ├── handler.go         # Handlers HTTP (apenas delega para os serviços)
│   │   └── middleware.go      # Middlewares CORS, logging, pânico e BuildHandler
│   ├── repository/
│   │   ├── timesheet_repository.go # Interface de repositório de folhas de ponto
│   │   ├── settings_repository.go  # Interface de repositório de configurações
│   │   └── json_repository.go      # Persistência concreta em arquivos JSON local
│   ├── service/
│   │   ├── timesheet_service.go    # Casos de uso e orquestração de negócios
│   │   └── timesheet_service_test.go # Testes de cobertura do serviço
│   ├── extraction/
│   │   ├── provider.go        # Interface de extração IA e catálogo de modelos
│   │   ├── factory.go         # Fábrica de extratores baseada em registro modular
│   │   ├── gemini.go          # Integração Google Gemini
│   │   ├── openrouter.go      # Integração OpenRouter API
│   │   ├── prompt.go          # Prompt de extração (vocabulário e layout do SFR)
│   │   ├── prompt_adjust.go   # Prompt de ajuste
│   │   ├── rules_adjuster.go  # Ajuste baseado em regras (preserva saldo original, plausibilidade de horários)
│   │   ├── align.go           # Realinhamento de observações deslocadas usando o calendário como âncora
│   │   └── align_test.go      # Testes de realinhamento e reatribuição de horários
│   ├── models/
│   │   └── timesheet.go       # Modelos de dados de domínio e DTOs
│   └── rules/
│       ├── engine.go          # Motor de regras
│       ├── engine_test.go     # Testes
│       ├── observacoes.go     # Vocabulário centralizado de observações/ocorrências da ficha SFR
│       └── rules.json         # Regras UEG
└── data/                      # Dados salvos (meses, apenas localmente)
```

### Stack Tecnológica
- **Backend**: Go (net/http, embed, serverless-ready)
- **Frontend**: HTML5, CSS3, JavaScript vanilla (Rich-Text clipboard support)
- **IA**: Google Gemini API (direto) & OpenRouter (multi-provedor: Gemini, GPT-4o mini, Qwen, Llama, Nemotron), selecionáveis por chave própria do usuário ou variável de ambiente
- **Persistência**: JSON em disco local (opcional)
- **Tipografia**: Inter + JetBrains Mono (Google Fonts)

---

## 📝 Changelog

### v1.1.2 — Um gerador só por horário inventado (Agosto 2026)
- ♻️ **`adjustDay` reescrito**: 232 linhas viraram 99. Onde havia 12 combinações de batimentos, cada uma com a sua própria cópia da aritmética, agora há **14 casos nomeados** de duas a quatro linhas — e **cada coluna inventada nasce num gerador só**. O que muda de um caso para outro é de qual horário conhecido o gerador se ancora e em que ordem os quatro se resolvem.
- 🐛 **Sumiu o aviso *"combinação não prevista"*.** Ele não denunciava defeito nenhum: eram duas combinações legítimas — o dia em que o servidor bateu na saída para o almoço, ou nela e no retorno, e esqueceu as duas pontas. Hoje cada uma tem nome e comentário próprios.
- ✅ **Os horários gerados passaram a ser reproduzíveis.** O sorteio saiu do `math/rand` global e vive no ajustador, semeado pelo relógio em produção e por semente no teste. Sem isso não havia como travar valor nenhum: a suíte ficava verde tanto para o horário certo quanto para o errado.
- ✅ **Snapshot das 14 combinações possíveis de batimentos**, com o dia que entra e o dia inteiro que sai, usando dado real do usuário onde ele existe. Somar 45 minutos a uma saída inventada — que antes passava despercebido por toda a suíte — agora quebra o teste com os dois horários lado a lado.
- 🔒 **Nada mudou de horário.** A reescrita foi validada contra a implementação anterior em **206.808 comparações** (as 14 combinações × 24 horários de borda, em 7 regimes de sorteio), com zero diferenças, e o snapshot não se moveu um minuto — o que prova que também a ordem dos sorteios foi preservada.
- ✅ **Teste novo travando um acoplamento invisível**: nos dois casos sem piso de tarde, o que impede a saída de sair antes do retorno do almoço não está no gerador, e sim nas janelas de plausibilidade de outra função. Alargá-las agora quebra um teste que explica o motivo. A folga real é de **70 minutos** — metade do que se supunha.

### v1.1.1 — Horários gerados sob controle (Agosto 2026)
- 🐛 **Entrada da manhã nunca mais é inventada de madrugada.** Um dos geradores estava sem a trava das 07:00 que os outros têm: um único batimento às 11:30 no retorno do almoço produzia entrada às 05:56. A manhã passa a ser recontada depois da trava — sem isso a tarde compensava uma manhã que não aconteceu e o dia fechava com uma hora a mais do que o servidor trabalhou.
- 🐛 **O almoço gerado não cai mais abaixo do mínimo legal.** O desvio dos minutos redondos era aplicado como lei e comia o intervalo por cima, produzindo almoço de 58 minutos. A regra virou preferência: tenta o outro lado antes, e só aceita o minuto redondo quando nada mais cabe (acontece em ~0,3% dos horários gerados).
- ✅ **Três testes novos** travando o piso das 07:00, o mínimo do almoço e o comportamento flexível do desvio — incluindo varredura de todo minuto do dia nas quatro colunas.
- 🔧 **CI de lint consertado**: a action estava presa ao golangci-lint v1, que não roda contra Go 1.25 nem entende o `.golangci.yml` no formato v2. Passa a rodar a mesma versão usada em desenvolvimento.
- 📖 **[MANUAL.md](MANUAL.md)**: manual detalhado das regras — o que o sistema inventa, o que ele nunca toca, como cada saldo é apurado e onde a conferência humana é obrigatória.

### v1.1.0 — Conferência Inteligente & Ocorrência Manual (Agosto 2026)
- ✅ **Realinhamento automático** de observações deslocadas usando o calendário como âncora.
- ✅ **Reatribuição de horários por plausibilidade**, evitando geração indevida de horário em dias sem batimento esperado.
- ✅ **Vocabulário centralizado de observações/ocorrências** da ficha SFR, aplicado no motor de regras.
- ✅ **Recalculo de avisos de conferência** ao processar e reabrir um mês salvo.
- ✅ **Expediente reduzido por decreto** com carga horária configurável por dia e justificativa editável no frontend.
- ✅ **Ocorrência manual**: toggle por dia para adicionar/remover ocorrências e sobrepor a detecção automática no documento gerado pelo SEI.
- ✅ **Visualizador lateral do documento enviado**, com zoom por scroll, arraste (pan) e paginação de PDF.
- ✅ Novos modelos no OpenRouter: **Qwen 3.7 Flash** e **Nemotron 3 Nano Omni**.
- ✅ Correções de roteamento entre arquivos estáticos e `/api` no servidor local e de duplicidade na lista de modelos do OpenRouter.

### v1.0.0 — Stable Release & Clean Architecture (Junho 2026)
- ✅ **Refatoração para Clean Architecture e SOLID**: Separação total da camada de negócio (`service`), persistência (`repository`), inteligência artificial (`extraction/factory`) e transporte (`api/handler`).
- ✅ **Injeção de Dependências**: Inicializações explícitas de repositórios e serviços no bootstrap, eliminando acoplamentos rígidos e variáveis globais.
- ✅ **Provedores de IA Plugáveis**: Fábrica baseada em registro de builders para facilidade na expansão de novos extratores (ex: Claude, Document AI).
- ✅ **Ajustes Finos de Design**: Modais responsivos com barra de rolagem inteligente para evitar estouros na vertical (`max-height: 90vh`).
- ✅ **Interface Clean**: Ocultação global de barras de rolagem nativas para um visual moderno e fluido em todos os dispositivos.
- ✅ **Integração com OpenRouter** suportando múltiplos modelos (incluindo Gemini 2.5/2.0 Flash, GPT-4o mini, Qwen 2.5 VL).
- ✅ **Configuração serverless** e suporte completo para **Vercel** (`vercel.json`, `api/index.go`).
- ✅ **Geração e Exportação de Formulário SEI**: Gerador completo de formulário de ocorrências e justificativas formatados com layout SEI e copiável como Rich Text (HTML).
- ✅ **Extração de Período por Nome do Arquivo**: Fallback automático para descobrir mês/ano a partir do título do arquivo de upload (e.g. `abril2026.png`).
- ✅ **Preservação de Saldo Original**: O saldo original da folha é preservado e pode ser copiado individualmente na célula divergente.

### v0.3.0 — Tutorial & Cópia (Abril 2026)
- ✅ Tutorial interativo com spotlight e navegação
- ✅ Sistema de cópia completo (células, linhas, tabela, seções, tudo)
- ✅ Saldo original acessível via tooltip em cada célula
- ✅ Barra de cópia (toolbar) com botões dedicados
- ✅ Botões "Copiar" em cada seção (Ocorrência, Justificativa)
- ✅ Footer com assinatura do desenvolvedor e PIX copiável
- ✅ Metatags Open Graph para compartilhamento

### v0.2.0 — Tipos de Dia & Dispensa (Abril 2026)
- ✅ Seletor de tipo de dia (Útil, FDS, Dispensa, Feriado, Folga, Convocação)
- ✅ Detecção automática de FDS via calendário real
- ✅ Modo Dispensa com cálculo parcial de horas
- ✅ Recálculo automático de saldo por tipo de dia
- ✅ Favicon personalizado (SVG + PNG multi-resolução)
- ✅ Correção de tema claro/escuro em todos os elementos

### v0.1.0 — Fundação (Março-Abril 2026)
- ✅ Servidor Go com assets embeddados
- ✅ Upload de imagem com extração via Gemini
- ✅ Tabela de dias com E1/S1/E2/S2 editáveis
- ✅ Auto-complemento inteligente de horários
- ✅ Cálculo de saldo diário e mensal
- ✅ Geração de ocorrências e justificativas
- ✅ Persistência em JSON (salvar/carregar meses)
- ✅ Navegação entre meses salvos
- ✅ Tema claro e escuro
- ✅ Status bar com contadores
- ✅ Toast de feedback visual
- ✅ Regras UEG (Universidade Estadual de Goiás)

---

## 🔮 Roadmap

- [ ] Exportação para PDF
- [ ] Exportação para CSV
- [ ] Banco de horas acumulado
- [ ] Modo offline (PWA)
- [ ] Suporte a múltiplas instituições

---

## 👨‍💻 Desenvolvedor

**FGC** — Fernando Gomes Cortes

- 📧 vozesdoasfalto@gmail.com
- ☕ PIX: `00833238132`

---

## 📄 Licença

Este projeto é distribuído sob a licença MIT. Veja [LICENSE](LICENSE) para mais detalhes.
