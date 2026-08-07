// Testes do domínio do front-end. Rodam sem navegador: `npm test`.
//
// Até a Fase 3 esta lógica vivia num escopo global de 2500 linhas e só era
// verificável abrindo a página. É a mesma aritmética que decide o saldo que o
// servidor público vai assinar.

import test from 'node:test';
import assert from 'node:assert/strict';

import { CONFIG, CARGA_DIARIA } from './config.js';
import { t2m, m2t, m2tUnsigned, parseOrigSaldo, isTimeValid, esc, normalizeObs } from './util.js';
import {
    classifyDay, detectarTipoDia, tipoSelecionado, isWeekend,
    isAutoOccurrence, isOccurrenceDay, deveAparecerNaOcorrencia, camposFaltantes,
    minutosTrabalhados, saldoDoDia, calcularTotais, deWire, paraWire,
    avisoDeRevisao, MSG_CARGA_DISPENSA, MSG_CARGA_REDUZIDA,
} from './domain.js';
import { montarJustificativa } from './justificativa.js';

// Junho de 2026: dia 1 é segunda; 6 e 7 são sábado e domingo.
const usarJunho2026 = () => { CONFIG.mesAno = '06/2026'; };

const dia = (over = {}) => ({
    d: 1, w: 'Seg', e1: '', s1: '', e2: '', s2: '',
    es: '', saldo: '', ocor: '', mot: '',
    o: undefined, carga: undefined, dayTypeOverride: null,
    ocorrenciaManual: null, justManual: '',
    ...over,
});

const diaCheio = (over = {}) => dia({ e1: '08:00', s1: '12:00', e2: '13:00', s2: '17:00', ...over });

// --- util ---

test('t2m converte HH:MM em minutos', () => {
    assert.equal(t2m('08:00'), 480);
    assert.equal(t2m('12:30'), 750);
    assert.equal(t2m('00:00'), 0);
    assert.equal(t2m('**:**'), 0);
    assert.equal(t2m(''), 0);
});

test('m2t sempre traz sinal; m2tUnsigned nunca', () => {
    assert.equal(m2t(0), '+00:00');
    assert.equal(m2t(90), '+01:30');
    assert.equal(m2t(-480), '-08:00');
    assert.equal(m2tUnsigned(-480), '08:00');
});

test('parseOrigSaldo lê o saldo como veio da ficha', () => {
    assert.equal(parseOrigSaldo('-08:00'), -480);
    assert.equal(parseOrigSaldo('+01:20'), 80);
    assert.equal(parseOrigSaldo('00:18'), 18);
    assert.equal(parseOrigSaldo(''), 0);
});

test('isTimeValid recusa hora fora da faixa', () => {
    assert.equal(isTimeValid('23:59'), true);
    assert.equal(isTimeValid('24:00'), false);
    assert.equal(isTimeValid('08:60'), false);
    assert.equal(isTimeValid('**:**'), false);
});

test('esc neutraliza os caracteres que quebram um atributo HTML', () => {
    assert.equal(esc('DISPENSA "X"'), 'DISPENSA &quot;X&quot;');
    assert.equal(esc('<b>a</b>'), '&lt;b&gt;a&lt;/b&gt;');
    assert.equal(esc(null), '');
});

test('normalizeObs ignora acento e caixa', () => {
    assert.equal(normalizeObs('Compensação'), 'COMPENSACAO');
    assert.equal(normalizeObs('sábado'), 'SABADO');
});

// --- classificação ---

test('classifyDay conta os batimentos', () => {
    usarJunho2026();
    assert.equal(classifyDay(diaCheio()), 'completo');
    assert.equal(classifyDay(dia({ e1: '08:00' })), 'parcial');
    assert.equal(classifyDay(dia({ saldo: '-08:00' })), 'falta');
    assert.equal(classifyDay(dia()), 'folga');
});

test('fim de semana vem do calendário, não do campo lido', () => {
    usarJunho2026();
    // 6 de junho de 2026 é sábado, mesmo que a ficha diga "Seg".
    assert.equal(isWeekend(dia({ d: 6, w: 'Seg' })), true);
    assert.equal(isWeekend(dia({ d: 8, w: 'Sáb' })), false);
});

