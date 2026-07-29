package initializer

const ConnectionConfigTemplateV2 = `# Versão do formato deste arquivo, não do executável.
version: 1

# Arquivo local que contém as variáveis usadas nas conexões.
environment:
  file: ./.env
  ambient: APP_ENV
  fallback: development

# Variável de ambiente que informa o dialeto/conexão padrão.
default:
  dialect: DB_DIALECT
  fallback: postgres

# Conexões disponíveis para ORM, factories e funcionalidades futuras.
connections:
  postgres:
    dialect: postgres
    url: postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@${POSTGRES_HOST}:${POSTGRES_PORT}/${POSTGRES_DB}?sslmode=${POSTGRES_SSLMODE:-disable}
    schema: ${POSTGRES_SCHEMA:-public}

  oracle:
    dialect: oracle
    url: oracle://${ORACLE_USER}:${ORACLE_PASSWORD}@${ORACLE_HOST}:${ORACLE_PORT}/${ORACLE_SERVICE}
    schema: ${ORACLE_SCHEMA:-FLEXBERRY}

  mysql:
    dialect: mysql
    url: ${MYSQL_USER}:${MYSQL_PASSWORD}@tcp(${MYSQL_HOST}:${MYSQL_PORT})/${MYSQL_DATABASE}?parseTime=true&charset=utf8mb4

  sqlserver:
    dialect: sqlserver
    url: sqlserver://${MSSQL_USER}:${MSSQL_SA_PASSWORD}@${MSSQL_HOST}:${MSSQL_PORT}?database=${MSSQL_DATABASE}&encrypt=disable
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

const MigrateConfigTemplateV1 = `# Versão do formato das migrations.
version: 1

# Entidades monitoradas para criação e evolução das tabelas.
entities:
  paths:
    - internal/modules/**/domain/*.go

  exclude:
    - "**/*_test.go"
    - "**/*.gen.go"

# Planos neutros gerados pelo Migrate Reload.
# Um mesmo plano será convertido para os quatro bancos durante o Run.
output:
  path: internal/database/migrations

# Histórico criado automaticamente em cada banco.
history_table: migrations_flex
`

const FactoryConfigTemplateV3 = `# Versão do formato das factories.
version: 1

# Destino dos arquivos Go das factories, relativo ao go.mod.
# O package Go será inferido automaticamente pelo último nome da pasta.
# Exemplo: internal/teste/factories usa package factories.
mapper:
  path: internal/database/factories

# Expressões Go que interceptam a geração automática das factories.
# Ao alterar uma regra, execute Factory Reload para atualizar os arquivos.
expressions:
  # TABELA.COLUNA cria uma regra específica; COLUNA cria uma regra global.
  # Prioridade: vínculo ORM > exact específico > exact global > contains > tipo Go.
  exact:
    # Campos lógicos armazenados como 0 e 1.
    ATIVO: flexberry.FakeIntRange(index, 0, 1)
    ACTIVE: flexberry.FakeBool(index)
    ENABLED: flexberry.FakeBool(index)

    # Campos com formato curto e conhecido.
    UF: flexberry.FakeUF(index)
    SEXO: 'flexberry.FakeChoice(index, "M", "F")'

  # Correspondência semântica parcial, avaliada na ordem abaixo.
  # Os limites podem ser alterados conforme o schema do projeto.
  contains:
    - pattern: SENHA
      expression: flexberry.FakePasswordHash(60)
    - pattern: PASSWORD
      expression: flexberry.FakePasswordHash(60)
    - pattern: HASH
      expression: flexberry.FakeHash(index, 60)
    - pattern: EMAIL
      expression: flexberry.FakeEmail(index, 150)
    - pattern: CNPJ
      expression: flexberry.FakeCNPJ(index, 18)
    - pattern: CPF
      expression: flexberry.FakeCPF(index, 14)
    - pattern: CEP
      expression: flexberry.FakeCEP(index, 9)
    - pattern: TELEFONE
      expression: flexberry.FakePhone(index, 20)
    - pattern: CELULAR
      expression: flexberry.FakePhone(index, 20)
    - pattern: FONE
      expression: flexberry.FakePhone(index, 20)
    - pattern: UUID
      expression: flexberry.FakeUUID(index, 36)
    - pattern: REQUEST_ID
      expression: flexberry.FakeUUID(index, 36)
    - pattern: NOME_ARQUIVO
      expression: flexberry.FakeFileName(index, 120)
    - pattern: ARQUIVO
      expression: flexberry.FakeFileName(index, 120)
    - pattern: USERNAME
      expression: flexberry.FakeUsername(index, 80)
    - pattern: LOGIN
      expression: flexberry.FakeUsername(index, 80)
    - pattern: USUARIO
      expression: flexberry.FakeUsername(index, 80)
    - pattern: DESCRICAO
      expression: flexberry.FakeText(index, 255)
    - pattern: OBSERVACAO
      expression: flexberry.FakeText(index, 255)
    - pattern: LOGRADOURO
      expression: flexberry.FakeStreet(index, 150)
    - pattern: ENDERECO
      expression: flexberry.FakeStreet(index, 150)
    - pattern: CIDADE
      expression: flexberry.FakeCity(index, 100)
    - pattern: MUNICIPIO
      expression: flexberry.FakeCity(index, 100)
    - pattern: NOME
      expression: flexberry.FakeName(index, 150)

# Valores aplicados na criação de toda nova factory.
defaults:
  # Quantidade de registros gerados.
  count: 10

  # true limpa os registros existentes antes de inserir os novos.
  update: true

  # true inclui a factory no comando factory run.
  active: true
`
