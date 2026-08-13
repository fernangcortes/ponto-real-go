// ============================================
// Ponto Real Go — Front-end Application
// ============================================
//
// Este arquivo concentra o que depende do DOM: renderização, eventos, upload,
// visualizador de documento e persistência. A regra de negócio e as conversões
// puras vivem em js/domain.js e js/util.js, onde são testáveis sem navegador.

import {
    CONFIG,
    resolverApiBase,
    CAMPOS_HORARIO,
    MESES_NOMES,
    TIPO_POR_SELECAO,
    TIPOS_DE_DIA,
    CARGA_POR_ATO,
    PRESETS_CARGA,
} from './js/config.js';
import {
    t2m,
    m2t,
    m2tUnsigned,
    isTimeValid,
    esc,
    copiavel,
    clamp,
    randBetween,
    avoidRoundMins,
} from './js/util.js';
import {
    classifyDay,
    tipoSelecionado,
    isAutoOccurrence,
    isOccurrenceDay,
    camposFaltantes,
    calcularTotais,
    avisoDeRevisao,
    propostaDeColunas,
    isWeekend,
    deWire,
    paraWire,
    minutosTrabalhados,
} from './js/domain.js';
import {
    getJustTemplate,
    salvarJustTemplate,
    JUST_TEMPLATE_PADRAO,
    textoDaJustificativa,
    semPrefixoDeData,
    aplicarLacunas,
    frasesParaOTipo,
    sugestaoParaOTipo,
    motivoDoSilencio,
    LACUNAS,
} from './js/justificativa.js';
import { carregarBiblioteca, frasesAtuais, guardarFrase, esquecerFrase } from './js/biblioteca.js';

CONFIG.apiBase = resolverApiBase(window.location);

// --- Dados (começa vazio, populado via upload) ---
const daysData = [];
let activeServidor = {};

// --- Ocorrência manual: adicionar/remover dia da seção "Ocorrência" do SEI ---

const toggleOcorrenciaManual = (idx, checked) => {
    const d = daysData[idx];
    // Se o novo valor coincide com o que a detecção automática já daria,
    // não precisa de override — volta para "automático".
    d.ocorrenciaManual = checked === isAutoOccurrence(d) ? null : checked;
    if (!isOccurrenceDay(d)) d.justManual = '';
    updateAll();
    scheduleSave();
};

const removeOcorrencia = (idx) => {
    toggleOcorrenciaManual(idx, false);
};

// syncJustManual grava o que o usuário escreveu na justificativa do dia.
//
// Guarda o texto SEM a data: a frase precisa servir em qualquer mês para poder
// ser reaproveitada. A data é reposta na exibição.
//
// Texto igual ao que a montagem automática produziria não vira texto manual —
// senão bastava passar o foco pelo campo para congelar a frase, e uma correção
// futura na regra nunca mais alcançaria aquele dia.
const syncJustManual = (idx, value) => {
    const d = daysData[idx];
    const faltantes = camposFaltantes(d, classifyDay(d) === 'dispensa');
    // A frase automática DESTE dia, e não a de um dia genérico: ela depende dos
    // horários e da jornada do ato, então precisa ser montada com o dia inteiro
    // — só sem o texto próprio, que é justamente o que estamos comparando.
    const automatica = textoDaJustificativa({ ...d, justManual: '' }, dataDoDia(d), faltantes);

    const escrito = semPrefixoDeData(value);
    d.justManual = escrito && `${dataDoDia(d)} - ${escrito}` !== automatica ? escrito : '';
    scheduleSave();
};

// --- Biblioteca de frases ---

// guardarFraseDoDia põe na biblioteca a frase que está no campo daquele dia.
//
// Guarda o texto CRU, com as lacunas intactas: é a versão que serve em outro
// mês. O que está na tela já tem {jornada} resolvido para o dia de hoje, então
// a fonte é d.justManual e não o valor do input.
const guardarFraseDoDia = async (idx) => {
    const d = daysData[idx];
    const texto = semPrefixoDeData(d.justManual);
    if (!texto) {
        showToast('Escreva a justificativa antes de guardá-la.', 'info');
        return;
    }

    const gravou = await guardarFrase(CONFIG.apiBase, texto, classifyDay(d));
    showToast(
        gravou
            ? 'Frase guardada na biblioteca.'
            : 'Frase guardada aqui; será enviada ao servidor depois.',
        gravou ? 'success' : 'info',
    );
    renderBibliotecaFrases();
    updateAll();
};

// aceitarSugestao é o Tab no campo vazio: escreve a sugestão que estava em
// cinza. Com o campo já preenchido, o Tab volta a ser navegação — sobrescrever
// o que a pessoa digitou seria pior que não sugerir nada.
const aceitarSugestao = (input, idx) => {
    const sugestao = input.dataset.sugestao;
    if (!sugestao || input.value.trim()) return false;

    daysData[idx].justManual = sugestao;
    scheduleSave();
    updateAll();

    // O render trocou o input; devolver o foco ao novo mantém a edição fluindo.
    const novo = document.getElementById(input.id);
    if (novo) {
        novo.focus();
        novo.setSelectionRange(novo.value.length, novo.value.length);
    }
    return true;
};

// --- Escolha manual do tipo de dia ---
// Guarda inclusive "util": é assim que o usuário desfaz uma detecção
// equivocada. Antes "util" virava null e a detecção automática reescrevia por
// cima no render seguinte, tornando a escolha impossível.
// redesenharTabela é necessário quando a mudança altera a ESTRUTURA da linha —
// trocar o tipo do dia faz aparecer ou sumir o seletor de jornada, por exemplo.
// Mudanças que só alteram valores exibidos bastam chamar updateAll().
const redesenharTabela = () => {
    document.getElementById('tablesContainer').innerHTML = '';
    renderTables();
    updateAll();
};

const changeDayType = (idx, newType) => {
    daysData[idx].dayTypeOverride = newType;
    redesenharTabela();
    scheduleSave();
};

// changeCargaPreset trata os atalhos do seletor de jornada.
const changeCargaPreset = (idx, valor) => {
    const d = daysData[idx];
    if (valor === 'custom') {
        d.cargaLivre = true; // só de interface; não é gravado no mês
    } else {
        d.cargaLivre = false;
        const min = parseInt(valor, 10);
        d.carga = Number.isFinite(min) && min > 0 ? min : undefined;
    }
    redesenharTabela(); // o campo livre aparece/some conforme a escolha
    scheduleSave();
};

// changeCarga trata o campo livre de minutos. Não redesenha: a estrutura da
// linha não muda, e redesenhar tiraria o foco do campo a cada digitação.
const changeCarga = (idx, valor) => {
    const min = parseInt(valor, 10);
    daysData[idx].carga = Number.isFinite(min) && min > 0 ? min : undefined;
    updateAll();
    scheduleSave();
};

// --- Auto-fill no cliente (espelho do RulesAdjuster do backend) ---
// randBetween e avoidRoundMins vêm de js/util.js.
const autoFillDay = (idx, changedField) => {
    const d = daysData[idx];
    if (!d.o) return; // sem info de originais, não auto-preencher

    const fields = ['e1', 's1', 'e2', 's2'];

    // === Smart Shift: ordenar cronologicamente todos os valores preenchidos ===
    // Quando o usuário preenche um campo fora de ordem, os valores são
    // automaticamente reorganizados em ordem cronológica.
    // Ex: e1=10:05, s1=12:50, e2=19:48, s2=11:50 → e1=10:05, s1=11:50, e2=12:50, s2=19:48
    const filledVals = fields.map((f) => (isTimeValid(d[f]) ? t2m(d[f]) : null));
    const filledEntries = filledVals
        .map((v, i) => ({ idx: i, val: v }))
        .filter((e) => e.val !== null);

    if (filledEntries.length >= 2) {
        const sorted = [...filledEntries].sort((a, b) => a.val - b.val);
        const isOutOfOrder = filledEntries.some((e, i) => e.val !== sorted[i].val);

        if (isOutOfOrder) {
            // Salvar o array de bloqueio original antes de reordenar
            const oldO = d.o ? [...d.o] : [1, 1, 1, 1];

            // Reorganizar: atribuir valores sorted nas posições dos campos preenchidos
            // E mover os flags de bloqueio (o) junto com os valores
            const newO = [...oldO];
            filledEntries.forEach((entry, i) => {
                d[fields[entry.idx]] = m2tUnsigned(sorted[i].val);
                // O valor que agora está na posição entry.idx veio de sorted[i].idx
                // → o flag de bloqueio deve acompanhar
                newO[entry.idx] = oldO[sorted[i].idx];
            });
            d.o = newO;

            // Atualizar todos os inputs no DOM (valor + classe visual)
            fields.forEach((f, fi) => {
                const inp = document.getElementById(`m_${f}_${d.d}`);
                if (inp) {
                    inp.value = d[f];
                    // Atualizar classe visual: original (readonly) vs editável
                    if (d.o[fi] === 1) {
                        inp.classList.add('readonly');
                        inp.classList.remove('draggable-time');
                        inp.setAttribute('readonly', 'readonly');
                    } else {
                        inp.classList.remove('readonly');
                        inp.classList.add('draggable-time');
                        inp.removeAttribute('readonly');
                    }
                }
            });
        }
    }

    // === Dispensa / Expediente reduzido: NÃO auto-preencher campos vazios ===
    // Em dispensa o usuário pode editar 1 ou mais horários sem que o sistema
    // complete os restantes. Em expediente reduzido a jornada exigida é menor
    // que a padrão, então o ponto batido já basta — completar até 8h inventaria
    // horas que o servidor não deveria cumprir.
    const tipo = classifyDay(d);
    if (tipo === 'dispensa' || tipo === 'reduzido') return;

    // === Auto-fill: preencher campos vazios restantes ===
    // Campo que o usuário apagou de propósito fica fora: antes ele era
    // repreenchido no mesmo instante em que perdia o foco, e não havia como
    // deixar um horário em branco.
    const limpos = d.limpos || [];
    const podePreencher = (i) => d.o[i] === 0 && !limpos.includes(i);

    const vals = fields.map((f) => (isTimeValid(d[f]) ? t2m(d[f]) : null));
    const empty = fields.filter((f, i) => vals[i] === null && podePreencher(i));

    if (empty.length === 0) return; // nada pra preencher

    const CARGA = 480; // 8h em min
    const ALMOCO_MIN = 60,
        ALMOCO_MAX = 75;

    // Tentar preencher campos vazios com base nos preenchidos
    let changed = false;
    const set = (f, m) => {
        const fi = fields.indexOf(f);
        if (vals[fi] === null && podePreencher(fi)) {
            const finalM = avoidRoundMins(m);
            d[f] = m2tUnsigned(Math.max(0, Math.min(1439, finalM)));
            vals[fi] = finalM;
            changed = true;
        }
    };

    // Caso: e1 e s2 conhecidos, s1 ou e2 faltando
    if (vals[0] !== null && vals[3] !== null) {
        const almoco = randBetween(ALMOCO_MIN, ALMOCO_MAX);
        const totalDisp = vals[3] - vals[0] - almoco;
        const manha = Math.round(totalDisp * 0.48) + randBetween(-5, 5);
        if (vals[1] === null && podePreencher(1)) set('s1', vals[0] + manha);
        if (vals[2] === null && podePreencher(2)) {
            const s1Val = vals[1] !== null ? vals[1] : vals[0] + manha;
            set('e2', s1Val + almoco);
        }
    }
    // Caso: e1 conhecido, s2 faltando
    if (vals[0] !== null && vals[3] === null && podePreencher(3)) {
        const almoco = randBetween(ALMOCO_MIN, ALMOCO_MAX);
        const manha = Math.round(CARGA * 0.48) + randBetween(-5, 5);
        if (vals[1] === null && podePreencher(1)) set('s1', vals[0] + manha);
        const s1Val = vals[1] !== null ? vals[1] : vals[0] + manha;
        if (vals[2] === null && podePreencher(2)) set('e2', s1Val + almoco);
        const e2Val = vals[2] !== null ? vals[2] : s1Val + almoco;
        const tarde = CARGA - (s1Val - vals[0]);
        set('s2', e2Val + Math.max(tarde, 180));
    }
    // Caso: s2 conhecido, e1 faltando
    if (vals[3] !== null && vals[0] === null && podePreencher(0)) {
        const almoco = randBetween(ALMOCO_MIN, ALMOCO_MAX);
        set('e1', vals[3] - CARGA - almoco);
        const e1Val = vals[0] !== null ? vals[0] : vals[3] - CARGA - almoco;
        const manha = Math.round(CARGA * 0.48) + randBetween(-5, 5);
        if (vals[1] === null && podePreencher(1)) set('s1', e1Val + manha);
        const s1Val = vals[1] !== null ? vals[1] : e1Val + manha;
        if (vals[2] === null && podePreencher(2)) set('e2', s1Val + almoco);
    }
    // Caso: s1 e e2 conhecidos mas e1 ou s2 faltando
    if (vals[1] !== null && vals[2] !== null) {
        if (vals[0] === null && podePreencher(0)) {
            const manha = Math.round(CARGA * 0.48) + randBetween(-5, 5);
            set('e1', vals[1] - manha);
        }
        if (vals[3] === null && podePreencher(3)) {
            const e1Val = vals[0] !== null ? vals[0] : vals[1] - 240;
            const tarde = CARGA - (vals[1] - e1Val);
            set('s2', vals[2] + Math.max(tarde, 180));
        }
    }

    if (changed) {
        // Atualizar inputs no DOM
        fields.forEach((f) => {
            const inp = document.getElementById(`m_${f}_${d.d}`);
            if (inp) inp.value = d[f];
        });
    }
};

