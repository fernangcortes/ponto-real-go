// Espelho de pkg/service/regras_compartilhadas_test.go.
//
// Os dois rodam os MESMOS casos (testdata/regras.json) contra as duas
// implementações de regra que o projeto tem — o motor Go e este domínio — e
// conferem contra os MESMOS valores esperados.
//
// É o que impede a duplicação de divergir de novo. Antes de existir, as duas
// discordavam em 72 horas num único mês e ninguém percebia: férias homologadas
// eram dia neutro na tela e falta no servidor.
//
// Ao mudar uma regra, o valor esperado muda em testdata/regras.json, uma vez, e
// os dois testes passam a exigir a mudança dos dois lados.

import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

import { CONFIG } from './config.js';
import {
    classifyDay, saldoDoDia, deWire,
    avisoDeRevisao, MSG_CARGA_DISPENSA, MSG_CARGA_REDUZIDA,
} from './domain.js';

const AVISO_ESPERADO = {
    dispensa: MSG_CARGA_DISPENSA,
    reduzido: MSG_CARGA_REDUZIDA,
    '': '',
};

const aqui = path.dirname(fileURLToPath(import.meta.url));
const fixture = JSON.parse(
    fs.readFileSync(path.join(aqui, '..', '..', 'testdata', 'regras.json'), 'utf8')
);

test('o arquivo compartilhado tem casos', () => {
    assert.ok(fixture.casos.length > 0, 'testdata/regras.json está vazio');
});

for (const caso of fixture.casos) {
    test(`[compartilhado] ${caso.nome}`, () => {
        CONFIG.mesAno = fixture.mes_ano;

        const d = deWire(caso.dia);
        const tipo = classifyDay(d);
        const saldo = saldoDoDia(d, tipo);

        assert.equal(tipo, caso.tipo, 'classificação do dia');
        assert.equal(saldo.contribui, caso.contribui, 'contribuição ao saldo real');

        // O aviso de conferência também é regra: se um lado pedir a jornada do
        // ato e o outro não, o usuário vê ⚠️ num e não no outro.
        assert.equal(avisoDeRevisao(d, tipo), AVISO_ESPERADO[caso.aviso_carga], 'aviso de conferência');
    });
}
