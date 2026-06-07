package repository

import "github.com/fernangcortes/ponto-real-go/pkg/models"

// SettingsRepository define o contrato para persistência de configurações do sistema.
type SettingsRepository interface {
	Load() (models.AppSettings, error)
	Save(settings models.AppSettings) error
}
