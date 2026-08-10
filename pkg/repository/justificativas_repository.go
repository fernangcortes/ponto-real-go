package repository

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fernangcortes/ponto-real-go/pkg/models"
)

// JustificativasRepository define o contrato para persistência da biblioteca de
// frases de justificativa.
type JustificativasRepository interface {
	Load() (models.BibliotecaJustificativas, error)
	Save(models.BibliotecaJustificativas) error
}

// JSONJustificativasRepository guarda a biblioteca num arquivo JSON ao lado do
// executável — o mesmo lugar de settings.json.
//
// Deliberadamente FORA de data/: o List() do JSONTimesheetRepository trata todo
// .json daquela pasta como um mês salvo, e a biblioteca apareceria no seletor
// de meses como se fosse um.
type JSONJustificativasRepository struct {
	filename string
}

func NewJSONJustificativasRepository(filename string) *JSONJustificativasRepository {
	return &JSONJustificativasRepository{filename: filename}
}

// filepath resolve o arquivo ao lado do executável, como o de configurações.
//
// Caminho absoluto passa direto: filepath.Join não trata a raiz de forma
// especial, então juntá-lo à pasta do executável grudaria os dois e gravaria
// em lugar nenhum parecido com o que foi pedido.
func (r *JSONJustificativasRepository) filepath() string {
	if filepath.IsAbs(r.filename) {
		return r.filename
	}
	exe, err := os.Executable()
	if err != nil {
		return r.filename
	}
	return filepath.Join(filepath.Dir(exe), r.filename)
}

// Load lê a biblioteca do disco. Arquivo ausente é a primeira execução, não
// erro: devolve a biblioteca vazia.
func (r *JSONJustificativasRepository) Load() (models.BibliotecaJustificativas, error) {
	var b models.BibliotecaJustificativas

	data, err := os.ReadFile(r.filepath())
	if err != nil {
		if os.IsNotExist(err) {
			return b, nil
		}
		return b, err
	}

	err = json.Unmarshal(data, &b)
	return b, err
}

// Save grava a biblioteca inteira, normalizada.
func (r *JSONJustificativasRepository) Save(b models.BibliotecaJustificativas) error {
	b.Frases = NormalizarFrases(b.Frases)
	b.UpdatedAt = time.Now()

	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(r.filepath(), data, 0600)
}

// NormalizarFrases limpa a lista antes de gravar: descarta as vazias e funde as
// repetidas.
//
// Duas frases iguais na biblioteca são um item morto na lista suspensa — e elas
// aparecem com facilidade, porque a mesma frase é salva de meses diferentes.
// A comparação ignora espaço e caixa; a contagem de usos das duplicatas é
// somada, para que a fusão não zere o histórico que decide a sugestão do dia.
func NormalizarFrases(frases []models.Justificativa) []models.Justificativa {
	limpas := make([]models.Justificativa, 0, len(frases))
	posicao := make(map[string]int, len(frases))

	for _, f := range frases {
		f.Texto = strings.TrimSpace(f.Texto)
		if f.Texto == "" {
			continue
		}

		chave := strings.ToLower(f.Texto)
		if i, jaExiste := posicao[chave]; jaExiste {
			limpas[i].Usos += f.Usos
			// Sem tipo registrado, herda o da repetida: uma frase salva de novo
			// num dia de dispensa passa a ordenar junto às de dispensa.
			if limpas[i].Tipo == "" {
				limpas[i].Tipo = f.Tipo
			}
			continue
		}

		posicao[chave] = len(limpas)
		limpas = append(limpas, f)
	}

	return limpas
}

// NoopJustificativasRepository aceita gravações e as descarta, para ambientes
// de disco efêmero. O front-end mantém a sua cópia em localStorage, então a
// biblioteca continua servindo — só não atravessa navegadores.
type NoopJustificativasRepository struct{}

func (NoopJustificativasRepository) Load() (models.BibliotecaJustificativas, error) {
	return models.BibliotecaJustificativas{}, nil
}

func (NoopJustificativasRepository) Save(models.BibliotecaJustificativas) error { return nil }
