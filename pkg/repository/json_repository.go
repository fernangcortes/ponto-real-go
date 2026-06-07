package repository

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fernangcortes/ponto-real-go/pkg/models"
)

// JSONTimesheetRepository implementa TimesheetRepository usando arquivos JSON locais.
type JSONTimesheetRepository struct {
	dataDir string
}

// NewJSONTimesheetRepository cria um novo repositório JSON para folhas de ponto.
func NewJSONTimesheetRepository(dataDir string) *JSONTimesheetRepository {
	return &JSONTimesheetRepository{dataDir: dataDir}
}

func (r *JSONTimesheetRepository) ensureDataDir() error {
	return os.MkdirAll(r.dataDir, 0755)
}

func (r *JSONTimesheetRepository) mesAnoToFilename(mesAno string) string {
	safe := strings.ReplaceAll(mesAno, "/", "_")
	return safe + ".json"
}

func (r *JSONTimesheetRepository) filenameToMesAno(filename string) string {
	name := strings.TrimSuffix(filename, ".json")
	return strings.ReplaceAll(name, "_", "/")
}

// Save persiste o estado completo de um mês em disco.
func (r *JSONTimesheetRepository) Save(data models.MonthData) error {
	if os.Getenv("VERCEL") == "1" {
		fmt.Println("[Storage] Executando no Vercel: persistência de mês desativada.")
		return nil
	}

	if err := r.ensureDataDir(); err != nil {
		return fmt.Errorf("erro ao criar diretório: %w", err)
	}

	data.UpdatedAt = time.Now()

	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("erro ao serializar: %w", err)
	}

	path := filepath.Join(r.dataDir, r.mesAnoToFilename(data.MesAno))
	if err := os.WriteFile(path, bytes, 0644); err != nil {
		return fmt.Errorf("erro ao salvar: %w", err)
	}

	fmt.Printf("[Storage] Mês %s salvo (%d dias)\n", data.MesAno, len(data.Dias))
	return nil
}

// Load carrega o estado de um mês do disco.
func (r *JSONTimesheetRepository) Load(mesAno string) (*models.MonthData, error) {
	if os.Getenv("VERCEL") == "1" {
		return nil, fmt.Errorf("persistência desativada no Vercel")
	}

	path := filepath.Join(r.dataDir, r.mesAnoToFilename(mesAno))
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("mês não encontrado: %w", err)
	}

	var data models.MonthData
	if err := json.Unmarshal(bytes, &data); err != nil {
		return nil, fmt.Errorf("erro ao ler dados: %w", err)
	}

	return &data, nil
}

// List retorna a lista de meses salvos, ordenada do mais recente ao mais antigo.
func (r *JSONTimesheetRepository) List() ([]models.MonthSummary, error) {
	if os.Getenv("VERCEL") == "1" {
		return []models.MonthSummary{}, nil
	}

	if err := r.ensureDataDir(); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(r.dataDir)
	if err != nil {
		return nil, fmt.Errorf("erro ao listar: %w", err)
	}

	var summaries []models.MonthSummary
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		mesAno := r.filenameToMesAno(entry.Name())

		// Ler resumo sem carregar todos os dados
		path := filepath.Join(r.dataDir, entry.Name())
		bytes, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var data models.MonthData
		if err := json.Unmarshal(bytes, &data); err != nil {
			continue
		}

		summaries = append(summaries, models.MonthSummary{
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

// Delete remove um mês salvo do disco.
func (r *JSONTimesheetRepository) Delete(mesAno string) error {
	if os.Getenv("VERCEL") == "1" {
		return nil
	}
	path := filepath.Join(r.dataDir, r.mesAnoToFilename(mesAno))
	return os.Remove(path)
}

// JSONSettingsRepository implementa SettingsRepository usando um arquivo JSON local.
type JSONSettingsRepository struct {
	filename string
}

// NewJSONSettingsRepository cria um novo repositório para configurações do aplicativo.
func NewJSONSettingsRepository(filename string) *JSONSettingsRepository {
	return &JSONSettingsRepository{filename: filename}
}

func (r *JSONSettingsRepository) filepath() string {
	if os.Getenv("VERCEL") == "1" {
		return "/tmp/" + r.filename
	}
	exe, err := os.Executable()
	if err != nil {
		return r.filename
	}
	return filepath.Join(filepath.Dir(exe), r.filename)
}

// Load carrega as configurações do arquivo JSON.
func (r *JSONSettingsRepository) Load() (models.AppSettings, error) {
	var s models.AppSettings
	path := r.filepath()
	data, err := os.ReadFile(path)
	if err != nil {
		return s, err
	}
	err = json.Unmarshal(data, &s)
	return s, err
}

// Save persiste as configurações no arquivo JSON.
func (r *JSONSettingsRepository) Save(s models.AppSettings) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(r.filepath(), data, 0600)
}