// --- Feature 4: Validação visual inline ---
const validateDayVisual = (idx) => {
    const d = daysData[idx];
    const row = document.getElementById(`row_${d.d}`);
    if (!row) return;

    row.classList.remove('validation-error');
    row.querySelectorAll('.validation-badge').forEach((b) => b.remove());

    if (!isTimeValid(d.e1) || !isTimeValid(d.s1) || !isTimeValid(d.e2) || !isTimeValid(d.s2))
        return;

    const m1 = t2m(d.e1),
        m2 = t2m(d.s1),
        m3 = t2m(d.e2),
        m4 = t2m(d.s2);
    const errors = [];
    if (m1 >= m2) errors.push('Entrada ≥ Saída almoço');
    if (m2 >= m3) errors.push('Saída almoço ≥ Retorno');
    if (m3 >= m4) errors.push('Retorno ≥ Saída');
    if (m2 - m1 + (m4 - m3) < 480) errors.push('Carga < 8h');
    if (m3 - m2 < 60) errors.push('Almoço < 1h');

    if (errors.length > 0) {
        row.classList.add('validation-error');
        const saldoCell = document.getElementById(`m_saldo_${d.d}`);
        if (saldoCell) {
            const badge = document.createElement('span');
            badge.className = 'validation-badge';
            badge.textContent = ' ⚠️';
            badge.title = errors.join('\n');
            saldoCell.appendChild(badge);
        }
    }
};

// --- Toast ---
const showToast = (message, type = 'info') => {
    const container = document.getElementById('toastContainer');
    if (!container) {
        console.warn('[Toast]', message);
        return;
    }
    const toast = document.createElement('div');
    toast.className = `toast toast-${type}`;
    toast.textContent = message;
    container.appendChild(toast);
    setTimeout(() => toast.remove(), 3000);
};

// --- Copiar ---
const copyToClipboard = (text, sourceEl) => {
    navigator.clipboard
        .writeText(text)
        .then(() => {
            if (sourceEl) {
                // Feedback visual inline
                const orig = sourceEl.title;
                sourceEl.classList.add('just-copied');
                sourceEl.title = '✓ Copiado!';
                setTimeout(() => {
                    sourceEl.classList.remove('just-copied');
                    sourceEl.title = orig || '';
                }, 1200);
            }
            showToast('Copiado!', 'success');
        })
        .catch(() => {
            showToast('Erro ao copiar', 'error');
        });
};

// Em window porque só é chamada dos onclick embutidos no HTML gerado.
// Como const de topo também funcionaria, mas ficava indistinguível de uma
// função morta — e havia um `const copyBtn` local mais abaixo sombreando esta.
const copyBtn = (targetId) => {
    const el = document.getElementById(targetId);
    const val = el.value || el.innerText;
    copyToClipboard(val, el);
};

// Copiar célula ao clicar, por delegação: um listener só cobre toda célula
// marcada com data-copy, presente e futura.
//
// Substituiu o window.copyCell chamado de onclick embutido, onde o valor
// precisava atravessar uma string JS dentro de um atributo HTML.
document.addEventListener('click', (e) => {
    const alvo = e.target.closest('[data-copy]');
    if (alvo) {
        const texto = (alvo.dataset.copy || '').trim();
        if (texto) copyToClipboard(texto, alvo);
        return;
    }

    const copiarCampo = e.target.closest('[data-copy-input]');
    if (copiarCampo) {
        copyBtn(copiarCampo.dataset.copyInput);
        return;
    }

    const copiarLinha = e.target.closest('[data-copiar-linha]');
    if (copiarLinha) {
        copyRow(Number(copiarLinha.dataset.copiarLinha));
        return;
    }

    const remover = e.target.closest('[data-remover-ocorrencia]');
    if (remover) {
        removeOcorrencia(Number(remover.dataset.removerOcorrencia));
        return;
    }

    const guardar = e.target.closest('[data-guardar-frase]');
    if (guardar) {
        guardarFraseDoDia(Number(guardar.dataset.guardarFrase));
        return;
    }

    const alinhar = e.target.closest('[data-alinhar-colunas]');
    if (alinhar) {
        alinharColunasDoDia(Number(alinhar.dataset.alinharColunas));
    }
});

// Tab no campo de justificativa vazio aceita a sugestão em cinza.
document.addEventListener('keydown', (e) => {
    if (e.key !== 'Tab' || e.shiftKey) return;

    const just = e.target.closest('[data-just-manual]');
    if (!just) return;

    // Só engole o Tab quando ele de fato escreveu algo: sem sugestão, ou com o
    // campo já preenchido, o Tab tem de continuar navegando entre os campos.
    if (aceitarSugestao(just, Number(just.dataset.justManual))) e.preventDefault();
});

// focus/blur não borbulham; focusin/focusout sim.
document.addEventListener('focusin', (e) => {
    const campo = e.target.closest('[data-dia]');
    if (campo) saveState(Number(campo.dataset.dia));
});

document.addEventListener('focusout', (e) => {
    const campo = e.target.closest('[data-dia][data-campo]');
    if (campo) {
        syncChange(Number(campo.dataset.dia), campo.dataset.campo, campo.value);
        return;
    }

    const just = e.target.closest('[data-just-manual]');
    if (just) {
        syncJustManual(Number(just.dataset.justManual), just.value);
        return;
    }

    const carga = e.target.closest('[data-carga]');
    if (carga) changeCarga(Number(carga.dataset.carga), carga.value);
});

document.addEventListener('change', (e) => {
    const tipo = e.target.closest('[data-tipo-dia]');
    if (tipo) {
        changeDayType(Number(tipo.dataset.tipoDia), tipo.value);
        return;
    }

    const preset = e.target.closest('[data-carga-preset]');
    if (preset) {
        changeCargaPreset(Number(preset.dataset.cargaPreset), preset.value);
        return;
    }

    const ocorrencia = e.target.closest('[data-ocorrencia-manual]');
    if (ocorrencia) {
        toggleOcorrenciaManual(Number(ocorrencia.dataset.ocorrenciaManual), ocorrencia.checked);
    }
});

// Copiar linha completa — chamada apenas dos onclick embutidos no HTML gerado.
const copyRow = (idx) => {
    const d = daysData[idx];
    const dia = String(d.d).padStart(2, '0');
    const parts = [dia + '/' + CONFIG.mesAno];
    if (d.e1) parts.push(d.e1);
    if (d.s1) parts.push(d.s1);
    if (d.e2) parts.push(d.e2);
    if (d.s2) parts.push(d.s2);
    if (d.es) parts.push('E/S:' + d.es);
    if (d.saldo) parts.push('Saldo Original:' + d.saldo);
    copyToClipboard(parts.join('\t'));
};

// Copiar tabela inteira (tab-separated)
window.copyTable = () => {
    const header = 'Dia\tE1\tS1\tE2\tS2\tE/S\tSaldo Orig\tOcor\tMotivo\tDia Sem';
    const rows = daysData.map((d) => {
        const dia = String(d.d).padStart(2, '0');
        return [dia, d.e1, d.s1, d.e2, d.s2, d.es, d.saldo, d.ocor, d.mot, d.w].join('\t');
    });
    copyToClipboard(header + '\n' + rows.join('\n'));
};

// Copiar seção Ocorrência
window.copyOcorrencia = () => {
    const body = document.getElementById('ocorrenciaBody');
    if (!body || !body.rows.length) {
        showToast('Nenhuma ocorrência', 'info');
        return;
    }
    const header = 'Data\tEntrada\tSaída(Intervalo)\tEntrada(Intervalo)\tSaída';
    const rows = Array.from(body.rows).map((r) => {
        return Array.from(r.cells)
            .filter((c) => !c.classList.contains('col-acao-manual'))
            .map((c) => {
                const input = c.querySelector('input');
                return input ? input.value : c.innerText.trim();
            })
            .join('\t');
    });
    copyToClipboard(header + '\n' + rows.join('\n'));
};

// Copiar seção Justificativa
window.copyJustificativa = () => {
    const body = document.getElementById('justificativaBody');
    if (!body || !body.rows.length) {
        showToast('Nenhuma justificativa', 'info');
        return;
    }
    const rows = Array.from(body.rows).map((r) => {
        const input = r.querySelector('input');
        return input ? input.value : r.innerText.trim();
    });
    copyToClipboard(rows.join('\n'));
};

// Copiar TUDO (tabela + ocorrência + justificativa)
window.copyAll = () => {
    let out = '=== TABELA DE FREQUÊNCIA ===\n';
    out += 'Dia\tE1\tS1\tE2\tS2\tE/S\tSaldo Original\tOcor\tMotivo\n';
    daysData.forEach((d) => {
        const dia = String(d.d).padStart(2, '0');
        out += [dia, d.e1, d.s1, d.e2, d.s2, d.es, d.saldo, d.ocor, d.mot].join('\t') + '\n';
    });

    out += '\n=== OCORRÊNCIA ===\n';
    const ocBody = document.getElementById('ocorrenciaBody');
    if (ocBody) {
        out += 'Data\tEntrada\tSaída(Int.)\tEntrada(Int.)\tSaída\n';
        Array.from(ocBody.rows).forEach((r) => {
            out +=
                Array.from(r.cells)
                    .filter((c) => !c.classList.contains('col-acao-manual'))
                    .map((c) => {
                        const input = c.querySelector('input');
                        return input ? input.value : c.innerText.trim();
                    })
                    .join('\t') + '\n';
        });
    }

    out += '\n=== JUSTIFICATIVA DO SERVIDOR ===\n';
    const juBody = document.getElementById('justificativaBody');
    if (juBody) {
        Array.from(juBody.rows).forEach((r) => {
            const input = r.querySelector('input');
            out += (input ? input.value : r.innerText.trim()) + '\n';
        });
    }
    copyToClipboard(out);
};

// --- Backup para undo ---
let backupRow = {};
const saveState = (idx) => {
    const d = daysData[idx];
    backupRow[d.d] = { e1: d.e1, s1: d.s1, e2: d.e2, s2: d.s2 };
};

// --- Renderizar conteúdo da célula de Saldo (suporta original e real/calculado) ---
const renderSaldoCellContent = (d, realDiff) => {
    if (
        d.dayTypeOverride === 'feriado' ||
        d.dayTypeOverride === 'folga' ||
        d.dayTypeOverride === 'fds' ||
        d.dayTypeOverride === 'convocacao'
    ) {
        const labels = { feriado: 'Feriado', folga: 'Folga', fds: 'FDS', convocacao: 'Convocação' };
        const label = labels[d.dayTypeOverride] || d.dayTypeOverride;
        return `<span style="color:var(--text-muted);font-size:10px;">${esc(label)}</span>`;
    }

    // Saldo riscado (o lido da imagem) sobre o calculado.
    const saldoOriginal = (valor) =>
        `<span class="original-saldo" ${copiavel(valor)} title="Saldo original da imagem (Clique para copiar)" style="text-decoration:line-through; color:var(--text-muted); font-size:10px; cursor:pointer;">${esc(valor)}</span>`;

    const saldoCalculado = (valor, cor) =>
        `<span class="real-saldo" ${copiavel(valor)} title="Saldo calculado (Clique para copiar)" style="color:${cor}; font-weight:600; cursor:pointer;">${esc(valor.replace('+', ''))}</span>`;

    const empilhado = (cima, baixo) =>
        `<div style="display:flex; flex-direction:column; align-items:center; gap:2px;">${cima}${baixo}</div>`;

    const tipo = classifyDay(d);
    const origSaldo = d.saldo || '';

    if (tipo === 'falta') {
        const calculado = saldoCalculado('-08:00', 'var(--danger)');
        if (origSaldo && origSaldo !== '-08:00') {
            return empilhado(saldoOriginal(origSaldo), calculado);
        }
        return calculado;
    }

    // Determinar saldo real (calculado)
    let realFormatted = '';
    if (realDiff !== null) {
        realFormatted = m2t(realDiff);
    } else if (d.saldo_real) {
        realFormatted = d.saldo_real;
    }

    // Se ambos existem e são diferentes
    if (
        realFormatted &&
        origSaldo &&
        realFormatted.replace('+', '') !== origSaldo.replace('+', '')
    ) {
        const cor = realFormatted.includes('-') ? 'var(--danger)' : 'var(--success)';
        return empilhado(saldoOriginal(origSaldo), saldoCalculado(realFormatted, cor));
    }

    // Se apenas o real/calculado existe (ou são iguais)
    if (realFormatted) {
        const cor = realFormatted.includes('-') ? 'var(--danger)' : 'var(--text)';
        return saldoCalculado(realFormatted, cor);
    }

    // Se apenas o original existe
    if (origSaldo) {
        const cor = origSaldo.includes('-') ? 'var(--danger)' : 'var(--text)';
        return `<span ${copiavel(origSaldo)} title="Saldo original da imagem (Clique para copiar)" style="color:${cor}; cursor:pointer;">${esc(origSaldo)}</span>`;
    }

    return `<span class="empty-time">——:——</span>`;
};

// ============================================
// Renderização — consome o resultado do cálculo
// ============================================

const botaoCopiar = (alvo) =>
    `<button class="icon-btn" data-copy-input="${esc(alvo)}"><svg viewBox="0 0 24 24"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path></svg></button>`;

// atualizarLinhaPrincipal reflete na tabela o que o cálculo apurou para o dia.
const atualizarLinhaPrincipal = ({ d, tipo, diff, trabalhado }) => {
    const linha = document.getElementById(`row_${d.d}`);
    if (linha) linha.setAttribute('data-type', tipo);

    if (trabalhado !== null) {
        const esEl = document.getElementById(`m_es_${d.d}`);
        if (esEl) esEl.innerText = m2tUnsigned(trabalhado);
    }

    const saldoEl = document.getElementById(`m_saldo_${d.d}`);
    if (saldoEl) saldoEl.innerHTML = renderSaldoCellContent(d, diff);
};

const linhaOcorrencia = ({ d, idx, tipo }) => {
    const isDispensa = tipo === 'dispensa';
    const bloqueio = d.o || [1, 1, 1, 1];

    const campo = (valor, isOrig, f) => {
        // Dispensa: campos não preenchidos ficam como placeholder editável.
        const vazio = isDispensa && !isTimeValid(valor) && !isOrig;
        const attrs = isOrig ? 'class="readonly" readonly' : '';
        return `<input type="time" value="${vazio ? '' : esc(valor)}" ${attrs} data-dia="${idx}" data-campo="${f}">`;
    };

    const tr = document.createElement('tr');
    tr.innerHTML = `<td>${esc(String(d.d).padStart(2, '0'))}/${esc(CONFIG.mesAno)}</td>
        ${CAMPOS_HORARIO.map((f, fi) => `<td>${campo(d[f], bloqueio[fi], f)}</td>`).join('')}
        <td class="col-acao-manual"><button class="icon-btn" title="Remover esta ocorrência do documento SEI" data-remover-ocorrencia="${idx}">✕</button></td>`;
    return tr;
};

