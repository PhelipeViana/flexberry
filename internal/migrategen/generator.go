package migrategen

import (
	"encoding/json"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/PhelipeViana/flexberry/internal/config"
	"github.com/PhelipeViana/flexberry/internal/migrationgo"
	"github.com/PhelipeViana/flexberry/internal/scanner"
	"github.com/PhelipeViana/flexberry/migration/acao"
)

const snapshotName = "schema.snapshot.json"

type Column = acao.ColunaDefinicao
type ForeignKey = acao.ForeignKey

type Table struct {
	Name        string       `json:"name"`
	Columns     []Column     `json:"columns"`
	ForeignKeys []ForeignKey `json:"foreign_keys,omitempty"`
}

type Snapshot struct {
	Version int     `json:"version"`
	Tables  []Table `json:"tables"`
}

type Operation = acao.Operacao

type Plan struct {
	Version    int         `json:"version"`
	Migration  string      `json:"migration"`
	CreatedAt  time.Time   `json:"created_at"`
	Operations []Operation `json:"operations"`
}

type Result struct {
	Path       string
	Paths      []string
	Migration  string
	Operations int
	Unchanged  bool
}

func Generate(root string, cfg config.MigrateConfig, entities []scanner.Entity) (Result, error) {
	return generate(root, cfg, scanner.Result{Entities: entities})
}

// GenerateFromScan creates a migration from a previously validated execution
// plan. When the scan is partial, the last valid snapshot is merged into the
// current one so ignored source files never become destructive operations.
func GenerateFromScan(root string, cfg config.MigrateConfig, scan scanner.Result) (Result, error) {
	return generate(root, cfg, scan)
}

func generate(root string, cfg config.MigrateConfig, scan scanner.Result) (Result, error) {
	output := filepath.Join(root, filepath.FromSlash(cfg.Output.Path))
	relative, err := filepath.Rel(root, output)
	if err != nil || relative == "." || relative == ".." || filepath.IsAbs(relative) {
		return Result{}, fmt.Errorf("migrate output.path precisa ficar dentro do projeto")
	}
	if err := os.MkdirAll(output, 0o755); err != nil {
		return Result{}, fmt.Errorf("criar pasta de migrations: %w", err)
	}

	core := filepath.Join(root, "internal", "flexberry", "core", "migration")
	if err := os.MkdirAll(core, 0o755); err != nil {
		return Result{}, fmt.Errorf("criar core de migrations: %w", err)
	}
	modulePath, err := readModulePath(root)
	if err != nil {
		return Result{}, err
	}
	if err := migrateLegacyHelpers(output, modulePath); err != nil {
		return Result{}, err
	}
	current, err := buildSnapshot(scan.Entities)
	if err != nil {
		return Result{}, err
	}
	coreSnapshot := filepath.Join(core, snapshotName)
	previous, err := loadSnapshot(coreSnapshot)
	if err == nil && len(previous.Tables) == 0 {
		previous, err = loadSnapshot(filepath.Join(output, snapshotName))
	}
	if err != nil {
		return Result{}, err
	}
	hasMigrations, err := hasMigrationFiles(output)
	if err != nil {
		return Result{}, err
	}
	if !hasMigrations {
		// O snapshot não substitui o histórico. Se os arquivos foram removidos,
		// o Reload precisa reconstruir as migrations a partir das entidades.
		previous = Snapshot{Version: 1}
	}
	if len(scan.Warnings) > 0 {
		current = preservePreviousSnapshot(previous, current)
	}
	operations, err := diff(previous, current)
	if err != nil {
		return Result{}, err
	}
	aliases := map[string]string{}
	for _, table := range current.Tables {
		aliases[tableIdentifier(table.Name)] = table.Name
	}
	if err := migrationgo.WriteCoreCatalog(root, aliases); err != nil {
		return Result{}, err
	}
	snapshotData, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return Result{}, err
	}
	if err := os.WriteFile(coreSnapshot, append(snapshotData, '\n'), 0o644); err != nil {
		return Result{}, fmt.Errorf("escrever snapshot: %w", err)
	}
	_ = os.Remove(filepath.Join(output, snapshotName))
	if len(operations) == 0 {
		return Result{Unchanged: true}, nil
	}

	createdAt := time.Now()
	paths := make([]string, 0, len(operations))
	for index, operation := range operations {
		migration := nextMigrationName(output, createdAt.Add(time.Duration(index)*time.Second))
		plan := Plan{Version: 1, Migration: migration, CreatedAt: createdAt.UTC(), Operations: []Operation{operation}}
		data, err := renderGoMigration(plan, modulePath)
		if err != nil {
			return Result{}, err
		}
		path := filepath.Join(output, migration)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return Result{}, fmt.Errorf("escrever migration: %w", err)
		}
		paths = append(paths, filepath.ToSlash(filepath.Join(cfg.Output.Path, migration)))
	}
	return Result{
		Path:       paths[0],
		Paths:      paths,
		Migration:  filepath.Base(paths[0]),
		Operations: len(operations),
	}, nil
}

