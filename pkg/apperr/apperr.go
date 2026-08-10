// Package apperr reúne os erros de domínio que a camada HTTP precisa
// distinguir para escolher o status certo.
//
// Ficam num pacote próprio porque são produzidos em três camadas diferentes
// (extração, serviço, repositório) e consumidos num lugar só — a tabela de
// mapeamento erro→status em pkg/api. Espalhá-los pelos pacotes de origem faria
// essa tabela importar tudo e tornaria fácil esquecer um caso, que é
// exatamente como toda falha virava 500 antes.
//
// Uso: envolva com %w para preservar o detalhe legível pelo usuário.
//
//	fmt.Errorf("%w: %s", apperr.ErrArquivoInvalido, mimeType)
package apperr

import "errors"

var (
	// ErrChaveAusente: o provedor de IA selecionado não tem chave configurada.
	ErrChaveAusente = errors.New("chave da API não configurada")

	// ErrProvedorInvalido: provedor de extração desconhecido.
	ErrProvedorInvalido = errors.New("provedor de extração não suportado")

	// ErrModeloInvalido: o modelo pedido não existe no catálogo do provedor.
	ErrModeloInvalido = errors.New("modelo inválido")

	// ErrArquivoInvalido: tipo de arquivo que não dá para enviar ao provedor.
	ErrArquivoInvalido = errors.New("tipo de arquivo não suportado")

	// ErrRequisicaoInvalida: payload malformado ou incompleto.
	ErrRequisicaoInvalida = errors.New("requisição inválida")

	// ErrMesNaoEncontrado: nenhum mês salvo com o identificador pedido.
	ErrMesNaoEncontrado = errors.New("mês não encontrado")

	// ErrProvedorRecusou: a API de IA respondeu com erro próprio — chave
	// inválida, crédito insuficiente, payload recusado.
	//
	// Distinguir isso de falha do servidor importa: são problemas que o
	// usuário resolve sozinho nas Configurações, e mandar 500 escondia isso.
	ErrProvedorRecusou = errors.New("o provedor de IA recusou a requisição")

	// ErrExtracaoFalhou: o provedor foi consultado mas não devolveu uma folha
	// utilizável (resposta vazia, JSON inválido, zero dias) depois das
	// tentativas. Diferente de ErrProvedorRecusou: aqui não há o que o usuário
	// configurar, resta tentar outro modelo ou outro arquivo.
	ErrExtracaoFalhou = errors.New("não foi possível ler a folha de ponto")
)