// dataDoDia formata o dia como ele aparece no documento: DD/MM/AAAA.
const dataDoDia = (d) => `${String(d.d).padStart(2, '0')}/${CONFIG.mesAno || '??/????'}`;

// DICA_LACUNAS explica os marcadores no tooltip do campo. Marcador que ninguém
// sabe que existe não é usado por ninguém.
const DICA_LACUNAS = 'Marcadores: ' + LACUNAS.map((l) => `${l.marcador} = ${l.ajuda}`).join('; ');

const linhaJustificativa = ({ d, idx, tipo }) => {
    const faltantes = camposFaltantes(d, tipo === 'dispensa');
    const frase = textoDaJustificativa(d, dataDoDia(d), faltantes);

    // A biblioteca alimenta o datalist ordenada para ESTE dia: as frases do
    // mesmo tipo primeiro, e entre elas a mais usada. Um datalist por linha
    // (e não um global) é o que permite ordem diferente por dia.
    const opcoes = frasesParaOTipo(frasesAtuais(), tipo)
        .map((f) => `<option value="${esc(f.texto)}">`)
        .join('');

    // Campo vazio por falta de um dado — a jornada do ato — não recebe sugestão:
    // uma frase com {jornada} resolveria para "a jornada de exigida pelo ato".
    // Nesse caso o placeholder explica o que falta, em vez de oferecer atalho.
    const silencio = frase ? '' : motivoDoSilencio(d, tipo);

    // A sugestão vive no placeholder, em cinza, e só entra no campo se o usuário
    // apertar Tab. Nada é escrito num documento assinado sem um gesto dele.
    const sugestao = frase || silencio ? '' : sugestaoParaOTipo(frasesAtuais(), tipo);

    let dica = 'Descreva o motivo da ocorrência...';
    if (silencio) dica = silencio;
    else if (sugestao) {
        dica = aplicarLacunas(sugestao, {
            jornada: d.carga,
            trabalhado: minutosTrabalhados(d),
            data: dataDoDia(d),
        });
    }

    // O campo é editável e salvo em TODOS os dias, inclusive quando a frase veio
    // pronta. Antes o dia com horário gerado recebia a frase automática num
    // input sem data-just-manual: aceitava a digitação e a descartava no render
    // seguinte, de modo que era impossível dizer outra coisa num dia ajustado.
    const tr = document.createElement('tr');
    tr.innerHTML = `<td><input type="text" id="just_${d.d}" class="just-input" list="frases_${d.d}"
            placeholder="${esc(dica)}" value="${esc(frase)}"
            title="${esc(DICA_LACUNAS)}"
            data-just-manual="${idx}" data-sugestao="${esc(sugestao)}">
        <datalist id="frases_${d.d}">${opcoes}</datalist>
        <button class="icon-btn" title="Guardar esta frase na biblioteca, para reusar nos outros meses" data-guardar-frase="${idx}">★</button>
        ${botaoCopiar(`just_${d.d}`)}
        <button class="icon-btn" title="Remover este dia do documento SEI" data-remover-ocorrencia="${idx}">✕</button></td>`;
    return tr;
};

// renderOcorrencias monta as duas seções do documento.
//
// Um critério só, o mesmo do checkbox: marcou, o dia sai nas duas. Não há
// exceção a lembrar — nem para dispensa, nem para dia sem horário nenhum.
const renderOcorrencias = (porDia) => {
    const ocorBody = document.getElementById('ocorrenciaBody');
    const justBody = document.getElementById('justificativaBody');
    ocorBody.innerHTML = '';
    justBody.innerHTML = '';

    porDia
        .filter(({ d }) => isOccurrenceDay(d))
        .forEach((item) => {
            ocorBody.appendChild(linhaOcorrencia(item));
            justBody.appendChild(linhaJustificativa(item));
        });
};

const renderStatusBar = (totais) => {
    document.getElementById('totalFaltas').innerText = totais.faltas;
    document.getElementById('totalAjustados').innerText = totais.ajustados;
    document.getElementById('totalCompletos').innerText = totais.completos;

    const oficialEl = document.getElementById('saldoTotalOficial');
    if (oficialEl) oficialEl.innerText = m2t(totais.saldoOficial);

    const realEl = document.getElementById('saldoTotal');
    if (realEl) {
        realEl.innerText = m2t(totais.saldoReal);
        realEl.className =
            'status-value ' + (totais.saldoReal < 0 ? 'status-danger' : 'status-success');
    }
};

// --- Atualizar tudo ---
// Calcula uma vez e distribui o resultado para as três áreas da tela.
const updateAll = () => {
    const totais = calcularTotais(daysData);
    totais.porDia.forEach(atualizarLinhaPrincipal);
    renderOcorrencias(totais.porDia);
    renderStatusBar(totais);
};

// alinharColunasDoDia aplica a proposta do botão: lê os dois batimentos como
// entrada e saída do expediente.
//
// Nenhum horário muda de valor — só de coluna. É exatamente o que o usuário faz
// arrastando, e a marca de "batimento real" viaja junto com o horário: quem era
// do servidor continua sendo, e as colunas que esvaziam ficam marcadas como
// apagadas de propósito, para nada as repreencher.
//
// Não sobrevive a um reprocessamento do mês, e não precisa: a extração volta a
// errar a coluna, o aviso volta a aparecer e o botão volta a estar aqui. É o que
// torna esta correção diferente de uma edição manual perdida.
const alinharColunasDoDia = (idx) => {
    const d = daysData[idx];
    const proposta = propostaDeColunas(d, classifyDay(d));
    if (!proposta) return;

    saveState(idx);

    const { entrada, saida } = proposta;
    d.e1 = entrada.valor;
    d.s1 = saida.valor;
    d.e2 = '';
    d.s2 = '';
    d.o = [entrada.real ? 1 : 0, saida.real ? 1 : 0, 0, 0];
    d.limpos = [2, 3];

    const container = document.getElementById('tablesContainer');
    container.innerHTML = '';
    renderTables();
    updateAll();
    scheduleSave();
    showToast(`Dia ${d.d}: lido como ${entrada.valor} → ${saida.valor}.`, 'success');
};

// --- Sync Change (Two-Way Binding) — Feature 4: sem confirm(), com auto-fill ---
const syncChange = (idx, field, newVal) => {
    const d = daysData[idx];
    d[field] = newVal;

    // Registrar se o campo foi apagado DE PROPÓSITO.
    //
    // Sem isso não havia como deixar um horário em branco: o auto-preencher
    // via o campo vazio e marcado como gerado, e o repreenchia no mesmo
    // instante em que perdia o foco. Digitar um valor de volta desfaz a marca.
    const fi = CAMPOS_HORARIO.indexOf(field);
    if (fi !== -1) {
        const limpos = new Set(d.limpos || []);
        if (isTimeValid(newVal)) {
            limpos.delete(fi);
        } else {
            limpos.add(fi);
        }
        d.limpos = [...limpos].sort();
    }

    // Sync para input da tabela principal (se editado via ocorrência)
    const mainInput = document.getElementById(`m_${field}_${d.d}`);
    if (mainInput) mainInput.value = newVal;

    // Auto-fill: preencher campos vazios com lógica inteligente
    autoFillDay(idx, field);

    updateAll();

    // Validação visual apenas (sem confirm bloqueante)
    validateDayVisual(idx);

    // Auto-save após cada edição
    scheduleSave();
};

// controleDeCarga monta o seletor de jornada exigida no dia.
//
// A jornada de uma dispensa ou de um expediente reduzido vem do ato que a
// concedeu e muda caso a caso: pode liberar o dia inteiro, meio período ou a
// partir de determinado horário. Em vez de o sistema arbitrar um número, o
// usuário informa o que o documento diz — com atalhos para os casos comuns e
// um campo livre para o resto.
const controleDeCarga = (d, idx, tipo) => {
    const atual = d.carga || 0;
    // cargaLivre registra a INTENÇÃO de digitar um valor próprio. Sem ela,
    // escolher "Outra…" com uma jornada que por acaso coincide com um atalho
    // não mostrava campo nenhum: o seletor voltava ao atalho e o usuário ficava
    // sem entender por que o clique não fez nada.
    const usarCampoLivre =
        d.cargaLivre || (atual > 0 && !PRESETS_CARGA.some((p) => p.valor === atual));
    const origem = tipo === 'dispensa' ? 'ato de dispensa' : 'decreto';

    const ajuda =
        `Jornada que o servidor ainda devia cumprir neste dia, conforme o ${origem}. ` +
        'Em branco, o dia não gera saldo nem déficit e fica marcado para conferência.';

    const opcoes = [
        `<option value=""${atual === 0 && !usarCampoLivre ? ' selected' : ''}>A conferir</option>`,
        ...PRESETS_CARGA.map(
            (p) =>
                `<option value="${p.valor}"${!usarCampoLivre && atual === p.valor ? ' selected' : ''}>${esc(p.rotulo)}</option>`,
        ),
        `<option value="custom"${usarCampoLivre ? ' selected' : ''}>Outra…</option>`,
    ].join('');

    const livre = usarCampoLivre
        ? `<input type="number" class="carga-input" min="1" max="1440" step="15"
                value="${atual || ''}" placeholder="min"
                title="Jornada exigida neste dia, em minutos." data-carga="${idx}">`
        : '';

    return `<span class="carga-controle" title="${esc(ajuda)}">
        <select class="carga-select" data-carga-preset="${idx}">${opcoes}</select>${livre}
    </span>`;
};

