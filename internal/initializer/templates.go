package initializer

const EnvTemplate = `# Ambiente atual.
APP_ENV=development

# Conexão padrão: postgres, oracle, mysql ou sqlserver.
DB_DIALECT=postgres

# PostgreSQL
POSTGRES_HOST=127.0.0.1
POSTGRES_PORT=5499
POSTGRES_USER=flexberry
POSTGRES_PASSWORD=change-me
POSTGRES_DB=flexberry
POSTGRES_SCHEMA=public
POSTGRES_SSLMODE=disable

# Oracle
ORACLE_HOST=127.0.0.1
ORACLE_PORT=1599
ORACLE_USER=flexberry
ORACLE_PASSWORD=change-me
ORACLE_SERVICE=FREEPDB1
ORACLE_SCHEMA=FLEXBERRY

# MySQL
MYSQL_HOST=127.0.0.1
MYSQL_PORT=3399
MYSQL_USER=flexberry
MYSQL_PASSWORD=change-me
MYSQL_DATABASE=flexberry

# SQL Server
MSSQL_HOST=127.0.0.1
MSSQL_PORT=1499
MSSQL_USER=sa
MSSQL_SA_PASSWORD=change-me
MSSQL_DATABASE=flexberry
MSSQL_SCHEMA=dbo
`

const GitIgnoreTemplate = `# Credenciais locais do Flexberry
.env
`
