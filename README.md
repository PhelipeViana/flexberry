# Flexberry

Flexberry é um gerador de ORM para Go orientado pelas entidades já existentes
no projeto. A proposta é semelhante ao `sqlc`: o usuário mantém suas structs e
o Flexberry gera uma camada tipada e multibanco sem modificar os arquivos
originais.

> Estado atual: fundação da configuração e da CLI. O gerador ORM será a próxima
> fase.

## Objetivos

- usar structs Go como fonte do mapping;
- gerar código legível e descartável;
- concentrar toda a integração em `internal/flexberry`;
- suportar PostgreSQL, Oracle e MySQL inicialmente;
- selecionar conexão e ambiente por variáveis;
- oferecer paginação e relacionamentos tipados;
- permitir adoção gradual em projetos existentes.

Migrate, seed e factory serão adicionados depois da estabilização do ORM.

## Instalação

Durante o desenvolvimento local:

```powershell
go install ./cmd/flexberry
```

Depois da primeira versão pública:

```powershell
go install github.com/PhelipeViana/flexberry/cmd/flexberry@latest
```

Confirme:

```powershell
flexberry version
```

## Inicialização

Execute dentro de um projeto Go que possua `go.mod`:

```powershell
flexberry init
```

O comando encontra a raiz do módulo e cria:

```text
internal/
└── flexberry/
    ├── flexberry.yaml       # editável
    ├── custom.go            # editável
    └── orm/                 # arquivos gerados, não editar

seguranca/
├── database.env             # credenciais locais, ignorado pelo Git
└── database.example.env     # exemplo versionável
```

Arquivos existentes são preservados. `--force` recria apenas arquivos seguros:

```powershell
flexberry init --force
```

O arquivo de credenciais nunca é sobrescrito.

## Configuração gerada

```yaml
version: 1

environment:
  file: ./seguranca/database.env
  variable: APPLICATION_ENV
  fallback: development

default:
  variable: DATABASE_CONNECTION
  fallback: postgres

connections:
  postgres:
    dialect: postgres
    url: postgres://${POSTGRES_USERNAME}:${POSTGRES_PASSWORD}@${POSTGRES_HOST}:${POSTGRES_PORT}/${POSTGRES_DATABASE}?sslmode=${POSTGRES_SSLMODE:-disable}
    schema: ${POSTGRES_SCHEMA:-public}

  oracle:
    dialect: oracle
    url: oracle://${ORACLE_USERNAME}:${ORACLE_PASSWORD}@${ORACLE_HOST}:${ORACLE_PORT}/${ORACLE_SERVICE_NAME}
    schema: ${ORACLE_SCHEMA}

  mysql:
    dialect: mysql
    url: ${MYSQL_USERNAME}:${MYSQL_PASSWORD}@tcp(${MYSQL_HOST}:${MYSQL_PORT})/${MYSQL_DATABASE}?parseTime=true&charset=utf8mb4

entities:
  paths:
    - internal/modules/**/domain/*.go
  exclude:
    - "**/*_test.go"
    - "**/*.gen.go"

generate:
  output: internal/flexberry/orm
  package: flexberry

pagination:
  default_per_page: 15
  max_per_page: 100
```

### Variáveis

Uma variável obrigatória:

```yaml
schema: ${ORACLE_SCHEMA}
```

Uma variável com fallback:

```yaml
schema: ${POSTGRES_SCHEMA:-public}
```

Variáveis do processo, Docker ou sistema operacional têm prioridade sobre o
arquivo configurado em `environment.file`.

## Validar

Validação estrutural, sem exigir credenciais:

```powershell
flexberry validate
```

Validação completa, carregando o arquivo de ambiente:

```powershell
flexberry validate --resolve
```

Saída esperada:

```text
✓ configuração válida
✓ conexões: mysql, oracle, postgres
✓ entidades: 1 caminho(s)
✓ saída: internal/flexberry/orm
✓ ambiente carregado
✓ profile: development
✓ conexão padrão resolvida: postgres
```

## Ambientes

O ambiente atual é definido por:

```env
APPLICATION_ENV=development
```

Exemplos permitidos pelo projeto:

```text
development
test
staging
production
```

O Flexberry não impõe nomes fixos; o valor fica disponível para decisões
futuras de geração e execução.

