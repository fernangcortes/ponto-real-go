// Conversões de horário e escape de HTML. Sem DOM e sem estado: tudo aqui é
// função pura, testável com `node --test`.

// t2m converte "HH:MM" em minutos desde a meia-noite.
export const t2m = (t) => {
    if (!t || t === '**:**') return 0;
    const parts = t.split(':');
    if (parts.length !== 2) return 0;
    return parseInt(parts[0]) * 60 + parseInt(parts[1]);
};

// m2t formata minutos como saldo, sempre com sinal: "+01:30", "-08:00".
export const m2t = (m) => {
    const sign = m < 0 ? '-' : '+';
    const abs = Math.abs(m);
    return sign + String(Math.floor(abs / 60)).padStart(2, '0') + ':' + String(abs % 60).padStart(2, '0');
};

// m2tUnsigned formata minutos como duração, sem sinal: "08:00".
export const m2tUnsigned = (m) => {
    const abs = Math.abs(m);
    return String(Math.floor(abs / 60)).padStart(2, '0') + ':' + String(abs % 60).padStart(2, '0');
};

// parseOrigSaldo lê o saldo como veio da ficha ("-08:00", "+01:20", "00:18").
export const parseOrigSaldo = (s) => {
    if (!s) return 0;
    const isNeg = s.includes('-');
    const pts = s.replace('-', '').replace('+', '').split(':');
    return pts.length === 2 ? (isNeg ? -1 : 1) * (parseInt(pts[0]) * 60 + parseInt(pts[1])) : 0;
};

export const isTimeValid = (t) => {
    if (!t || t === '**:**') return false;
    const parts = t.split(':');
    if (parts.length !== 2) return false;
    const h = parseInt(parts[0]), m = parseInt(parts[1]);
    return !isNaN(h) && !isNaN(m) && h >= 0 && h <= 23 && m >= 0 && m <= 59;
};

// esc — todo dado que entra num template de HTML passa por aqui.
//
// Os textos vêm da leitura do documento por IA: uma aspa numa observação
// ("DISPENSA P/ CURSO "X"") quebrava a linha inteira da tabela. O escape
// existia, mas aplicado caso a caso — `revisar_motivo` tinha, `mot` não.
export const esc = (v) => String(v ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');

// copiavel marca um elemento como clicável para copiar.
//
// Substituiu onclick="copyCell('${valor}', this)": ali o dado entrava numa
// string JS dentro de um atributo HTML, o que exige dois níveis de escape
// diferentes — e o código não fazia nenhum. Com data-attribute + delegação de
// clique sobra um nível só, que o esc() resolve.
export const copiavel = (valor) => `data-copy="${esc(valor)}"`;

// Remove acentos para comparar texto de observação vindo da ficha.
export const normalizeObs = (s) => (s || '').toUpperCase()
    .normalize('NFD').replace(/[\u0300-\u036f]/g, '');

export const clamp = (v, min, max) => Math.min(max, Math.max(min, v));

export const randBetween = (min, max) => Math.floor(Math.random() * (max - min + 1)) + min;

// avoidRoundMins afasta o horário gerado dos minutos "redondos": um ponto que
// cai sempre em :00 ou :30 denuncia que foi inventado.
export const avoidRoundMins = (m) => {
    const mins = m % 60;
    if (mins === 0) return m + randBetween(1, 5);
    if (mins % 5 === 0) return m + randBetween(1, 3);
    return m;
};
