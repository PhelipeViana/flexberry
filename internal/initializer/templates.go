package initializer

const ConfigTemplate = `# Versão do formato de configuração do Flexberry.
version: 1

# Define de onde as variáveis serão carregadas e qual ambiente está ativo.
environment:
  # Caminho relativo à raiz do projeto. Variáveis do sistema têm prioridade.
  file: ./seguranca/database.env
  # Exemplos de valor: development, test, staging ou production.
  variable: APPLICATION_ENV
  fallback: development

# Seleciona uma das chaves declaradas em "connections".
default:
  variable: DATABASE_CONNECTION
  fallback: postgres

# Conexões disponíveis para ORM e, futuramente, migrate, seed e factory.
connections:
  # PostgreSQL: driver padrão pgx; schema padrão public.
  postgres:
    dialect: postgres
    url: postgres://${POSTGRES_USERNAME}:${POSTGRES_PASSWORD}@${POSTGRES_HOST}:${POSTGRES_PORT}/${POSTGRES_DATABASE}?sslmode=${POSTGRES_SSLMODE:-disable}
    schema: ${POSTGRES_SCHEMA:-public}

  # Oracle: SERVICE_NAME/PDB e schema são informações diferentes.
  oracle:
    dialect: oracle
    url: oracle://${ORACLE_USERNAME}:${ORACLE_PASSWORD}@${ORACLE_HOST}:${ORACLE_PORT}/${ORACLE_SERVICE_NAME}
    schema: ${ORACLE_SCHEMA}

  # MySQL: o nome do database faz o papel do schema principal.
  mysql:
    dialect: mysql
    url: ${MYSQL_USERNAME}:${MYSQL_PASSWORD}@tcp(${MYSQL_HOST}:${MYSQL_PORT})/${MYSQL_DATABASE}?parseTime=true&charset=utf8mb4

# Entidades Go usadas como fonte do mapping.
entities:
  paths:
    - internal/modules/**/domain/*.go
  exclude:
    - "**/*_test.go"
    - "**/*.gen.go"

# Todo código não editável será concentrado nesta pasta.
generate:
  output: internal/flexberry/orm
  package: flexberry

# Valores compartilhados pelo método Paginate.
pagination:
  default_per_page: 15
  max_per_page: 100
`

const EnvTemplate = `# Ambiente atual: development, test, staging ou production.
APPLICATION_ENV=development

# Conexão padrão: postgres, oracle ou mysql.
DATABASE_CONNECTION=postgres

# PostgreSQL
POSTGRES_HOST=127.0.0.1
POSTGRES_PORT=5433
POSTGRES_USERNAME=flexberry
POSTGRES_PASSWORD=change-me
POSTGRES_DATABASE=flexberry
POSTGRES_SCHEMA=public
POSTGRES_SSLMODE=disable

# Oracle
ORACLE_HOST=127.0.0.1
ORACLE_PORT=1522
ORACLE_USERNAME=flexberry
ORACLE_PASSWORD=change-me
ORACLE_SERVICE_NAME=FREEPDB1
ORACLE_SCHEMA=FLEXBERRY

# MySQL
MYSQL_HOST=127.0.0.1
MYSQL_PORT=3307
MYSQL_USERNAME=flexberry
MYSQL_PASSWORD=change-me
MYSQL_DATABASE=flexberry
`

const CustomTemplate = `// Package flexberry contains project-specific extensions.
//
// This file belongs to the application and is never changed by flexberry run.
package flexberry
`

const GitIgnoreTemplate = `# Credenciais locais do Flexberry
seguranca/database.env
`

const SimplifiedConfigTemplate = `version: 1

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
`

const FactoryConfigTemplate = `version: 1

expressions:
  # Use COLUNA para regra global ou TABELA.COLUNA para regra específica.
  exact: {}
  contains: []
`
