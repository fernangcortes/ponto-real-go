// Regra de negócio do front-end: classificação de dias e apuração de saldos.
//
// Sem DOM e sem fetch — só depende de CONFIG.mesAno para saber em que mês está.
// É o que torna possível testar a aritmética com `node --test`, e é também o
// que a Fase 4 vai substituir por chamadas a /api/process.

import { CONFIG, CAMPOS_HORARIO, NOMES_CAMPOS, DIAS_SEMANA, CARGA_DIARIA, CARGA_POR_ATO, TIPOS_NEUTROS, TIPO_POR_SELECAO } from './config.js';
import { t2m, isTimeValid, parseOrigSaldo, normalizeObs } from './util.js';

// --- Calendário real ---

// getRealWeekday devolve o dia da semana pelo calendário, e não pelo campo "w"
// lido do documento — que às vezes vem deslocado.
export const getRealWeekday = (dayNum) => {
    if (!CONFIG.mesAno) return null;
    const [mm, yyyy] = CONFIG.mesAno.split('/');
    const date = new Date(parseInt(yyyy), parseInt(mm) - 1, dayNum);
    return DIAS_SEMANA[date.getDay()];
};

export const isWeekend = (d) => {
    const realW = getRealWeekday(d.d);
    if (realW) return realW === 'Sáb' || realW === 'Dom';
    // Sem mês definido, resta o campo lido do documento.
    return d.w === 'Sáb' || d.w === 'Sab' || d.w === 'Dom';
};

// Expediente reduzido por decreto: o ponto batido já vale como cumprido, então
// o sistema nunca gera horário nesses dias.
export const isExpedienteReduzido = (d) => {
    const t = normalizeObs(d.mot) + ' ' + normalizeObs(d.ocor);
    return t.includes('EXPEDIENTE REDUZIDO') || t.includes('HORARIO REDUZIDO') || t.includes('JORNADA REDUZIDA');
};

// --- Tipo de dia ---

// detectarTipoDia deriva o tipo a partir da observação lida e do calendário.
// É função PURA: só olha o dia e devolve o valor.
//
// Antes essa detecção acontecia dentro do renderTables e GRAVAVA o resultado em
// d.dayTypeOverride. Duas consequências ruins:
//   1. A detecção automática virava override "manual" e era salva como tal, de
//      modo que uma correção futura na regra nunca alcançava os meses salvos.
//   2. Escolher "Útil" num dia com DISPENSA no motivo não funcionava: o
//      changeDayType zerava o override, o render seguinte redetectava dispensa
//      e reescrevia por cima. O usuário não conseguia desfazer a detecção.
//
// A ORDEM importa e é a do antigo classifyDay, não a da antiga auto-detecção do
// render — as duas divergiam. O render checava fim de semana antes da
// observação, o classifyDay depois; num sábado com "DISPENSA" no motivo o
// seletor mostrava "FDS" enquanto o cálculo tratava como dispensa. Mantida a
// ordem do classifyDay, que é a que sempre determinou os saldos.
export const detectarTipoDia = (d) => {
    const motivo = normalizeObs(d.mot);
    // Expediente reduzido vence tudo: um decreto pode reduzir jornada em dia
    // útil, e é o caso que não pode gerar horário automático.
    if (isExpedienteReduzido(d)) return 'reduzido';
    // FERIAS antes de DISPENSA: são coisas distintas e ambas aparecem na coluna
    // de observações. Férias homologadas são ausência autorizada, não falta.
    if (motivo.includes('FERIAS')) return 'ferias';
    if (motivo.includes('DISPENSA')) return 'dispensa';
    if (motivo.includes('RECESSO') || motivo.includes('FERIADO') || motivo.includes('FACULTATIVO')) return 'feriado';
    if (isWeekend(d)) return 'fds';
    return 'util';
};

// tipoSelecionado é o que o seletor da linha mostra: a escolha do usuário
// quando existe, senão a detecção automática.
export const tipoSelecionado = (d) => d.dayTypeOverride || detectarTipoDia(d);

