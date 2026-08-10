package repository

import "github.com/fernangcortes/ponto-real-go/pkg/models"

// TimesheetRepository define o contrato para persistência de folhas de frequência.
type TimesheetRepository interface {
	Save(data models.MonthData) error
	Load(mesAno string) (*models.MonthData, error)
	List() ([]models.MonthSummary, error)
}