func hasMigrationFiles(output string) (bool, error) {
	entries, err := os.ReadDir(output)
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if !entry.IsDir() && goMigrationNamePattern.MatchString(entry.Name()) ||
			!entry.IsDir() && strings.HasSuffix(entry.Name(), "_migration.go") {
			return true, nil
		}
	}
	return false, nil
}

func renderGoMigration(plan Plan, modulePath string) ([]byte, error) {
	var builder strings.Builder
	builder.WriteString("// Code generated by Flexberry. Revise antes de executar.\n")
	builder.WriteString("package migrations\n\n")
	builder.WriteString("import migrate \"github.com/PhelipeViana/flexberry/migration\"\n")
	if len(plan.Operations) > 0 && plan.Operations[0].Kind != "create_table" {
		fmt.Fprintf(&builder, "import alias %q\n", modulePath+"/internal/flexberry/core/migration/alias")
	}
	builder.WriteString("\n")
	name := "Migration" + strings.NewReplacer("_", "", "-", "").Replace(strings.TrimSuffix(plan.Migration, ".go"))
	fmt.Fprintf(&builder, "func %s() migrate.Definition {\n", name)
	for _, operation := range plan.Operations {
		fmt.Fprintf(&builder, "\t// %s em %s.\n", actionComment(operation.Kind), operation.Table)
		if operation.Kind == string(acao.CreateIndex) {
			method := "CreateIndex"
			if operation.Unique {
				method = "CreateUniqueIndex"
			}
			fmt.Fprintf(&builder, "\treturn migrate.%s(alias.%s, %q", method, exportedAliasIdentifier(operation.Table), operation.Name)
			for _, column := range operation.IndexColumns {
				fmt.Fprintf(&builder, ", %q", column)
			}
			builder.WriteString(")\n")
			continue
		}
		if operation.Kind == string(acao.DropIndex) {
			fmt.Fprintf(&builder, "\treturn migrate.DropIndex(alias.%s, %q)\n", exportedAliasIdentifier(operation.Table), operation.Name)
			continue
		}
		if operation.Kind == "create_table" {
			fmt.Fprintf(&builder, "\treturn migrate.CreateTable(%q,", operation.Table)
		} else {
			fmt.Fprintf(&builder, "\treturn migrate.%s(alias.%s,", actionName(operation.Kind), exportedAliasIdentifier(operation.Table))
		}
		for _, column := range operation.Columns {
			fmt.Fprintf(&builder, "\n\t\t%s,", renderColumn(column))
		}
		if operation.Column != nil {
			fmt.Fprintf(&builder, "\n\t\t%s,", renderColumn(*operation.Column))
		}
		if operation.ForeignKey != nil {
			fmt.Fprintf(&builder, "\n\t\tmigrate.Col(%q).References(%q, %q),",
				operation.ForeignKey.Column, operation.ForeignKey.ReferenceTable, operation.ForeignKey.ReferenceColumn)
		}
		if operation.Kind == "create_table" {
			fmt.Fprintf(&builder, "\n\t).Alias(%q)\n", tableIdentifier(operation.Table))
		} else {
			builder.WriteString("\n\t)\n")
		}
	}
	builder.WriteString("}\n")
	data, err := format.Source([]byte(builder.String()))
	if err != nil {
		return nil, fmt.Errorf("formatar migration Go: %w\n%s", err, builder.String())
	}
	return data, nil
}

