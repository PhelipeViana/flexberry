package initializer

const EnvTemplate = `# Ambiente atual.
APPLICATION_ENV=development

# Conexão padrão: postgres, oracle, mysql ou sqlserver.
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

# SQL Server
MSSQL_HOST=127.0.0.1
MSSQL_PORT=1434
MSSQL_USERNAME=sa
MSSQL_PASSWORD=change-me
MSSQL_DATABASE=flexberry
MSSQL_SCHEMA=dbo
`

const GitIgnoreTemplate = `# Credenciais locais do Flexberry
seguranca/database.env
`
