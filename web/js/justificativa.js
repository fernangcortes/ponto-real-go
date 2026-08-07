// Frase padrão da justificativa, configurável pelo usuário.
//
// Separado do domain.js porque depende de localStorage: manter o domínio livre
// de APIs de navegador é o que permite testá-lo com `node --test`.

// Fica antes dos horários faltantes. Trocar para "esqueci de registrar" produz
// "esqueci de registrar a saída".
export const JUST_TEMPLATE_PADRAO = 'O ponto não registrou';
const JUST_TEMPLATE_KEY = 'pontoReal.justTemplate';

export const getJustTemplate = () => {
    const salvo = (localStorage.getItem(JUST_TEMPLATE_KEY) || '').trim();
    return salvo || JUST_TEMPLATE_PADRAO;
};

// Texto vazio remove a chave em vez de gravar "", para o getJustTemplate voltar
// ao padrão em vez de montar uma frase sem verbo.
export const salvarJustTemplate = (texto) => {
    if (texto) {
        localStorage.setItem(JUST_TEMPLATE_KEY, texto);
    } else {
        localStorage.removeItem(JUST_TEMPLATE_KEY);
    }
};

// montarJustificativa produz "DD/MM/AAAA - <texto padrão> a entrada e a saída."
//
// O template é injetável para que a montagem da frase — que é regra — possa ser
// testada sem localStorage.
export const montarJustificativa = (dataFmt, faltantes, template = getJustTemplate()) => {
    const lista = faltantes.join(', ').replace(/,([^,]*)$/, ' e$1');
    return `${dataFmt} - ${template} ${lista}.`;
};
