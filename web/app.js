// ============================================
// Ponto Real Go — Front-end Application
// ============================================

// --- Configuração ---
const CONFIG = {
    version: '0.3.0',
    mesAno: '',
    mesNome: '',
    apiBase: '',
};

// --- Dados (começa vazio, populado via upload) ---
let daysData = [];

// --- Utilidades ---
const t2m = (t) => {
    if (!t || t === '**:**') return 0;
    const parts = t.split(':');
    if (parts.length !== 2) return 0;
    return parseInt(parts[0]) * 60 + parseInt(parts[1]);
};

const m2t = (m) => {
    const sign = m < 0 ? '-' : '+';
    const abs = Math.abs(m);
    return sign + String(Math.floor(abs / 60)).padStart(2, '0') + ':' + String(abs % 60).padStart(2, '0');
};

const m2tUnsigned = (m) => {
    const abs = Math.abs(m);
    return String(Math.floor(abs / 60)).padStart(2, '0') + ':' + String(abs % 60).padStart(2, '0');
};

const parseOrigSaldo = (s) => {
    if (!s) return 0;
    const isNeg = s.includes('-');
    const pts = s.replace('-', '').replace('+', '').split(':');
    return pts.length === 2 ? (isNeg ? -1 : 1) * (parseInt(pts[0]) * 60 + parseInt(pts[1])) : 0;
};

const isTimeValid = (t) => {
    if (!t || t === '**:**') return false;
    const parts = t.split(':');
    if (parts.length !== 2) return false;
    const h = parseInt(parts[0]), m = parseInt(parts[1]);
    return !isNaN(h) && !isNaN(m) && h >= 0 && h <= 23 && m >= 0 && m <= 59;
};

// --- Calendário real: obter dia da semana a partir do mês/ano ---
const DIAS_SEMANA = ['Dom', 'Seg', 'Ter', 'Qua', 'Qui', 'Sex', 'Sáb'];
const getRealWeekday = (dayNum) => {
    if (!CONFIG.mesAno) return null;
    const [mm, yyyy] = CONFIG.mesAno.split('/');
    const date = new Date(parseInt(yyyy), parseInt(mm) - 1, dayNum);
    return DIAS_SEMANA[date.getDay()];
};

const isWeekend = (d) => {
    // Primeiro: tentar pelo calendário real
    const realW = getRealWeekday(d.d);
    if (realW) return realW === 'Sáb' || realW === 'Dom';
    // Fallback: usar campo w extraído
    return d.w === 'Sáb' || d.w === 'Sab' || d.w === 'Dom';
};

// --- Classificação de dia ---
const classifyDay = (d) => {
    // Respeitar override manual do usuário
    if (d.dayTypeOverride === 'fds') return 'folga';
    if (d.dayTypeOverride === 'feriado') return 'recesso';
    if (d.dayTypeOverride === 'folga') return 'folga';
    if (d.dayTypeOverride === 'convocacao') return 'recesso';
    if (d.dayTypeOverride === 'dispensa') return 'dispensa';

    if (d.mot && d.mot.toUpperCase().includes('DISPENSA')) return 'dispensa';
    if (d.mot && (d.mot.toUpperCase().includes('RECESSO') || d.mot.toUpperCase().includes('FERIADO') || d.mot.toUpperCase().includes('FACULTATIVO'))) return 'recesso';
    if (isWeekend(d)) return 'folga';

    let pontos = 0;
    if (isTimeValid(d.e1)) pontos++;
    if (isTimeValid(d.s1)) pontos++;
    if (isTimeValid(d.e2)) pontos++;
    if (isTimeValid(d.s2)) pontos++;

    if (pontos === 4) return 'completo';
    if (pontos > 0) return 'parcial';
    if (!isWeekend(d) && d.saldo === '-08:00') return 'falta';
    return 'folga';
};

// --- Feature 2: Day Type Override ---
window.changeDayType = (idx, newType) => {
    daysData[idx].dayTypeOverride = newType === 'util' ? null : newType;
    const container = document.getElementById('tablesContainer');
    container.innerHTML = '';
    renderTables();
    updateAll();
    scheduleSave();
};

// --- Feature 4: Auto-fill client-side (espelho do RulesAdjuster) ---
const randBetween = (min, max) => Math.floor(Math.random() * (max - min + 1)) + min;
const avoidRoundMins = (m) => {
    const mins = m % 60;
    if (mins === 0) return m + randBetween(1, 5);
    if (mins % 5 === 0) return m + randBetween(1, 3);
    return m;
};