// --- Renderizar tabela principal ---
const renderTables = () => {
    const container = document.getElementById('tablesContainer');
    if (daysData.length === 0) return;

    // Gerar blocos dinamicamente (semanas)
    const blocks = [];
    for (let i = 0; i < daysData.length; i += 7) {
        blocks.push([i, Math.min(i + 6, daysData.length - 1)]);
    }

    blocks.forEach((block) => {
        const table = document.createElement('table');
        table.className = 'main-table';

        // Feature 5: Tooltips nos cabeçalhos
        const thead = `<thead><tr>
            <th class="col-dia has-tooltip" title="Dia do mês">Dia</th>
            <th class="has-tooltip" title="Entrada — Início do expediente">E</th>
            <th class="has-tooltip" title="Saída — Saída para intervalo/almoço">S</th>
            <th class="has-tooltip" title="Entrada — Retorno do intervalo/almoço">E</th>
            <th class="has-tooltip" title="Saída — Fim do expediente">S</th>
            <th class="col-es has-tooltip" title="Total de horas trabalhadas no dia">E/S</th>
            <th class="has-tooltip" title="Diferença entre horas trabalhadas e carga horária (8h)">Saldo</th>
            <th class="has-tooltip" title="Compensações, atrasos ou observações">Ocor.</th>
            <th class="col-motivo has-tooltip" title="Justificativa ou observação do sistema">Motivo</th>
            <th class="col-week"></th>
            <th class="has-tooltip" title="Copiar dados da linha">Ações</th>
        </tr></thead>`;

        const tbody = document.createElement('tbody');

        for (let i = block[0]; i <= block[1]; i++) {
            if (i >= daysData.length) break;
            const d = daysData[i];
            const tr = document.createElement('tr');
            const tipo = classifyDay(d);
            tr.id = `row_${d.d}`;
            tr.setAttribute('data-type', tipo);
            // Classe visual para tipo de dia override
            if (d.dayTypeOverride === 'feriado') tr.classList.add('row-feriado');
            if (d.dayTypeOverride === 'folga') tr.classList.add('row-folga');
            if (d.dayTypeOverride === 'fds') tr.classList.add('row-folga');
            if (d.dayTypeOverride === 'convocacao') tr.classList.add('row-convocacao');
            if (d.dayTypeOverride === 'dispensa') tr.classList.add('row-dispensa');
            const diaFmt = String(d.d).padStart(2, '0');

            // Feature 4: usar onblur ao invés de onchange para evitar disparo prematuro
            const mkMainInput = (v, isOrig, f) => {
                const canDrag = v ? 'draggable="true"' : '';
                if (
                    !v &&
                    (d.dayTypeOverride === 'feriado' ||
                        d.dayTypeOverride === 'folga' ||
                        d.dayTypeOverride === 'fds' ||
                        d.dayTypeOverride === 'convocacao')
                )
                    return `<span class="empty-time">——:——</span>`;
                if (!v && d.dayTypeOverride !== 'dispensa' && isWeekend(d) && !d.mot)
                    return `<span class="empty-time">——:——</span>`;
                if (
                    !v &&
                    d.dayTypeOverride !== 'dispensa' &&
                    d.saldo === '-08:00' &&
                    !d.mot &&
                    !isWeekend(d)
                )
                    return `<span class="empty-time">——:——</span>`;
                if (!v)
                    return `<input type="time" id="m_${f}_${d.d}" value="" title="Digite o horário, ou arraste um horário de outra coluna para cá." data-dia="${i}" data-campo="${f}">`;
                const dica = isOrig
                    ? 'Horário lido da ficha: não se digita por cima, mas ARRASTE para mudar de coluna.'
                    : 'Horário gerado pelo sistema: pode ser digitado, ou arrastado para outra coluna.';
                return `<input type="time" id="m_${f}_${d.d}" value="${v}" title="${esc(dica)}" ${isOrig ? `class="readonly draggable-time"` : `${canDrag} class="draggable-time"`} ${canDrag} data-dia="${i}" data-campo="${f}">`;
            };

            const tdSaldo = `<td id="m_saldo_${d.d}" class="saldo-cell" style="font-family: 'JetBrains Mono', monospace; font-size: 11px;" title="Clique nos valores para copiar">${renderSaldoCellContent(d, null)}</td>`;

            let diaDisplay = diaFmt;
            if (tipo === 'falta') {
                diaDisplay = `<span title="Falta injustificada" style="color:var(--danger)">${diaFmt} 🔴</span>`;
            }
            // Seletor de tipo de dia. A detecção automática é só LIDA aqui —
            // gravá-la em d.dayTypeOverride durante o render transformava um
            // palpite do sistema em escolha do usuário, e a tornava permanente.
            const selected = tipoSelecionado(d);

            // Aviso de conferência. Derivado do estado atual, não só do que o
            // backend mandou: limpar a jornada de um dia de dispensa precisa
            // trazer o ⚠️ de volta na hora, sem recarregar a página.
            const aviso = avisoDeRevisao(d, tipo);
            if (aviso) {
                diaDisplay += ` <span class="revisar-badge" title="${esc(aviso)}">⚠️</span>`;
            }

            const selectClass = TIPO_POR_SELECAO[selected] ? ` is-${selected}` : '';
            const motivoText = d.mot ? ` title="${esc(d.mot)}"` : '';

            // Dispensa e expediente reduzido têm a jornada definida pelo ato que
            // os concedeu — decreto, portaria — e ela varia caso a caso. Em
            // branco o dia fica neutro em vez de o sistema arbitrar uma jornada.
            const cargaHtml = CARGA_POR_ATO.includes(selected)
                ? controleDeCarga(d, i, selected)
                : '';

            // Nenhum turno fecha e os dois batimentos cabem como entrada e saída
            // do expediente: propõe a leitura, sem aplicá-la. O batimento é do
            // servidor — o sistema pode apontar que a coluna não fecha, não
            // decidir por ele.
            const proposta = propostaDeColunas(d, tipo);
            const propostaHtml = proposta
                ? `<button type="button" class="proposta-colunas" data-alinhar-colunas="${i}"
                        title="${esc(`Numa jornada de ${m2tUnsigned(d.carga)}, o mais provável é que ${proposta.entrada.valor} e ${proposta.saida.valor} sejam a entrada e a saída do expediente, e não um par de almoço. Nenhum horário será alterado: só a coluna em que cada um é lido.`)}">
                        Ler como ${esc(proposta.entrada.valor)} → ${esc(proposta.saida.valor)}
                   </button>`
                : '';

            // Toggle manual da seção "Ocorrência" do documento Gerar SEI.
            // Reflete o estado efetivo (automático ou sobreposto) e alterna
            // entre incluir/excluir manualmente este dia.
            const occChecked = isOccurrenceDay(d);
            const occOverridden = d.ocorrenciaManual === true || d.ocorrenciaManual === false;
            const occHtml = `<label class="ocor-manual-toggle${occOverridden ? ' is-overridden' : ''}" title="Incluir/excluir manualmente esta data na seção Ocorrência do documento Gerar SEI">
                <input type="checkbox" ${occChecked ? 'checked' : ''} data-ocorrencia-manual="${i}">
                Ocorrência
            </label>`;

            const opcoesTipo = TIPOS_DE_DIA.map(
                (t) =>
                    `<option value="${t.valor}"${selected === t.valor ? ' selected' : ''} title="${esc(t.ajuda)}">${esc(t.rotulo)}</option>`,
            ).join('');
            const ajudaTipo = TIPOS_DE_DIA.find((t) => t.valor === selected)?.ajuda || '';

            let motivoHtml = `<td class="col-motivo"${motivoText}>
                <select class="day-type-select${selectClass}" data-tipo-dia="${i}" title="${esc(ajudaTipo)}">
                    ${opcoesTipo}
                </select>
                ${cargaHtml}
                ${propostaHtml}
                ${occHtml}
                ${d.mot ? `<span class="mot-text" title="${esc(d.mot)}">${esc(d.mot.substring(0, 60))}${d.mot.length > 60 ? '...' : ''}</span>` : ''}
            </td>`;

            tr.innerHTML = `
                <td class="col-dia">${diaDisplay}</td>
                <td>${mkMainInput(d.e1, d.o ? d.o[0] : 1, 'e1')}</td><td>${mkMainInput(d.s1, d.o ? d.o[1] : 1, 's1')}</td>
                <td>${mkMainInput(d.e2, d.o ? d.o[2] : 1, 'e2')}</td><td>${mkMainInput(d.s2, d.o ? d.o[3] : 1, 's2')}</td>
                <td class="col-es" id="m_es_${d.d}" style="font-family:'JetBrains Mono',monospace;font-size:11px;cursor:pointer" ${copiavel(d.es)} title="Clique p/ copiar">${esc(d.es)}</td>
                ${tdSaldo}
                <td style="font-family:'JetBrains Mono',monospace;font-size:11px;cursor:pointer" ${copiavel(d.ocor)} title="Clique p/ copiar">${esc(d.ocor)}</td>
                ${motivoHtml}
                <td class="col-week">${d.w}</td>
                <td class="col-acao">
                    <button class="icon-btn" data-copiar-linha="${i}" title="Copiar linha">
                        <svg viewBox="0 0 24 24"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path></svg>
                    </button>
                </td>
            `;
            tbody.appendChild(tr);
        }
        table.innerHTML = thead;
        table.appendChild(tbody);
        container.appendChild(table);
    });
};

// --- Drag and Drop Swap ---
document.getElementById('tablesContainer').addEventListener('dragstart', (e) => {
    if (e.target.tagName === 'INPUT' && e.target.type === 'time') {
        const val = e.target.value;
        if (!val) {
            e.preventDefault(); // Não arrastar inputs vazios
            return;
        }
        e.dataTransfer.setData('text/plain', e.target.id);
        e.dataTransfer.effectAllowed = 'move';
        e.target.style.opacity = '0.4';
        // Acende os campos que aceitam receber o horário. Sem isso não há como
        // saber, no meio do arrasto, que soltar é uma operação possível.
        document.body.classList.add('arrastando-horario');
    }
});

document.getElementById('tablesContainer').addEventListener('dragover', (e) => {
    if (e.target.tagName === 'INPUT' && e.target.type === 'time') {
        e.preventDefault(); // Necessário para permitir o drop
        e.dataTransfer.dropEffect = 'move';
        e.target.style.boxShadow = '0 0 0 2px var(--primary) inset';
    }
});

document.getElementById('tablesContainer').addEventListener('dragleave', (e) => {
    if (e.target.tagName === 'INPUT' && e.target.type === 'time') {
        e.target.style.boxShadow = '';
    }
});

document.getElementById('tablesContainer').addEventListener('dragend', (e) => {
    // Apagar as luzes sai do dragend e não do drop: soltar fora de um campo
    // válido, ou apertar Esc, cancela o arrasto sem passar pelo drop — e a
    // tabela ficaria acesa até o próximo render.
    document.body.classList.remove('arrastando-horario');
    if (e.target.tagName === 'INPUT') {
        e.target.style.opacity = '1';
    }
});

document.getElementById('tablesContainer').addEventListener('drop', (e) => {
    if (e.target.tagName === 'INPUT' && e.target.type === 'time') {
        e.preventDefault();
        e.target.style.boxShadow = '';
        const srcId = e.dataTransfer.getData('text/plain');
        if (!srcId || srcId === e.target.id) return;

        const srcEl = document.getElementById(srcId);
        if (!srcEl) return;

        const srcParts = srcId.split('_');
        const tgtParts = e.target.id.split('_');
        if (srcParts.length !== 3 || tgtParts.length !== 3) return;

        const srcField = srcParts[1];
        const srcDia = parseInt(srcParts[2]);
        const tgtField = tgtParts[1];
        const tgtDia = parseInt(tgtParts[2]);

        if (srcDia !== tgtDia) {
            showToast('Arraste permitido apenas no mesmo dia.', 'error');
            return;
        }

        const idx = daysData.findIndex((d) => d.d === srcDia);
        if (idx === -1) return;

        let d = daysData[idx];
        saveState(idx);

        const valToMove = srcEl.value;
        const valDest = e.target.value;

        // Swap (troca) de valores
        d[tgtField] = valToMove;
        d[srcField] = valDest;

        if (d.o) {
            const fi = ['e1', 's1', 'e2', 's2'].indexOf(srcField);
            const ti = ['e1', 's1', 'e2', 's2'].indexOf(tgtField);
            if (fi > -1 && ti > -1) {
                // Swap the original status too
                const originalStatusOfSource = d.o[fi];
                const originalStatusOfTarget = d.o[ti];

                d.o[fi] = originalStatusOfTarget;
                d.o[ti] = originalStatusOfSource;
            }
        }

        const container = document.getElementById('tablesContainer');
        container.innerHTML = '';
        renderTables();
        updateAll();
        showToast(`Horário movido com sucesso!`, 'success');
    }
});

// --- Tema ---
document.getElementById('themeToggle').addEventListener('click', () => {
    const current = document.body.getAttribute('data-theme');
    const next = current === 'dark' ? 'light' : 'dark';
    document.body.setAttribute('data-theme', next);
    localStorage.setItem('theme', next);
});

// Restaurar tema salvo
const savedTheme = localStorage.getItem('theme');
if (savedTheme) {
    document.body.setAttribute('data-theme', savedTheme);
} else if (window.matchMedia('(prefers-color-scheme: dark)').matches) {
    document.body.setAttribute('data-theme', 'dark');
}

// --- Versão: vem do backend, que é o único lugar onde o número mora ---
const loadVersion = async () => {
    const badge = document.getElementById('versionBadge');
    try {
        const res = await fetch(`${CONFIG.apiBase}/api/health`);
        if (!res.ok) return;
        const { version } = await res.json();
        if (!version) return;
        CONFIG.version = version;
        if (badge) badge.textContent = `v${version}`;
    } catch (e) {
        // Backend fora do ar: o resto da página continua utilizável, então só
        // deixamos o selo vazio em vez de mostrar um número possivelmente errado.
        console.error('Erro ao obter a versão:', e);
    }
};
loadVersion();

// --- Biblioteca de justificativas ---

// renderBibliotecaFrases desenha a lista de frases guardadas, com o ✕ que
// apaga. Sem um lugar para ver e apagar, a biblioteca só cresce.
const renderBibliotecaFrases = () => {
    const lista = document.getElementById('bibliotecaFrases');
    if (!lista) return;

    const frases = frasesAtuais();
    if (!frases.length) {
        lista.innerHTML = `<li class="biblioteca-vazia">Nenhuma frase guardada. Use o ★ ao lado de uma justificativa para guardá-la aqui.</li>`;
        return;
    }

    lista.innerHTML = frases
        .map(
            (f) => `<li>
        <span class="biblioteca-texto" title="${esc(f.tipo ? `Guardada num dia de tipo "${f.tipo}"` : 'Sem tipo de dia registrado')}">${esc(f.texto)}</span>
        <button class="icon-btn" title="Apagar esta frase da biblioteca" data-esquecer-frase="${esc(f.texto)}">✕</button>
    </li>`,
        )
        .join('');
};

document.addEventListener('click', (e) => {
    const esquecer = e.target.closest('[data-esquecer-frase]');
    if (!esquecer) return;

    esquecerFrase(CONFIG.apiBase, esquecer.dataset.esquecerFrase).then((gravou) => {
        if (!gravou) showToast('Apagada aqui; o servidor será atualizado depois.', 'info');
        renderBibliotecaFrases();
        if (daysData.length > 0) updateAll();
    });
});

// Buscar a biblioteca no início da sessão, sem bloquear a tela: a lista aparece
// no seletor assim que chega, e enquanto isso a folha já pode ser conferida. Se
// o servidor não responder, o cache local assume — o módulo trata disso.
carregarBiblioteca(CONFIG.apiBase).then(() => {
    renderBibliotecaFrases();
    if (daysData.length > 0) updateAll();
});

