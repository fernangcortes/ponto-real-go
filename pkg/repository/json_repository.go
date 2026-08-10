package repository

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fernangcortes/ponto-real-go/pkg/apperr"
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

	slog.Debug("mês gravado em disco", "mes_ano", data.MesAno, "dias", len(data.Dias), "caminho", path)
	return nil
}

// Load carrega o estado de um mês do disco.
func (r *JSONTimesheetRepository) Load(mesAno string) (*models.MonthData, error) {
	path := filepath.Join(r.dataDir, r.mesAnoToFilename(mesAno))
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", apperr.ErrMesNaoEncontrado, mesAno)
	}

	var data models.MonthData
	if err := json.Unmarshal(bytes, &data); err != nil {
		return nil, fmt.Errorf("erro ao ler dados: %w", err)
	}

	return &data, nil
}

// List retorna a lista de meses salvos, ordenada do mais recente ao mais antigo.
func (r *JSONTimesheetRepository) List() ([]models.MonthSummary, error) {
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

	// Ordenar do mês mais recente para o mais antigo.
	//
	// Precisa comparar (ano, mês) e não a string: "MM/AAAA" tem o mês na frente,
	// então a ordem lexicográfica põe 12/2025 na frente de 01/2026 — o ano seria
	// simplesmente ignorado na virada do ano.
	sort.Slice(summaries, func(i, j int) bool {
		ai, mi := ordemMesAno(summaries[i].MesAno)
		aj, mj := ordemMesAno(summaries[j].MesAno)
		if ai != aj {
			return ai > aj
		}
		return mi > mj
	})

	return summaries, nil
}

// ordemMesAno extrai (ano, mês) de "MM/AAAA" para efeito de ordenação.
// Formato irreconhecível vai para o fim da lista.
func ordemMesAno(mesAno string) (ano, mes int) {
	partes := strings.Split(mesAno, "/")
	if len(partes) != 2 {
		return -1, -1
	}
	m, err1 := strconv.Atoi(partes[0])
	a, err2 := strconv.Atoi(partes[1])
	if err1 != nil || err2 != nil {
		return -1, -1
	}
	return a, m
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