const autoFillDay = (idx, changedField) => {
    const d = daysData[idx];
    if (!d.o) return; // sem info de originais, não auto-preencher

    const fields = ['e1', 's1', 'e2', 's2'];

    // === Smart Shift: ordenar cronologicamente todos os valores preenchidos ===
    // Quando o usuário preenche um campo fora de ordem, os valores são
    // automaticamente reorganizados em ordem cronológica.
    // Ex: e1=10:05, s1=12:50, e2=19:48, s2=11:50 → e1=10:05, s1=11:50, e2=12:50, s2=19:48
    const filledVals = fields.map(f => isTimeValid(d[f]) ? t2m(d[f]) : null);
    const filledEntries = filledVals
        .map((v, i) => ({ idx: i, val: v }))
        .filter(e => e.val !== null);

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

    // === Dispensa: NÃO auto-preencher campos vazios ===
    // Em dias de dispensa o usuário pode editar 1 ou mais horários
    // sem que o sistema complete automaticamente os restantes.
    const tipo = classifyDay(d);
    if (tipo === 'dispensa') return;

    // === Auto-fill: preencher campos vazios restantes ===
    const vals = fields.map(f => isTimeValid(d[f]) ? t2m(d[f]) : null);
    const empty = fields.filter((f, i) => vals[i] === null && d.o[i] === 0);

    if (empty.length === 0) return; // nada pra preencher

    const CARGA = 480; // 8h em min
    const ALMOCO_MIN = 60, ALMOCO_MAX = 75;

    // Tentar preencher campos vazios com base nos preenchidos
    let changed = false;
    const set = (f, m) => {
        const fi = fields.indexOf(f);
        if (vals[fi] === null && d.o[fi] === 0) {
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
        if (vals[1] === null && d.o[1] === 0) set('s1', vals[0] + manha);
        if (vals[2] === null && d.o[2] === 0) {
            const s1Val = vals[1] !== null ? vals[1] : vals[0] + manha;
            set('e2', s1Val + almoco);
        }
    }
    // Caso: e1 conhecido, s2 faltando
    if (vals[0] !== null && vals[3] === null && d.o[3] === 0) {
        const almoco = randBetween(ALMOCO_MIN, ALMOCO_MAX);
        const manha = Math.round(CARGA * 0.48) + randBetween(-5, 5);
        if (vals[1] === null && d.o[1] === 0) set('s1', vals[0] + manha);
        const s1Val = vals[1] !== null ? vals[1] : vals[0] + manha;
        if (vals[2] === null && d.o[2] === 0) set('e2', s1Val + almoco);
        const e2Val = vals[2] !== null ? vals[2] : s1Val + almoco;
        const tarde = CARGA - (s1Val - vals[0]);
        set('s2', e2Val + Math.max(tarde, 180));
    }
    // Caso: s2 conhecido, e1 faltando
    if (vals[3] !== null && vals[0] === null && d.o[0] === 0) {
        const almoco = randBetween(ALMOCO_MIN, ALMOCO_MAX);
        set('e1', vals[3] - CARGA - almoco);
        const e1Val = vals[0] !== null ? vals[0] : vals[3] - CARGA - almoco;
        const manha = Math.round(CARGA * 0.48) + randBetween(-5, 5);
        if (vals[1] === null && d.o[1] === 0) set('s1', e1Val + manha);
        const s1Val = vals[1] !== null ? vals[1] : e1Val + manha;
        if (vals[2] === null && d.o[2] === 0) set('e2', s1Val + almoco);
    }
    // Caso: s1 e e2 conhecidos mas e1 ou s2 faltando
    if (vals[1] !== null && vals[2] !== null) {
        if (vals[0] === null && d.o[0] === 0) {
            const manha = Math.round(CARGA * 0.48) + randBetween(-5, 5);
            set('e1', vals[1] - manha);
        }
        if (vals[3] === null && d.o[3] === 0) {
            const e1Val = vals[0] !== null ? vals[0] : vals[1] - 240;
            const tarde = CARGA - (vals[1] - e1Val);
            set('s2', vals[2] + Math.max(tarde, 180));
        }
    }

    if (changed) {
        // Atualizar inputs no DOM
        fields.forEach(f => {
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
    row.querySelectorAll('.validation-badge').forEach(b => b.remove());

    if (!isTimeValid(d.e1) || !isTimeValid(d.s1) || !isTimeValid(d.e2) || !isTimeValid(d.s2)) return;

    const m1 = t2m(d.e1), m2 = t2m(d.s1), m3 = t2m(d.e2), m4 = t2m(d.s2);
    const errors = [];
    if (m1 >= m2) errors.push('Entrada ≥ Saída almoço');
    if (m2 >= m3) errors.push('Saída almoço ≥ Retorno');
    if (m3 >= m4) errors.push('Retorno ≥ Saída');
    if ((m2 - m1) + (m4 - m3) < 480) errors.push('Carga < 8h');
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
    navigator.clipboard.writeText(text).then(() => {
        if (sourceEl) {
            // Feedback visual inline
            const orig = sourceEl.title;
            sourceEl.classList.add('just-copied');
            sourceEl.title = '✓ Copiado!';
            setTimeout(() => { sourceEl.classList.remove('just-copied'); sourceEl.title = orig || ''; }, 1200);
        }
        showToast('Copiado!', 'success');
    }).catch(() => {
        showToast('Erro ao copiar', 'error');
    });
};

const copyBtn = (targetId) => {
    const el = document.getElementById(targetId);
    const val = el.value || el.innerText;
    copyToClipboard(val, el);
};

// Copiar célula ao clicar (delegado)
window.copyCell = (text, el) => {
    if (text && text.trim()) copyToClipboard(text.trim(), el);
};

// Copiar linha completa
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
    const rows = daysData.map(d => {
        const dia = String(d.d).padStart(2, '0');
        return [dia, d.e1, d.s1, d.e2, d.s2, d.es, d.saldo, d.ocor, d.mot, d.w].join('\t');
    });
    copyToClipboard(header + '\n' + rows.join('\n'));
};

// Copiar seção Ocorrência
window.copyOcorrencia = () => {
    const body = document.getElementById('ocorrenciaBody');
    if (!body || !body.rows.length) { showToast('Nenhuma ocorrência', 'info'); return; }
    const header = 'Data\tEntrada\tSaída(Intervalo)\tEntrada(Intervalo)\tSaída';
    const rows = Array.from(body.rows).map(r => {
        return Array.from(r.cells).map(c => {
            const input = c.querySelector('input');
            return input ? input.value : c.innerText.trim();
        }).join('\t');
    });
    copyToClipboard(header + '\n' + rows.join('\n'));
};

// Copiar seção Justificativa
window.copyJustificativa = () => {
    const body = document.getElementById('justificativaBody');
    if (!body || !body.rows.length) { showToast('Nenhuma justificativa', 'info'); return; }
    const rows = Array.from(body.rows).map(r => {
        const input = r.querySelector('input');
        return input ? input.value : r.innerText.trim();
    });
    copyToClipboard(rows.join('\n'));
};

// Copiar TUDO (tabela + ocorrência + justificativa)
window.copyAll = () => {
    let out = '=== TABELA DE FREQUÊNCIA ===\n';
    out += 'Dia\tE1\tS1\tE2\tS2\tE/S\tSaldo Original\tOcor\tMotivo\n';
    daysData.forEach(d => {
        const dia = String(d.d).padStart(2, '0');
        out += [dia, d.e1, d.s1, d.e2, d.s2, d.es, d.saldo, d.ocor, d.mot].join('\t') + '\n';
    });

    out += '\n=== OCORRÊNCIA ===\n';
    const ocBody = document.getElementById('ocorrenciaBody');
    if (ocBody) {
        out += 'Data\tEntrada\tSaída(Int.)\tEntrada(Int.)\tSaída\n';
        Array.from(ocBody.rows).forEach(r => {
            out += Array.from(r.cells).map(c => {
                const input = c.querySelector('input');
                return input ? input.value : c.innerText.trim();
            }).join('\t') + '\n';
        });
    }

    out += '\n=== JUSTIFICATIVA DO SERVIDOR ===\n';
    const juBody = document.getElementById('justificativaBody');
    if (juBody) {
        Array.from(juBody.rows).forEach(r => {
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

// --- Atualizar tudo ---
const updateAll = () => {
    let totalSaldoOficial = 0;
    let totalSaldoReal = 0;
    let totalFaltas = 0;
    let totalAjustados = 0;
    let totalCompletos = 0;

    const ocorBody = document.getElementById('ocorrenciaBody');
    const justBody = document.getElementById('justificativaBody');
    ocorBody.innerHTML = '';
    justBody.innerHTML = '';

    daysData.forEach((d, idx) => {
        const tipo = classifyDay(d);
        const fields = ['e1', 's1', 'e2', 's2'];

        // Atualizar tipo visual na tabela principal
        const mainRow = document.getElementById(`row_${d.d}`);
        if (mainRow) mainRow.setAttribute('data-type', tipo);

        // Contagens
        if (tipo === 'falta') totalFaltas++;
        if (tipo === 'completo') totalCompletos++;
        if (d.o && d.o.includes(0)) totalAjustados++;

        let realDiff = null;

        // Feature 2: Se override feriado/folga/fds/convocação, saldo = 0
        if (d.dayTypeOverride === 'feriado' || d.dayTypeOverride === 'folga' || d.dayTypeOverride === 'fds' || d.dayTypeOverride === 'convocacao') {
            realDiff = 0;
            // Não adiciona nada ao totalSaldoReal (dia neutro)
        } else if (tipo === 'dispensa') {
            // Dispensa: calcular saldo com base nos horários preenchidos
            // Esperado = metade da carga diária (4h = 240 min)
            const CARGA_DISPENSA = 240;
            let worked = 0;
            if (isTimeValid(d.e1) && isTimeValid(d.s1)) worked += t2m(d.s1) - t2m(d.e1);
            if (isTimeValid(d.e2) && isTimeValid(d.s2)) worked += t2m(d.s2) - t2m(d.e2);
            if (worked > 0) {
                realDiff = worked - CARGA_DISPENSA;
                totalSaldoReal += realDiff;
                const esEl = document.getElementById(`m_es_${d.d}`);
                if (esEl) esEl.innerText = m2tUnsigned(worked);
            } else {
                totalSaldoReal += parseOrigSaldo(d.saldo);
            }
        } else if (isTimeValid(d.e1) && isTimeValid(d.s1) && isTimeValid(d.e2) && isTimeValid(d.s2)) {
            const m1 = t2m(d.e1), m2 = t2m(d.s1), m3 = t2m(d.e2), m4 = t2m(d.s2);
            realDiff = ((m2 - m1) + (m4 - m3)) - 480;
            totalSaldoReal += realDiff;

            const esEl = document.getElementById(`m_es_${d.d}`);
            if (esEl) esEl.innerText = m2tUnsigned(realDiff + 480);
        } else if (tipo === 'falta') {
            realDiff = -480;
            totalSaldoReal += realDiff;
        } else {
            totalSaldoReal += parseOrigSaldo(d.saldo);
        }

        // Saldo extraído sempre entra pro total oficial
        totalSaldoOficial += parseOrigSaldo(d.saldo);

        // Renderizar Saldo na tabela principal
        const saldoEl = document.getElementById(`m_saldo_${d.d}`);
        if (saldoEl) {
            if (d.dayTypeOverride === 'feriado' || d.dayTypeOverride === 'folga' || d.dayTypeOverride === 'fds' || d.dayTypeOverride === 'convocacao') {
                // Feriado/Folga/FDS/Convocação override -> saldo neutro
                const labels = { feriado: 'Feriado', folga: 'Folga', fds: 'FDS', convocacao: 'Convocação' };
                const label = labels[d.dayTypeOverride] || d.dayTypeOverride;
                saldoEl.innerHTML = `<span style="color:var(--text-muted);font-size:10px;">${label}</span>`;
            } else if (tipo === 'dispensa' && realDiff !== null) {
                // Dispensa: mostrar saldo recalculado
                const origSaldo = d.saldo || '';
                if (origSaldo && m2t(realDiff) !== origSaldo) {
                    saldoEl.innerHTML = `<del style="color:var(--text-muted);font-size:10px">${origSaldo}</del><br><span style="color:${realDiff < 0 ? 'var(--danger)' : 'var(--success)'};font-weight:600">${m2t(realDiff)}</span>`;
                } else {
                    saldoEl.innerHTML = `<span style="color:${realDiff < 0 ? 'var(--danger)' : 'var(--success)'};font-weight:600">${m2t(realDiff)}</span>`;
                }
            } else if (realDiff !== null && isTimeValid(d.e1) && isTimeValid(d.s1) && isTimeValid(d.e2) && isTimeValid(d.s2)) {
                // Matemática do realDiff
                const realFormatted = m2t(realDiff).replace('+', '');
                if (realFormatted !== d.saldo && d.saldo !== "") {
                    // Diferente do oficial -> Riscado em cima, destacado embaixo
                    saldoEl.innerHTML = `<del style="color:var(--text-muted);font-size:10px">${d.saldo}</del><br><span style="color:${realDiff < 0 ? 'var(--danger)' : 'var(--success)'};font-weight:600">${m2t(realDiff)}</span>`;
                } else if (d.saldo !== "") {
                    saldoEl.innerHTML = `<span style="color:${realDiff < 0 ? 'var(--danger)' : 'var(--text)'};font-weight:600">${m2t(realDiff)}</span>`;
                } else {
                    saldoEl.innerHTML = `<span style="color:${realDiff < 0 ? 'var(--danger)' : 'var(--text)'};font-weight:600">${m2t(realDiff)}</span>`;
                }
            } else if (tipo === 'falta') {
                saldoEl.innerHTML = `<del style="color:var(--text-muted);font-size:10px">${d.saldo}</del><br><span style="color:var(--danger);font-weight:600">-08:00</span>`;
            } else {
                saldoEl.innerHTML = `<span style="color:${d.saldo.includes('-') ? 'var(--danger)' : 'var(--text)'};">${d.saldo}</span>`;
            }
        }

        if (d.o && d.o.includes(0)) {
            const isDispensa = tipo === 'dispensa';

            // Para dispensa: só mostrar se o campo editável foi preenchido
            // Para dias normais: mostrar todos os campos (inclusive gerados)
            const hasEditedFields = isDispensa
                ? fields.some((f, fi) => d.o[fi] === 0 && isTimeValid(d[f]))
                : true;

            if (hasEditedFields) {
                // Renderizar ocorrência (Feature 4: onblur em vez de onchange)
                const mkOcorInput = (val, isOrig, f, fi) => {
                    // Dispensa: campos não preenchidos ficam como placeholder
                    if (isDispensa && !isTimeValid(val) && !isOrig) {
                        return `<input type="time" value="" onfocus="saveState(${idx})" onblur="syncChange(${idx}, '${f}', this.value)">`;
                    }
                    return `<input type="time" value="${val}" ${isOrig ? 'class="readonly" readonly' : ''} onfocus="saveState(${idx})" onblur="syncChange(${idx}, '${f}', this.value)">`;
                };

                const trOc = document.createElement('tr');
                trOc.innerHTML = `<td>${String(d.d).padStart(2, '0')}/${CONFIG.mesAno}</td>
                    <td>${mkOcorInput(d.e1, d.o[0], 'e1', 0)}</td><td>${mkOcorInput(d.s1, d.o[1], 's1', 1)}</td>
                    <td>${mkOcorInput(d.e2, d.o[2], 'e2', 2)}</td><td>${mkOcorInput(d.s2, d.o[3], 's2', 3)}</td>`;
                ocorBody.appendChild(trOc);

                // Renderizar justificativa
                const faltantes = [];
                const fields_names = ['a entrada', 'a saída do almoço', 'a entrada do almoço', 'a saída'];
                fields.forEach((f, fi) => {
                    if (d.o[fi] === 0) {
                        // Dispensa: só listar se o campo foi preenchido pelo usuário
                        if (!isDispensa || isTimeValid(d[f])) {
                            faltantes.push(fields_names[fi]);
                        }
                    }
                });

                if (faltantes.length > 0) {
                    const mesAnoPrint = CONFIG.mesAno ? CONFIG.mesAno : '??/????';
                    const frase = `${String(d.d).padStart(2, '0')}/${mesAnoPrint} - O ponto não registrou ${faltantes.join(', ').replace(/,([^,]*)$/, ' e$1')}.`;

                    const trJu = document.createElement('tr');
                    trJu.innerHTML = `<td><input type="text" id="just_${d.d}" class="just-input" value="${frase}">
                <button class="icon-btn" onclick="copyBtn('just_${d.d}')"><svg viewBox="0 0 24 24"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path></svg></button></td>`;
                    justBody.appendChild(trJu);
                }
            }
        }
    });

    // Atualizar status bar
    document.getElementById('totalFaltas').innerText = totalFaltas;
    document.getElementById('totalAjustados').innerText = totalAjustados;
    document.getElementById('totalCompletos').innerText = totalCompletos;

    const saldoOfcEl = document.getElementById('saldoTotalOficial');
    if (saldoOfcEl) saldoOfcEl.innerText = m2t(totalSaldoOficial);

    const saldoEl = document.getElementById('saldoTotal');
    if (saldoEl) {
        saldoEl.innerText = m2t(totalSaldoReal);
        saldoEl.className = 'status-value ' + (totalSaldoReal < 0 ? 'status-danger' : 'status-success');
    }
};

// --- Sync Change (Two-Way Binding) — Feature 4: sem confirm(), com auto-fill ---
window.syncChange = (idx, field, newVal) => {
    const d = daysData[idx];
    d[field] = newVal;

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

// --- Renderizar tabela principal ---
const renderTables = () => {
    const container = document.getElementById('tablesContainer');
    if (daysData.length === 0) return;

    // Gerar blocos dinamicamente (semanas)
    const blocks = [];
    for (let i = 0; i < daysData.length; i += 7) {
        blocks.push([i, Math.min(i + 6, daysData.length - 1)]);
    }

    blocks.forEach(block => {
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
                const emptyClass = isOrig && !v ? 'class="readonly empty-time" readonly' : '';
                if (!v && (d.dayTypeOverride === 'feriado' || d.dayTypeOverride === 'folga' || d.dayTypeOverride === 'fds' || d.dayTypeOverride === 'convocacao')) return `<span class="empty-time">——:——</span>`;
                if (!v && d.dayTypeOverride !== 'dispensa' && isWeekend(d) && !d.mot) return `<span class="empty-time">——:——</span>`;
                if (!v && d.dayTypeOverride !== 'dispensa' && d.saldo === '-08:00' && !d.mot && !isWeekend(d)) return `<span class="empty-time">——:——</span>`;
                if (!v) return `<input type="time" id="m_${f}_${d.d}" value="" onfocus="saveState(${i})" onblur="syncChange(${i}, '${f}', this.value)">`;
                return `<input type="time" id="m_${f}_${d.d}" value="${v}" ${isOrig ? `class="readonly draggable-time"` : `${canDrag} class="draggable-time"`} ${canDrag} onfocus="saveState(${i})" onblur="syncChange(${i}, '${f}', this.value)">`;
            };

            let saldoHtml = '';
            if (d.saldo_real && d.saldo_real !== d.saldo && d.saldo !== "") {
                const isNeg = d.saldo_real.includes('-');
                saldoHtml = `<del style="color:var(--text-muted);font-size:10px">${d.saldo}</del><br><span style="color:${isNeg ? 'var(--danger)' : 'var(--success)'};font-weight:600">${d.saldo_real.replace('+', '')}</span>`;
            } else if (d.saldo_real && d.saldo !== "") {
                const isNeg = d.saldo_real.includes('-');
                saldoHtml = `<span style="color:${isNeg ? 'var(--danger)' : 'var(--text)'};font-weight:600">${d.saldo_real.replace('+', '')}</span>`;
            } else if (d.saldo !== "") {
                const isNeg = d.saldo.includes('-');
                saldoHtml = `<span style="color:${isNeg ? 'var(--danger)' : 'var(--text)'};">${d.saldo}</span>`;
            }

            const tdSaldo = `<td id="m_saldo_${d.d}" class="saldo-cell" style="font-family: 'JetBrains Mono', monospace; font-size: 11px;cursor:pointer" onclick="copyCell('${d.saldo}', this)" title="Saldo orig: ${d.saldo} — Clique p/ copiar">${saldoHtml}</td>`;

            let diaDisplay = diaFmt;
            if (tipo === 'falta') {
                diaDisplay = `<span title="Falta injustificada" style="color:var(--danger)">${diaFmt} 🔴</span>`;
            }

            // Seletor de tipo de dia — disponível para todos os dias
            // Auto-detecção inteligente: FDS pelo calendário, dispensa/feriado pelo motivo
            const autoDetectDispensa = d.mot && d.mot.toUpperCase().includes('DISPENSA');
            const autoDetectFeriado = d.mot && (d.mot.toUpperCase().includes('RECESSO') || d.mot.toUpperCase().includes('FERIADO') || d.mot.toUpperCase().includes('FACULTATIVO'));
            const autoDetectFDS = isWeekend(d);

            let selected = d.dayTypeOverride || 'util';
            // Auto-setar override se detectado e ainda não definido manualmente
            if (!d.dayTypeOverride) {
                if (autoDetectFDS) { selected = 'fds'; d.dayTypeOverride = 'fds'; }
                else if (autoDetectDispensa) { selected = 'dispensa'; d.dayTypeOverride = 'dispensa'; }
                else if (autoDetectFeriado) { selected = 'feriado'; d.dayTypeOverride = 'feriado'; }
            }

            const selectClass = ['feriado','folga','convocacao','dispensa','fds'].includes(selected) ? ` is-${selected}` : '';
            const motivoText = d.mot ? ` title="${d.mot}"` : '';
            let motivoHtml = `<td class="col-motivo"${motivoText}>
                <select class="day-type-select${selectClass}" onchange="changeDayType(${i}, this.value)">
                    <option value="util"${selected === 'util' ? ' selected' : ''}>Útil</option>
                    <option value="fds"${selected === 'fds' ? ' selected' : ''}>FDS</option>
                    <option value="dispensa"${selected === 'dispensa' ? ' selected' : ''}>Dispensa</option>
                    <option value="feriado"${selected === 'feriado' ? ' selected' : ''}>Feriado</option>
                    <option value="folga"${selected === 'folga' ? ' selected' : ''}>Folga</option>
                    <option value="convocacao"${selected === 'convocacao' ? ' selected' : ''}>Convocação</option>
                </select>
                ${d.mot ? `<span class="mot-text" title="${d.mot}">${d.mot.substring(0, 60)}${d.mot.length > 60 ? '...' : ''}</span>` : ''}
            </td>`;

            tr.innerHTML = `
                <td class="col-dia">${diaDisplay}</td>
                <td>${mkMainInput(d.e1, d.o ? d.o[0] : 1, 'e1')}</td><td>${mkMainInput(d.s1, d.o ? d.o[1] : 1, 's1')}</td>
                <td>${mkMainInput(d.e2, d.o ? d.o[2] : 1, 'e2')}</td><td>${mkMainInput(d.s2, d.o ? d.o[3] : 1, 's2')}</td>
                <td class="col-es" id="m_es_${d.d}" style="font-family:'JetBrains Mono',monospace;font-size:11px;cursor:pointer" onclick="copyCell('${d.es}', this)" title="Clique p/ copiar">${d.es}</td>
                ${tdSaldo}
                <td style="font-family:'JetBrains Mono',monospace;font-size:11px;cursor:pointer" onclick="copyCell('${d.ocor}', this)" title="Clique p/ copiar">${d.ocor}</td>
                ${motivoHtml}
                <td class="col-week">${d.w}</td>
                <td class="col-acao">
                    <button class="icon-btn" onclick="copyRow(${i})" title="Copiar linha">
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

        const idx = daysData.findIndex(d => d.d === srcDia);
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

// --- Inicialização ---
const hdrInput = document.getElementById('headerMonthInput');
if (hdrInput) {
    hdrInput.value = '';
    hdrInput.addEventListener('change', (e) => {
        CONFIG.mesAno = e.target.value.trim();
        if (CONFIG.mesAno.length >= 7) {
            const [mm, yyyy] = CONFIG.mesAno.split('/');
            const meses = ['', 'Janeiro', 'Fevereiro', 'Março', 'Abril', 'Maio', 'Junho', 'Julho', 'Agosto', 'Setembro', 'Outubro', 'Novembro', 'Dezembro'];
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
modelSelect.addEventListener('change', () => { modelSelect2.value = modelSelect.value; });
modelSelect2.addEventListener('change', () => { modelSelect.value = modelSelect2.value; });

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
    }
});

// Click para selecionar
uploadZone.addEventListener('click', (e) => {
    if (e.target.closest('.upload-controls') || e.target.closest('.upload-success') ||
        e.target.closest('.upload-file-selected') || e.target.closest('.upload-collapsed') ||
        e.target.closest('.btn-link') || e.target.closest('.btn-toggle')) return;
    fileInput.click();
});

fileInput.addEventListener('change', () => {
    if (fileInput.files.length > 0) {
        selectedFile = fileInput.files[0];
        showFileSelected(selectedFile);
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

        const response = await fetch('/api/upload', {
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

        const nAjustados = data.timesheet.dias.filter(d => d.o && d.o.includes(0)).length;
        hideAllUploadStates();
        uploadSuccess.style.display = 'flex';
        uploadZone.classList.add('has-success');
        uploadSuccessDetail.textContent = `${file.name} — ${data.timesheet.dias.length} dias • ${nAjustados} ajustados • ${modelName}`;
        showToast(`Pronto! ${data.timesheet.dias.length} dias processados, ${nAjustados} ajustados.`, 'success');

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
        const meses = ['', 'Janeiro', 'Fevereiro', 'Março', 'Abril', 'Maio', 'Junho', 'Julho', 'Agosto', 'Setembro', 'Outubro', 'Novembro', 'Dezembro'];
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
        const info = ts.servidor;
        document.getElementById('serverInfo').innerHTML =
            `<strong>${info.nome}</strong><br>${info.matricula || ''} • ${info.cpf || ''}`;
    }

    // Substituir dados
    daysData.length = 0;
    ts.dias.forEach(d => {
        daysData.push({
            d: d.d,
            w: d.w || '',
            e1: d.e1 || '',
            s1: d.s1 || '',
            e2: d.e2 || '',
            s2: d.s2 || '',
            es: d.es || '',
            saldo: d.saldo || '',
            ocor: d.ocor || '',
            mot: d.mot || '',
            o: d.o || undefined,
            tipo: d.tipo || undefined,
            dayTypeOverride: null, // Feature 2
        });
    });

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

// --- Configurações (Settings) ---
const settingsBtn = document.getElementById('settingsBtn');
const settingsModal = document.getElementById('settingsModal');
const closeSettingsModal = document.getElementById('closeSettingsModal');
const saveSettingsBtn = document.getElementById('saveSettingsBtn');
const geminiApiKeyInput = document.getElementById('geminiApiKey');

const openSettings = async () => {
    settingsModal.classList.add('show');
    try {
        const res = await fetch('/api/settings');
        if (res.ok) {
            const data = await res.json();
            if (data.has_key) {
                geminiApiKeyInput.placeholder = data.masked_key;
                geminiApiKeyInput.value = ''; // não expor a chave real
            }
        }
    } catch (e) {
        console.error('Erro ao carregar settings:', e);
    }
};

const closeSettings = () => {
    settingsModal.classList.remove('show');
    geminiApiKeyInput.value = '';
};

const saveSettings = async () => {
    const key = geminiApiKeyInput.value.trim();
    if (!key && !geminiApiKeyInput.placeholder.includes('*') && !geminiApiKeyInput.placeholder.includes('...')) {
        showToast('A chave não pode estar vazia', 'error');
        return;
    }
    if (!key) {
        closeSettings();
        return; // não mudou nada
    }

    try {
        saveSettingsBtn.textContent = 'Salvando...';
        saveSettingsBtn.disabled = true;
        const res = await fetch('/api/settings', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ gemini_api_key: key })
        });
        const data = await res.json();

        if (res.ok) {
            showToast('Chave salva com sucesso!', 'success');
            closeSettings();
        } else {
            showToast(data.error || 'Erro ao salvar', 'error');
        }
    } catch (e) {
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

// ============================================
// Persistência por Mês
// ============================================

let savedMonths = [];
let saveTimeout = null;

const MESES_NOMES = ['', 'Janeiro', 'Fevereiro', 'Março', 'Abril', 'Maio', 'Junho',
    'Julho', 'Agosto', 'Setembro', 'Outubro', 'Novembro', 'Dezembro'];

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
        setTimeout(() => { el.textContent = ''; }, 3000);
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

    const monthDays = daysData.map(d => ({
        d: d.d, w: d.w,
        e1: d.e1, s1: d.s1, e2: d.e2, s2: d.s2,
        es: d.es, saldo: d.saldo, saldo_real: d.saldo_real || '',
        ocor: d.ocor, mot: d.mot,
        o: d.o || undefined, tipo: d.tipo || undefined,
        day_type_override: d.dayTypeOverride || undefined,
    }));

    const serverInfoEl = document.getElementById('serverInfo');
    const servidorNome = serverInfoEl ? serverInfoEl.querySelector('strong')?.textContent || '' : '';

    const payload = {
        mes_ano: CONFIG.mesAno,
        servidor: { nome: servidorNome },
        dias: monthDays,
    };

    try {
        const res = await fetch(`/api/month/${mesAnoToPath(CONFIG.mesAno)}`, {
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
        const res = await fetch('/api/months');
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

    savedMonths.forEach(m => {
        const opt = document.createElement('option');
        opt.value = m.mes_ano;
        opt.textContent = mesAnoToLabel(m.mes_ano);
        if (m.mes_ano === CONFIG.mesAno) opt.selected = true;
        select.appendChild(opt);
    });

    // Se o mês atual não está na lista (recém processado), adicionar
    if (CONFIG.mesAno && !savedMonths.find(m => m.mes_ano === CONFIG.mesAno)) {
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
        const res = await fetch(`/api/month/${mesAnoToPath(mesAno)}`);
        if (!res.ok) {
            showToast('Mês não encontrado', 'error');
            return;
        }
        const data = await res.json();

        // Converter para formato do loadFromAPI
        CONFIG.mesAno = data.mes_ano;
        const [mm, yyyy] = data.mes_ano.split('/');
        CONFIG.mesNome = mesAnoToLabel(data.mes_ano);

        // Atualizar info do servidor
        if (data.servidor && data.servidor.nome) {
            document.getElementById('serverInfo').innerHTML =
                `<strong>${data.servidor.nome}</strong>`;
        }

        // Substituir dados
        daysData.length = 0;
        data.dias.forEach(d => {
            daysData.push({
                d: d.d, w: d.w || '',
                e1: d.e1 || '', s1: d.s1 || '',
                e2: d.e2 || '', s2: d.s2 || '',
                es: d.es || '', saldo: d.saldo || '',
                saldo_real: d.saldo_real || '',
                ocor: d.ocor || '', mot: d.mot || '',
                o: d.o || undefined, tipo: d.tipo || undefined,
                dayTypeOverride: d.day_type_override || null,
            });
        });

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
    savedMonths.forEach(m => {
        const card = document.createElement('div');
        card.className = 'month-card';
        card.onclick = () => loadMonthData(m.mes_ano);

        const updatedStr = m.updated_at
            ? new Date(m.updated_at).toLocaleDateString('pt-BR', { day: '2-digit', month: '2-digit', year: 'numeric', hour: '2-digit', minute: '2-digit' })
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
            pos: 'bottom'
        },
        {
            target: '.upload-zone',
            title: '📤 Upload da Folha de Ponto',
            text: 'Arraste uma imagem (PNG, JPEG) ou PDF da sua folha de frequência aqui, ou clique para selecionar o arquivo. O sistema usa IA para extrair os dados.',
            pos: 'bottom'
        },
        {
            target: '.model-select',
            title: '🤖 Modelo de IA',
            text: '<b>Flash Lite</b> é mais rápido e gratuito. <b>Pro</b> é mais preciso para imagens difíceis. Recomendamos começar com Flash Lite.',
            pos: 'bottom'
        },
        {
            target: '.btn-upload',
            title: '▶️ Processar',
            text: 'Após selecionar o arquivo, clique aqui para enviar a imagem para a IA. O processamento leva de 5 a 15 segundos.',
            pos: 'bottom'
        },
        {
            target: '#settingsBtn',
            title: '⚙️ Configurações',
            text: 'Configure sua chave da API Gemini aqui. Necessário para o processamento de imagens. Obtenha sua chave gratuita em <b>aistudio.google.com</b>.',
            pos: 'bottom-left'
        },
        {
            target: '#helpBtn',
            title: '❓ Ajuda',
            text: 'Clique aqui a qualquer momento para iniciar este tutorial novamente. Agora faça o upload da sua folha de ponto!',
            pos: 'bottom-left',
            last: true
        }
    ];

    // Passos PÓS-upload (tabelas carregadas)
    const postUploadSteps = [
        {
            target: '.status-bar',
            title: '📊 Barra de Status',
            text: 'Resumo do mês: <b>Faltas</b> não justificadas, dias <b>Ajustados</b> pelo sistema, dias <b>Completos</b>, e os saldos <b>Extraído</b> (da imagem) e <b>Real</b> (calculado).',
            pos: 'bottom'
        },
        {
            target: '.main-table',
            title: '📋 Tabela Principal',
            text: 'Todos os dias do mês com seus horários. Cada coluna representa: <b>E</b>=Entrada, <b>S</b>=Saída do almoço, <b>E</b>=Retorno do almoço, <b>S</b>=Saída final.',
            pos: 'bottom'
        },
        {
            target: '.main-table input[type="time"]:not(.readonly)',
            title: '✏️ Campos Editáveis',
            text: 'Horários em <b style="color:var(--info)">azul</b> foram gerados automaticamente e podem ser editados. Horários <b>pretos</b> são os originais extraídos da imagem.',
            pos: 'bottom'
        },
        {
            target: '.day-type-select',
            title: '🏷️ Tipo de Dia',
            text: 'Cada dia tem um seletor: <b>Útil</b>, <b>FDS</b> (auto-detectado), <b>Dispensa</b> (meio período), <b>Feriado</b>, <b>Folga</b> ou <b>Convocação</b>. Altere conforme necessário.',
            pos: 'top'
        },
        {
            target: '.saldo-cell',
            title: '📌 Saldo (Clique p/ Copiar)',
            text: 'O saldo do dia. <b>Passe o mouse</b> para ver o saldo original da imagem no tooltip. <b>Clique</b> para copiar o valor. Valores <span style="color:var(--danger)">negativos</span> indicam horas devidas.',
            pos: 'top'
        },
        {
            target: '.icon-btn[onclick*="copyRow"]',
            title: '📋 Copiar Linha',
            text: 'Copia os dados da linha com dia, horários e saldo original. Ideal para colar no sistema do estado.',
            pos: 'left'
        },
        {
            target: '.copy-toolbar',
            title: '📑 Barra de Cópia',
            text: 'Copie seções inteiras: <b>Tabela</b> (dados completos), <b>Ocorrência</b>, <b>Justificativa</b>, ou <b>Tudo</b> de uma vez. Dados são separados por TAB — cole diretamente no Excel!',
            pos: 'bottom'
        },
        {
            target: '.doc-section-header',
            title: '📝 Ocorrência & Justificativa',
            text: 'As seções de <b>Ocorrência</b> mostram os dias ajustados, e a <b>Justificativa</b> gera frases automáticas para o sistema do estado. Cada seção tem botão "Copiar".',
            pos: 'top'
        },
        {
            target: '#monthNav',
            title: '📅 Navegação de Meses',
            text: 'Navegue entre meses salvos usando as setas ou o seletor. O sistema salva automaticamente cada análise. O indicador 💾 mostra quando há salvamento pendente.',
            pos: 'bottom'
        },
        {
            target: '#themeToggle',
            title: '🌙 Tema Claro/Escuro',
            text: 'Alterne entre tema claro e escuro conforme sua preferência. A escolha é salva automaticamente.',
            pos: 'bottom-left',
            last: true
        }
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

    function showStep(idx) {
        currentStep = idx;
        steps = getSteps();
        if (idx >= steps.length) { end(); return; }

        const step = steps[idx];
        const target = document.querySelector(step.target);

        if (!target) {
            // Skip invisible steps
            if (idx < steps.length - 1) showStep(idx + 1);
            else end();
            return;
        }

        // Scroll target into view
        target.scrollIntoView({ behavior: 'smooth', block: 'center' });

        setTimeout(() => {
            // Spotlight
            const rect = target.getBoundingClientRect();
            spotlight.style.top = (rect.top - 6) + 'px';
            spotlight.style.left = (rect.left - 6) + 'px';
            spotlight.style.width = (rect.width + 12) + 'px';
            spotlight.style.height = (rect.height + 12) + 'px';

            // Content
            badgeEl.textContent = `${idx + 1}/${steps.length}`;
            titleEl.textContent = step.title;
            textEl.innerHTML = step.text;

            // Dots
            dotsEl.innerHTML = steps.map((_, i) =>
                `<span class="tour-dot${i === idx ? ' active' : ''}"></span>`
            ).join('');

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
        if (currentStep < steps.length - 1) showStep(currentStep + 1);
        else end();
    }

    function prev() {
        if (currentStep > 0) showStep(currentStep - 1);
    }

    // Events
    if (nextBtn) nextBtn.addEventListener('click', next);
    if (prevBtn) prevBtn.addEventListener('click', prev);
    if (skipBtn) skipBtn.addEventListener('click', end);
    if (overlay) overlay.addEventListener('click', (e) => {
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
const origUpdateAll = window._tourUpdateAll || null;
let postTourShown = false;
const checkPostUploadTour = () => {
    if (!postTourShown && daysData.length > 0 && !localStorage.getItem('postTourShown')) {
        postTourShown = true;
        localStorage.setItem('postTourShown', 'true');
        setTimeout(() => Tour.start(), 500);
    }
};
// Hook into updateAll to detect data load
const _origRenderTables = renderTables;
// We'll check after renders via a simpler mechanism
setInterval(() => {
    if (daysData.length > 0 && !postTourShown && !localStorage.getItem('postTourShown')) {
        checkPostUploadTour();
    }
}, 2000);
