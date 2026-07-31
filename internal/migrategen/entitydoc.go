package migrategen

import (
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/PhelipeViana/flexberry/internal/config"
	"github.com/PhelipeViana/flexberry/internal/migrationgo"
	"github.com/PhelipeViana/flexberry/internal/scanner"
	"github.com/PhelipeViana/flexberry/migration/acao"
)

const entityDocName = "doc_entities.txt"

// WriteEntityDocumentation rebuilds the schema from the ordered migration
// history and writes domain struct examples that can be copied by developers.
func WriteEntityDocumentation(root string, cfg config.MigrateConfig, scannedEntities ...scanner.Entity) (string, int, error) {
	if err := ensureManualAliasImports(root, cfg); err != nil {
		return "", 0, err
	}
	output := filepath.Join(root, filepath.FromSlash(cfg.Output.Path))
	entries, err := os.ReadDir(output)
	if err != nil {
		return "", 0, fmt.Errorf("ler migrations para documentar entidades: %w", err)
	}
	var files []string
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() && (goMigrationNamePattern.MatchString(name) || strings.HasSuffix(name, "_migration.go")) {
			files = append(files, filepath.Join(output, name))
		}
	}
	sort.Strings(files)
	tables, aliases := map[string]*Table{}, map[string]string{}
	for _, file := range files {
		operations, parseErr := migrationgo.ParseFile(file)
		if parseErr != nil {
			return "", 0, fmt.Errorf("ler %s para gerar %s: %w", filepath.Base(file), entityDocName, parseErr)
		}
		for _, operation := range operations {
			operation = resolveDocumentationAlias(operation, aliases)
			applyDocumentationOperation(tables, aliases, operation)
		}
		// O catálogo é reconstruído em ordem histórica. Assim uma migration
		// posterior já pode usar alias.X criado por uma migration anterior.
		if err := writeDocumentationCatalog(root, tables, aliases); err != nil {
			return "", 0, err
		}
	}
	path := filepath.Join(output, entityDocName)
	if err := writeDocumentationCatalog(root, tables, aliases); err != nil {
		return "", 0, err
	}
	entities := map[string]scanner.Entity{}
	for _, entity := range scannedEntities {
		entities[entity.Table] = entity
	}
	if err := os.WriteFile(path, []byte(renderEntityDocumentation(tables, aliases, entities)), 0o644); err != nil {
		return "", 0, fmt.Errorf("escrever %s: %w", entityDocName, err)
	}
	return path, len(tables), nil
}

func resolveDocumentationAlias(operation acao.Operacao, aliases map[string]string) acao.Operacao {
	resolve := func(value string) string {
		for physical, alias := range aliases {
			if alias == value {
				return physical
			}
		}
		return value
	}
	if acao.Tipo(operation.Kind) != acao.CreateTable {
		operation.Table = resolve(operation.Table)
	}
	if operation.ForeignKey != nil {
		copied := *operation.ForeignKey
		copied.ReferenceTable = resolve(copied.ReferenceTable)
		operation.ForeignKey = &copied
	}
	return operation
}

func writeDocumentationCatalog(root string, tables map[string]*Table, aliases map[string]string) error {
	catalog := map[string]string{}
	for tableName := range tables {
		alias := aliases[tableName]
		if alias == "" {
			alias = tableIdentifier(tableName)
		}
		catalog[alias] = tableName
	}
	if err := migrationgo.WriteCoreCatalog(root, catalog); err != nil {
		return fmt.Errorf("atualizar autocomplete table: %w", err)
	}
	return nil
}

