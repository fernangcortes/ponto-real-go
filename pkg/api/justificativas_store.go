package api

import (
	"sync"

	"github.com/fernangcortes/ponto-real-go/pkg/models"
	"github.com/fernangcortes/ponto-real-go/pkg/repository"
)

// justificativasStore guarda a biblioteca de frases com acesso seguro entre
// goroutines, pelo mesmo motivo do settingsStore: o servidor atende cada
// requisição na sua própria goroutine, e POST /api/justificativas escreve
// enquanto GET lê.
type justificativasStore struct {
	mu    sync.RWMutex
	repo  repository.JustificativasRepository
	atual models.BibliotecaJustificativas
}

// newJustificativasStore carrega a biblioteca salva. Falha na leitura é
// esperada na primeira execução: segue com a biblioteca vazia.
func newJustificativasStore(repo repository.JustificativasRepository) *justificativasStore {
	s := &justificativasStore{repo: repo}
	if carregada, err := repo.Load(); err == nil {
		s.atual = carregada
	}
	return s
}

// Get devolve uma cópia da biblioteca, segura para uso fora do lock.
//
// A cópia é do slice, não só do struct: devolver models.BibliotecaJustificativas
// por valor ainda entregaria o mesmo array por baixo, e quem recebesse poderia
// escrever nele enquanto outra goroutine lê.
func (s *justificativasStore) Get() models.BibliotecaJustificativas {
	s.mu.RLock()
	defer s.mu.RUnlock()

	copia := s.atual
	copia.Frases = make([]models.Justificativa, len(s.atual.Frases))
	copy(copia.Frases, s.atual.Frases)
	return copia
}

// Substituir troca a biblioteca inteira pela lista recebida e grava.
//
// Não há fusão com o que já estava: o cliente manda o estado completo, e é
// assim que apagar uma frase funciona. Fundir aqui ressuscitaria toda frase
// excluída na gravação seguinte.
func (s *justificativasStore) Substituir(frases []models.Justificativa) (models.BibliotecaJustificativas, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.atual.Frases = repository.NormalizarFrases(frases)

	if err := s.repo.Save(s.atual); err != nil {
		return s.atual, err
	}

	// Reler o que foi gravado alinha o UpdatedAt que o repositório carimbou;
	// sem isso o store devolveria uma data mais velha que a do disco.
	if salva, err := s.repo.Load(); err == nil && !salva.UpdatedAt.IsZero() {
		s.atual = salva
	}

	copia := s.atual
	copia.Frases = make([]models.Justificativa, len(s.atual.Frases))
	copy(copia.Frases, s.atual.Frases)
	return copia, nil
}
