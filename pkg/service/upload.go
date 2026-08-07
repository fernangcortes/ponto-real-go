package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/fernangcortes/ponto-real-go/pkg/apperr"
	"github.com/fernangcortes/ponto-real-go/pkg/extraction"
	"github.com/fernangcortes/ponto-real-go/pkg/models"
)

// UploadRequest descreve um envio de folha de ponto para extração.
//
// Existe para que o handler HTTP só precise ler o multipart e repassar: escolha
// de modelo, detecção e validação de tipo de arquivo e checagem de chave são
// regra de aplicação, não de transporte, e viviam nas 107 linhas do Upload.
type UploadRequest struct {
	Arquivo  []byte
	MimeType string // vazio ou genérico: será inferido de Filename + magic bytes
	Filename string
	Modelo   string // vazio: usa o padrão do provedor
	Provider string
	APIKey   string
}

// tiposAceitos são os formatos que os provedores de visão conseguem ler.
var tiposAceitos = map[string]bool{
	"image/png":       true,
	"image/jpeg":      true,
	"image/webp":      true,
	"application/pdf": true,
}

// normalizar preenche os padrões e recusa o que não dá para processar.
func (r *UploadRequest) normalizar() error {
	if r.APIKey == "" {
		return fmt.Errorf("%w: configure a chave da API %s nas Configurações (⚙️)",
			apperr.ErrChaveAusente, extraction.NomeAmigavel(r.Provider))
	}

	if len(r.Arquivo) == 0 {
		return fmt.Errorf("%w: arquivo vazio", apperr.ErrArquivoInvalido)
	}

	// O navegador manda application/octet-stream quando não reconhece o tipo.
	if r.MimeType == "" || r.MimeType == "application/octet-stream" {
		r.MimeType = DetectMimeType(r.Filename, r.Arquivo)
	}
	if !tiposAceitos[r.MimeType] {
		return fmt.Errorf("%w: %s. Use PNG, JPEG, WebP ou PDF", apperr.ErrArquivoInvalido, r.MimeType)
	}

	if r.Modelo == "" {
		r.Modelo = extraction.ModeloPadrao(r.Provider)
	}
	if !extraction.ModeloValido(r.Provider, r.Modelo) {
		return fmt.Errorf("%w para %s: %s",
			apperr.ErrModeloInvalido, extraction.NomeAmigavel(r.Provider), r.Modelo)
	}

	return nil
}

// ProcessUpload extrai a folha do arquivo, ajusta os horários faltantes,
// classifica os dias e salva o mês.
func (s *TimesheetService) ProcessUpload(ctx context.Context, req UploadRequest) (*models.ProcessResponse, error) {
	if err := req.normalizar(); err != nil {
		return nil, err
	}

	slog.Info("processando upload",
		"arquivo", req.Filename,
		"mime", req.MimeType,
		"bytes", len(req.Arquivo),
		"provedor", req.Provider,
		"modelo", req.Modelo,
	)

	extractor, err := s.extractorFactory.Create(req.Provider, req.APIKey, req.Modelo)
	if err != nil {
		return nil, err
	}

	timesheet, err := extractor.Extract(ctx, req.Arquivo, req.MimeType)
	if err != nil {
		// Já vem classificado como ErrProvedorRecusou ou ErrExtracaoFalhou;
		// envolver de novo só somaria ruído à mensagem que o usuário lê.
		return nil, err
	}

	// Preencher MesAno se estiver ausente ou inválido usando o nome do arquivo
	if !s.IsValidMesAno(timesheet.MesAno) {
		if data := s.ExtractDateFromFilename(req.Filename); data != "" {
			timesheet.MesAno = data
			slog.Info("mês inferido do nome do arquivo", "arquivo", req.Filename, "mes_ano", data)
		}
	}

	// Realinhar a coluna de observações usando o calendário real como âncora.
	// Precisa vir antes do ajuste, pois é a observação que define se o dia pode
	// ter horário gerado (ex: expediente reduzido não pode).
	extraction.AlignObservacoes(timesheet)

	adjusted := extraction.NewRulesAdjuster(s.engine).Adjust(timesheet)

	for i := range adjusted.Dias {
		adjusted.Dias[i].Tipo = s.engine.ClassifyDay(&adjusted.Dias[i])
	}

	resp := &models.ProcessResponse{
		Timesheet: *adjusted,
		Summary:   s.engine.CalculateSummary(adjusted.Dias),
	}

	s.autoSalvar(adjusted)
	return resp, nil
}

// autoSalvar grava o mês recém-processado. Falha aqui não invalida a extração:
// o usuário já tem o resultado na tela e pode salvar de novo.
func (s *TimesheetService) autoSalvar(ts *models.Timesheet) {
	if ts.MesAno == "" {
		return
	}

	dias := make([]models.MonthDayRecord, len(ts.Dias))
	for i, d := range ts.Dias {
		dias[i] = models.MonthDayRecord{DayRecord: d}
	}

	err := s.repo.Save(models.MonthData{
		MesAno:   ts.MesAno,
		Servidor: ts.Servidor,
		Dias:     dias,
	})
	if err != nil {
		slog.Warn("não foi possível auto-salvar o mês", "mes_ano", ts.MesAno, "erro", err)
	}
}

// DetectMimeType descobre o tipo do arquivo pelo nome ou pelos magic bytes.
func DetectMimeType(filename string, data []byte) string {
	switch lower := strings.ToLower(filename); {
	case strings.HasSuffix(lower, ".png"):
		return "image/png"
	case strings.HasSuffix(lower, ".jpg"), strings.HasSuffix(lower, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(lower, ".webp"):
		return "image/webp"
	case strings.HasSuffix(lower, ".pdf"):
		return "application/pdf"
	}

	if len(data) >= 4 {
		switch {
		case data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47:
			return "image/png"
		case data[0] == 0xFF && data[1] == 0xD8:
			return "image/jpeg"
		case data[0] == 0x25 && data[1] == 0x50 && data[2] == 0x44 && data[3] == 0x46:
			return "application/pdf"
		}
	}

	return "application/octet-stream"
}
