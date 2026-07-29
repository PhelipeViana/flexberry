# ORM Codegen — Progresso

## Status: Runtime de leitura concluído

## 2026-07-28 — Fase 1

- Módulo Go e CLI `flexberry`.
- `flexberry init` cria a estrutura isolada.
- `flexberry validate` valida YAML e variáveis.
- Credenciais locais protegidas pelo `.gitignore`.

## 2026-07-28 — Fase 2

- Scanner AST de entidades com tags `db`.
- Inferência de tabela, chave primária, nulabilidade e relacionamentos simples.
- Overrides por entidade no `connection.yaml`.
- `flexberry run` e `flexberry run --dry-run`.
- Geração determinística de `entities.gen.go` e `manifest.gen.json`.
- Teste no projeto `back`: 20 entidades mapeadas e `go test ./...` aprovado.

## Próxima fase

Relacionamentos tipados e operações de escrita (`Create`, `Update` e `Save`).

## 2026-07-28 — Runtime ORM

- Registro desacoplado de `*sql.DB` e `*sqlx.DB`.
- Conexão padrão e seleção por consulta.
- `Where`, operadores, ordenação, limit e offset.
- `Get`, `Exec`, `First`, `Count`, `Exists` e `Paginate`.
- `Delete` protegido contra execução sem filtro.
- Placeholders e paginação para PostgreSQL, Oracle e MySQL.
- Leitura tipada por tags `db`.
- Integração no bootstrap do projeto `back`.
- Suíte completa do `back` aprovada.

## 2026-07-28 — Primeiro uso em endpoints

- Repositório de autenticação migrado para `orm.UsrLogin()`.
- `POST /api/auth/login`, `GET /api/auth/me`, `POST /api/auth/refresh` e
  `GET /api/users` exercitam o runtime.
- Registro da conexão centralizado em `httpserver.NewRouter`.
- Teste HTTP real aprovado contra o Oracle em Docker.