test('a ordem da detecção põe a observação na frente do fim de semana', () => {
    usarJunho2026();
    // Regressão: a antiga auto-detecção do render checava fim de semana
    // primeiro, e num sábado com DISPENSA o seletor divergia do cálculo.
    const sabadoComDispensa = dia({ d: 6, mot: 'DISPENSA PARA CURSO' });
    assert.equal(detectarTipoDia(sabadoComDispensa), 'dispensa');
    assert.equal(classifyDay(sabadoComDispensa), 'dispensa');
});

test('expediente reduzido vence qualquer outra detecção', () => {
    usarJunho2026();
    const d = dia({ d: 6, mot: 'EXPEDIENTE REDUZIDO - DEC. 10.925' });
    assert.equal(detectarTipoDia(d), 'reduzido');
});

test('escolher "Útil" desfaz a detecção automática', () => {
    usarJunho2026();
    const d = diaCheio({ mot: 'DISPENSA PARA CURSO' });

    assert.equal(classifyDay(d), 'dispensa', 'sem escolha do usuário, detecta dispensa');

    d.dayTypeOverride = 'util';
    assert.equal(tipoSelecionado(d), 'util');
    assert.equal(classifyDay(d), 'completo', 'com "util" explícito, volta a contar batimentos');
});

test('override do usuário manda sobre a observação', () => {
    usarJunho2026();
    assert.equal(classifyDay(diaCheio({ dayTypeOverride: 'fds' })), 'folga');
    assert.equal(classifyDay(diaCheio({ dayTypeOverride: 'feriado' })), 'recesso');
    assert.equal(classifyDay(diaCheio({ dayTypeOverride: 'dispensa' })), 'dispensa');
});

// --- saldos ---

test('minutosTrabalhados soma só os turnos completos', () => {
    assert.equal(minutosTrabalhados(diaCheio()), 480);
    assert.equal(minutosTrabalhados(dia({ e1: '08:00', s1: '12:00' })), 240);
    assert.equal(minutosTrabalhados(dia({ e1: '08:00' })), 0);
});

test('dia completo gera saldo contra a carga diária', () => {
    usarJunho2026();
    const d = diaCheio({ s2: '17:30' });
    const s = saldoDoDia(d, classifyDay(d));
    assert.equal(s.diff, 510 - CARGA_DIARIA);
    assert.equal(s.contribui, 30);
    assert.equal(s.trabalhado, 510);
});

test('dia neutro mostra 00:00 mas NÃO entra no total', () => {
    usarJunho2026();
    const d = diaCheio({ dayTypeOverride: 'feriado' });
    const s = saldoDoDia(d, classifyDay(d));
    assert.equal(s.diff, 0, 'a célula mostra zero');
    assert.equal(s.contribui, 0, 'e o mês não é afetado');
});

test('expediente reduzido sem carga informada é neutro', () => {
    usarJunho2026();
    // Nunca herda o saldo negativo da ficha, que foi apurado contra a jornada
    // cheia — seria cobrar déficit de um dia que o decreto encurtou.
    const d = dia({ mot: 'EXPEDIENTE REDUZIDO', e1: '08:00', s1: '12:00', saldo: '-04:00' });
    const s = saldoDoDia(d, classifyDay(d));
    assert.equal(s.contribui, 0);
});

test('expediente reduzido com carga do decreto apura contra ela', () => {
    usarJunho2026();
    const d = dia({ mot: 'EXPEDIENTE REDUZIDO', e1: '08:00', s1: '12:00', carga: 180 });
    const s = saldoDoDia(d, classifyDay(d));
    assert.equal(s.contribui, 240 - 180);
});

test('dispensa sem jornada informada é neutra', () => {
    usarJunho2026();
    // A jornada de uma dispensa vem do ato que a concedeu e varia caso a caso.
    // Antes o front-end arbitrava 4h e o backend 8h — dois números que não
    // vinham de lugar nenhum e discordavam entre si.
    const d = dia({ mot: 'DISPENSA PARA CURSO', e1: '08:00', s1: '12:00', saldo: '-04:00' });
    const s = saldoDoDia(d, classifyDay(d));
    assert.equal(s.contribui, 0, 'sem jornada informada, o dia não gera saldo nem déficit');
});

