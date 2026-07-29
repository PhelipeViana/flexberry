package migraterun

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/PhelipeViana/flexberry/internal/cliui"
	"github.com/PhelipeViana/flexberry/internal/config"
	"github.com/PhelipeViana/flexberry/internal/migrategen"
)

type migrationFile struct {
	Name, Checksum string
	Plan           migrategen.Plan
}

func Run(root string, base *config.Config, cfg config.MigrateConfig) error {
	files, err := loadPlans(filepath.Join(root, filepath.FromSlash(cfg.Output.Path)))
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return cliui.NewUserError(
			"Nenhuma migration foi gerada.",
			`Execute .\flexberry.exe migrate reload antes de migrate run.`,
		)
	}
	envPath, err := base.EnvironmentFile(root, config.OSLookup)
	if err != nil {
		return err
	}
	if envPath != "" {
		if err := config.LoadEnvFile(envPath); err != nil {
			return err
		}
	}

	cliui.PrintTitle("Flexberry · Migrations")
	name, err := base.DefaultConnection(config.OSLookup)
	if err != nil {
		return err
	}
	connection, err := base.ResolvedConnection(name, config.OSLookup)
	if err != nil {
		return err
	}
	fmt.Printf("%s %s (%s) %s\n", cliui.Info("→"), name, connection.Dialect, cliui.Info("[DEFAULT]"))
	applied, skipped, runErr := runConnection(connection, cfg.History, files)
	if runErr != nil {
		fmt.Printf("  %s %s\n", cliui.Failure("✗ ERRO:"), runErr)
		return cliui.NewUserError(
			"A conexão padrão não concluiu as migrations.",
			"Corrija a conexão indicada acima e execute Migrate Run novamente.",
		)
	}
	fmt.Printf("  %s %d aplicada(s), %d já executada(s)\n", cliui.Success("✓ OK"), applied, skipped)
	fmt.Println("\n" + cliui.Success("✓ Migrations atualizadas na conexão padrão."))
	return nil
}