func readModulePath(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		if os.IsNotExist(err) {
			return "example.test/project", nil
		}
		return "", fmt.Errorf("ler go.mod: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "module" {
			return fields[1], nil
		}
	}
	return "", fmt.Errorf("go.mod não declara module")
}

func exportedAliasIdentifier(value string) string {
	if !strings.ContainsAny(value, "_- .") && value != "" {
		return strings.ToUpper(value[:1]) + value[1:]
	}
	identifier := tableIdentifier(value)
	return strings.ToUpper(identifier[:1]) + identifier[1:]
}

func renderColumn(column Column) string {
	value := fmt.Sprintf("migrate.Col(%q)", column.Name)
	switch column.Type {
	case "integer":
		value += ".Integer()"
	case "string":
		length := column.Length
		if length == 0 {
			length = 255
		}
		value += fmt.Sprintf(".Varchar(%d)", length)
	case "boolean":
		value += ".Boolean()"
	case "decimal":
		precision, scale := column.Precision, column.Scale
		if precision == 0 {
			precision, scale = 19, 4
		}
		value += fmt.Sprintf(".Decimal(%d, %d)", precision, scale)
	case "datetime":
		value += ".DateTime()"
	case "binary":
		value += ".Binary()"
	}
	if column.Nullable {
		value += ".Nullable()"
	}
	if column.PrimaryKey {
		value += ".PrimaryKey()"
	}
	if column.AutoIncrement {
		value += ".AutoIncrement()"
	}
	if column.Unique {
		value += ".Unique()"
	}
	if column.Default != "" {
		value += fmt.Sprintf(".Default(%q)", column.Default)
	}
	return value
}

func migrateLegacyHelpers(output, modulePath string) error {
	entries, err := os.ReadDir(output)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || entry.Name() == "dsl.gen.go" {
			continue
		}
		path := filepath.Join(output, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		updated := strings.ReplaceAll(string(data), "col(", "migrate.Col(")
		tablePattern := regexp.MustCompile(`\btable\.([A-Za-z_][A-Za-z0-9_]*)`)
		updated = tablePattern.ReplaceAllStringFunc(updated, func(value string) string {
			name := strings.TrimPrefix(value, "table.")
			return "alias." + strings.ToUpper(name[:1]) + name[1:]
		})
		updated = strings.ReplaceAll(updated, modulePath+"/internal/flexberry/core/migration/table", modulePath+"/internal/flexberry/core/migration/alias")
		updated = strings.ReplaceAll(updated, `table "`+modulePath+`/internal/flexberry/core/migration/alias"`, `alias "`+modulePath+`/internal/flexberry/core/migration/alias"`)
		if strings.Contains(updated, "alias.") && !strings.Contains(updated, "/core/migration/alias\"") {
			oldImport := `import migrate "github.com/PhelipeViana/flexberry/migration"`
			newImport := "import (\n\tmigrate \"github.com/PhelipeViana/flexberry/migration\"\n\talias \"" + modulePath + "/internal/flexberry/core/migration/alias\"\n)"
			updated = strings.Replace(updated, oldImport, newImport, 1)
		}
		if goMigrationNamePattern.MatchString(entry.Name()) && strings.Contains(updated, "return migrate.Define(") {
			updated = strings.Replace(updated, "return migrate.Define(\n", "return ", 1)
			updated = strings.Replace(updated, "return migrate.Define(\r\n", "return ", 1)
			if end := strings.LastIndex(updated, ",\n\t)\n}"); end >= 0 {
				updated = updated[:end] + "\n}" + updated[end+len(",\n\t)\n}"):]
			}
			if formatted, formatErr := format.Source([]byte(updated)); formatErr == nil {
				updated = string(formatted)
			}
		}
		if updated != string(data) {
			if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
				return err
			}
		}
	}
	_ = os.Remove(filepath.Join(output, "dsl.gen.go"))
	return nil
}