## Conexão padrão

```env
DATABASE_CONNECTION=postgres
```

O valor deve corresponder a uma chave de `connections`.

Ferramentas futuras poderão sobrescrever a escolha:

```powershell
flexberry migrate --connection oracle
flexberry seed --connection postgres
flexberry factory --connection mysql
```

## ORM planejado

Depois da implementação de `flexberry run`, uma entidade como:

```go
type UsrLogin struct {
	UsrLoginId int64  `db:"USR_LOGIN_ID"`
	PessoaId   *int64 `db:"PESSOA_ID"`
	Login      string `db:"LOGIN"`
}
```

produzirá uma API semelhante a:

```go
usuarios, err := flexberry.
	UsrLogin(db).
	Where("ATIVO", 1).
	Get(ctx)
```

Paginação:

```go
pagina, err := flexberry.
	UsrLogin(db).
	Where("ATIVO", 1).
	Paginate(ctx, flexberry.PageRequest{
		Page:    1,
		PerPage: 15,
	})
```

Relacionamentos:

```go
usuario, err := flexberry.
	UsrLogin(db).
	WithPessoa().
	Where("USR_LOGIN_ID", id).
	First(ctx)
```

## Segurança da geração

O futuro `flexberry run` seguirá estas regras:

- entidades originais nunca serão alteradas;
- `custom.go` nunca será alterado;
- somente `internal/flexberry/orm` pertence ao gerador;
- geração ocorrerá primeiro em uma pasta temporária;
- o código será formatado e compilado antes da substituição;
- um manifesto controlará quais arquivos podem ser removidos;
- `--dry-run` mostrará mudanças sem gravá-las.

## Gerar o mapping

Analise as entidades sem alterar arquivos:

```powershell
flexberry run --dry-run
```

Gere ou atualize o pacote não editável:

```powershell
flexberry run
```

O comando encontra structs exportadas com tags `db`, infere tabela, chave
primária, nulabilidade e relacionamentos simples. O resultado fica concentrado
em `internal/flexberry/orm`, nos arquivos `entities.gen.go` e
`manifest.gen.json`.

Casos fora da convenção podem ser ajustados sem alterar as entidades:

```yaml
entities:
  overrides:
    UsrLogin:
      table: USR_LOGIN
      primary_key: USR_LOGIN_ID
      connection: oracle
```

O mapping tipado é a base usada pelo runtime abaixo. Relacionamentos tipados
continuam reservados para a próxima fase.

## Runtime ORM

Registre a conexão já aberta pelo projeto uma única vez:

```go
if err := flexberry.Register("oracle", db, "oracle"); err != nil {
	return err
}
```

A primeira conexão registrada torna-se a padrão. Em aplicações multibanco:

```go
flexberry.Register("main", postgresDB, "postgres")
flexberry.Register("legacy", oracleDB, "oracle")
flexberry.SetDefault("main")
```

Depois disso, o código gerado pode ser usado sem repetir a conexão:

```go
pessoas, err := orm.Pessoas().
	Where("ATIVO", int64(1)).
	OrderBy("NOME").
	Exec(ctx)

pessoa, err := orm.Pessoas().
	Where("PESSOA_ID", id).
	First(ctx)

pagina, err := orm.Pessoas().
	Where("ATIVO", int64(1)).
	Paginate(ctx, flexberry.PageRequest{Page: 1, PerPage: 20})
```

Para escolher outra conexão apenas em uma consulta:

```go
pessoas, err := orm.Pessoas().
	Connection("legacy").
	Where("ATIVO", int64(1)).
	Get(ctx)
```

O retorno de `Get` e `Exec` é `[]Entidade`; `First` retorna `*Entidade`.
Esses valores podem ser serializados normalmente com `encoding/json`.

Exclusões exigem ao menos um `Where`:

```go
result, err := orm.Pessoas().
	Where("PESSOA_ID", id).
	Delete(ctx)
```

## Desenvolvimento

```powershell
go fmt ./...
go vet ./...
go test ./...
```

Planejamento e histórico:

- `docs/orm-codegen/RESEARCH.md`
- `docs/orm-codegen/IMPLEMENTATION.md`
- `docs/orm-codegen/PROGRESS.md`