func loadPlans(folder string) ([]migrationFile, error) {
	entries, err := os.ReadDir(folder)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var result []migrationFile
	for _, entry := range entries {
		if entry.IsDir() || (!strings.HasSuffix(entry.Name(), "_migration.json") && !strings.HasSuffix(entry.Name(), "_schema.json")) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(folder, entry.Name()))
		if err != nil {
			return nil, err
		}
		var plan migrategen.Plan
		if err := json.Unmarshal(data, &plan); err != nil {
			return nil, fmt.Errorf("ler migration %s: %w", entry.Name(), err)
		}
		sum := sha256.Sum256(data)
		result = append(result, migrationFile{
			Name: entry.Name(), Checksum: hex.EncodeToString(sum[:]), Plan: plan,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func runConnection(connection config.Connection, historyTable string, files []migrationFile) (int, int, error) {
	dialect := strings.ToLower(connection.Dialect)
	driver := map[string]string{
		"oracle": "oracle", "postgres": "pgx", "mysql": "mysql", "sqlserver": "sqlserver",
	}[dialect]
	if driver == "" {
		return 0, 0, fmt.Errorf("dialeto não suportado: %s", dialect)
	}
	db, err := sql.Open(driver, connection.URL)
	if err != nil {
		return 0, 0, err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return 0, 0, err
	}
	if err := ensureHistory(ctx, db, dialect, connection.Schema, historyTable); err != nil {
		return 0, 0, fmt.Errorf("criar %s: %w", historyTable, err)
	}
	history, batch, err := loadHistory(ctx, db, dialect, connection.Schema, historyTable)
	if err != nil {
		return 0, 0, err
	}
	batch++
	applied, skipped := 0, 0
	for _, file := range files {
		if checksum, exists := history[file.Name]; exists {
			if checksum != file.Checksum {
				return applied, skipped, fmt.Errorf("migration %s foi alterada depois de executada", file.Name)
			}
			skipped++
			continue
		}
		created := map[string]bool{}
		for _, operation := range file.Plan.Operations {
			if err := executeOperation(ctx, db, dialect, connection.Schema, operation, created); err != nil {
				return applied, skipped, fmt.Errorf("%s: %w", file.Name, err)
			}
		}
		if err := insertHistory(ctx, db, dialect, connection.Schema, historyTable, file, batch); err != nil {
			return applied, skipped, fmt.Errorf("registrar %s: %w", file.Name, err)
		}
		applied++
	}
	return applied, skipped, nil
}

func executeOperation(ctx context.Context, db *sql.DB, dialect, schema string, operation migrategen.Operation, created map[string]bool) error {
	switch operation.Kind {
	case "create_table":
		exists, err := tableExists(ctx, db, dialect, schema, operation.Table)
		if err != nil {
			return err
		}
		if exists {
			return nil
		}
		query, err := createTableSQL(dialect, schema, operation.Table, operation.Columns)
		if err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("criar tabela %s: %w", operation.Table, err)
		}
		created[operation.Table] = true
	case "add_column":
		if operation.Column == nil {
			return fmt.Errorf("operação add_column inválida")
		}
		exists, err := columnExists(ctx, db, dialect, schema, operation.Table, operation.Column.Name)
		if err != nil {
			return err
		}
		if exists {
			return nil
		}
		query := fmt.Sprintf("ALTER TABLE %s ADD %s", qualified(dialect, schema, operation.Table), columnDefinition(dialect, *operation.Column, false))
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("adicionar %s.%s: %w", operation.Table, operation.Column.Name, err)
		}
	case "alter_column":
		if operation.Column == nil {
			return fmt.Errorf("operação alter_column inválida")
		}
		for _, query := range alterColumnSQL(dialect, schema, operation.Table, *operation.Column) {
			if _, err := db.ExecContext(ctx, query); err != nil {
				return fmt.Errorf("alterar %s.%s: %w", operation.Table, operation.Column.Name, err)
			}
		}
	case "drop_column":
		if operation.Column == nil {
			return fmt.Errorf("operação drop_column inválida")
		}
		query := fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", qualified(dialect, schema, operation.Table), quote(dialect, operation.Column.Name))
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("remover %s.%s: %w", operation.Table, operation.Column.Name, err)
		}
	case "drop_table":
		if _, err := db.ExecContext(ctx, dropTableSQL(dialect, schema, operation.Table)); err != nil {
			return fmt.Errorf("remover tabela %s: %w", operation.Table, err)
		}
	case "add_foreign_key":
		if operation.ForeignKey == nil || !created[operation.Table] {
			return nil
		}
		fk := operation.ForeignKey
		name := "fk_" + operation.Table + "_" + fk.Column
		if len(name) > 30 {
			name = name[:30]
		}
		query := fmt.Sprintf(
			"ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s)",
			qualified(dialect, schema, operation.Table), quote(dialect, name), quote(dialect, fk.Column),
			qualified(dialect, schema, fk.ReferenceTable), quote(dialect, fk.ReferenceColumn),
		)
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("criar relacionamento %s.%s: %w", operation.Table, fk.Column, err)
		}
	default:
		return fmt.Errorf("operação desconhecida: %s", operation.Kind)
	}
	return nil
}

func ensureHistory(ctx context.Context, db *sql.DB, dialect, schema, table string) error {
	target := qualified(dialect, schema, table)
	var query string
	switch dialect {
	case "postgres":
		query = fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
			id BIGSERIAL PRIMARY KEY, migration VARCHAR(255) NOT NULL UNIQUE,
			checksum VARCHAR(64) NOT NULL, batch INTEGER NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP)`, target)
	case "mysql":
		query = fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
			id BIGINT AUTO_INCREMENT PRIMARY KEY, migration VARCHAR(255) NOT NULL UNIQUE,
			checksum VARCHAR(64) NOT NULL, batch INT NOT NULL,
			applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP)`, target)
	case "sqlserver":
		query = fmt.Sprintf(`IF OBJECT_ID(N'%s', N'U') IS NULL CREATE TABLE %s (
			id BIGINT IDENTITY(1,1) PRIMARY KEY, migration NVARCHAR(255) NOT NULL UNIQUE,
			checksum VARCHAR(64) NOT NULL, batch INT NOT NULL,
			applied_at DATETIME2 NOT NULL DEFAULT SYSUTCDATETIME())`,
			objectName(schema, table), target)
	case "oracle":
		query = fmt.Sprintf(`BEGIN EXECUTE IMMEDIATE 'CREATE TABLE %s (
			id NUMBER(19) GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
			migration VARCHAR2(255) NOT NULL UNIQUE, checksum VARCHAR2(64) NOT NULL,
			batch NUMBER(10) NOT NULL, applied_at TIMESTAMP DEFAULT SYSTIMESTAMP NOT NULL)';
			EXCEPTION WHEN OTHERS THEN IF SQLCODE != -955 THEN RAISE; END IF; END;`, target)
	default:
		return fmt.Errorf("dialeto não suportado: %s", dialect)
	}
	_, err := db.ExecContext(ctx, query)
	return err
}

