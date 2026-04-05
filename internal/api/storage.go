package api

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fernangcortes/ponto-real-go/internal/models"
)

const dataDir = "data"

// MonthData é o estado completo de um mês salvo em disco.
type MonthData struct {
	MesAno    string             `json:"mes_ano"`
	Servidor  models.ServerInfo  `json:"servidor"`
	Dias      []MonthDayRecord   `json:"dias"`
	UpdatedAt time.Time          `json:"updated_at"`
}

// MonthDayRecord estende DayRecord com o override do tipo de dia.
type MonthDayRecord struct {
	models.DayRecord
	DayTypeOverride string `json:"day_type_override,omitempty"`
}

// MonthSummary é o resumo de um mês para listagem.
type MonthSummary struct {
	MesAno       string    `json:"mes_ano"`
	ServidorNome string    `json:"servidor_nome"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ensureDataDir cria o diretório de dados se não existir.
func ensureDataDir() error {
	return os.MkdirAll(dataDir, 0755)
}

// mesAnoToFilename converte "02/2026" para "02_2026.json".
func mesAnoToFilename(mesAno string) string {
	safe := strings.ReplaceAll(mesAno, "/", "_")
	return safe + ".json"
}

// filenameToMesAno converte "02_2026.json" para "02/2026".
func filenameToMesAno(filename string) string {
	name := strings.TrimSuffix(filename, ".json")
	return strings.ReplaceAll(name, "_", "/")
}

// SaveMonth salva o estado completo de um mês em disco.
func SaveMonth(data MonthData) error {
	if err := ensureDataDir(); err != nil {
		return fmt.Errorf("erro ao criar diretório: %w", err)
	}

	data.UpdatedAt = time.Now()

	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("erro ao serializar: %w", err)
	}

	path := filepath.Join(dataDir, mesAnoToFilename(data.MesAno))
	if err := os.WriteFile(path, bytes, 0644); err != nil {
		return fmt.Errorf("erro ao salvar: %w", err)
	}

	fmt.Printf("[Storage] Mês %s salvo (%d dias)\n", data.MesAno, len(data.Dias))
	return nil
}

// LoadMonth carrega o estado de um mês do disco.
func LoadMonth(mesAno string) (*MonthData, error) {
	path := filepath.Join(dataDir, mesAnoToFilename(mesAno))
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("mês não encontrado: %w", err)
	}

	var data MonthData
	if err := json.Unmarshal(bytes, &data); err != nil {
		return nil, fmt.Errorf("erro ao ler dados: %w", err)
	}

	return &data, nil
}

// ListMonths retorna a lista de meses salvos, ordenada do mais recente ao mais antigo.
func ListMonths() ([]MonthSummary, error) {
	if err := ensureDataDir(); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dataDir)
	if err != nil {
		return nil, fmt.Errorf("erro ao listar: %w", err)
	}

	var summaries []MonthSummary
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		mesAno := filenameToMesAno(entry.Name())

		// Ler resumo sem carregar todos os dados
		path := filepath.Join(dataDir, entry.Name())
		bytes, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var data MonthData
		if err := json.Unmarshal(bytes, &data); err != nil {
			continue
		}

		summaries = append(summaries, MonthSummary{
			MesAno:       mesAno,
			ServidorNome: data.Servidor.Nome,
			UpdatedAt:    data.UpdatedAt,
		})
	}

	// Ordenar por mes_ano decrescente (mais recente primeiro)
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].MesAno > summaries[j].MesAno
	})

	return summaries, nil
}

// DeleteMonth remove um mês salvo.
func DeleteMonth(mesAno string) error {
	path := filepath.Join(dataDir, mesAnoToFilename(mesAno))
	return os.Remove(path)
}
