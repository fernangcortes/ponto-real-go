// Frase padrão da justificativa e a biblioteca de frases do usuário.
//
// Separado do domain.js porque depende de localStorage: manter o domínio livre
// de APIs de navegador é o que permite testá-lo com `node --test`. O que aqui é
// função pura — montagem da frase, lacunas, ordenação — continua testável.

import { minutosTrabalhados, classifyDay } from './domain.js';
import { CARGA_POR_ATO } from './config.js';
import { isTimeValid } from './util.js';

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

// PREFIXO_DATA reconhece o "DD/MM/AAAA - " que abre toda linha de justificativa.
const PREFIXO_DATA = /^\s*\d{2}\/\d{2}\/\d{4}\s*-\s*/;

// semPrefixoDeData tira a data do começo da frase.
//
// O campo da tela mostra a linha inteira, com data, porque é o que o usuário
// copia para o SEI. Mas o que se GUARDA é só o texto: uma frase com "10/07/2026"
// grudado nela não serve para reaproveitar em agosto, nem para entrar na
// biblioteca. A data é reposta na hora de exibir.
export const semPrefixoDeData = (texto) => (texto || '').replace(PREFIXO_DATA, '').trim();

// comPrefixoDeData devolve a linha como ela vai para o documento. Frase vazia
// não vira uma linha só com a data pendurada.
export const comPrefixoDeData = (dataFmt, texto) => {
    const limpo = semPrefixoDeData(texto);
    return limpo ? `${dataFmt} - ${limpo}` : '';
};

// --- Lacunas ---
//
// Uma frase guardada pode citar números que mudam de dia para dia: "cumpri a
// jornada de 4h exigida pelo ato" fica errada no dia em que o ato exigiu 2h.
// As lacunas resolvem isso — a frase guarda {jornada} e o valor entra na hora.

// LACUNAS documenta o que cada marcador vira. É o que a tela mostra ao usuário:
// marcador que ninguém sabe que existe não é usado por ninguém.
export const LACUNAS = [
    { marcador: '{jornada}', ajuda: 'a jornada exigida no dia (ex.: 4h)' },
    { marcador: '{trabalhado}', ajuda: 'o total efetivamente trabalhado (ex.: 4h19)' },
    { marcador: '{data}', ajuda: 'a data do dia (ex.: 03/07/2026)' },
];

// duracaoHumana formata minutos como "4h" ou "4h19" — como se fala, e não como
// se lê num relógio de ponto. A frase é lida por uma pessoa, não por um sistema.
export const duracaoHumana = (min) => {
    if (!(min > 0)) return '';
    const h = Math.floor(min / 60);
    const m = min % 60;
    if (!h) return `${m}min`;
    return m ? `${h}h${String(m).padStart(2, '0')}` : `${h}h`;
};

// aplicarLacunas troca os marcadores pelos valores do dia.
//
// Marcador sem valor correspondente é apagado junto com o espaço que o precede,
// para a frase não sair com um "{jornada}" cru dentro de um documento assinado.
export const aplicarLacunas = (texto, { jornada, trabalhado, data } = {}) => {
    const valores = {
        '{jornada}': duracaoHumana(jornada),
        '{trabalhado}': duracaoHumana(trabalhado),
        '{data}': data || '',
    };

    return LACUNAS.reduce((frase, { marcador }) => {
        const valor = valores[marcador];
        // Sem valor, leva embora o espaço à esquerda: "de {jornada} exigida"
        // vira "de exigida" e não "de  exigida".
        const alvo = valor ? marcador : `\\s*${marcador}`;
        return frase.replace(new RegExp(alvo, 'g'), valor);
    }, texto || '');
};

// --- Ordem da biblioteca ---

// frasesParaOTipo ordena a biblioteca para o dia que está sendo preenchido.
//
// A lista não filtra nada: toda frase continua disponível, porque nenhuma regra
// impede usar num dia útil o que se escreveu numa dispensa. O que muda é a
// ordem — as do mesmo tipo sobem, e entre elas vence a mais usada.
export const frasesParaOTipo = (frases, tipo) =>
    [...(frases || [])].sort((a, b) => {
        const mesmoTipoA = a.tipo === tipo ? 0 : 1;
        const mesmoTipoB = b.tipo === tipo ? 0 : 1;
        if (mesmoTipoA !== mesmoTipoB) return mesmoTipoA - mesmoTipoB;
        return (b.usos || 0) - (a.usos || 0);
    });

// sugestaoParaOTipo é a frase que aparece em cinza no campo vazio.
//
// Só sugere o que já foi escrito NAQUELE tipo de dia: propor a frase de um dia
// ajustado num dia de dispensa seria pior que não propor nada. Devolve '' se
// não houver.
export const sugestaoParaOTipo = (frases, tipo) => {
    const doTipo = (frases || []).filter((f) => f.tipo === tipo);
    if (!doTipo.length) return '';
    return doTipo.reduce((melhor, f) => ((f.usos || 0) > (melhor.usos || 0) ? f : melhor)).texto;
};