export const classifyDay = (d) => {
    // "Útil" não está no mapa de propósito: escolhê-lo explicitamente derruba a
    // detecção e leva o dia à contagem normal de batimentos, que é como o
    // usuário desfaz uma detecção equivocada.
    const especial = TIPO_POR_SELECAO[tipoSelecionado(d)];
    if (especial) return especial;

    const pontos = CAMPOS_HORARIO.filter((f) => isTimeValid(d[f])).length;

    if (pontos === 4) return 'completo';
    if (pontos > 0) return 'parcial';
    if (!isWeekend(d) && d.saldo === '-08:00') return 'falta';
    return 'folga';
};

// --- Ocorrência (seção do documento SEI) ---

// Por padrão a ocorrência é derivada de d.o (existe algum horário gerado).
// d.ocorrenciaManual permite sobrepor: true força inclusão, false força
// exclusão, null/undefined mantém o comportamento automático.
export const isAutoOccurrence = (d) => !!(d.o && d.o.includes(0));

export const isOccurrenceDay = (d) => {
    if (d.ocorrenciaManual === true) return true;
    if (d.ocorrenciaManual === false) return false;
    return isAutoOccurrence(d);
};

// camposFaltantes lista os horários que o sistema gerou (bloqueio === 0) e que
// portanto precisam de justificativa.
export const camposFaltantes = (d, isDispensa) => {
    const bloqueio = d.o || [1, 1, 1, 1];
    return NOMES_CAMPOS.filter((_, fi) => {
        if (bloqueio[fi] !== 0) return false;
        // Dispensa: só listar se o campo foi preenchido pelo usuário.
        return !isDispensa || isTimeValid(d[CAMPOS_HORARIO[fi]]);
    });
};

// deveAparecerNaOcorrencia decide se o dia entra nas tabelas de ocorrência e
// justificativa do documento SEI.
export const deveAparecerNaOcorrencia = (d, tipo) => {
    if (!isOccurrenceDay(d)) return false;

    const isDispensa = tipo === 'dispensa';
    const isManual = d.ocorrenciaManual === true;
    // Ocorrência manual é sempre exibida: o usuário pediu explicitamente.
    if (!isDispensa || isManual) return true;

    // Dispensa detectada automaticamente só aparece se algum campo editável
    // foi preenchido.
    const bloqueio = d.o || [1, 1, 1, 1];
    return CAMPOS_HORARIO.some((f, fi) => bloqueio[fi] === 0 && isTimeValid(d[f]));
};

// --- Saldos ---

export const diaCompleto = (d) => CAMPOS_HORARIO.every((f) => isTimeValid(d[f]));

// minutosTrabalhados soma os turnos que estiverem completos.
export const minutosTrabalhados = (d) => {
    let total = 0;
    if (isTimeValid(d.e1) && isTimeValid(d.s1)) total += t2m(d.s1) - t2m(d.e1);
    if (isTimeValid(d.e2) && isTimeValid(d.s2)) total += t2m(d.s2) - t2m(d.e2);
    return total;
};

// saldoDoDia devolve { diff, contribui, trabalhado }:
//   diff       — saldo a exibir na célula, ou null se não há o que calcular
//   contribui  — quanto o dia soma ao total do mês
//   trabalhado — minutos a exibir na coluna E/S, ou null para não mexer nela
//
// diff e contribui não são a mesma coisa: dia neutro mostra 00:00 mas não entra
// no total, e dia sem cálculo possível herda o saldo lido da ficha.
export const saldoDoDia = (d, tipo) => {
    if (TIPOS_NEUTROS.includes(d.dayTypeOverride)) {
        return { diff: 0, contribui: 0, trabalhado: null };
    }

    const trabalhado = minutosTrabalhados(d);

    // Dias cuja jornada exigida vem de um ato — decreto de expediente reduzido,
    // ato de dispensa — e portanto varia caso a caso.
    //
    // Sem a carga informada o dia é neutro: não herda o saldo negativo da ficha
    // (apurado contra a jornada cheia) nem arbitra uma jornada própria. Antes a
    // dispensa usava 4h fixas, número que não vinha de lugar nenhum e divergia
    // do backend, que usava 8h.
    if (CARGA_POR_ATO.includes(tipo)) {
        const diff = d.carga > 0 ? trabalhado - d.carga : 0;
        return { diff, contribui: diff, trabalhado: trabalhado > 0 ? trabalhado : null };
    }

    if (diaCompleto(d)) {
        const diff = trabalhado - CARGA_DIARIA;
        return { diff, contribui: diff, trabalhado };
    }

    if (tipo === 'falta') {
        return { diff: -CARGA_DIARIA, contribui: -CARGA_DIARIA, trabalhado: null };
    }

    return { diff: null, contribui: parseOrigSaldo(d.saldo), trabalhado: null };
};

