// Configuração deliberadamente mínima: só regras que apontam DEFEITO real.
// Nada de opinião de estilo — isso é trabalho do Prettier, e a formatação do
// front-end não é o gargalo deste projeto.
//
// O front-end usa módulos ES nativos, sem bundler: o navegador resolve os
// imports sozinho e o projeto segue sem etapa de build.
export default [
  {
    files: ["web/**/*.js"],
    ignores: ["web/js/**/*.test.js"],
    languageOptions: {
      ecmaVersion: 2022,
      sourceType: "module",
      globals: {
        window: "readonly",
        document: "readonly",
        console: "readonly",
        localStorage: "readonly",
        fetch: "readonly",
        setTimeout: "readonly",
        clearTimeout: "readonly",
        setInterval: "readonly",
        clearInterval: "readonly",
        requestAnimationFrame: "readonly",
        navigator: "readonly",
        getComputedStyle: "readonly",
        Blob: "readonly",
        URL: "readonly",
        Image: "readonly",
        FileReader: "readonly",
        FormData: "readonly",
        ClipboardItem: "readonly",
        Event: "readonly",
        alert: "readonly",
        confirm: "readonly",
        // Carregada sob demanda pelo visualizador de documento.
        pdfjsLib: "readonly",
      },
    },
    rules: {
      // Pega typo em nome de variável e função esquecida — o defeito mais
      // provável num arquivo de escopo global deste tamanho.
      "no-undef": "error",
      // Pega binding morto. `args: "none"` porque parâmetro não usado em
      // callback de evento é normal e não indica erro.
      "no-unused-vars": ["error", { args: "none" }],
      "no-redeclare": "error",
      // Comparação frouxa já causou confusão neste código (d.saldo === '-08:00'
      // como heurística); manter a exigência de estrito.
      eqeqeq: ["error", "always"],
    },
  },
];