test('dispensa com jornada informada apura contra ela', () => {
    usarJunho2026();
    const d = dia({ mot: 'DISPENSA PARA CURSO', e1: '08:00', s1: '12:00', carga: 240 });
    const s = saldoDoDia(d, classifyDay(d));
    assert.equal(s.contribui, 0, '4h trabalhadas contra 4h exigidas');

    d.carga = 120;
    assert.equal(saldoDoDia(d, classifyDay(d)).contribui, 120, 'com 2h exigidas, sobram 2h');
});

test('férias homologadas não são falta', () => {
    usarJunho2026();
    // Regressão de 72 horas: o backend classificava "Férias" como dia útil sem
    // batimento e descontava a jornada inteira de cada um dos 9 dias.
    const d = dia({ d: 8, mot: 'Férias - Estatutário' });
    assert.equal(detectarTipoDia(d), 'ferias');
    assert.equal(classifyDay(d), 'ferias');
    assert.equal(saldoDoDia(d, classifyDay(d)).contribui, 0);
});

test('"Férias" e "Feriado" não se confundem', () => {
    usarJunho2026();
    assert.equal(detectarTipoDia(dia({ d: 8, mot: 'FERIADO MUNICIPAL' })), 'feriado');
    assert.equal(detectarTipoDia(dia({ d: 8, mot: 'FÉRIAS' })), 'ferias');
});

test('falta desconta a jornada cheia', () => {
    usarJunho2026();
    const d = dia({ saldo: '-08:00' });
    const s = saldoDoDia(d, classifyDay(d));
    assert.equal(s.contribui, -CARGA_DIARIA);
});

test('calcularTotais soma o mês e conta cada tipo', () => {
    usarJunho2026();
    const dias = [
        diaCheio({ d: 1, s2: '17:30' }),                 // +30
        dia({ d: 6 }),                                    // sábado: folga
        dia({ d: 8, saldo: '-08:00' }),                   // falta: -480
        diaCheio({ d: 9 }),                               // 0
    ];

    const t = calcularTotais(dias);
    assert.equal(t.completos, 2);
    assert.equal(t.faltas, 1);
    assert.equal(t.saldoReal, 30 - 480);
    assert.equal(t.saldoOficial, -480);
    assert.equal(t.porDia.length, 4);
    assert.equal(t.porDia[0].tipo, 'completo');
    assert.equal(t.porDia[1].tipo, 'folga');
});

// --- ocorrência ---

test('ocorrência é automática quando há horário gerado', () => {
    const gerado = dia({ o: [1, 0, 1, 1] });
    assert.equal(isAutoOccurrence(gerado), true);
    assert.equal(isOccurrenceDay(gerado), true);

    const original = dia({ o: [1, 1, 1, 1] });
    assert.equal(isOccurrenceDay(original), false);
});

test('escolha manual sobrepõe a detecção nos dois sentidos', () => {
    const gerado = dia({ o: [1, 0, 1, 1], ocorrenciaManual: false });
    assert.equal(isOccurrenceDay(gerado), false, 'exclusão manual vale');

    const original = dia({ o: [1, 1, 1, 1], ocorrenciaManual: true });
    assert.equal(isOccurrenceDay(original), true, 'inclusão manual vale');
});

test('dispensa detectada só aparece se o usuário preencheu algum campo', () => {
    usarJunho2026();
    const vazia = dia({ mot: 'DISPENSA', o: [1, 0, 1, 1] });
    assert.equal(deveAparecerNaOcorrencia(vazia, 'dispensa'), false);

    const preenchida = dia({ mot: 'DISPENSA', o: [1, 0, 1, 1], s1: '12:00' });
    assert.equal(deveAparecerNaOcorrencia(preenchida, 'dispensa'), true);
});

test('camposFaltantes nomeia só os horários gerados', () => {
    const d = dia({ o: [1, 0, 0, 1] });
    assert.deepEqual(camposFaltantes(d, false), ['a saída do almoço', 'a entrada do almoço']);
});

test('montarJustificativa liga os campos com "e"', () => {
    const frase = montarJustificativa('01/06/2026', ['a entrada', 'a saída'], 'O ponto não registrou');
    assert.equal(frase, '01/06/2026 - O ponto não registrou a entrada e a saída.');

    const tres = montarJustificativa('01/06/2026', ['a', 'b', 'c'], 'X');
    assert.equal(tres, '01/06/2026 - X a, b e c.');
});

// --- conversão com o backend ---