var goMigrationNamePattern = regexp.MustCompile(`^\d{4}_\d{2}_\d{2}_\d{6}\.go$`)

func actionName(kind string) string {
	return map[string]string{
		"create_table": "CreateTable", "drop_table": "DropTable",
		"add_column": "AddColumn", "alter_column": "AlterColumn",
		"drop_column": "DropColumn", "add_foreign_key": "AddForeignKey",
		"drop_foreign_key": "DropForeignKey",
	}[kind]
}

func actionComment(kind string) string {
	return map[string]string{
		"create_table": "Cria tabela", "drop_table": "Remove tabela",
		"add_column": "Adiciona coluna", "alter_column": "Altera coluna",
		"drop_column": "Remove coluna", "add_foreign_key": "Adiciona relacionamento",
		"drop_foreign_key": "Remove relacionamento", "create_index": "Cria índice", "drop_index": "Remove índice",
	}[kind]
}

func writeDSLHelpers(output string, tables []Table) error {
	var helpers strings.Builder
	helpers.WriteString(`// Code generated by Flexberry. DO NOT EDIT.
package migrations

import migrate "github.com/PhelipeViana/flexberry/migration"

func col(nome string) migrate.Column {
	return migrate.Col(nome)
}

var alias = struct {
`)
	for _, table := range tables {
		fmt.Fprintf(&helpers, "\t%s migrate.Table\n", tableIdentifier(table.Name))
	}
	helpers.WriteString("}{\n")
	for _, table := range tables {
		fmt.Fprintf(&helpers, "\t%s: migrate.Table(%q),\n", tableIdentifier(table.Name), tableIdentifier(table.Name))
	}
	helpers.WriteString("}\n")
	data, err := format.Source([]byte(helpers.String()))
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(output, "dsl.gen.go"), data, 0o644); err != nil {
		return fmt.Errorf("escrever helpers da migration: %w", err)
	}
	return nil
}

