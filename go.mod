module github.com/fernangcortes/ponto-real-go

go 1.25.0

// O ESLint traz dependências que contêm arquivos .go de exemplo. Sem isto,
// `go build ./...`, `go vet ./...` e `go test ./...` passam a varrer
// node_modules e a reportar pacotes de terceiros como se fossem do projeto.
ignore ./node_modules
