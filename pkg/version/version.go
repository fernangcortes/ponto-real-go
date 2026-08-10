// Package version guarda a versão do aplicativo em um único lugar.
//
// Antes o número aparecia solto em três arquivos (main.go, pkg/api/handler.go e
// web/app.js) e já tinha divergido do README, que anuncia a 1.1.0.
package version

// Atual é a versão do aplicativo, servida em GET /api/health.
const Atual = "1.1.0"