// --- Inicialização ---
const hdrInput = document.getElementById('headerMonthInput');
if (hdrInput) {
    hdrInput.value = '';
    hdrInput.addEventListener('change', (e) => {
        CONFIG.mesAno = e.target.value.trim();
        if (CONFIG.mesAno.length >= 7) {
            const [mm, yyyy] = CONFIG.mesAno.split('/');
            const meses = [
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
            if (parseInt(mm) >= 1 && parseInt(mm) <= 12) {
                CONFIG.mesNome = `${meses[parseInt(mm)]} ${yyyy}`;
            }
        }
        if (daysData.length > 0) {
            const container = document.getElementById('tablesContainer');
            container.innerHTML = '';
            renderTables();
            updateAll();
        }
    });
}
// Estado vazio — tabelas só aparecem após upload

// ============================================
// Upload + Extração
// ============================================

let selectedFile = null;
let lastUploadedFileName = '';

const uploadZone = document.getElementById('uploadZone');
const uploadContent = document.getElementById('uploadContent');
const uploadFileSelected = document.getElementById('uploadFileSelected');
const uploadLoading = document.getElementById('uploadLoading');
const uploadSuccess = document.getElementById('uploadSuccess');
const uploadSuccessDetail = document.getElementById('uploadSuccessDetail');
const uploadCollapsed = document.getElementById('uploadCollapsed');
const collapsedName = document.getElementById('collapsedName');
const loadingModel = document.getElementById('loadingModel');
const fileInput = document.getElementById('fileInput');
const uploadBtn = document.getElementById('uploadBtn');
const uploadBtn2 = document.getElementById('uploadBtn2');
const modelSelect = document.getElementById('modelSelect');
const modelSelect2 = document.getElementById('modelSelect2');
const uploadClear = document.getElementById('uploadClear');
const uploadToggle = document.getElementById('uploadToggle');
const uploadExpand = document.getElementById('uploadExpand');
const fileChange = document.getElementById('fileChange');
const emptyState = document.getElementById('emptyState');
const statusBar = document.getElementById('statusBar');
const summaryCard = document.querySelector('.summary-card');

// Esconder summary card no início
if (summaryCard) summaryCard.style.display = 'none';
const copyToolbar = document.getElementById('copyToolbar');
if (copyToolbar) copyToolbar.style.display = 'none';

// --- Utilidades de upload ---
const formatFileSize = (bytes) => {
    if (bytes < 1024) return bytes + ' B';
    if (bytes < 1048576) return (bytes / 1024).toFixed(1) + ' KB';
    return (bytes / 1048576).toFixed(1) + ' MB';
};

const showFileSelected = (file) => {
    const ext = file.name.split('.').pop().toLowerCase();
    const icon = ['pdf'].includes(ext) ? '📄' : '🖼️';
    document.getElementById('fileIcon').textContent = icon;
    document.getElementById('fileName').textContent = file.name;
    document.getElementById('fileSize').textContent = `(${formatFileSize(file.size)})`;
    uploadContent.style.display = 'none';
    uploadFileSelected.style.display = 'flex';
    uploadZone.classList.add('has-file');
};

const hideAllUploadStates = () => {
    uploadContent.style.display = 'none';
    uploadFileSelected.style.display = 'none';
    uploadLoading.style.display = 'none';
    uploadSuccess.style.display = 'none';
    uploadCollapsed.style.display = 'none';
    uploadZone.classList.remove('has-file', 'has-success');
};

// Sync both model selects
modelSelect.addEventListener('change', () => {
    modelSelect2.value = modelSelect.value;
});
modelSelect2.addEventListener('change', () => {
    modelSelect.value = modelSelect2.value;
});

// Drag & drop
uploadZone.addEventListener('dragover', (e) => {
    e.preventDefault();
    uploadZone.classList.add('drag-over');
});

uploadZone.addEventListener('dragleave', () => {
    uploadZone.classList.remove('drag-over');
});

uploadZone.addEventListener('drop', (e) => {
    e.preventDefault();
    uploadZone.classList.remove('drag-over');
    const files = e.dataTransfer.files;
    if (files.length > 0) {
        selectedFile = files[0];
        showFileSelected(selectedFile);
        updateViewDocBtnVisibility();
    }
});

// Click para selecionar
uploadZone.addEventListener('click', (e) => {
    if (
        e.target.closest('.upload-controls') ||
        e.target.closest('.upload-success') ||
        e.target.closest('.upload-file-selected') ||
        e.target.closest('.upload-collapsed') ||
        e.target.closest('.btn-link') ||
        e.target.closest('.btn-toggle')
    )
        return;
    fileInput.click();
});

fileInput.addEventListener('change', () => {
    if (fileInput.files.length > 0) {
        selectedFile = fileInput.files[0];
        showFileSelected(selectedFile);
        updateViewDocBtnVisibility();
    }
});

// Botão trocar arquivo
fileChange.addEventListener('click', (e) => {
    e.stopPropagation();
    fileInput.click();
});

// Botão processar (ambos)
const handleProcess = async (e) => {
    e.stopPropagation();
    if (!selectedFile) {
        showToast('Selecione um arquivo primeiro', 'error');
        return;
    }
    await processUpload(selectedFile, modelSelect.value);
};
uploadBtn.addEventListener('click', handleProcess);
uploadBtn2.addEventListener('click', handleProcess);

// Botão limpar (novo upload)
uploadClear.addEventListener('click', (e) => {
    e.stopPropagation();
    selectedFile = null;
    lastUploadedFileName = '';
    fileInput.value = '';
    closeDocViewerPanel();
    docViewerLoadedFile = null;
    updateViewDocBtnVisibility();
    daysData.length = 0;
    document.getElementById('tablesContainer').innerHTML = '';
    hideAllUploadStates();
    uploadContent.style.display = 'flex';
    statusBar.style.display = 'none';
    emptyState.style.display = 'flex';
    if (summaryCard) summaryCard.style.display = 'none';
    const ct1 = document.getElementById('copyToolbar');
    if (ct1) ct1.style.display = 'none';
    document.getElementById('headerMonth').textContent = `v${CONFIG.version}`;
    document.getElementById('serverInfo').innerHTML = '';
    localStorage.removeItem('uploadCollapsed');
    showToast('Pronto para novo upload', 'info');
});

// Toggle retrátil
uploadToggle.addEventListener('click', (e) => {
    e.stopPropagation();
    hideAllUploadStates();
    uploadCollapsed.style.display = 'flex';
    collapsedName.textContent = lastUploadedFileName;
    uploadZone.classList.add('has-success');
    localStorage.setItem('uploadCollapsed', 'true');
});

uploadExpand.addEventListener('click', (e) => {
    e.stopPropagation();
    hideAllUploadStates();
    uploadSuccess.style.display = 'flex';
    uploadZone.classList.add('has-success');
    localStorage.removeItem('uploadCollapsed');
});

// Função de upload
async function processUpload(file, model) {
    hideAllUploadStates();
    uploadLoading.style.display = 'flex';
    const modelName = modelSelect.options[modelSelect.selectedIndex].text;
    loadingModel.textContent = modelName;

    try {
        const formData = new FormData();
        formData.append('file', file);
        formData.append('model', model);

        const response = await fetch(`${CONFIG.apiBase}/api/upload`, {
            method: 'POST',
            body: formData,
        });

        const data = await response.json();

        if (!response.ok) {
            throw new Error(data.error || `Erro HTTP ${response.status}`);
        }

        // Sucesso!
        loadFromAPI(data);
        lastUploadedFileName = file.name;

        const nAjustados = data.timesheet.dias.filter((d) => d.o && d.o.includes(0)).length;
        hideAllUploadStates();
        uploadSuccess.style.display = 'flex';
        uploadZone.classList.add('has-success');
        uploadSuccessDetail.textContent = `${file.name} — ${data.timesheet.dias.length} dias • ${nAjustados} ajustados • ${modelName}`;
        showToast(
            `Pronto! ${data.timesheet.dias.length} dias processados, ${nAjustados} ajustados.`,
            'success',
        );

        // Auto-collapse if saved preference
        if (localStorage.getItem('uploadCollapsed') === 'true') {
            hideAllUploadStates();
            uploadCollapsed.style.display = 'flex';
            collapsedName.textContent = file.name;
            uploadZone.classList.add('has-success');
        }
    } catch (err) {
        hideAllUploadStates();
        uploadContent.style.display = 'flex';
        showToast(`Erro: ${err.message}`, 'error');
        console.error('Upload error:', err);
    }
}

// Carregar dados da API na tabela
function loadFromAPI(data) {
    const ts = data.timesheet;

    // Atualizar CONFIG
    if (ts.mes_ano) {
        CONFIG.mesAno = ts.mes_ano;
        const [mm, yyyy] = ts.mes_ano.split('/');
        const meses = [
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
        CONFIG.mesNome = mm ? `${meses[parseInt(mm)]} ${yyyy}` : '';
        const hdrInput = document.getElementById('headerMonthInput');
        if (hdrInput) hdrInput.value = ts.mes_ano;
    } else {
        CONFIG.mesAno = '';
        CONFIG.mesNome = '';
        const hdrInput = document.getElementById('headerMonthInput');
        if (hdrInput) hdrInput.value = '';
    }

    // Atualizar info do servidor
    if (ts.servidor && ts.servidor.nome) {
        activeServidor = ts.servidor;
        const info = ts.servidor;
        document.getElementById('serverInfo').innerHTML =
            `<strong>${info.nome}</strong><br>${info.matricula || ''} • ${info.cpf || ''}`;
    }

    // Substituir dados. Upload novo não traz escolhas manuais: elas só existem
    // em mês já salvo, e deWire devolve os padrões corretos.
    daysData.length = 0;
    ts.dias.forEach((d) => daysData.push(deWire(d)));

    // Mostrar elementos
    emptyState.style.display = 'none';
    statusBar.style.display = 'flex';
    if (summaryCard) summaryCard.style.display = 'block';
    const ct2 = document.getElementById('copyToolbar');
    if (ct2) ct2.style.display = 'flex';

    // Re-renderizar
    const container = document.getElementById('tablesContainer');
    container.innerHTML = '';
    renderTables();
    updateAll();
}

// ============================================
// Visualizador do documento original (painel lateral com zoom/pan)
// ============================================

const viewDocBtn = document.getElementById('viewDocBtn');
const docViewerPanel = document.getElementById('docViewerPanel');
const docViewerBody = document.getElementById('docViewerBody');
const docViewerStage = document.getElementById('docViewerStage');
const docViewerResize = document.getElementById('docViewerResize');
const docViewerPageNav = document.getElementById('docViewerPageNav');

let docViewerOpen = false;
let docViewerLoadedFile = null;
let docObjectUrl = null;
let docZoomScale = 1;
let docPanX = 0;
let docPanY = 0;
let docNaturalWidth = 0;
let docNaturalHeight = 0;
let docIsPanning = false;
let docPanStartMouse = { x: 0, y: 0 };
let docPanStartOffset = { x: 0, y: 0 };
let docPdfDoc = null;
let docPdfPage = 1;
let docPdfNumPages = 1;
let docPanelWidth = parseInt(localStorage.getItem('docViewerWidth') || '420', 10);
document.documentElement.style.setProperty('--doc-panel-width', docPanelWidth + 'px');

function updateViewDocBtnVisibility() {
    viewDocBtn.style.display = selectedFile ? 'flex' : 'none';
}

function applyDocTransform() {
    docViewerStage.style.transform = `translate(${docPanX}px, ${docPanY}px) scale(${docZoomScale})`;
    document.getElementById('docZoomPct').textContent = Math.round(docZoomScale * 100) + '%';
}

function fitDocToScreen() {
    if (!docNaturalWidth || !docNaturalHeight) return;
    const rect = docViewerBody.getBoundingClientRect();
    const scale = Math.min(
        (rect.width - 32) / docNaturalWidth,
        (rect.height - 32) / docNaturalHeight,
        4,
    );
    docZoomScale = scale > 0 ? scale : 1;
    docPanX = (rect.width - docNaturalWidth * docZoomScale) / 2;
    docPanY = (rect.height - docNaturalHeight * docZoomScale) / 2;
    applyDocTransform();
}

function zoomDocBy(factor, centerX, centerY) {
    const rect = docViewerBody.getBoundingClientRect();
    const cx = centerX !== undefined ? centerX : rect.width / 2;
    const cy = centerY !== undefined ? centerY : rect.height / 2;
    const newScale = clamp(docZoomScale * factor, 0.1, 10);
    const stagePointX = (cx - docPanX) / docZoomScale;
    const stagePointY = (cy - docPanY) / docZoomScale;
    docPanX = cx - stagePointX * newScale;
    docPanY = cy - stagePointY * newScale;
    docZoomScale = newScale;
    applyDocTransform();
}

let pdfJsPromise = null;
function ensurePdfJs() {
    if (window.pdfjsLib) return Promise.resolve(window.pdfjsLib);
    if (pdfJsPromise) return pdfJsPromise;
    pdfJsPromise = new Promise((resolve, reject) => {
        const script = document.createElement('script');
        script.src = 'https://cdnjs.cloudflare.com/ajax/libs/pdf.js/3.11.174/pdf.min.js';
        script.onload = () => {
            window.pdfjsLib.GlobalWorkerOptions.workerSrc =
                'https://cdnjs.cloudflare.com/ajax/libs/pdf.js/3.11.174/pdf.worker.min.js';
            resolve(window.pdfjsLib);
        };
        script.onerror = () => reject(new Error('Falha ao carregar leitor de PDF'));
        document.head.appendChild(script);
    });
    return pdfJsPromise;
}

function updatePdfPageIndicator() {
    document.getElementById('docPageIndicator').textContent = `${docPdfPage} / ${docPdfNumPages}`;
}

async function renderPdfPage(pageNum) {
    const page = await docPdfDoc.getPage(pageNum);
    const viewport = page.getViewport({ scale: 2 });
    const canvas = document.createElement('canvas');
    canvas.width = viewport.width;
    canvas.height = viewport.height;
    canvas.style.width = viewport.width / 2 + 'px';
    canvas.style.height = viewport.height / 2 + 'px';
    await page.render({ canvasContext: canvas.getContext('2d'), viewport }).promise;
    docViewerStage.innerHTML = '';
    docViewerStage.appendChild(canvas);
    docNaturalWidth = viewport.width / 2;
    docNaturalHeight = viewport.height / 2;
    fitDocToScreen();
    updatePdfPageIndicator();
}

async function renderPdfIntoStage(file) {
    docViewerStage.innerHTML = '<div class="doc-viewer-loading">Carregando PDF…</div>';
    try {
        const pdfjsLib = await ensurePdfJs();
        const bytes = new Uint8Array(await file.arrayBuffer());
        docPdfDoc = await pdfjsLib.getDocument({ data: bytes }).promise;
        docPdfNumPages = docPdfDoc.numPages;
        docPdfPage = 1;
        await renderPdfPage(docPdfPage);
        docViewerPageNav.style.display = docPdfNumPages > 1 ? 'flex' : 'none';
    } catch (err) {
        docViewerStage.innerHTML = '<div class="doc-viewer-loading">Erro ao carregar o PDF.</div>';
        console.error('Erro ao renderizar PDF:', err);
    }
}

function loadImageIntoStage(url) {
    return new Promise((resolve) => {
        docViewerStage.innerHTML = '';
        const img = document.createElement('img');
        img.draggable = false;
        img.onload = () => {
            docNaturalWidth = img.naturalWidth;
            docNaturalHeight = img.naturalHeight;
            fitDocToScreen();
            resolve();
        };
        img.src = url;
        docViewerStage.appendChild(img);
    });
}

async function loadDocumentIntoViewer(file) {
    document.getElementById('docViewerFilename').textContent = file.name;
    docPdfDoc = null;
    docViewerPageNav.style.display = 'none';
    if (docObjectUrl) {
        URL.revokeObjectURL(docObjectUrl);
    }
    const ext = file.name.split('.').pop().toLowerCase();
    if (ext === 'pdf') {
        await renderPdfIntoStage(file);
    } else {
        docObjectUrl = URL.createObjectURL(file);
        await loadImageIntoStage(docObjectUrl);
    }
}

async function openDocViewerPanel() {
    if (!selectedFile) return;
    docViewerOpen = true;
    document.body.classList.add('doc-panel-open');
    docViewerPanel.classList.add('show');
    viewDocBtn.classList.add('active');
    if (docViewerLoadedFile !== selectedFile) {
        docViewerLoadedFile = selectedFile;
        await loadDocumentIntoViewer(selectedFile);
    }
}

function closeDocViewerPanel() {
    docViewerOpen = false;
    document.body.classList.remove('doc-panel-open');
    docViewerPanel.classList.remove('show');
    viewDocBtn.classList.remove('active');
}

viewDocBtn.addEventListener('click', () => {
    if (docViewerOpen) closeDocViewerPanel();
    else openDocViewerPanel();
});
document.getElementById('docViewerClose').addEventListener('click', closeDocViewerPanel);

// Zoom por scroll, centrado na posição do mouse
docViewerBody.addEventListener(
    'wheel',
    (e) => {
        e.preventDefault();
        const rect = docViewerBody.getBoundingClientRect();
        const factor = e.deltaY < 0 ? 1.15 : 1 / 1.15;
        zoomDocBy(factor, e.clientX - rect.left, e.clientY - rect.top);
    },
    { passive: false },
);

// Pan por arraste
docViewerBody.addEventListener('mousedown', (e) => {
    if (e.button !== 0) return;
    docIsPanning = true;
    docPanStartMouse = { x: e.clientX, y: e.clientY };
    docPanStartOffset = { x: docPanX, y: docPanY };
    docViewerBody.classList.add('panning');
});
window.addEventListener('mousemove', (e) => {
    if (!docIsPanning) return;
    docPanX = docPanStartOffset.x + (e.clientX - docPanStartMouse.x);
    docPanY = docPanStartOffset.y + (e.clientY - docPanStartMouse.y);
    applyDocTransform();
});
window.addEventListener('mouseup', () => {
    docIsPanning = false;
    docViewerBody.classList.remove('panning');
});

// Botões de zoom
document.getElementById('docZoomIn').addEventListener('click', () => zoomDocBy(1.25));
document.getElementById('docZoomOut').addEventListener('click', () => zoomDocBy(1 / 1.25));
document.getElementById('docZoomReset').addEventListener('click', () => fitDocToScreen());

// Navegação de páginas (PDF)
document.getElementById('docPagePrev').addEventListener('click', () => {
    if (docPdfPage > 1) {
        docPdfPage--;
        renderPdfPage(docPdfPage);
    }
});
document.getElementById('docPageNext').addEventListener('click', () => {
    if (docPdfPage < docPdfNumPages) {
        docPdfPage++;
        renderPdfPage(docPdfPage);
    }
});

// Redimensionar o painel arrastando a borda
let docResizingPanel = false;
docViewerResize.addEventListener('mousedown', (e) => {
    docResizingPanel = true;
    e.preventDefault();
});
window.addEventListener('mousemove', (e) => {
    if (!docResizingPanel) return;
    docPanelWidth = clamp(e.clientX, 260, Math.min(900, window.innerWidth - 300));
    document.documentElement.style.setProperty('--doc-panel-width', docPanelWidth + 'px');
});
window.addEventListener('mouseup', () => {
    if (docResizingPanel) {
        docResizingPanel = false;
        localStorage.setItem('docViewerWidth', String(docPanelWidth));
    }
});

// --- Configurações (Settings) ---
const settingsBtn = document.getElementById('settingsBtn');
const settingsModal = document.getElementById('settingsModal');
const closeSettingsModal = document.getElementById('closeSettingsModal');
const saveSettingsBtn = document.getElementById('saveSettingsBtn');
const apiProviderSelect = document.getElementById('apiProvider');
const geminiApiKeyInput = document.getElementById('geminiApiKey');
const openRouterApiKeyInput = document.getElementById('openRouterApiKey');
const geminiKeyGroup = document.getElementById('geminiKeyGroup');
const openRouterKeyGroup = document.getElementById('openRouterKeyGroup');

// Armazenamento local temporário dos modelos
let availableModels = {
    provider: 'gemini',
    gemini_models: [],
    openrouter_models: [],
};

// Atualiza os seletores de modelo do upload conforme o provedor ativo
const updateModelSelectors = () => {
    const provider = apiProviderSelect.value;
    const models =
        provider === 'openrouter'
            ? availableModels.openrouter_models
            : availableModels.gemini_models;

    // Atualizar selects no DOM
    [modelSelect, modelSelect2].forEach((select) => {
        if (!select) return;
        select.innerHTML = '';
        models.forEach((m) => {
            const opt = document.createElement('option');
            opt.value = m.id;
            opt.textContent = m.name;
            opt.title = m.description;
            select.appendChild(opt);
        });
    });
};

// Alternar exibição dos campos de chave com base no provedor selecionado
apiProviderSelect.addEventListener('change', () => {
    const val = apiProviderSelect.value;
    if (val === 'openrouter') {
        geminiKeyGroup.style.display = 'none';
        openRouterKeyGroup.style.display = 'block';
    } else {
        geminiKeyGroup.style.display = 'block';
        openRouterKeyGroup.style.display = 'none';
    }
});

// Carrega os modelos disponíveis da API
const loadModelsList = async () => {
    try {
        const res = await fetch(`${CONFIG.apiBase}/api/models`);
        if (res.ok) {
            const data = await res.json();
            availableModels.provider = data.provider || 'gemini';
            availableModels.gemini_models = data.gemini_models || [];
            availableModels.openrouter_models = data.openrouter_models || [];

            apiProviderSelect.value = availableModels.provider;
            apiProviderSelect.dispatchEvent(new Event('change'));
            updateModelSelectors();
        }
    } catch (e) {
        console.error('Erro ao buscar modelos:', e);
    }
};

const openSettings = async () => {
    settingsModal.classList.add('show');
    try {
        const res = await fetch(`${CONFIG.apiBase}/api/settings`);
        if (res.ok) {
            const data = await res.json();
            apiProviderSelect.value = data.provider || 'gemini';
            apiProviderSelect.dispatchEvent(new Event('change'));

            if (data.has_gemini_key) {
                geminiApiKeyInput.placeholder = data.masked_gemini_key || 'Chave configurada';
            } else {
                geminiApiKeyInput.placeholder = 'AIzaSy...';
            }
            geminiApiKeyInput.value = '';

            if (data.has_openrouter_key) {
                openRouterApiKeyInput.placeholder =
                    data.masked_openrouter_key || 'Chave configurada';
            } else {
                openRouterApiKeyInput.placeholder = 'sk-or-...';
            }
            openRouterApiKeyInput.value = '';
        }
    } catch (e) {
        console.error('Erro ao carregar settings:', e);
    }
};

const closeSettings = () => {
    settingsModal.classList.remove('show');
    geminiApiKeyInput.value = '';
    openRouterApiKeyInput.value = '';
};

const saveSettings = async () => {
    const provider = apiProviderSelect.value;
    const geminiKey = geminiApiKeyInput.value.trim();
    const orKey = openRouterApiKeyInput.value.trim();

    // Validação básica se novas chaves estão sendo fornecidas
    if (
        provider === 'gemini' &&
        !geminiKey &&
        !geminiApiKeyInput.placeholder.includes('*') &&
        !geminiApiKeyInput.placeholder.includes('...')
    ) {
        showToast('A chave Gemini não pode estar vazia', 'error');
        return;
    }
    if (
        provider === 'openrouter' &&
        !orKey &&
        !openRouterApiKeyInput.placeholder.includes('*') &&
        !openRouterApiKeyInput.placeholder.includes('...')
    ) {
        showToast('A chave OpenRouter não pode estar vazia', 'error');
        return;
    }

    try {
        saveSettingsBtn.textContent = 'Salvando...';
        saveSettingsBtn.disabled = true;

        const payload = {
            provider: provider,
            gemini_api_key: geminiKey,
            open_router_api_key: orKey,
        };

        const res = await fetch(`${CONFIG.apiBase}/api/settings`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload),
        });
        const data = await res.json();

        if (res.ok) {
            showToast('Configurações salvas com sucesso!', 'success');
            // Recarregar os seletores de modelo baseados nas novas configurações
            updateModelSelectors();
            closeSettings();
        } else {
            showToast(data.error || 'Erro ao salvar', 'error');
        }
    } catch {
        showToast('Erro de rede ao salvar', 'error');
    } finally {
        saveSettingsBtn.textContent = 'Salvar';
        saveSettingsBtn.disabled = false;
    }
};