func loadHistory(ctx context.Context, db *sql.DB, dialect, schema, table string) (map[string]string, int, error) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf("SELECT migration, checksum, batch FROM %s ORDER BY id", qualified(dialect, schema, table)))
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	history, maxBatch := map[string]string{}, 0
	for rows.Next() {
		var migration, checksum string
		var batch int
		if err := rows.Scan(&migration, &checksum, &batch); err != nil {
			return nil, 0, err
		}
		history[migration] = checksum
		if batch > maxBatch {
			maxBatch = batch
		}
	}
	return history, maxBatch, rows.Err()
}

func insertHistory(ctx context.Context, db *sql.DB, dialect, schema, table string, file migrationFile, batch int) error {
	target := qualified(dialect, schema, table)
	query := map[string]string{
		"postgres":  fmt.Sprintf("INSERT INTO %s (migration, checksum, batch) VALUES ($1, $2, $3)", target),
		"oracle":    fmt.Sprintf("INSERT INTO %s (migration, checksum, batch) VALUES (:1, :2, :3)", target),
		"mysql":     fmt.Sprintf("INSERT INTO %s (migration, checksum, batch) VALUES (?, ?, ?)", target),
		"sqlserver": fmt.Sprintf("INSERT INTO %s (migration, checksum, batch) VALUES (@p1, @p2, @p3)", target),
	}[dialect]
	_, err := db.ExecContext(ctx, query, file.Name, file.Checksum, batch)
	return err
}

func createTableSQL(dialect, schema, table string, columns []migrategen.Column) (string, error) {
	if len(columns) == 0 {
		return "", fmt.Errorf("tabela %s não possui colunas", table)
	}
	definitions := make([]string, 0, len(columns))
	for _, column := range columns {
		definitions = append(definitions, columnDefinition(dialect, column, true))
	}
	return fmt.Sprintf("CREATE TABLE %s (%s)", qualified(dialect, schema, table), strings.Join(definitions, ", ")), nil
}

