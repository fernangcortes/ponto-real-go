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
    isAutoOccurrence, isOccurrenceDay, camposFaltantes,
    minutosTrabalhados, saldoDoDia, calcularTotais, deWire, paraWire,
    avisoDeRevisao, MSG_CARGA_DISPENSA, MSG_CARGA_REDUZIDA,
} from './domain.js';
import {
    montarJustificativa, semPrefixoDeData, comPrefixoDeData, textoDaJustificativa,
    aplicarLacunas, duracaoHumana, frasesParaOTipo, sugestaoParaOTipo,
    justificativaDeAto, motivoDoSilencio,
} from './justificativa.js';

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

// O defeito que motivou a mudança: 03/07/2026, dispensa para curso. Os dois
// batimentos são reais (09:26 e 13:45), nada foi gerado. O checkbox aparecia
// marcado e o dia não saía em nenhuma das duas seções do documento — e
// desmarcar e remarcar devolvia ao estado "automático", que era exatamente o
// que a tabela ignorava. O dia era inalcançável.
test('dispensa sem horário gerado entra no documento com o checkbox marcado', () => {
    usarJunho2026();
    const d = dia({ mot: 'DISPENSA', e1: '09:26', e2: '13:45', o: [1, 0, 1, 0] });

    assert.equal(isAutoOccurrence(d), true);
    assert.equal(isOccurrenceDay(d), true, 'marcado no checkbox É estar no documento');
});

test('exclusão manual tira o dia do documento', () => {
    const d = dia({ o: [1, 0, 1, 1], s1: '12:00', ocorrenciaManual: false });
    assert.equal(isOccurrenceDay(d), false);
});

