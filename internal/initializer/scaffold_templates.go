package initializer

const ConnectionConfigTemplateV2 = `# Versão do formato deste arquivo, não do executável.
version: 1

# Arquivo local que contém as variáveis usadas nas conexões.
environment:
  file: ./seguranca/database.env
  variable: APPLICATION_ENV
  fallback: development

# Variável que seleciona a conexão padrão.
default:
  variable: DATABASE_CONNECTION
  fallback: postgres

# Conexões disponíveis para ORM, factories e funcionalidades futuras.
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

  sqlserver:
    dialect: sqlserver
    url: sqlserver://${MSSQL_USERNAME}:${MSSQL_PASSWORD}@${MSSQL_HOST}:${MSSQL_PORT}?database=${MSSQL_DATABASE}&encrypt=disable
    schema: ${MSSQL_SCHEMA:-dbo}
`

const ORMConfigTemplateV2 = `# Versão do formato do mapping ORM.
version: 1

# Localização das structs Go com tags db que serão mapeadas.
entities:
  paths:
    - internal/modules/**/domain/*.go

  # Arquivos ignorados durante o mapeamento.
  exclude:
    - "**/*_test.go"
    - "**/*.gen.go"

# Destino do ORM, próximo aos paths para facilitar a configuração.
# O package Go será inferido automaticamente: internal/orm usa package orm.
output:
  path: internal/orm

# Exceções para bancos que não seguem a convenção automática.
# Exemplo:
# overrides:
#   UsrLogin:
#     table: USR_LOGIN
#     primary_key: USR_LOGIN_ID
overrides: {}
`

const FactoryConfigTemplateV3 = `# Versão do formato das factories.
version: 1

# Destino dos arquivos Go das factories, relativo ao go.mod.
# O package Go será inferido automaticamente pelo último nome da pasta.
# Exemplo: internal/teste/factories usa package factories.
mapper:
  path: internal/database/factories

# Expressões Go escolhidas automaticamente para campos gerados.
expressions:
  # TABELA.COLUNA cria uma regra específica; COLUNA cria uma regra global.
  # A regra específica possui prioridade.
  exact:
    # Exemplo global: qualquer coluna ATIVO alternará entre 0 e 1.
    ATIVO: int64(index % 2)

  # Correspondência parcial usada quando exact não encontrar uma regra.
  # Exemplo:
  # contains:
  #   - pattern: ATIVO
  #     expression: int64(index % 2)
  contains: []

# Valores aplicados na criação de toda nova factory.
defaults:
  # Quantidade de registros gerados.
  count: 10

  # true limpa os registros existentes antes de inserir os novos.
  update: true

  # true inclui a factory no comando factory run.
  active: true
`