// calcularTotais percorre o mês uma vez e devolve tudo que a tela precisa.
// Sem DOM aqui: dá para conferir a conta sem abrir o navegador.
export const calcularTotais = (dias) => {
    const totais = {
        saldoOficial: 0,
        saldoReal: 0,
        faltas: 0,
        ajustados: 0,
        completos: 0,
        porDia: [],
    };

    dias.forEach((d, idx) => {
        const tipo = classifyDay(d);
        const saldo = saldoDoDia(d, tipo);

        if (tipo === 'falta') totais.faltas++;
        if (tipo === 'completo') totais.completos++;
        if (isOccurrenceDay(d)) totais.ajustados++;

        totais.saldoReal += saldo.contribui;
        // O saldo lido da ficha sempre entra no total "oficial".
        totais.saldoOficial += parseOrigSaldo(d.saldo);

        totais.porDia.push({ d, idx, tipo, ...saldo });
    });

    return totais;
};

// --- Conversão entre o formato do backend e o do front-end ---
//
// O backend usa snake_case (day_type_override) e o front-end camelCase
// (dayTypeOverride). A tradução ficava escrita campo a campo em TRÊS lugares —
// carregar upload, carregar mês salvo e salvar — que precisavam ser mantidos em
// sincronia com o MonthDayRecord do Go. Agora é um par de funções só.

export const deWire = (d) => ({
    d: d.d,
    w: d.w || '',
    e1: d.e1 || '',
    s1: d.s1 || '',
    e2: d.e2 || '',
    s2: d.s2 || '',
    es: d.es || '',
    saldo: d.saldo || '',
    saldo_real: d.saldo_real || '',
    ocor: d.ocor || '',
    mot: d.mot || '',
    o: d.o || undefined,
    tipo: d.tipo || undefined,
    carga: d.carga || undefined,
    revisar: d.revisar || false,
    revisar_motivo: d.revisar_motivo || '',
    dayTypeOverride: d.day_type_override || null,
    // typeof: false é valor válido (exclusão manual da ocorrência) e não pode
    // ser confundido com "ausente".
    ocorrenciaManual: typeof d.ocorrencia_manual === 'boolean' ? d.ocorrencia_manual : null,
    justManual: d.justificativa_manual || '',
});

export const paraWire = (d) => ({
    d: d.d,
    w: d.w,
    e1: d.e1,
    s1: d.s1,
    e2: d.e2,
    s2: d.s2,
    es: d.es,
    saldo: d.saldo,
    saldo_real: d.saldo_real || '',
    ocor: d.ocor,
    mot: d.mot,
    o: d.o || undefined,
    tipo: d.tipo || undefined,
    carga: d.carga || undefined,
    // revisar/revisar_motivo não são salvos: o backend os recalcula na leitura,
    // senão um aviso já resolvido ficaria colado no mês para sempre.
    day_type_override: d.dayTypeOverride || undefined,
    // Não usar `||`: false é exclusão manual e viraria undefined, perdendo a
    // escolha do usuário ao salvar.
    ocorrencia_manual: typeof d.ocorrenciaManual === 'boolean' ? d.ocorrenciaManual : undefined,
    justificativa_manual: d.justManual || undefined,
});