// Marcar à mão um dia sem horário gerado nenhum tem de valer: é o caso de
// declarar uma ocorrência que a ficha não denuncia sozinha.
test('inclusão manual vale mesmo sem nenhum horário gerado', () => {
    const d = dia({ o: [1, 1, 1, 1], ocorrenciaManual: true });
    assert.equal(isAutoOccurrence(d), false, 'a detecção automática deixaria de fora');
    assert.equal(isOccurrenceDay(d), true, 'a escolha do usuário prevalece');
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

// A frase é guardada sem a data para poder ser reaproveitada em outro mês.
test('semPrefixoDeData tira a data e devolve só o texto', () => {
    assert.equal(semPrefixoDeData('10/07/2026 - Cumpri a jornada.'), 'Cumpri a jornada.');
    assert.equal(semPrefixoDeData('Cumpri a jornada.'), 'Cumpri a jornada.', 'sem data, não mexe');
    assert.equal(semPrefixoDeData(''), '');
    assert.equal(semPrefixoDeData(undefined), '');
    assert.equal(semPrefixoDeData('01/2026 - x'), '01/2026 - x', 'não é DD/MM/AAAA');
});

test('comPrefixoDeData não gera linha só com a data', () => {
    assert.equal(comPrefixoDeData('10/07/2026', 'Cumpri.'), '10/07/2026 - Cumpri.');
    assert.equal(comPrefixoDeData('10/07/2026', '  '), '', 'texto vazio não vira linha');
    // Repor a data em cima de um texto que já a tem não a duplica.
    assert.equal(comPrefixoDeData('10/07/2026', '10/07/2026 - Cumpri.'), '10/07/2026 - Cumpri.');
});

test('o que o usuário escreveu vence a frase automática', () => {
    const template = 'O ponto não registrou';
    const ajustado = dia({ o: [1, 0, 1, 1] });

    assert.equal(
        textoDaJustificativa(ajustado, '10/07/2026', ['a saída do almoço'], template),
        '10/07/2026 - O ponto não registrou a saída do almoço.',
        'sem texto próprio, monta a automática');

    ajustado.justManual = 'Compareci a reunião externa.';
    assert.equal(
        textoDaJustificativa(ajustado, '10/07/2026', ['a saída do almoço'], template),
        '10/07/2026 - Compareci a reunião externa.',
        'o texto do usuário vence mesmo havendo horário gerado');

    ajustado.justManual = '';
    assert.equal(
        textoDaJustificativa(ajustado, '10/07/2026', ['a saída do almoço'], template),
        '10/07/2026 - O ponto não registrou a saída do almoço.',
        'apagar o texto devolve a frase automática');
});

test('dia sem horário gerado e sem texto não produz linha', () => {
    const d = dia({ o: [1, 0, 1, 0] });
    assert.equal(textoDaJustificativa(d, '03/07/2026', [], 'X'), '');
});

// --- frase automática do dia de dispensa ---

// O caso de 03/07/2026: dispensa de 4h, expediente das 09:26 às 13:45. Nada foi
// inventado, então não há frase de "o ponto não registrou" — mas o dia precisa
// dizer no documento por que fecha em dia.
test('dispensa cumprida se explica sozinha', () => {
    usarJunho2026();
    const d = dia({ mot: 'DISPENSA', e1: '09:26', s1: '13:45', carga: 240, o: [1, 1, 0, 0] });

    assert.equal(
        justificativaDeAto(d),
        'Dia de dispensa: cumpri 4h19 da jornada de 4h exigida pelo ato, com expediente das 09:26 às 13:45.');

    // E chega ao campo pela via normal, sem o usuário digitar nada.
    assert.equal(
        textoDaJustificativa(d, '03/07/2026', [], 'X'),
        '03/07/2026 - Dia de dispensa: cumpri 4h19 da jornada de 4h exigida pelo ato, com expediente das 09:26 às 13:45.');
});

test('jornada cumprida na medida não diz que houve excedente', () => {
    usarJunho2026();
    const d = dia({ mot: 'DISPENSA', e1: '09:00', s1: '13:00', carga: 240, o: [1, 1, 0, 0] });

    assert.match(justificativaDeAto(d), /cumpri integralmente a jornada de 4h/);
});

// Sem a jornada do ato não há contra o que comparar: afirmar cumprimento seria
// inventar. O ⚠️ da linha já está pedindo esse número.
test('sem a jornada informada, o sistema não afirma nada', () => {
    usarJunho2026();
    const d = dia({ mot: 'DISPENSA', e1: '09:26', s1: '13:45', o: [1, 1, 0, 0] });

    assert.equal(justificativaDeAto(d), '');
    assert.equal(textoDaJustificativa(d, '03/07/2026', [], 'X'), '');
});

// Déficit não ganha desculpa automática: o campo fica para o usuário dizer o
// que de fato aconteceu.
test('jornada não cumprida não gera frase', () => {
    usarJunho2026();
    const d = dia({ mot: 'DISPENSA', e1: '09:26', s1: '11:00', carga: 240, o: [1, 1, 0, 0] });

    assert.equal(justificativaDeAto(d), '');
});

// O 13:45 lido como retorno do almoço não fecha turno nenhum: trabalhado = 0.
// O sistema tem de ficar calado até o horário estar na coluna certa.
test('batimento na coluna errada não vira jornada cumprida', () => {
    usarJunho2026();
    const d = dia({ mot: 'DISPENSA', e1: '09:26', e2: '13:45', carga: 240, o: [1, 0, 1, 0] });

    assert.equal(minutosTrabalhados(d), 0);
    assert.equal(justificativaDeAto(d), '');
});

test('expediente reduzido tem a sua própria redação', () => {
    usarJunho2026();
    const d = dia({ mot: 'EXPEDIENTE REDUZIDO', e1: '09:00', s1: '13:00', carga: 240, o: [1, 1, 0, 0] });

    assert.match(justificativaDeAto(d), /^Dia de expediente reduzido:/);
});

test('dia útil comum não recebe frase de ato', () => {
    usarJunho2026();
    const d = dia({ e1: '08:00', s1: '12:00', e2: '13:00', s2: '17:00' });

    assert.equal(justificativaDeAto(d), '');
});

// A frase automática cede lugar ao que o usuário escrever.
test('o texto do usuário vence a frase de ato', () => {
    usarJunho2026();
    const d = dia({ mot: 'DISPENSA', e1: '09:26', s1: '13:45', carga: 240, o: [1, 1, 0, 0] });
    d.justManual = 'Compareci ao curso de doutorado no período da tarde.';

    assert.equal(
        textoDaJustificativa(d, '03/07/2026', [], 'X'),
        '03/07/2026 - Compareci ao curso de doutorado no período da tarde.');
});

// Campo em branco sem explicação é indistinguível de defeito — foi exatamente
// assim que o silêncio da dispensa sem jornada apareceu na tela.
test('o campo vazio diz por que o sistema não escreveu', () => {
    usarJunho2026();

    const semJornada = dia({ mot: 'DISPENSA', e1: '09:26', s1: '13:45', o: [1, 1, 0, 0] });
    assert.match(motivoDoSilencio(semJornada), /Informe a jornada da dispensa/);

    const naoFecha = dia({ mot: 'DISPENSA', e1: '09:26', s1: '11:00', carga: 240, o: [1, 1, 0, 0] });
    assert.match(motivoDoSilencio(naoFecha), /não fecha neste dia/);

    // Cumprida: há frase, então não há silêncio a explicar.
    const fecha = dia({ mot: 'DISPENSA', e1: '09:26', s1: '13:45', carga: 240, o: [1, 1, 0, 0] });
    assert.equal(motivoDoSilencio(fecha), '');

    // Dia comum não tem jornada de ato: nada a explicar.
    assert.equal(motivoDoSilencio(dia({ e1: '08:00' })), '');
});

test('expediente reduzido cita o decreto, não a dispensa', () => {
    usarJunho2026();
    const d = dia({ mot: 'EXPEDIENTE REDUZIDO', e1: '09:00', s1: '11:00', o: [1, 1, 0, 0] });
    assert.match(motivoDoSilencio(d), /jornada do decreto/);
});

// --- biblioteca de frases ---

test('duracaoHumana escreve como se fala', () => {
    assert.equal(duracaoHumana(240), '4h');
    assert.equal(duracaoHumana(259), '4h19');
    assert.equal(duracaoHumana(600), '10h');
    assert.equal(duracaoHumana(45), '45min');
    assert.equal(duracaoHumana(0), '');
    assert.equal(duracaoHumana(undefined), '');
});

test('as lacunas recebem os valores do dia', () => {
    const frase = 'Cumpri {trabalhado} da jornada de {jornada} exigida em {data}.';
    assert.equal(
        aplicarLacunas(frase, { jornada: 240, trabalhado: 259, data: '03/07/2026' }),
        'Cumpri 4h19 da jornada de 4h exigida em 03/07/2026.');
});

// A mesma frase guardada precisa servir num dia de 2h sem sair errada.
test('a mesma frase serve para jornadas diferentes', () => {
    const frase = 'Cumpri a jornada de {jornada} exigida pelo ato.';
    assert.equal(aplicarLacunas(frase, { jornada: 240 }), 'Cumpri a jornada de 4h exigida pelo ato.');
    assert.equal(aplicarLacunas(frase, { jornada: 120 }), 'Cumpri a jornada de 2h exigida pelo ato.');
});

// Marcador cru num documento assinado é pior que uma frase incompleta.
test('lacuna sem valor some junto com o espaço que a precede', () => {
    assert.equal(
        aplicarLacunas('Cumpri a jornada de {jornada} exigida.', {}),
        'Cumpri a jornada de exigida.');
    assert.ok(!aplicarLacunas('x {jornada} y', {}).includes('{'), 'nenhum marcador sobra');
});

test('a frase guardada é aplicada com as lacunas do dia', () => {
    const d = dia({ o: [1, 0, 1, 0], e1: '09:26', s1: '13:45', carga: 240 });
    d.justManual = 'Cumpri {trabalhado} da jornada de {jornada} do ato.';

    assert.equal(
        textoDaJustificativa(d, '03/07/2026', [], 'X'),
        '03/07/2026 - Cumpri 4h19 da jornada de 4h do ato.');
});

test('as frases do tipo do dia sobem, e entre elas a mais usada', () => {
    const frases = [
        { texto: 'útil pouco usada', tipo: 'util', usos: 1 },
        { texto: 'dispensa pouco usada', tipo: 'dispensa', usos: 1 },
        { texto: 'dispensa muito usada', tipo: 'dispensa', usos: 9 },
    ];

    assert.deepEqual(
        frasesParaOTipo(frases, 'dispensa').map((f) => f.texto),
        ['dispensa muito usada', 'dispensa pouco usada', 'útil pouco usada']);

    // Nenhuma frase é filtrada: só a ordem muda.
    assert.equal(frasesParaOTipo(frases, 'util').length, 3);
});

test('frasesParaOTipo não altera a lista original', () => {
    const frases = [{ texto: 'a', tipo: 'util' }, { texto: 'b', tipo: 'dispensa' }];
    frasesParaOTipo(frases, 'dispensa');
    assert.equal(frases[0].texto, 'a', 'a biblioteca em memória foi reordenada por acidente');
});

test('a sugestão é a mais usada do mesmo tipo, e só dele', () => {
    const frases = [
        { texto: 'de dispensa', tipo: 'dispensa', usos: 2 },
        { texto: 'de dispensa, campeã', tipo: 'dispensa', usos: 7 },
        { texto: 'de dia útil', tipo: 'util', usos: 99 },
    ];

    assert.equal(sugestaoParaOTipo(frases, 'dispensa'), 'de dispensa, campeã');
    // Sem frase do tipo, não sugere nada — propor a de outro tipo seria pior
    // que não propor.
    assert.equal(sugestaoParaOTipo(frases, 'reduzido'), '');
    assert.equal(sugestaoParaOTipo([], 'util'), '');
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