settingsBtn.addEventListener('click', openSettings);
closeSettingsModal.addEventListener('click', closeSettings);
saveSettingsBtn.addEventListener('click', saveSettings);
settingsModal.addEventListener('click', (e) => {
    if (e.target === settingsModal) closeSettings();
});

// Carregar modelos e inicializar
loadModelsList();

// --- Frase padrão da justificativa ---
const justTemplateInput = document.getElementById('justTemplate');
if (justTemplateInput) {
    const salvo = getJustTemplate();
    if (salvo !== JUST_TEMPLATE_PADRAO) justTemplateInput.value = salvo;

    justTemplateInput.addEventListener('input', () => {
        salvarJustTemplate(justTemplateInput.value.trim());
        updateAll(); // regenera todas as frases já exibidas
    });
}

// ============================================
// Persistência por Mês
// ============================================

let savedMonths = [];
let saveTimeout = null;

const mesAnoToLabel = (mesAno) => {
    if (!mesAno) return '';
    const [mm, yyyy] = mesAno.split('/');
    const mi = parseInt(mm);
    return mi >= 1 && mi <= 12 ? `${MESES_NOMES[mi]} ${yyyy}` : mesAno;
};

const mesAnoToPath = (mesAno) => mesAno.replace('/', '_');

// --- Indicador de salvamento ---
const showSaveStatus = (status) => {
    const el = document.getElementById('saveIndicator');
    if (!el) return;
    el.className = 'save-indicator';
    if (status === 'saving') {
        el.textContent = '⏳ Salvando...';
        el.classList.add('saving');
    } else if (status === 'saved') {
        el.textContent = '✓ Salvo';
        el.classList.add('saved');
        setTimeout(() => {
            el.textContent = '';
        }, 3000);
    } else if (status === 'error') {
        el.textContent = '✗ Erro';
        el.classList.add('saving');
    } else {
        el.textContent = '';
    }
};

// --- Auto-save debounced ---
const scheduleSave = () => {
    if (!CONFIG.mesAno) return;
    clearTimeout(saveTimeout);
    showSaveStatus('saving');
    saveTimeout = setTimeout(() => saveCurrentMonth(), 2000);
};

const saveCurrentMonth = async () => {
    if (!CONFIG.mesAno || daysData.length === 0) return;

    const monthDays = daysData.map(paraWire);

    // Gravar o servidor INTEIRO. Antes o payload mandava só
    // `{ nome: <texto raspado do DOM> }`, e cada auto-save apagava do arquivo o
    // CPF, a matrícula, o horário contratual, a unidade e o órgão — os campos
    // que o documento SEI preenche. Bastava editar um horário para perdê-los.
    const payload = {
        mes_ano: CONFIG.mesAno,
        servidor: activeServidor,
        dias: monthDays,
    };

    try {
        const res = await fetch(`${CONFIG.apiBase}/api/month/${mesAnoToPath(CONFIG.mesAno)}`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload),
        });
        if (res.ok) {
            showSaveStatus('saved');
            // Atualizar lista de meses se mudou
            await refreshMonthList();
        } else {
            showSaveStatus('error');
        }
    } catch (e) {
        console.error('Erro ao salvar mês:', e);
        showSaveStatus('error');
    }
};

// --- Carregar lista de meses ---
const refreshMonthList = async () => {
    try {
        const res = await fetch(`${CONFIG.apiBase}/api/months`);
        if (res.ok) {
            savedMonths = await res.json();
            updateMonthSelector();
        }
    } catch (e) {
        console.error('Erro ao listar meses:', e);
    }
};

// --- Atualizar seletor de mês no header ---
const updateMonthSelector = () => {
    const select = document.getElementById('monthSelect');
    const nav = document.getElementById('monthNav');
    if (!select || !nav) return;

    if (savedMonths.length === 0 && !CONFIG.mesAno) {
        nav.style.display = 'none';
        return;
    }

    nav.style.display = 'flex';
    select.innerHTML = '';

    savedMonths.forEach((m) => {
        const opt = document.createElement('option');
        opt.value = m.mes_ano;
        opt.textContent = mesAnoToLabel(m.mes_ano);
        if (m.mes_ano === CONFIG.mesAno) opt.selected = true;
        select.appendChild(opt);
    });

    // Se o mês atual não está na lista (recém processado), adicionar
    if (CONFIG.mesAno && !savedMonths.find((m) => m.mes_ano === CONFIG.mesAno)) {
        const opt = document.createElement('option');
        opt.value = CONFIG.mesAno;
        opt.textContent = mesAnoToLabel(CONFIG.mesAno) + ' (novo)';
        opt.selected = true;
        select.insertBefore(opt, select.firstChild);
    }
};

// --- Carregar um mês salvo ---
const loadMonthData = async (mesAno) => {
    try {
        const res = await fetch(`${CONFIG.apiBase}/api/month/${mesAnoToPath(mesAno)}`);
        if (!res.ok) {
            showToast('Mês não encontrado', 'error');
            return;
        }
        const data = await res.json();

        // Converter para formato do loadFromAPI
        CONFIG.mesAno = data.mes_ano;
        CONFIG.mesNome = mesAnoToLabel(data.mes_ano);

        // Atualizar info do servidor
        if (data.servidor && data.servidor.nome) {
            activeServidor = data.servidor;
            document.getElementById('serverInfo').innerHTML =
                `<strong>${activeServidor.nome}</strong><br>${activeServidor.matricula || ''} • ${activeServidor.cpf || ''}`;
        }

        // Substituir dados
        daysData.length = 0;
        data.dias.forEach((d) => daysData.push(deWire(d)));

        // Mostrar elementos
        emptyState.style.display = 'none';
        statusBar.style.display = 'flex';
        const summaryCard = document.querySelector('.summary-card');
        if (summaryCard) summaryCard.style.display = 'block';
        const ct3 = document.getElementById('copyToolbar');
        if (ct3) ct3.style.display = 'flex';
        const uploadZone = document.getElementById('uploadZone');
        if (uploadZone) uploadZone.style.display = 'block';

        // Re-renderizar
        const container = document.getElementById('tablesContainer');
        container.innerHTML = '';
        renderTables();
        updateAll();
        updateMonthSelector();

        showToast(`${mesAnoToLabel(mesAno)} carregado`, 'success');
    } catch (e) {
        console.error('Erro ao carregar mês:', e);
        showToast('Erro ao carregar mês', 'error');
    }
};

