// Biblioteca de frases de justificativa: servidor com cópia local.
//
// A política é uma só e vale para tudo aqui: **o servidor é a verdade**. O
// localStorage é cache de leitura, para a lista aparecer instantaneamente e
// continuar existindo quando o servidor não responde; e é fila de escrita, para
// que uma frase salva offline não se perca.
//
// A alternativa — fundir os dois lados — foi descartada porque torna a exclusão
// impossível: a frase apagada num navegador voltaria na próxima fusão com o
// outro. Com o servidor mandando, apagar é gravar a lista sem ela.

const CHAVE_CACHE = 'pontoReal.biblioteca';
const CHAVE_PENDENTE = 'pontoReal.bibliotecaPendente';

// Estado em memória. É o que a tela lê a cada render, então precisa estar
// sempre atualizado — antes mesmo de o servidor confirmar.
let frases = [];

export const frasesAtuais = () => frases;

// --- Cache local ---

const lerCache = () => {
    try {
        const bruto = localStorage.getItem(CHAVE_CACHE);
        const lista = bruto ? JSON.parse(bruto) : [];
        return Array.isArray(lista) ? lista : [];
    } catch {
        // JSON corrompido no localStorage não pode derrubar o app: a biblioteca
        // é uma comodidade, não um pré-requisito para conferir a folha.
        return [];
    }
};

const gravarCache = (lista) => {
    try {
        localStorage.setItem(CHAVE_CACHE, JSON.stringify(lista));
    } catch {
        // Cota estourada ou modo privativo: seguir sem cache é aceitável.
    }
};

const marcarPendente = (pendente) => {
    try {
        if (pendente) localStorage.setItem(CHAVE_PENDENTE, '1');
        else localStorage.removeItem(CHAVE_PENDENTE);
    } catch {
        // Idem.
    }
};

const temPendencia = () => {
    try {
        return localStorage.getItem(CHAVE_PENDENTE) === '1';
    } catch {
        return false;
    }
};

// --- Servidor ---

const enviar = async (apiBase, lista) => {
    const resp = await fetch(`${apiBase}/api/justificativas`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ frases: lista }),
    });
    if (!resp.ok) throw new Error(`servidor respondeu ${resp.status}`);
    const b = await resp.json();
    return Array.isArray(b.frases) ? b.frases : [];
};

// sincronizar grava a lista no servidor e alinha o cache.
//
// A memória e o cache são atualizados ANTES da ida à rede: a lista tem de
// aparecer na tela no instante em que o usuário clica em salvar, não quando o
// servidor confirmar. Se o envio falhar, o estado local continua valendo e a
// pendência fica marcada para a próxima tentativa.
const sincronizar = async (apiBase, lista) => {
    frases = lista;
    gravarCache(lista);

    try {
        frases = await enviar(apiBase, lista);
        gravarCache(frases);
        marcarPendente(false);
        return true;
    } catch {
        marcarPendente(true);
        return false;
    }
};

// carregarBiblioteca busca a lista do servidor no início da sessão.
//
// Havendo escrita pendente de uma sessão offline, ela é reenviada ANTES da
// leitura — senão a resposta do servidor (que não a tem) sobrescreveria a frase
// que ficou só no cache, e o trabalho do usuário sumiria.
export const carregarBiblioteca = async (apiBase) => {
    frases = lerCache();

    if (temPendencia() && frases.length) {
        await sincronizar(apiBase, frases);
        return frases;
    }

    try {
        const resp = await fetch(`${apiBase}/api/justificativas`);
        if (!resp.ok) throw new Error(`servidor respondeu ${resp.status}`);
        const b = await resp.json();
        frases = Array.isArray(b.frases) ? b.frases : [];
        gravarCache(frases);
    } catch {
        // Sem servidor, o cache é o que existe. Não é erro a mostrar: a folha
        // do mês continua inteiramente utilizável sem a biblioteca.
    }

    return frases;
};

// mesmoTexto compara frases como o backend compara: sem caixa e sem espaço nas
// pontas. As duas pontas precisam concordar sobre o que é "a mesma frase",
// senão salvar duas vezes cria uma duplicata que só o servidor funde.
const mesmoTexto = (a, b) => (a || '').trim().toLowerCase() === (b || '').trim().toLowerCase();

// guardarFrase acrescenta uma frase à biblioteca, ou soma um uso se ela já
// estiver lá. Devolve false quando a gravação no servidor falhou — a frase
// continua valendo localmente e será reenviada depois.
export const guardarFrase = async (apiBase, texto, tipo) => {
    const limpo = (texto || '').trim();
    if (!limpo) return true;

    const lista = frases.map((f) => ({ ...f }));
    const existente = lista.find((f) => mesmoTexto(f.texto, limpo));

    if (existente) {
        existente.usos = (existente.usos || 0) + 1;
        // Uma frase salva de novo num dia de dispensa passa a ordenar junto às
        // de dispensa, mesmo tendo nascido sem tipo.
        if (!existente.tipo && tipo) existente.tipo = tipo;
    } else {
        lista.push({ texto: limpo, tipo: tipo || '', usos: 1 });
    }

    return sincronizar(apiBase, lista);
};

// esquecerFrase tira a frase da biblioteca.
export const esquecerFrase = async (apiBase, texto) =>
    sincronizar(
        apiBase,
        frases.filter((f) => !mesmoTexto(f.texto, texto)),
    );
