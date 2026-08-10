package repository

import (
	"errors"
	"fmt"

	"github.com/fernangcortes/ponto-real-go/pkg/apperr"
	"github.com/fernangcortes/ponto-real-go/pkg/models"
)

// Repositórios que não guardam nada, para ambientes de disco efêmero como o
// Vercel — onde gravar não dá erro, mas o arquivo some no próximo request.
//
// Antes essa decisão vivia dentro do repositório JSON, que consultava
// os.Getenv("VERCEL") em cinco pontos: a camada de persistência sabia em que
// nuvem estava rodando e mudava de comportamento sozinha. Agora é escolha de
// composição — quem monta o grafo decide qual implementação injetar — e o
// repositório JSON voltou a ser só um repositório JSON, testável sem mexer em
// variável de ambiente.

// NoopTimesheetRepository aceita gravações e as descarta.
type NoopTimesheetRepository struct{}

func (NoopTimesheetRepository) Save(models.MonthData) error { return nil }

func (NoopTimesheetRepository) Load(mesAno string) (*models.MonthData, error) {
	return nil, fmt.Errorf("%w: %s", apperr.ErrMesNaoEncontrado, mesAno)
}

func (NoopTimesheetRepository) List() ([]models.MonthSummary, error) {
	return []models.MonthSummary{}, nil
}

// NoopSettingsRepository não lê nem grava configurações. O settingsStore mantém
// o que receber em memória e completa o resto pelas variáveis de ambiente, que
// é como um deploy serverless deve ser configurado de qualquer forma.
type NoopSettingsRepository struct{}

// ErrSemPersistencia indica que não há de onde ler: não é falha, é a ausência
// de armazenamento sendo dita em voz alta.
var ErrSemPersistencia = errors.New("persistência desativada neste ambiente")

func (NoopSettingsRepository) Load() (models.AppSettings, error) {
	return models.AppSettings{}, ErrSemPersistencia
}

func (NoopSettingsRepository) Save(models.AppSettings) error { return nil }