// --- Renderizar painel de meses salvos ---
const renderMonthListPanel = () => {
    const panel = document.getElementById('monthListPanel');
    const defaultState = document.getElementById('emptyStateDefault');
    const list = document.getElementById('monthList');
    if (!panel || !list) return;

    if (savedMonths.length === 0) {
        panel.style.display = 'none';
        if (defaultState) defaultState.style.display = 'block';
        return;
    }

    panel.style.display = 'block';
    if (defaultState) defaultState.style.display = 'none';

    list.innerHTML = '';
    savedMonths.forEach((m) => {
        const card = document.createElement('div');
        card.className = 'month-card';
        card.onclick = () => loadMonthData(m.mes_ano);

        const updatedStr = m.updated_at
            ? new Date(m.updated_at).toLocaleDateString('pt-BR', {
                  day: '2-digit',
                  month: '2-digit',
                  year: 'numeric',
                  hour: '2-digit',
                  minute: '2-digit',
              })
            : '';

        card.innerHTML = `
            <div class="month-card-left">
                <span class="month-card-title">${mesAnoToLabel(m.mes_ano)}</span>
                <span class="month-card-sub">${m.servidor_nome || 'Servidor'}</span>
            </div>
            <span class="month-card-date">${updatedStr}</span>
        `;
        list.appendChild(card);
    });
};

// --- Event Listeners: Navegação ◀▶ e seletor ---
document.getElementById('monthSelect')?.addEventListener('change', (e) => {
    loadMonthData(e.target.value);
});

document.getElementById('monthPrev')?.addEventListener('click', () => {
    const select = document.getElementById('monthSelect');
    if (select && select.selectedIndex < select.options.length - 1) {
        select.selectedIndex++;
        loadMonthData(select.value);
    }
});

document.getElementById('monthNext')?.addEventListener('click', () => {
    const select = document.getElementById('monthSelect');
    if (select && select.selectedIndex > 0) {
        select.selectedIndex--;
        loadMonthData(select.value);
    }
});

// Botão "Novo Mês" — mostra upload zone com estado vazio
document.getElementById('btnNewMonth')?.addEventListener('click', () => {
    // Resetar estado
    daysData.length = 0;
    CONFIG.mesAno = '';
    CONFIG.mesNome = '';

    // Esconder tabelas e mostrar upload
    document.getElementById('tablesContainer').innerHTML = '';
    document.getElementById('statusBar').style.display = 'none';
    const summaryCard = document.querySelector('.summary-card');
    if (summaryCard) summaryCard.style.display = 'none';
    const ct4 = document.getElementById('copyToolbar');
    if (ct4) ct4.style.display = 'none';
    document.getElementById('emptyState').style.display = 'none';

    // Mostrar upload zone em estado inicial
    const uploadZone = document.getElementById('uploadZone');
    if (uploadZone) uploadZone.style.display = 'block';
    const uploadContent = document.getElementById('uploadContent');
    if (uploadContent) uploadContent.style.display = 'flex';
    const uploadFileSelected = document.getElementById('uploadFileSelected');
    if (uploadFileSelected) uploadFileSelected.style.display = 'none';
    const uploadSuccess = document.getElementById('uploadSuccess');
    if (uploadSuccess) uploadSuccess.style.display = 'none';
    const uploadCollapsed = document.getElementById('uploadCollapsed');
    if (uploadCollapsed) uploadCollapsed.style.display = 'none';

    document.getElementById('monthNav').style.display = savedMonths.length > 0 ? 'flex' : 'none';
});

// --- Inicialização: carregar meses salvos ao abrir o app ---
(async () => {
    await refreshMonthList();
    if (savedMonths.length > 0) {
        // Mostrar painel de seleção de meses
        renderMonthListPanel();
    }
})();

// ============================================
// Tutorial Interativo (Tour)
// ============================================

const Tour = (() => {
    let currentStep = 0;
    let steps = [];
    let active = false;

    const overlay = document.getElementById('tourOverlay');
    const spotlight = document.getElementById('tourSpotlight');
    const tooltip = document.getElementById('tourTooltip');
    const titleEl = document.getElementById('tourTitle');
    const textEl = document.getElementById('tourText');
    const badgeEl = document.getElementById('tourBadge');
    const dotsEl = document.getElementById('tourDots');
    const arrowEl = document.getElementById('tourArrow');
    const prevBtn = document.getElementById('tourPrev');
    const nextBtn = document.getElementById('tourNext');
    const skipBtn = document.getElementById('tourSkip');

    // Passos PRÉ-upload
    const preUploadSteps = [
        {
            target: '.header-bar',
            title: '👋 Bem-vindo ao Ponto Real Go!',
            text: 'Este sistema lê sua folha de frequência, calcula saldos e gera justificativas automaticamente. Vamos aprender como usar!',
            pos: 'bottom',
        },
        {
            target: '.upload-zone',
            title: '📤 Upload da Folha de Ponto',
            text: 'Arraste uma imagem (PNG, JPEG) ou PDF da sua folha de frequência aqui, ou clique para selecionar o arquivo. O sistema usa IA para extrair os dados.',
            pos: 'bottom',
        },
        {
            target: '.model-select',
            title: '🤖 Modelo de IA',
            text: '<b>Gemini 2.5 Flash</b> (recomendado) é o mais preciso e rápido para a maioria das folhas. <b>Flash Lite</b> é uma opção mais econômica para layouts simples, e <b>Pro</b> é indicado para folhas com layout complexo ou baixa qualidade.',
            pos: 'bottom',
        },
        {
            target: '.btn-upload',
            title: '▶️ Processar',
            text: 'Após selecionar o arquivo, clique aqui para enviar a imagem para a IA. O processamento leva de 5 a 15 segundos.',
            pos: 'bottom',
        },
        {
            target: '#settingsBtn',
            title: '⚙️ Configurações',
            text: 'Configure sua chave de API aqui. Escolha entre <b>Google Gemini</b> (direto, chave gratuita em aistudio.google.com) ou <b>OpenRouter</b> como provedor de IA.',
            pos: 'bottom-left',
        },
        {
            target: '#helpBtn',
            title: '❓ Ajuda',
            text: 'Clique aqui a qualquer momento para iniciar este tutorial novamente. Agora faça o upload da sua folha de ponto!',
            pos: 'bottom-left',
            last: true,
        },
    ];

    // Passos PÓS-upload (tabelas carregadas)
    const postUploadSteps = [
        {
            target: '.status-bar',
            title: '📊 Barra de Status',
            text: 'Resumo do mês: <b>Faltas</b> não justificadas, dias <b>Ajustados</b> pelo sistema, dias <b>Completos</b>, e os saldos <b>Extraído</b> (da imagem) e <b>Real</b> (calculado).',
            pos: 'bottom',
        },
        {
            target: '#viewDocBtn',
            title: '🔍 Ver Documento Original',
            text: 'Abre um painel lateral com a imagem ou PDF que você enviou. Dá para dar <b>zoom</b>, arrastar (<b>pan</b>) e navegar entre páginas — útil para conferir os dados extraídos lado a lado com a tabela.',
            pos: 'bottom-left',
        },
        {
            target: '.main-table',
            title: '📋 Tabela Principal',
            text: 'Todos os dias do mês com seus horários. Cada coluna representa: <b>E</b>=Entrada, <b>S</b>=Saída do almoço, <b>E</b>=Retorno do almoço, <b>S</b>=Saída final.',
            pos: 'bottom',
        },
        {
            target: '.main-table input[type="time"]:not(.readonly)',
            title: '✏️ Campos Editáveis',
            text: 'Horários em <b style="color:var(--info)">azul</b> foram gerados automaticamente e podem ser editados. Horários <b>pretos</b> são os originais extraídos da imagem.',
            pos: 'bottom',
        },
        {
            target: '.col-motivo',
            title: '🏷️ Tipo de Dia & Ocorrência',
            text: 'Seletor de tipo: <b>Útil</b>, <b>FDS</b> (auto-detectado), <b>Dispensa</b> (meio período), <b>Feriado</b>, <b>Folga</b>, <b>Convocação</b> ou <b>Reduzido</b> (expediente reduzido por decreto — informe a carga exigida em minutos). O checkbox <b>Ocorrência</b> ao lado inclui ou exclui manualmente o dia da seção de Ocorrência do SEI.',
            pos: 'top',
        },
        {
            target: '.saldo-cell',
            title: '📌 Saldo (Clique p/ Copiar)',
            text: 'O saldo do dia. <b>Passe o mouse</b> para ver o saldo original da imagem no tooltip. <b>Clique</b> para copiar o valor. Valores <span style="color:var(--danger)">negativos</span> indicam horas devidas.',
            pos: 'top',
        },
        {
            target: '.icon-btn[onclick*="copyRow"]',
            title: '📋 Copiar Linha',
            text: 'Copia os dados da linha com dia, horários e saldo original. Ideal para colar no sistema do estado.',
            pos: 'left',
        },
        {
            target: '.copy-toolbar',
            title: '📑 Barra de Cópia',
            text: '<b>Gerar SEI</b> monta o formulário completo (dados do servidor e chefia) pronto para colar no editor do SEI. Os demais botões copiam seções soltas: <b>Tabela</b>, <b>Ocorrência</b>, <b>Justificativa</b> ou <b>Tudo</b> — separados por TAB, prontos para o Excel!',
            pos: 'bottom',
        },
        {
            target: '.doc-table:not(.justificativa)',
            title: '📝 Ocorrência',
            text: 'Lista os dias ajustados que entram no documento do SEI. Marque/desmarque o checkbox <b>Ocorrência</b> na tabela principal para incluir ou excluir um dia manualmente, ou clique no <b>✕</b> de uma linha aqui para removê-la rapidamente.',
            pos: 'top',
        },
        {
            target: '#justTemplate',
            title: '✍️ Justificativa Editável',
            text: 'Personalize a frase padrão usada em todas as justificativas (ex: "esqueci de registrar"). O sistema completa automaticamente com os horários faltantes de cada dia.',
            pos: 'top',
        },
        {
            target: '#monthNav',
            title: '📅 Navegação de Meses',
            text: 'Navegue entre meses salvos usando as setas ou o seletor. O sistema salva automaticamente cada análise. O indicador 💾 mostra quando há salvamento pendente.',
            pos: 'bottom',
        },
        {
            target: '#themeToggle',
            title: '🌙 Tema Claro/Escuro',
            text: 'Alterne entre tema claro e escuro conforme sua preferência. A escolha é salva automaticamente.',
            pos: 'bottom-left',
            last: true,
        },
    ];

    function getSteps() {
        const hasData = daysData.length > 0;
        return hasData ? postUploadSteps : preUploadSteps;
    }

    function positionTooltip(targetEl, pos) {
        const rect = targetEl.getBoundingClientRect();
        const tt = tooltip;
        const pad = 12;

        // Reset
        tt.style.top = '';
        tt.style.left = '';
        tt.style.right = '';
        tt.style.bottom = '';
        arrowEl.className = 'tour-tooltip-arrow';

        // Show first to calculate size
        tt.style.display = 'block';
        const ttRect = tt.getBoundingClientRect();

        let top, left;

        switch (pos) {
            case 'bottom':
                top = rect.bottom + pad;
                left = rect.left + rect.width / 2 - ttRect.width / 2;
                arrowEl.classList.add('arrow-top');
                break;
            case 'top':
                top = rect.top - ttRect.height - pad;
                left = rect.left + rect.width / 2 - ttRect.width / 2;
                arrowEl.classList.add('arrow-bottom');
                break;
            case 'left':
                top = rect.top + rect.height / 2 - ttRect.height / 2;
                left = rect.left - ttRect.width - pad;
                arrowEl.classList.add('arrow-right');
                break;
            case 'right':
                top = rect.top + rect.height / 2 - ttRect.height / 2;
                left = rect.right + pad;
                arrowEl.classList.add('arrow-left');
                break;
            case 'bottom-left':
                top = rect.bottom + pad;
                left = rect.right - ttRect.width;
                arrowEl.classList.add('arrow-top');
                break;
        }

        // Clamp to viewport
        left = Math.max(8, Math.min(left, window.innerWidth - ttRect.width - 8));
        top = Math.max(8, Math.min(top, window.innerHeight - ttRect.height - 8));

        tt.style.top = top + 'px';
        tt.style.left = left + 'px';
    }

    function isElementVisible(el) {
        if (!el || !el.isConnected) return false;
        const rect = el.getBoundingClientRect();
        if (rect.width === 0 && rect.height === 0) return false;
        const style = window.getComputedStyle(el);
        return style.display !== 'none' && style.visibility !== 'hidden';
    }

    function showStep(idx, direction = 1) {
        steps = getSteps();
        if (idx < 0) {
            end();
            return;
        }
        if (idx >= steps.length) {
            end();
            return;
        }

        const step = steps[idx];
        const target = document.querySelector(step.target);

        if (!isElementVisible(target)) {
            // Passo com alvo ausente/oculto no estado atual: pula na mesma direção
            const nextIdx = idx + direction;
            if (nextIdx >= 0 && nextIdx < steps.length) showStep(nextIdx, direction);
            else end();
            return;
        }
        currentStep = idx;

        // Scroll target into view
        target.scrollIntoView({ behavior: 'smooth', block: 'center' });

        setTimeout(() => {
            // Spotlight
            const rect = target.getBoundingClientRect();
            spotlight.style.top = rect.top - 6 + 'px';
            spotlight.style.left = rect.left - 6 + 'px';
            spotlight.style.width = rect.width + 12 + 'px';
            spotlight.style.height = rect.height + 12 + 'px';

            // Content
            badgeEl.textContent = `${idx + 1}/${steps.length}`;
            titleEl.textContent = step.title;
            textEl.innerHTML = step.text;

            // Dots
            dotsEl.innerHTML = steps
                .map((_, i) => `<span class="tour-dot${i === idx ? ' active' : ''}"></span>`)
                .join('');

            // Nav
            prevBtn.style.display = idx === 0 ? 'none' : 'inline-flex';
            nextBtn.textContent = step.last ? '✅ Concluir' : 'Próximo →';

            // Position
            positionTooltip(target, step.pos);

            // Show
            overlay.style.display = 'block';
            tooltip.style.display = 'block';
        }, 300);
    }

    function start() {
        active = true;
        currentStep = 0;
        steps = getSteps();
        overlay.style.display = 'block';
        showStep(0);
    }

    function end() {
        active = false;
        overlay.style.display = 'none';
        tooltip.style.display = 'none';
        localStorage.setItem('tourCompleted', 'true');
    }

    function next() {
        if (currentStep < steps.length - 1) showStep(currentStep + 1, 1);
        else end();
    }

    function prev() {
        if (currentStep > 0) showStep(currentStep - 1, -1);
    }

    // Events
    if (nextBtn) nextBtn.addEventListener('click', next);
    if (prevBtn) prevBtn.addEventListener('click', prev);
    if (skipBtn) skipBtn.addEventListener('click', end);
    if (overlay)
        overlay.addEventListener('click', (e) => {
            if (e.target === overlay) end();
        });

    // Keyboard
    document.addEventListener('keydown', (e) => {
        if (!active) return;
        if (e.key === 'Escape') end();
        if (e.key === 'ArrowRight' || e.key === 'Enter') next();
        if (e.key === 'ArrowLeft') prev();
    });

    // Window resize: reposition
    window.addEventListener('resize', () => {
        if (active) showStep(currentStep);
    });

    return { start, end, isActive: () => active };
})();

