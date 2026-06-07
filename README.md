# 🕐 Ponto Real Go

**Folha de Frequência Inteligente** — Sistema que lê automaticamente folhas de ponto por IA, calcula saldos, gera ocorrências e justificativas para servidores públicos.

[![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go&logoColor=white)](https://golang.org)
[![Gemini AI](https://img.shields.io/badge/Gemini_AI-Google-4285F4?logo=google&logoColor=white)](https://ai.google.dev)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

---

## 📋 Sobre

O **Ponto Real Go** é uma ferramenta web que automatiza o processo de análise de folhas de frequência (ponto eletrônico) para servidores públicos. O sistema utiliza IA (Google Gemini) para extrair dados de imagens/PDFs da folha de ponto, calcula automaticamente os saldos de horas e gera textos de ocorrência e justificativa prontos para uso no sistema do estado.

### 🎯 Para quem é?

Servidores públicos que precisam:
- Conferir e ajustar suas folhas de ponto mensais
- Calcular saldos de horas trabalhadas
- Gerar justificativas padronizadas para o sistema de ponto do estado
- Manter histórico organizado de todos os meses

---

## ✨ Funcionalidades

### 🤖 Leitura Inteligente por IA (Multi-provedor)
- Upload de imagem (PNG, JPEG, WebP) ou PDF da folha de ponto.
- Extração automática via **Google Gemini** (Gemini 2.5 Flash, Gemini 3.1 Flash Lite/Pro).
- Suporte nativo ao **OpenRouter** para modelos variados (Gemini 2.5/2.0 Flash, GPT-4o mini, Qwen 2.5 VL, Llama 3.2).
- **Extração por Nome de Arquivo**: se o Vision da IA falhar em achar o mês/ano no papel, o sistema analisa o nome do arquivo enviado (e.g. `abril2026.png`) e deduz o período.

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

### ✏️ Edição Inteligente
- Horários editáveis em azul (gerados automaticamente).
- Horários originais preservados em preto.
- Auto-complemento inteligente de horários faltantes.
- Modo Dispensa: edição parcial sem auto-completar.

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
- **Chave de API do Google Gemini** (gratuita em [aistudio.google.com](https://aistudio.google.com))

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
3. Insira sua chave da API Gemini
4. Pronto! Faça upload da sua folha de ponto

---

## ☁️ Deployment (Render)

Para hospedar o **Ponto Real Go** no [Render](https://render.com):

1. Conecte seu repositório GitHub ao Render.
2. O Render usará o arquivo `render.yaml` (Blueprint) automaticamente.
3. Se for configurar manualmente, use os seguintes parâmetros:
   - **Runtime**: `Go`
   - **Build Command**: `go build -o app .`
   - **Start Command**: `./app`
   - **Environment Variables**:
     - `PORT`: `10000` (ou sua preferência)
     - `GEMINI_API_KEY`: Sua chave do Google Gemini

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
│   │   ├── provider.go        # Interface de extração IA
│   │   ├── factory.go         # Fábrica de extratores baseada em registro modular
│   │   ├── gemini.go          # Integração Google Gemini
│   │   ├── openrouter.go      # Integração OpenRouter API
│   │   ├── prompt.go          # Prompt de extração
│   │   ├── prompt_adjust.go   # Prompt de ajuste
│   │   └── rules_adjuster.go  # Ajuste baseado em regras (preserva saldo original)
│   ├── models/
│   │   └── timesheet.go       # Modelos de dados de domínio e DTOs
│   └── rules/
│       ├── engine.go          # Motor de regras
│       ├── engine_test.go     # Testes
│       └── rules.json         # Regras UEG
└── data/                      # Dados salvos (meses, apenas localmente)
```

### Stack Tecnológica
- **Backend**: Go (net/http, embed, serverless-ready)
- **Frontend**: HTML5, CSS3, JavaScript vanilla (Rich-Text clipboard support)
- **IA**: Google Gemini API & OpenRouter (Gemini 2.5 Flash, GPT-4o mini, Qwen 2.5 VL)
- **Persistência**: JSON em disco local (opcional)
- **Tipografia**: Inter + JetBrains Mono (Google Fonts)

---

## 📝 Changelog

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
