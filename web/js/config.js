// Estado de configuração da sessão.
//
// É um objeto mutável de propósito: os módulos leem CONFIG.mesAno o tempo todo e
// o carregamento de mês o reescreve. Exportar um objeto (e não valores soltos)
// mantém essa leitura viva sem cada módulo precisar de um setter.
export const CONFIG = {
    // Preenchida a partir de /api/health: o número mora só no backend
    // (pkg/version), senão volta a divergir entre HTML, JS e Go.
    version: '',
    mesAno: '',
    mesNome: '',
    apiBase: '',
};

// resolverApiBase descobre para onde mandar as chamadas.
//
// Abrir o index.html direto do disco (file://) ou servi-lo por outra porta que
// não a do Go exige apontar para o servidor local explicitamente.
export const resolverApiBase = (location) => {
    const soltoNoDisco = location.protocol === 'file:';
    const outraPorta =
        location.hostname === 'localhost' && location.port !== '8080' && location.port !== '';
    return soltoNoDisco || outraPorta ? 'http://localhost:8080' : '';
};

// Carga diária padrão, em minutos. Espelha carga_horaria_diaria_min do
// rules.json; o backend é a fonte de verdade e a Fase 4 elimina esta cópia.
export const CARGA_DIARIA = 480;

// CARGA_POR_ATO lista os tipos cuja jornada exigida vem de um ato específico
// (decreto de expediente reduzido, ato de dispensa) e não da carga global.
//
// Cada ato define a sua: pode liberar o dia inteiro, meio período ou a partir
// de determinado horário. Por isso a jornada é informada pelo usuário, dia a
// dia. Antes a dispensa usava 4h fixas no front-end e 8h no backend — número
// que não vinha de lugar nenhum, em duas versões que discordavam.
export const CARGA_POR_ATO = ['reduzido', 'dispensa'];

// Atalhos oferecidos no seletor de jornada, em minutos.
export const PRESETS_CARGA = [
    { valor: 480, rotulo: 'Dia inteiro — 8h' },
    { valor: 240, rotulo: 'Meio período — 4h' },
    { valor: 120, rotulo: '2 horas' },
];

export const CAMPOS_HORARIO = ['e1', 's1', 'e2', 's2'];

export const NOMES_CAMPOS = ['a entrada', 'a saída do almoço', 'a entrada do almoço', 'a saída'];

export const DIAS_SEMANA = ['Dom', 'Seg', 'Ter', 'Qua', 'Qui', 'Sex', 'Sáb'];

export const MESES_NOMES = [
    '',
    'Janeiro',
    'Fevereiro',
    'Março',
    'Abril',
    'Maio',
    'Junho',
    'Julho',
    'Agosto',
    'Setembro',
    'Outubro',
    'Novembro',
    'Dezembro',
];

// Tipos que o usuário pode marcar como dia neutro: não geram saldo nem déficit.
export const TIPOS_NEUTROS = ['feriado', 'folga', 'fds', 'convocacao'];

// Tradução do valor do seletor de tipo de dia para a classificação usada nos
// cálculos. "util" não está aqui de propósito — ver classifyDay.
export const TIPO_POR_SELECAO = {
    fds: 'folga',
    folga: 'folga',
    feriado: 'recesso',
    convocacao: 'recesso',
    dispensa: 'dispensa',
    reduzido: 'reduzido',
    ferias: 'ferias',
};

// Rótulos do seletor de tipo de dia, com a explicação que vai no tooltip.
export const TIPOS_DE_DIA = [
    {
        valor: 'util',
        rotulo: 'Útil',
        ajuda: 'Dia normal de trabalho: o saldo sai da diferença entre o batido e a jornada.',
    },
    { valor: 'fds', rotulo: 'FDS', ajuda: 'Sábado ou domingo. Não gera saldo nem déficit.' },
    {
        valor: 'dispensa',
        rotulo: 'Dispensa',
        ajuda: 'Dispensa concedida por ato (curso, convocação). Informe ao lado a jornada que o ato ainda exige.',
    },
    {
        valor: 'ferias',
        rotulo: 'Férias',
        ajuda: 'Férias homologadas. Ausência autorizada: não conta como falta nem gera déficit.',
    },
    {
        valor: 'feriado',
        rotulo: 'Feriado',
        ajuda: 'Feriado, recesso ou ponto facultativo. Não gera saldo nem déficit.',
    },
    { valor: 'folga', rotulo: 'Folga', ajuda: 'Folga concedida. Não gera saldo nem déficit.' },
    {
        valor: 'convocacao',
        rotulo: 'Convocação',
        ajuda: 'Convocação institucional. Não gera saldo nem déficit.',
    },
    {
        valor: 'reduzido',
        rotulo: 'Reduzido',
        ajuda: 'Expediente reduzido por decreto. Informe ao lado a jornada exigida no dia.',
    },
];