// Help button
document.getElementById('helpBtn')?.addEventListener('click', () => Tour.start());

// Auto-start tour for first-time visitors
if (!localStorage.getItem('tourCompleted')) {
    setTimeout(() => Tour.start(), 800);
}

// Re-launch post-upload tour when data loads for first time
let postTourShown = false;
const checkPostUploadTour = () => {
    if (!postTourShown && daysData.length > 0 && !localStorage.getItem('postTourShown')) {
        postTourShown = true;
        localStorage.setItem('postTourShown', 'true');
        setTimeout(() => Tour.start(), 500);
    }
};
setInterval(() => {
    if (daysData.length > 0 && !postTourShown && !localStorage.getItem('postTourShown')) {
        checkPostUploadTour();
    }
}, 2000);

// ============================================
// --- Exportação para o SEI ---
// ============================================

const saveSeiFields = () => {
    localStorage.setItem('seiChefiaNome', document.getElementById('seiChefiaNome').value);
    localStorage.setItem('seiChefiaLotacao', document.getElementById('seiChefiaLotacao').value);
};

const generateSeiHtml = () => {
    const sNome = document.getElementById('seiServidorNome').value;
    const sLotacao = document.getElementById('seiServidorLotacao').value;
    const sCpf = document.getElementById('seiServidorCpf').value;
    const sSuperior = document.getElementById('seiServidorSuperior').value;
    const cNome = document.getElementById('seiChefiaNome').value;
    const cLotacao = document.getElementById('seiChefiaLotacao').value;

    // O MESMO predicado que monta as tabelas da tela. Enquanto a regra estava
    // reescrita aqui, o documento e a tela podiam discordar sobre quem entra —
    // e é o documento que vai assinado.
    const noDocumento = daysData.filter(isOccurrenceDay);

    const celula = (conteudo) =>
        `<td style="border: 1px solid #000000; text-align: center; font-family: Arial, sans-serif; font-size: 12px; padding: 6px;">${conteudo}</td>`;

    let ocorrenciasRowsHtml = '';
    let justificativasHtml = '';

    noDocumento.forEach((d) => {
        const horarios = CAMPOS_HORARIO.map((f) => celula(esc(d[f]) || '&nbsp;')).join('');
        ocorrenciasRowsHtml += `<tr>${celula(esc(dataDoDia(d)))}${horarios}</tr>`;

        // A justificativa é a única com filtro extra: um dia sem nada escrito
        // vira um parágrafo em branco no documento, e parágrafo em branco não é
        // justificativa de coisa nenhuma. Na TELA ele continua aparecendo, com o
        // campo vazio esperando o texto.
        const faltantes = camposFaltantes(d, classifyDay(d) === 'dispensa');
        const frase = textoDaJustificativa(d, dataDoDia(d), faltantes);
        if (!frase) return;
        justificativasHtml += `<p style="margin: 0 0 6px 0; font-family: Arial, sans-serif; font-size: 12px;">${esc(frase)}</p>`;
    });

    // Cada seção tem o seu vazio: um mês pode ter justificativa sem nenhuma
    // linha de horário a declarar.
    if (!ocorrenciasRowsHtml) {
        ocorrenciasRowsHtml = `<tr><td colspan="5" style="border: 1px solid #000000; text-align: center; font-family: Arial, sans-serif; font-size: 12px; padding: 10px; color: #888;">Nenhuma ocorrência gerada para este mês</td></tr>`;
    }
    if (!justificativasHtml) {
        justificativasHtml = `<p style="margin: 0; font-family: Arial, sans-serif; font-size: 12px; color: #888;">Nenhuma justificativa necessária</p>`;
    }

    const chefiaDisplay = cLotacao ? `${cNome} - ${cLotacao}` : cNome;

    const fullHtml = `
<div style="font-family: Arial, sans-serif; font-size: 12px; color: #000000; background-color: #ffffff; padding: 10px; max-width: 800px; margin: 0 auto;">
    <p style="font-family: Arial, sans-serif; font-size: 13px; font-weight: bold; margin-top: 0; margin-bottom: 5px;">1. Identificação</p>
    <table border="1" cellspacing="0" cellpadding="6" style="width:100%; border-collapse:collapse; font-family:Arial, sans-serif; font-size:12px; border:1px solid #000000; margin-bottom: 15px;">
        <tbody>
            <tr>
                <td style="width: 50%; font-weight: bold; text-align: center; background-color: #f2f2f2; border: 1px solid #000000; font-family: Arial, sans-serif; font-size: 12px; padding: 6px;">Nome do Servidor</td>
                <td style="width: 50%; font-weight: bold; text-align: center; background-color: #f2f2f2; border: 1px solid #000000; font-family: Arial, sans-serif; font-size: 12px; padding: 6px;">Lotação</td>
            </tr>
            <tr>
                <td style="border: 1px solid #000000; font-family: Arial, sans-serif; font-size: 12px; padding: 6px;">${sNome || '&nbsp;'}</td>
                <td style="border: 1px solid #000000; font-family: Arial, sans-serif; font-size: 12px; padding: 6px;">${sLotacao || '&nbsp;'}</td>
            </tr>
            <tr>
                <td style="font-weight: bold; text-align: center; background-color: #f2f2f2; border: 1px solid #000000; font-family: Arial, sans-serif; font-size: 12px; padding: 6px;">CPF</td>
                <td style="font-weight: bold; text-align: center; background-color: #f2f2f2; border: 1px solid #000000; font-family: Arial, sans-serif; font-size: 12px; padding: 6px;">Unidade Administrativa Superior</td>
            </tr>
            <tr>
                <td style="border: 1px solid #000000; font-family: Arial, sans-serif; font-size: 12px; padding: 6px;">${sCpf || '&nbsp;'}</td>
                <td style="border: 1px solid #000000; font-family: Arial, sans-serif; font-size: 12px; padding: 6px;">${sSuperior || '&nbsp;'}</td>
            </tr>
        </tbody>
    </table>

    <p style="font-family: Arial, sans-serif; font-size: 13px; font-weight: bold; margin-top: 15px; margin-bottom: 5px;">2. Ocorrência</p>
    <table border="1" cellspacing="0" cellpadding="6" style="width:100%; border-collapse:collapse; font-family:Arial, sans-serif; font-size:12px; border:1px solid #000000; margin-bottom: 15px;">
        <thead>
            <tr style="background-color: #f2f2f2;">
                <th style="font-weight: bold; text-align: center; border: 1px solid #000000; width: 20%; font-family: Arial, sans-serif; font-size: 12px; padding: 6px;">Data</th>
                <th style="font-weight: bold; text-align: center; border: 1px solid #000000; width: 20%; font-family: Arial, sans-serif; font-size: 12px; padding: 6px;">Entrada</th>
                <th style="font-weight: bold; text-align: center; border: 1px solid #000000; width: 20%; font-family: Arial, sans-serif; font-size: 12px; padding: 6px;">Saída(Intervalo)</th>
                <th style="font-weight: bold; text-align: center; border: 1px solid #000000; width: 20%; font-family: Arial, sans-serif; font-size: 12px; padding: 6px;">Entrada(intervalo)</th>
                <th style="font-weight: bold; text-align: center; border: 1px solid #000000; width: 20%; font-family: Arial, sans-serif; font-size: 12px; padding: 6px;">Saída</th>
            </tr>
        </thead>
        <tbody>
            ${ocorrenciasRowsHtml}
        </tbody>
    </table>

    <p style="font-family: Arial, sans-serif; font-size: 13px; font-weight: bold; margin-top: 15px; margin-bottom: 5px;">3. Justificativa do Servidor (Obrigatório)</p>
    <table border="1" cellspacing="0" cellpadding="6" style="width:100%; border-collapse:collapse; font-family:Arial, sans-serif; font-size:12px; border:1px solid #000000; margin-bottom: 15px;">
        <tbody>
            <tr style="background-color: #f2f2f2;">
                <td style="font-weight: bold; border: 1px solid #000000; font-family: Arial, sans-serif; font-size: 12px; padding: 6px;">Relatório de Atividades ou Documentação Comprobatória</td>
            </tr>
            <tr>
                <td style="border: 1px solid #000000; padding: 10px; line-height: 1.5; font-family: Arial, sans-serif; font-size: 12px;">
                    ${justificativasHtml}
                </td>
            </tr>
        </tbody>
    </table>

    <p style="font-family: Arial, sans-serif; font-size: 13px; font-weight: bold; margin-top: 15px; margin-bottom: 5px;">4. Observação da Chefia Imediata (Opcional)</p>
    <table border="1" cellspacing="0" cellpadding="6" style="width:100%; border-collapse:collapse; font-family:Arial, sans-serif; font-size:12px; border:1px solid #000000; margin-bottom: 15px;">
        <tbody>
            <tr style="background-color: #f2f2f2;">
                <td style="font-weight: bold; border: 1px solid #000000; font-family: Arial, sans-serif; font-size: 12px; padding: 6px;">Descrição</td>
            </tr>
            <tr>
                <td style="border: 1px solid #000000; height: 35px; font-family: Arial, sans-serif; font-size: 12px; padding: 6px;">&nbsp;</td>
            </tr>
        </tbody>
    </table>

    <p style="font-family: Arial, sans-serif; font-size: 13px; font-weight: bold; margin-top: 15px; margin-bottom: 5px;">5. Nome da Chefia Imediata</p>
    <table border="1" cellspacing="0" cellpadding="6" style="width:100%; border-collapse:collapse; font-family:Arial, sans-serif; font-size:12px; border:1px solid #000000; margin-bottom: 15px;">
        <tbody>
            <tr>
                <td style="border: 1px solid #000000; width: 50%; font-weight: bold; font-family: Arial, sans-serif; font-size: 12px; padding: 6px;">${chefiaDisplay || '&nbsp;'}</td>
                <td style="border: 1px solid #000000; width: 50%; font-family: Arial, sans-serif; font-size: 12px; padding: 6px;">&nbsp;</td>
            </tr>
        </tbody>
    </table>

    <p style="font-family: Arial, sans-serif; font-size: 13px; font-weight: bold; margin-top: 15px; margin-bottom: 5px;">6. Nome do Servidor</p>
    <table border="1" cellspacing="0" cellpadding="6" style="width:100%; border-collapse:collapse; font-family:Arial, sans-serif; font-size:12px; border:1px solid #000000;">
        <tbody>
            <tr>
                <td style="border: 1px solid #000000; font-weight: bold; height: 25px; font-family: Arial, sans-serif; font-size: 12px; padding: 6px;">${sNome || '&nbsp;'}</td>
            </tr>
        </tbody>
    </table>
</div>`;

    return fullHtml;
};

window.updateSeiPreview = () => {
    const container = document.getElementById('seiPreviewContainer');
    if (container) {
        container.innerHTML = generateSeiHtml();
    }
};

window.openSeiModal = () => {
    const modal = document.getElementById('seiModal');
    if (!modal) return;

    // Popular campos do Servidor a partir da variável global activeServidor
    document.getElementById('seiServidorNome').value = activeServidor.nome || '';
    document.getElementById('seiServidorLotacao').value = activeServidor.unidade || '';
    document.getElementById('seiServidorCpf').value = activeServidor.cpf || '';
    document.getElementById('seiServidorSuperior').value = activeServidor.orgao || '';

    // Popular chefia a partir do localStorage
    document.getElementById('seiChefiaNome').value = localStorage.getItem('seiChefiaNome') || '';
    document.getElementById('seiChefiaLotacao').value =
        localStorage.getItem('seiChefiaLotacao') || '';

    window.updateSeiPreview();

    modal.classList.add('show');
};

window.closeSeiModal = () => {
    const modal = document.getElementById('seiModal');
    if (modal) {
        modal.classList.remove('show');
    }
};

window.copySeiDocument = async () => {
    saveSeiFields();
    const htmlContent = generateSeiHtml();

    // Fallback de texto simples
    const tempDiv = document.createElement('div');
    tempDiv.innerHTML = htmlContent;
    const textContent = tempDiv.innerText;

    try {
        const blob = new Blob([htmlContent], { type: 'text/html' });
        const blobText = new Blob([textContent], { type: 'text/plain' });
        const data = [new ClipboardItem({ 'text/html': blob, 'text/plain': blobText })];
        await navigator.clipboard.write(data);

        const copyBtn = document.getElementById('copySeiBtn');
        const oldText = copyBtn.textContent;
        copyBtn.textContent = 'Copiado! ✓';
        copyBtn.style.background = 'var(--success)';
        setTimeout(() => {
            copyBtn.textContent = oldText;
            copyBtn.style.background = '';
        }, 2000);

        showToast(
            'Formulário SEI copiado como Rich Text! Agora é só colar direto no SEI.',
            'success',
        );
        window.closeSeiModal();
    } catch (err) {
        console.error('Erro ao copiar HTML: ', err);
        try {
            await navigator.clipboard.writeText(textContent);
            showToast('Copiado como texto simples (limitação do navegador)', 'warning');
            window.closeSeiModal();
        } catch {
            showToast('Erro ao copiar dados para a área de transferência', 'error');
        }
    }
};