// --- Frase automática do dia de jornada reduzida por ato ---

// turnosDoDia descreve o expediente efetivamente batido: "das 09:26 às 13:45".
// Só entram turnos completos — um par pela metade não descreve nada.
const turnosDoDia = (d) => {
    const partes = [];
    if (isTimeValid(d.e1) && isTimeValid(d.s1)) partes.push(`das ${d.e1} às ${d.s1}`);
    if (isTimeValid(d.e2) && isTimeValid(d.s2)) partes.push(`das ${d.e2} às ${d.s2}`);
    return partes.join(' e ');
};

// justificativaDeAto monta sozinha a frase do dia de dispensa ou de expediente
// reduzido em que a jornada exigida pelo ato foi cumprida.
//
// É a contraparte da frase de ponto não registrado: naquele caso o sistema
// explica um horário que ele mesmo inventou; aqui explica um dia em que nada
// foi inventado e o saldo fecha mesmo assim — que sem uma frase apareceria no
// documento como uma linha muda.
//
// Devolve '' quando não há o que AFIRMAR, e o silêncio é deliberado nos dois
// casos:
//
//   - jornada do ato ainda não informada: sem ela não há contra o que comparar,
//     e o ⚠️ da linha já está pedindo esse número;
//   - jornada não cumprida: o sistema não inventa desculpa para um déficit. O
//     campo fica para o usuário escrever o que de fato aconteceu.
//
// A frase vai para um documento assinado por servidor público: só pode afirmar
// o que o cálculo sustenta.
export const justificativaDeAto = (d, tipo = classifyDay(d)) => {
    if (!CARGA_POR_ATO.includes(tipo)) return '';
    if (!(d.carga > 0)) return '';

    const trabalhado = minutosTrabalhados(d);
    if (trabalhado < d.carga) return '';

    const nome = tipo === 'dispensa' ? 'dispensa' : 'expediente reduzido';
    const turnos = turnosDoDia(d);
    const cumprido = trabalhado > d.carga
        ? `cumpri ${duracaoHumana(trabalhado)} da jornada de ${duracaoHumana(d.carga)} exigida pelo ato`
        : `cumpri integralmente a jornada de ${duracaoHumana(d.carga)} exigida pelo ato`;

    return `Dia de ${nome}: ${cumprido}${turnos ? `, com expediente ${turnos}` : ''}.`;
};

// motivoDoSilencio diz, no próprio campo vazio, por que o sistema não escreveu
// a frase sozinho.
//
// Sem isto o silêncio de justificativaDeAto é indistinguível de um defeito: o
// campo aparece em branco e não há nada na tela ligando essa ausência à jornada
// do ato que falta informar três colunas ao lado. Quem sabe a regra é o
// sistema; então é ele que tem de contar.
export const motivoDoSilencio = (d, tipo = classifyDay(d)) => {
    if (!CARGA_POR_ATO.includes(tipo)) return '';

    const nome = tipo === 'dispensa' ? 'da dispensa' : 'do decreto';
    if (!(d.carga > 0)) {
        return `Informe a jornada ${nome} nesta linha e o sistema escreve a justificativa sozinho.`;
    }
    if (minutosTrabalhados(d) < d.carga) {
        return 'A jornada exigida não fecha neste dia — descreva o que aconteceu.';
    }
    return '';
};

// textoDaJustificativa devolve a linha completa que o dia leva ao documento.
//
// O que o usuário escreveu vence a frase automática. Campo vazio volta à
// automática: apagar o texto é como se recupera a frase padrão, e não um jeito
// de o dia ficar mudo no documento.
//
// Existe para que a tela e o "Gerar SEI" cheguem à mesma frase. Enquanto cada
// um montava a sua, o documento discordava do que estava na tela — e era o
// documento que ia assinado.
export const textoDaJustificativa = (d, dataFmt, faltantes, template = getJustTemplate()) => {
    const escrita = semPrefixoDeData(d.justManual);
    if (escrita) {
        // As lacunas são resolvidas na EXIBIÇÃO, nunca na gravação: guardar o
        // texto já preenchido faria a frase valer só para o dia em que foi
        // escrita, que é exatamente o que a biblioteca existe para evitar.
        const preenchida = aplicarLacunas(escrita, {
            jornada: d.carga,
            trabalhado: minutosTrabalhados(d),
            data: dataFmt,
        });
        return `${dataFmt} - ${preenchida}`;
    }
    if (faltantes.length > 0) return montarJustificativa(dataFmt, faltantes, template);

    // Nada foi inventado neste dia, mas ele ainda pode ter o que dizer: a
    // dispensa cumprida se explica sozinha.
    const doAto = justificativaDeAto(d);
    if (doAto) return `${dataFmt} - ${doAto}`;

    return '';
};
