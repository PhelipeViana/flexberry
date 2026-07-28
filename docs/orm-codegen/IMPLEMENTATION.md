# ORM Codegen — Plano de implementação

## Fase 1: Fundação e configuração

- [x] Inicializar o módulo Go.
- [x] Criar CLI com `init`, `validate` e `version`.
- [x] Gerar YAML comentado e ambiente de exemplo.
- [x] Validar templates, conexões e paginação.
- [x] Documentar instalação e uso no README.

## Fase 2: Scanner de entidades

- [ ] Encontrar arquivos pelos padrões configurados.
- [ ] Analisar structs e tags `db` com AST e `go/types`.
- [ ] Inferir tabela, chave primária, nulabilidade e relações.
- [ ] Permitir overrides no YAML.

## Fase 3: Gerador

- [ ] Implementar `flexberry run` e `--dry-run`.
- [ ] Gerar registry, mappings e wrappers tipados.
- [ ] Formatar e compilar antes de substituir os arquivos.
- [ ] Manter manifesto e remover somente artefatos antigos.

## Fase 4: Runtime de leitura

- [ ] Select, filtros, ordenação, Get, First, Count e Exists.
- [ ] Paginate com retorno tipado.
- [ ] Dialetos PostgreSQL, Oracle e MySQL.

## Fase 5: Relacionamentos

- [ ] belongsTo, hasOne e hasMany.
- [ ] Métodos tipados `WithPessoa`, `WithUsrLogins` e equivalentes.

## Fase 6: Escrita

- [ ] Create, Update, Save e Delete.
- [ ] Proteção contra alterações sem filtro.

## Fora do escopo atual

- Migrations.
- Seeds.
- Factories.
- SQL Server.

