# ORM Codegen — Pesquisa

## Objetivo

Gerar uma API ORM tipada a partir das structs Go existentes, sem modificar os
arquivos das entidades. O código gerado ficará concentrado em
`internal/flexberry/orm`.

## Decisões

- `internal/flexberry/connection.yaml` é editável.
- `internal/flexberry/custom.go` é editável e nunca será sobrescrito.
- `internal/flexberry/orm` pertence ao gerador.
- Conexões usam templates `${VAR}` e `${VAR:-fallback}`.
- A conexão padrão é escolhida por uma variável de ambiente.
- A primeira matriz terá PostgreSQL, Oracle e MySQL.
- Paginação e relacionamentos fazem parte do contrato do ORM.
- Migrate, seed e factory são fases futuras.

## Riscos

- Inferência incorreta de nomes de tabela e relacionamentos.
- Ciclos de importação entre código gerado e entidades.
- Sobrescrita acidental de código manual.

## Mitigações

- Usar AST e tipos do Go, não expressões regulares.
- Gerar em diretório temporário e validar antes da substituição.
- Controlar somente arquivos `.gen.go` e um manifesto.
- Permitir overrides no YAML para qualquer inferência ambígua.