func applyDocumentationOperation(tables map[string]*Table, aliases map[string]string, op acao.Operacao) {
	switch acao.Tipo(op.Kind) {
	case acao.CreateTable:
		table := &Table{Name: op.Table, Columns: append([]Column(nil), op.Columns...)}
		for _, column := range table.Columns {
			if column.ReferenceTable != "" {
				table.ForeignKeys = append(table.ForeignKeys, ForeignKey{Column: column.Name, ReferenceTable: column.ReferenceTable, ReferenceColumn: column.ReferenceColumn})
			}
		}
		tables[op.Table] = table
		if op.AliasName != "" {
			aliases[op.Table] = op.AliasName
		}
	case acao.DropTable:
		delete(tables, op.Table)
		delete(aliases, op.Table)
	case acao.AddColumn, acao.AlterColumn:
		if op.Column != nil {
			setDocumentationColumn(ensureDocumentationTable(tables, op.Table), *op.Column)
		}
	case acao.DropColumn:
		if op.Column != nil && tables[op.Table] != nil {
			tables[op.Table].Columns = removeDocumentationColumn(tables[op.Table].Columns, op.Column.Name)
			tables[op.Table].ForeignKeys = removeDocumentationForeignKey(tables[op.Table].ForeignKeys, op.Column.Name)
		}
	case acao.AddForeignKey:
		if op.ForeignKey != nil {
			table := ensureDocumentationTable(tables, op.Table)
			table.ForeignKeys = removeDocumentationForeignKey(table.ForeignKeys, op.ForeignKey.Column)
			table.ForeignKeys = append(table.ForeignKeys, *op.ForeignKey)
		}
	case acao.DropForeignKey:
		if op.ForeignKey != nil && tables[op.Table] != nil {
			tables[op.Table].ForeignKeys = removeDocumentationForeignKey(tables[op.Table].ForeignKeys, op.ForeignKey.Column)
		}
	case acao.RenameTable:
		if table := tables[op.Table]; table != nil {
			delete(tables, op.Table)
			table.Name = op.NewName
			tables[op.NewName] = table
			if alias := aliases[op.Table]; alias != "" {
				delete(aliases, op.Table)
				aliases[op.NewName] = alias
			}
		}
	case acao.CreateIndex, acao.DropIndex:
		if table := tables[op.Table]; table != nil {
			for index := range table.Columns {
				for _, name := range op.IndexColumns {
					if table.Columns[index].Name == name {
						table.Columns[index].Index = acao.Tipo(op.Kind) == acao.CreateIndex
						if op.Unique {
							table.Columns[index].Unique = true
						}
					}
				}
			}
		}
	}
}

func ensureDocumentationTable(tables map[string]*Table, name string) *Table {
	if tables[name] == nil {
		tables[name] = &Table{Name: name}
	}
	return tables[name]
}
func setDocumentationColumn(table *Table, column Column) {
	for i := range table.Columns {
		if table.Columns[i].Name == column.Name {
			table.Columns[i] = column
			table.ForeignKeys = removeDocumentationForeignKey(table.ForeignKeys, column.Name)
			if column.ReferenceTable != "" {
				table.ForeignKeys = append(table.ForeignKeys, ForeignKey{Column: column.Name, ReferenceTable: column.ReferenceTable, ReferenceColumn: column.ReferenceColumn})
			}
			return
		}
	}
	table.Columns = append(table.Columns, column)
	if column.ReferenceTable != "" {
		table.ForeignKeys = append(table.ForeignKeys, ForeignKey{Column: column.Name, ReferenceTable: column.ReferenceTable, ReferenceColumn: column.ReferenceColumn})
	}
}
func removeDocumentationColumn(columns []Column, name string) []Column {
	result := columns[:0]
	for _, column := range columns {
		if column.Name != name {
			result = append(result, column)
		}
	}
	return result
}
func removeDocumentationForeignKey(keys []ForeignKey, column string) []ForeignKey {
	result := keys[:0]
	for _, key := range keys {
		if key.Column != column {
			result = append(result, key)
		}
	}
	return result
}

