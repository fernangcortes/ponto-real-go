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

### 🤖 Leitura Inteligente por IA
- Upload de imagem (PNG, JPEG) ou PDF da folha de ponto
- Extração automática via **Google Gemini** (Flash Lite ou Pro)
- Detecção automática de horários, saldos e ocorrências

### 📊 Cálculo Automático
- Cálculo de saldo diário (horas trabalhadas vs. jornada de 6h)
- Saldo total do mês (extraído vs. calculado)
- Contagem de faltas, dias ajustados e completos
- Detecção automática de fins de semana via calendário real

### 🏷️ Tipos de Dia
| Tipo | Descrição |
|------|-----------|
| **Útil** | Dia normal de trabalho (jornada 6h) |
| **FDS** | Fim de semana (auto-detectado pelo calendário) |
| **Dispensa** | Meio período — calcula horas trabalhadas parcialmente |
| **Feriado** | Feriado — sem jornada obrigatória |
| **Folga** | Folga — sem jornada obrigatória |
| **Convocação** | Dia de convocação especial |

### ✏️ Edição Inteligente
- Horários editáveis em azul (gerados automaticamente)
- Horários originais preservados em preto
- Auto-complemento inteligente de horários faltantes
- Modo Dispensa: edição parcial sem auto-completar

### 📋 Sistema de Cópia
- **Clique em qualquer célula** para copiar o valor
- **Copiar Linha** — dia + horários + saldo original
- **Copiar Tabela** — dados tabulados (TAB-separated, Excel-compatível)
- **Copiar Ocorrência** / **Copiar Justificativa** separadamente
- **Copiar Tudo** — relatório completo de uma vez
- Saldo original sempre acessível via tooltip

### 💾 Persistência e Navegação
- Salvamento automático de cada mês analisado
- Navegação entre meses com seletor e setas
- Indicador de salvamento pendente

### 🎓 Tutorial Interativo
- Tour guiado para novos usuários (pré e pós-upload)
- Spotlight em cada elemento com explicação
- Navegação por teclado (← → Enter Esc)
- Botão "?" para reiniciar a qualquer momento

### 🎨 Interface
- Tema claro e escuro (toggle no header)
- Design responsivo e moderno
- Feedback visual em todas as ações (toasts)
- Favicon personalizado

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
go run ./cmd/server
```

### Configuração

1. Acesse `http://localhost:8080`
2. Clique no ⚙️ (Configurações) no header
3. Insira sua chave da API Gemini
4. Pronto! Faça upload da sua folha de ponto

### Uso

1. **Arraste** ou **clique** para selecionar a imagem/PDF da folha de ponto
2. Escolha o modelo de IA: **Flash Lite** (rápido) ou **Pro** (preciso)
3. Clique em **Processar**
4. Edite horários, altere tipos de dia conforme necessário
5. Copie ocorrências e justificativas para o sistema do estado

---

## 🏗️ Arquitetura

```
ponto-real-go/
├── cmd/
│   └── server/
│       ├── main.go            # Servidor HTTP e bootstrap
│       └── web/               # Frontend (embed no binário)
│           ├── index.html     # Interface principal
│           ├── app.js         # Lógica do frontend
│           ├── styles.css     # Estilos
│           └── favicon.*      # Ícones
├── internal/
│   ├── api/
│   │   ├── handler.go         # Handlers HTTP (upload, CRUD)
│   │   ├── middleware.go      # CORS e middlewares
│   │   └── storage.go         # Persistência em JSON
│   ├── extraction/
│   │   ├── provider.go        # Interface de extração IA
│   │   ├── gemini.go          # Integração Google Gemini
│   │   ├── prompt.go          # Prompt de extração
│   │   ├── prompt_adjust.go   # Prompt de ajuste
│   │   └── rules_adjuster.go  # Ajuste baseado em regras
│   ├── models/
│   │   └── timesheet.go       # Modelos de dados
│   └── rules/
│       ├── engine.go          # Motor de regras
│       ├── engine_test.go     # Testes
│       └── rules.json         # Regras UEG
└── data/                      # Dados salvos (meses)
```

### Stack Tecnológica
- **Backend**: Go (net/http, embed)
- **Frontend**: HTML5, CSS3, JavaScript vanilla
- **IA**: Google Gemini API (gemini-2.0-flash-lite, gemini-2.0-pro)
- **Persistência**: JSON em disco local
- **Tipografia**: Inter + JetBrains Mono (Google Fonts)

---

## 📝 Changelog

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