func tableIdentifier(value string) string {
	parts := strings.FieldsFunc(strings.ToLower(value), func(r rune) bool { return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9') })
	if len(parts) == 0 {
		return "tabela"
	}
	result := parts[0]
	for _, part := range parts[1:] {
		if part != "" {
			result += strings.ToUpper(part[:1]) + part[1:]
		}
	}
	if result[0] >= '0' && result[0] <= '9' {
		result = "t" + result
	}
	return result
}

func preservePreviousSnapshot(previous, current Snapshot) Snapshot {
	currentTables := tableMap(current.Tables)
	for _, oldTable := range previous.Tables {
		newTable, exists := currentTables[oldTable.Name]
		if !exists {
			current.Tables = append(current.Tables, oldTable)
			continue
		}
		newColumns := columnMap(newTable.Columns)
		for _, oldColumn := range oldTable.Columns {
			if _, exists := newColumns[oldColumn.Name]; !exists {
				newTable.Columns = append(newTable.Columns, oldColumn)
			}
		}
		sort.Slice(newTable.Columns, func(i, j int) bool {
			return newTable.Columns[i].Name < newTable.Columns[j].Name
		})
		for index := range current.Tables {
			if current.Tables[index].Name == newTable.Name {
				current.Tables[index] = newTable
				break
			}
		}
	}
	sort.Slice(current.Tables, func(i, j int) bool {
		return current.Tables[i].Name < current.Tables[j].Name
	})
	return current
}

func buildSnapshot(entities []scanner.Entity) (Snapshot, error) {
	byEntity := make(map[string]scanner.Entity, len(entities))
	for _, entity := range entities {
		byEntity[entity.Name] = entity
	}
	result := Snapshot{Version: 1}
	for _, entity := range entities {
		table := Table{Name: entity.Table}
		for _, field := range entity.Fields {
			kind, err := neutralType(field.GoType)
			if err != nil {
				return Snapshot{}, fmt.Errorf("%s.%s: %w", entity.Name, field.Name, err)
			}
			table.Columns = append(table.Columns, Column{
				Name: field.Column, Type: kind, Nullable: field.Nullable, PrimaryKey: field.PrimaryKey,
				AutoIncrement: field.AutoIncrement, Length: field.Length, Precision: field.Precision, Scale: field.Scale,
				Unique: field.Unique, Index: field.Index, Default: field.Default,
				ReferenceTable: field.ReferenceTable, ReferenceColumn: field.ReferenceColumn,
			})
			if field.ReferenceTable != "" {
				table.ForeignKeys = append(table.ForeignKeys, ForeignKey{Column: field.Column, ReferenceTable: field.ReferenceTable, ReferenceColumn: field.ReferenceColumn})
			}
		}
		for _, relation := range entity.Relations {
			if relation.Kind != "belongsTo" || relation.ForeignKey == "" {
				continue
			}
			target, exists := byEntity[relationTypeName(relation.Type)]
			if !exists {
				continue
			}
			candidate := ForeignKey{
				Column: relation.ForeignKey, ReferenceTable: target.Table, ReferenceColumn: target.PrimaryKey,
			}
			found := false
			for _, existing := range table.ForeignKeys {
				if existing.Column == candidate.Column {
					found = true
					break
				}
			}
			if !found {
				table.ForeignKeys = append(table.ForeignKeys, candidate)
			}
		}
		sort.Slice(table.Columns, func(i, j int) bool { return table.Columns[i].Name < table.Columns[j].Name })
		sort.Slice(table.ForeignKeys, func(i, j int) bool { return table.ForeignKeys[i].Column < table.ForeignKeys[j].Column })
		result.Tables = append(result.Tables, table)
	}
	sort.Slice(result.Tables, func(i, j int) bool { return result.Tables[i].Name < result.Tables[j].Name })
	return result, nil
}

func neutralType(goType string) (string, error) {
	value := strings.TrimLeft(strings.TrimSpace(goType), "*")
	switch value {
	case "string":
		return "string", nil
	case "bool":
		return "boolean", nil
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64":
		return "integer", nil
	case "float32", "float64":
		return "decimal", nil
	case "time.Time":
		return "datetime", nil
	case "[]byte":
		return "binary", nil
	default:
		return "", fmt.Errorf("tipo Go %q ainda não é suportado por migrations", goType)
	}
}

func loadSnapshot(path string) (Snapshot, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Snapshot{Version: 1}, nil
	}
	if err != nil {
		return Snapshot{}, err
	}
	var value Snapshot
	if err := json.Unmarshal(data, &value); err != nil {
		return Snapshot{}, fmt.Errorf("ler snapshot de migrations: %w", err)
	}
	return value, nil
}

func diff(previous, current Snapshot) ([]Operation, error) {
	oldTables := tableMap(previous.Tables)
	newTables := tableMap(current.Tables)
	var operations []Operation
	var oldTableNames []string
	for name := range oldTables {
		oldTableNames = append(oldTableNames, name)
	}
	sort.Strings(oldTableNames)
	for _, name := range oldTableNames {
		oldTable := oldTables[name]
		newTable, exists := newTables[name]
		if !exists {
			operations = append(operations, Operation{Kind: "drop_table", Table: name})
			continue
		}
		oldColumns, newColumns := columnMap(oldTable.Columns), columnMap(newTable.Columns)
		var oldColumnNames []string
		for column := range oldColumns {
			oldColumnNames = append(oldColumnNames, column)
		}
		sort.Strings(oldColumnNames)
		for _, column := range oldColumnNames {
			oldValue := oldColumns[column]
			newValue, exists := newColumns[column]
			if !exists {
				if foreignKeyColumn(oldTable, column) {
					return nil, fmt.Errorf("remoção de %s.%s foi bloqueada porque a coluna participa de um relacionamento", name, column)
				}
				copyColumn := oldValue
				operations = append(operations, Operation{Kind: "drop_column", Table: name, Column: &copyColumn})
				continue
			}
			if oldValue.Type != newValue.Type || oldValue.Nullable != newValue.Nullable || oldValue.Length != newValue.Length || oldValue.Precision != newValue.Precision || oldValue.Scale != newValue.Scale || oldValue.Unique != newValue.Unique || oldValue.Default != newValue.Default {
				if oldValue.PrimaryKey != newValue.PrimaryKey {
					return nil, fmt.Errorf("alteração automática de chave primária em %s.%s foi bloqueada", name, column)
				}
				copyColumn := newValue
				operations = append(operations, Operation{Kind: "alter_column", Table: name, Column: &copyColumn})
			}
		}
	}

	for _, table := range current.Tables {
		oldTable, exists := oldTables[table.Name]
		if !exists {
			operations = append(operations, Operation{Kind: "create_table", Table: table.Name, Columns: table.Columns})
			continue
		}
		oldColumns := columnMap(oldTable.Columns)
		for index := range table.Columns {
			column := table.Columns[index]
			if _, exists := oldColumns[column.Name]; !exists {
				copyColumn := column
				operations = append(operations, Operation{Kind: "add_column", Table: table.Name, Column: &copyColumn})
			}
		}
	}
	if len(previous.Tables) == 0 {
		for _, table := range current.Tables {
			for index := range table.ForeignKeys {
				fk := table.ForeignKeys[index]
				operations = append(operations, Operation{Kind: "add_foreign_key", Table: table.Name, ForeignKey: &fk})
			}
		}
	}
	for _, table := range current.Tables {
		oldTable, existed := oldTables[table.Name]
		oldColumns := columnMap(oldTable.Columns)
		for _, column := range table.Columns {
			old, hadColumn := oldColumns[column.Name]
			if column.Index && (!existed || !hadColumn || !old.Index) {
				operations = append(operations, Operation{Kind: string(acao.CreateIndex), Table: table.Name, Name: indexName(table.Name, column.Name), IndexColumns: []string{column.Name}})
			}
			if !column.Index && hadColumn && old.Index {
				operations = append(operations, Operation{Kind: string(acao.DropIndex), Table: table.Name, Name: indexName(table.Name, column.Name)})
			}
		}
	}
	return operations, nil
}

func indexName(table, column string) string {
	name := "idx_" + table + "_" + column
	if len(name) > 30 {
		name = name[:30]
	}
	return name
}

func nextMigrationName(output string, now time.Time) string {
	for {
		prefix := now.Format("2006_01_02_150405")
		matches, _ := filepath.Glob(filepath.Join(output, prefix+"*.go"))
		if len(matches) == 0 {
			name := prefix + ".go"
			return name
		}
		now = now.Add(time.Second)
	}
}

func tableMap(values []Table) map[string]Table {
	result := make(map[string]Table, len(values))
	for _, value := range values {
		result[value.Name] = value
	}
	return result
}

func columnMap(values []Column) map[string]Column {
	result := make(map[string]Column, len(values))
	for _, value := range values {
		result[value.Name] = value
	}
	return result
}

func foreignKeyColumn(table Table, column string) bool {
	for _, foreignKey := range table.ForeignKeys {
		if foreignKey.Column == column {
			return true
		}
	}
	return false
}

func relationTypeName(value string) string {
	value = strings.TrimLeft(value, "*[]")
	if index := strings.LastIndex(value, "."); index >= 0 {
		return value[index+1:]
	}
	return value
}