func renderEntityDocumentation(tables map[string]*Table, aliases map[string]string, entities map[string]scanner.Entity) string {
	var names []string
	for name := range tables {
		names = append(names, name)
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("// Code generated by Flexberry Migrate Reload. DO NOT EDIT.\n// Copie apenas as structs necessárias para seus arquivos de domínio.\n// Representa o estado acumulado de todas as migrations.\n// Campos DateTime usam time.Time e exigem import \"time\" no arquivo de destino.\n\n")
	for _, name := range names {
		table, typeName := tables[name], exportedIdentifier(singular(aliases[name]))
		entity, mapped := entities[name]
		if mapped {
			typeName = entity.Name
		}
		if typeName == "" {
			typeName = exportedIdentifier(singular(name))
		}
		b.WriteString("// Table: " + name + "\ntype " + typeName + " struct {\n")
		written := map[string]bool{}
		if mapped {
			columns := map[string]Column{}
			for _, column := range table.Columns {
				columns[column.Name] = documentedColumn(*table, column)
			}
			for _, field := range entity.Fields {
				if _, exists := columns[field.Column]; !exists {
					continue
				}
				fmt.Fprintf(&b, "\t%-24s %-12s %s\n", field.Name, field.GoType, documentationTags(field.Column, columns[field.Column]))
				written[field.Column] = true
			}
		}
		for _, column := range table.Columns {
			if written[column.Name] {
				continue
			}
			column = documentedColumn(*table, column)
			fmt.Fprintf(&b, "\t%-24s %-12s %s\n", exportedIdentifier(column.Name), documentationGoType(column), documentationTags(column.Name, column))
		}
		if mapped {
			for _, relation := range entity.Relations {
				fmt.Fprintf(&b, "\t%-24s %-12s `json:%q`\n", relation.Name, relation.Type, strings.ToLower(relation.Name))
			}
		} else {
			for _, relation := range table.ForeignKeys {
				relationType := exportedIdentifier(singular(aliases[relation.ReferenceTable]))
				if relationType == "" {
					relationType = exportedIdentifier(singular(relation.ReferenceTable))
				}
				if relationType != "" {
					fmt.Fprintf(&b, "\t%-24s %-12s `json:%q` // relation: %s -> %s.%s\n", relationType, relationType, strings.ToLower(relationType), relation.Column, relation.ReferenceTable, relation.ReferenceColumn)
				}
			}
		}
		b.WriteString("}\n\n")
	}
	if len(names) == 0 {
		b.WriteString("// Nenhuma tabela foi encontrada no histórico de migrations.\n")
	}
	formatted, err := format.Source([]byte("package domain\n\n" + b.String()))
	if err == nil {
		return strings.TrimPrefix(string(formatted), "package domain\n\n")
	}
	return b.String()
}

func documentedColumn(table Table, column Column) Column {
	for _, key := range table.ForeignKeys {
		if key.Column == column.Name {
			column.ReferenceTable, column.ReferenceColumn = key.ReferenceTable, key.ReferenceColumn
			break
		}
	}
	return column
}

func documentationTags(name string, column Column) string {
	var options []string
	if column.PrimaryKey {
		options = append(options, "primaryKey")
	}
	if column.AutoIncrement {
		options = append(options, "autoIncrement")
	}
	if column.Length > 0 && column.Type == "string" {
		options = append(options, fmt.Sprintf("size=%d", column.Length))
	}
	if column.Precision > 0 {
		options = append(options, fmt.Sprintf("precision=%d", column.Precision))
	}
	if column.Scale > 0 {
		options = append(options, fmt.Sprintf("scale=%d", column.Scale))
	}
	if column.Unique {
		options = append(options, "unique")
	}
	if column.Index {
		options = append(options, "index")
	}
	if column.Default != "" {
		options = append(options, "default="+column.Default)
	}
	if column.ReferenceTable != "" {
		options = append(options, "references="+column.ReferenceTable+"."+column.ReferenceColumn)
	}
	tag := fmt.Sprintf("`db:%q json:%q", name, name)
	if len(options) > 0 {
		tag += fmt.Sprintf(" migrate:%q", strings.Join(options, ","))
	}
	return tag + "`"
}

func documentationGoType(column Column) string {
	value := map[string]string{"integer": "int64", "string": "string", "boolean": "bool", "decimal": "float64", "datetime": "time.Time", "binary": "[]byte"}[column.Type]
	if value == "" {
		value = "any"
	}
	if column.Nullable && value != "[]byte" && value != "any" {
		value = "*" + value
	}
	return value
}

func exportedIdentifier(value string) string {
	var normalized strings.Builder
	var previous rune
	for index, current := range value {
		if index > 0 && unicode.IsUpper(current) && (unicode.IsLower(previous) || unicode.IsDigit(previous)) {
			normalized.WriteRune('_')
		}
		normalized.WriteRune(current)
		previous = current
	}
	parts := strings.FieldsFunc(normalized.String(), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	var result strings.Builder
	initialisms := map[string]string{"id": "ID", "cpf": "CPF", "cnpj": "CNPJ", "url": "URL", "uuid": "UUID", "ip": "IP", "cep": "CEP", "uf": "UF"}
	for _, part := range parts {
		lower := strings.ToLower(part)
		if known := initialisms[lower]; known != "" {
			result.WriteString(known)
			continue
		}
		runes := []rune(lower)
		if len(runes) > 0 {
			runes[0] = unicode.ToUpper(runes[0])
			result.WriteString(string(runes))
		}
	}
	return result.String()
}

func singular(value string) string {
	lower := strings.ToLower(value)
	if strings.HasSuffix(lower, "coes") {
		return value[:len(value)-4] + "cao"
	}
	if strings.HasSuffix(lower, "oes") {
		return value[:len(value)-3] + "ao"
	}
	if strings.HasSuffix(lower, "s") && len(value) > 1 {
		return value[:len(value)-1]
	}
	return value
}