test('deWire/paraWire fazem round-trip sem perder campo', () => {
    const doBackend = {
        d: 3, w: 'Qua', e1: '08:02', s1: '12:03', e2: '13:05', s2: '17:31',
        es: '08:27', saldo: '+00:27', saldo_real: '+00:27',
        ocor: '', mot: 'DISPENSA', o: [1, 1, 0, 1], tipo: 'dispensa',
        carga: 240, day_type_override: 'dispensa',
        ocorrencia_manual: false, justificativa_manual: 'texto',
    };

    const volta = paraWire(deWire(doBackend));
    for (const campo of Object.keys(doBackend)) {
        if (campo === 'saldo_real') continue; // sempre normalizado para string
        assert.deepEqual(volta[campo], doBackend[campo], `campo ${campo}`);
    }
});

test('paraWire preserva false em ocorrencia_manual', () => {
    // Armadilha: `d.ocorrenciaManual || undefined` transformaria a exclusão
    // manual em "sem escolha", e ela se perderia ao salvar.
    assert.equal(paraWire(dia({ ocorrenciaManual: false })).ocorrencia_manual, false);
    assert.equal(paraWire(dia({ ocorrenciaManual: true })).ocorrencia_manual, true);
    assert.equal(paraWire(dia({ ocorrenciaManual: null })).ocorrencia_manual, undefined);
});

test('deWire distingue ausente de false', () => {
    assert.equal(deWire({ d: 1 }).ocorrenciaManual, null);
    assert.equal(deWire({ d: 1, ocorrencia_manual: false }).ocorrenciaManual, false);
});

// --- avisos de conferência ---

test('dia de dispensa sem jornada informada pede conferência', () => {
    usarJunho2026();
    const d = dia({ mot: 'DISPENSA PARA CURSO', e1: '08:00', s1: '12:00' });
    assert.equal(avisoDeRevisao(d, classifyDay(d)), MSG_CARGA_DISPENSA);
});

test('informar a jornada tira o aviso na hora', () => {
    usarJunho2026();
    // Regressão: o aviso vinha só do backend, então limpar ou preencher a
    // jornada na tela não o atualizava até recarregar a página.
    const d = dia({
        mot: 'DISPENSA PARA CURSO', e1: '08:00', s1: '12:00',
        revisar: true, revisar_motivo: MSG_CARGA_DISPENSA,
    });

    d.carga = 240;
    assert.equal(avisoDeRevisao(d, classifyDay(d)), '', 'com jornada informada, sem aviso');

    d.carga = undefined;
    assert.equal(avisoDeRevisao(d, classifyDay(d)), MSG_CARGA_DISPENSA, 'ao limpar, o aviso volta');
});

test('expediente reduzido tem a sua própria mensagem', () => {
    usarJunho2026();
    const d = dia({ mot: 'EXPEDIENTE REDUZIDO - DEC. 10.925', e1: '08:00', s1: '12:00' });
    assert.equal(avisoDeRevisao(d, classifyDay(d)), MSG_CARGA_REDUZIDA);
});

test('aviso vindo do backend sobre outro assunto é preservado', () => {
    usarJunho2026();
    const outro = 'Observação diz "SÁBADO", mas o dia 8 é Seg.';
    const d = dia({ revisar: true, revisar_motivo: outro, e1: '08:00', s1: '12:00', e2: '13:00', s2: '17:00' });
    assert.equal(avisoDeRevisao(d, classifyDay(d)), outro);
});

test('dia sem pendência não mostra aviso', () => {
    usarJunho2026();
    assert.equal(avisoDeRevisao(diaCheio(), 'completo'), '');
});

// --- apagar um horário de propósito ---

test('limpos sobrevive ao round-trip com o backend', () => {
    // Sem persistir a intenção, o auto-preencher regenera o campo apagado
    // assim que o usuário edita qualquer outro horário do mesmo dia.
    const doBackend = { d: 1, o: [0, 1, 1, 1], limpos: [0] };
    assert.deepEqual(deWire(doBackend).limpos, [0]);
    assert.deepEqual(paraWire(deWire(doBackend)).limpos, [0]);
});

test('sem campos apagados, limpos não vai para o backend', () => {
    assert.equal(paraWire(dia()).limpos, undefined);
    assert.deepEqual(deWire({ d: 1 }).limpos, []);
});
