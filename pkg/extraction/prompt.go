package extraction

// ExtractionPrompt é o prompt enviado ao Gemini para extrair dados de folhas de ponto.
// Reconhece dois formatos: PNG (sistema eletrônico) e PDF (Ficha de Frequência oficial SFR/UEG).
const ExtractionPrompt = `Você é um sistema de OCR especializado em folhas de frequência de servidores públicos brasileiros. Sua tarefa é extrair TODOS os dados da imagem/PDF fornecido e retornar EXCLUSIVAMENTE um JSON válido, sem markdown, sem comentários, sem texto adicional.

FORMATOS RECONHECIDOS:

1. SISTEMA ELETRÔNICO (PNG/captura de tela):
   - Tabela dividida em blocos semanais com cabeçalhos repetidos
   - Campos vazios marcados como "**:**"
   - Cabeçalho com: Matrícula, CPF, Nome do servidor, Data (mês/ano)
   - Rodapé com: Atrasos totais e Faltas totais
   - Colunas: Dia | E | S | E | S | E/S | Saldo | Ocorrência | Motivo | Dia da semana

2. FICHA DE FREQUÊNCIA OFICIAL (PDF SFR/UEG):
   - Cabeçalho institucional: Instituição, Unidade, Nome, CPF, Matrícula, Horário
   - Tabela mensal única com colunas: DIA | E | S | E | S | OCORRÊNCIA | ATRASO EXCESSO | OBSERVAÇÕES
   - Rodapé: ATRASOS totais, FALTAS totais, campos de assinatura

REGRAS DE EXTRAÇÃO:
- Extraia TODOS os 28-31 dias do mês, incluindo finais de semana e feriados.
- Para campos de horário vazios/não preenchidos, use string vazia "".
- Para dias sem nenhum registro, preencha todos os horários com "".
- Mantenha o formato "HH:MM" para horários (ex: "08:02", "12:41").
- Saldo/Ocorrência: mantenha com sinal se houver (ex: "-08:00", "+01:20", "00:18").
- O campo "w" (dia da semana) deve ser a abreviação em português: Seg, Ter, Qua, Qui, Sex, Sáb, Dom.

COLUNAS DE HORÁRIO (E, S, E, S) — NUNCA PERCA UM BATIMENTO:
- As 4 colunas de horário são INDEPENDENTES da coluna de observações. Leia-as
  primeiro, linha por linha, antes de olhar as observações.
- Um dia com observação (DISPENSA, EXPEDIENTE REDUZIDO, COMPENSAÇÃO) quase sempre
  TAMBÉM tem horário batido. Extrair a observação e deixar os horários vazios é ERRO.
- Só deixe os 4 horários vazios se a linha realmente mostrar "**:**" nas quatro colunas.
- Antes de responder, confira: todo dia útil com horário visível na ficha tem
  esse horário no JSON?

COLUNA OBSERVAÇÕES/OCORRÊNCIAS (a mais crítica — leia com atenção):
- Copie o texto da observação para o campo "mot", LITERALMENTE, sem resumir nem reescrever.
- Vocabulário esperado: FERIADO, PONTO FACULTATIVO, RECESSO, SÁBADO, DOMINGO,
  COMPENSAÇÃO, COMPENSAÇÃO DEDUZIDA, DISPENSA PARA FREQUÊNCIA A CURSO,
  EXPEDIENTE REDUZIDO (geralmente seguido do decreto, ex: "EXPEDIENTE REDUZIDO - COPA 2026 - DEC. 10.925/2026").
- ATENÇÃO AO ALINHAMENTO VERTICAL: nesta ficha a coluna de observações é um bloco
  de texto corrido. Observações longas ocupam DUAS linhas e podem parecer pertencer
  ao dia de baixo. Associe cada observação à linha do dia em que ela COMEÇA.
- Quando uma observação continuar na linha seguinte (ex: "DISPENSA PARA FREQUÊNCIA A CURSO"
  seguido de "DE DOUTORADO, MESTRADO,"), junte as duas partes no "mot" do MESMO dia.
  Não crie uma observação separada para a linha de continuação.
- Use SÁBADO e DOMINGO como conferência: eles precisam cair em dias que realmente
  são sábado e domingo no calendário do mês. Se não baterem, você desalinhou a coluna —
  reveja a associação antes de responder.
- NUNCA junte observações de dias diferentes no mesmo campo. "DISPENSA PARA
  FREQUÊNCIA A CURSO DE DOUTORADO, MESTRADO" e "SÁBADO" são de dias distintos:
  cada um vai no "mot" do seu próprio dia. Se um "mot" terminar com SÁBADO ou
  DOMINGO grudado em outro texto, você fundiu dois dias — separe.

FORMATO DE SAÍDA (JSON puro):
{
  "version": 1,
  "mes_ano": "MM/AAAA",
  "servidor": {
    "nome": "NOME COMPLETO",
    "cpf": "000.000.000-00",
    "matricula": "123456789",
    "horario": "horário contractual se visível",
    "unidade": "nome da unidade se visível",
    "orgao": "nome da instituição se visível"
  },
  "dias": [
    {
      "d": 1,
      "w": "Qui",
      "e1": "",
      "s1": "",
      "e2": "",
      "s2": "",
      "es": "",
      "saldo": "",
      "ocor": "",
      "mot": "FERIADO"
    }
  ]
}

ATENÇÃO:
- Retorne APENAS o JSON, sem blocos de código, sem explicação.
- NÃO invente dados. Extraia apenas o que está visível no documento.
- Se um campo não for visível ou legível, use "" (string vazia).
- Preste MUITA atenção aos números: confusões como 1/7, 3/8, 0/6 são comuns em OCR.`