func alterColumnSQL(dialect, schema, table string, column migrategen.Column) []string {
	target, name := qualified(dialect, schema, table), quote(dialect, column.Name)
	definition := columnDefinition(dialect, column, false)
	switch dialect {
	case "postgres":
		dataType := strings.TrimSpace(strings.TrimPrefix(definition, name))
		dataType = strings.TrimSuffix(strings.TrimSuffix(dataType, " NOT NULL"), " NULL")
		nullability := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET NOT NULL", target, name)
		if column.Nullable {
			nullability = fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP NOT NULL", target, name)
		}
		return []string{
			fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s", target, name, dataType),
			nullability,
		}
	case "oracle":
		return []string{fmt.Sprintf("ALTER TABLE %s MODIFY (%s)", target, definition)}
	case "mysql":
		return []string{fmt.Sprintf("ALTER TABLE %s MODIFY COLUMN %s", target, definition)}
	default:
		return []string{fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s", target, definition)}
	}
}

func dropTableSQL(dialect, schema, table string) string {
	target := qualified(dialect, schema, table)
	switch dialect {
	case "postgres":
		return "DROP TABLE " + target + " CASCADE"
	case "oracle":
		return "DROP TABLE " + target + " CASCADE CONSTRAINTS PURGE"
	default:
		return "DROP TABLE " + target
	}
}

func columnDefinition(dialect string, column migrategen.Column, allowIdentity bool) string {
	name := quote(dialect, column.Name)
	if column.PrimaryKey && allowIdentity {
		identity := map[string]string{
			"postgres": "BIGSERIAL", "oracle": "NUMBER(19) GENERATED BY DEFAULT AS IDENTITY",
			"mysql": "BIGINT AUTO_INCREMENT", "sqlserver": "BIGINT IDENTITY(1,1)",
		}[dialect]
		return name + " " + identity + " PRIMARY KEY"
	}
	dataType := map[string]map[string]string{
		"integer":  {"postgres": "BIGINT", "oracle": "NUMBER(19)", "mysql": "BIGINT", "sqlserver": "BIGINT"},
		"string":   {"postgres": "VARCHAR(255)", "oracle": "VARCHAR2(255)", "mysql": "VARCHAR(255)", "sqlserver": "NVARCHAR(255)"},
		"boolean":  {"postgres": "BOOLEAN", "oracle": "NUMBER(1)", "mysql": "BOOLEAN", "sqlserver": "BIT"},
		"decimal":  {"postgres": "DECIMAL(19,4)", "oracle": "NUMBER(19,4)", "mysql": "DECIMAL(19,4)", "sqlserver": "DECIMAL(19,4)"},
		"datetime": {"postgres": "TIMESTAMPTZ", "oracle": "TIMESTAMP", "mysql": "DATETIME", "sqlserver": "DATETIME2"},
		"binary":   {"postgres": "BYTEA", "oracle": "BLOB", "mysql": "LONGBLOB", "sqlserver": "VARBINARY(MAX)"},
	}[column.Type][dialect]
	nullability := " NOT NULL"
	if column.Nullable {
		nullability = " NULL"
	}
	return name + " " + dataType + nullability
}

func tableExists(ctx context.Context, db *sql.DB, dialect, schema, table string) (bool, error) {
	var count int
	switch dialect {
	case "postgres":
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=$1 AND table_name=$2", schemaOr(schema, "public"), table).Scan(&count)
		return count > 0, err
	case "mysql":
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=?", table).Scan(&count)
		return count > 0, err
	case "sqlserver":
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=@p1 AND table_name=@p2", schemaOr(schema, "dbo"), table).Scan(&count)
		return count > 0, err
	default:
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM all_tables WHERE owner=:1 AND table_name=:2", strings.ToUpper(schema), strings.ToUpper(table)).Scan(&count)
		return count > 0, err
	}
}

func columnExists(ctx context.Context, db *sql.DB, dialect, schema, table, column string) (bool, error) {
	var count int
	switch dialect {
	case "postgres":
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=$1 AND table_name=$2 AND column_name=$3", schemaOr(schema, "public"), table, column).Scan(&count)
		return count > 0, err
	case "mysql":
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name=? AND column_name=?", table, column).Scan(&count)
		return count > 0, err
	case "sqlserver":
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=@p1 AND table_name=@p2 AND column_name=@p3", schemaOr(schema, "dbo"), table, column).Scan(&count)
		return count > 0, err
	default:
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM all_tab_columns WHERE owner=:1 AND table_name=:2 AND column_name=:3", strings.ToUpper(schema), strings.ToUpper(table), strings.ToUpper(column)).Scan(&count)
		return count > 0, err
	}
}

func qualified(dialect, schema, table string) string {
	if strings.TrimSpace(schema) == "" || dialect == "mysql" {
		return quote(dialect, table)
	}
	return quote(dialect, schema) + "." + quote(dialect, table)
}

func quote(dialect, value string) string {
	switch dialect {
	case "mysql":
		return "`" + strings.ReplaceAll(value, "`", "``") + "`"
	case "sqlserver":
		return "[" + strings.ReplaceAll(value, "]", "]]") + "]"
	default:
		return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
	}
}

func objectName(schema, table string) string {
	return schemaOr(schema, "dbo") + "." + table
}

func schemaOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
