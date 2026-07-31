// Package sqlimport converts a PostgreSQL migration history into a neutral,
// squashed Flexberry baseline. It is intended for onboarding existing schemas.
package sqlimport

import (
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/PhelipeViana/flexberry/internal/migrationgo"
)

type Column struct {
	Name, Kind, Default                                     string
	Length, Precision, Scale                                int
	Nullable, PrimaryKey, Unique, AutoIncrement, DefaultRaw bool
}

type Table struct {
	Name    string
	Columns []*Column
	byName  map[string]*Column
}

type Constraint struct {
	Table, Name, Kind    string
	Columns              []string
	ReferenceTable       string
	ReferenceColumns     []string
	Expression, OnDelete string
}

type Index struct {
	Table, Name string
	Columns     []string
	Unique      bool
}

type View struct{ Name, Query string }

type Result struct {
	Tables            []*Table
	Constraints       []Constraint
	Indexes           []Index
	Views             []View
	IgnoredData       int
	IgnoredSequences  []string
	NormalizedColumns []string
	Unsupported       []string
	constraints       map[string]Constraint
	indexes           map[string]Index
	views             map[string]View
}

var (
	createTablePattern    = regexp.MustCompile(`(?is)^CREATE\s+TABLE(?:\s+IF\s+NOT\s+EXISTS)?\s+([a-zA-Z_][a-zA-Z0-9_]*)\s*\((.*)\)$`)
	alterTablePattern     = regexp.MustCompile(`(?is)^ALTER\s+TABLE(?:\s+IF\s+EXISTS)?\s+([a-zA-Z_][a-zA-Z0-9_]*)\s+(.+)$`)
	createIndexPattern    = regexp.MustCompile(`(?is)^CREATE\s+(UNIQUE\s+)?INDEX(?:\s+IF\s+NOT\s+EXISTS)?\s+([a-zA-Z_][a-zA-Z0-9_]*)\s+ON\s+([a-zA-Z_][a-zA-Z0-9_]*)\s*\((.*)\)$`)
	createViewPattern     = regexp.MustCompile(`(?is)^CREATE\s+(?:OR\s+REPLACE\s+)?VIEW\s+([a-zA-Z_][a-zA-Z0-9_]*)\s+AS\s+(.+)$`)
	createSequencePattern = regexp.MustCompile(`(?is)^CREATE\s+SEQUENCE(?:\s+IF\s+NOT\s+EXISTS)?\s+([a-zA-Z_][a-zA-Z0-9_]*)`)
	constraintPattern     = regexp.MustCompile(`(?is)^CONSTRAINT\s+([a-zA-Z_][a-zA-Z0-9_]*)\s+(.+)$`)
	typePattern           = regexp.MustCompile(`(?is)^(BIGINT|INTEGER|NUMERIC(?:\s*\(\s*\d+\s*(?:,\s*\d+\s*)?\))?|DECIMAL(?:\s*\(\s*\d+\s*,\s*\d+\s*\))?|VARCHAR(?:\s*\(\s*\d+\s*\))|CHAR(?:\s*\(\s*\d+\s*\))|TEXT|TIMESTAMP(?:\s+(?:WITH|WITHOUT)\s+TIME\s+ZONE)?|BYTEA)\b?(.*)$`)
)

func ReadPostgres(folder string) (Result, error) {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return Result{}, err
	}
	result := Result{constraints: map[string]Constraint{}, indexes: map[string]Index{}, views: map[string]View{}}
	tables := map[string]*Table{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".sql") {
			continue
		}
		path := filepath.Join(folder, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return Result{}, err
		}
		for _, statement := range splitStatements(string(data)) {
			if err := applyStatement(&result, tables, statement); err != nil {
				result.Unsupported = append(result.Unsupported, entry.Name()+": "+err.Error()+" ["+preview(statement)+"]")
			}
		}
	}
	normalizeForeignKeyColumnTypes(&result, tables)
	result.Tables = make([]*Table, 0, len(tables))
	for _, table := range tables {
		result.Tables = append(result.Tables, table)
	}
	sort.Slice(result.Tables, func(i, j int) bool { return result.Tables[i].Name < result.Tables[j].Name })
	for _, constraint := range result.constraints {
		result.Constraints = append(result.Constraints, constraint)
	}
	for _, index := range result.indexes {
		result.Indexes = append(result.Indexes, index)
	}
	for _, view := range result.views {
		result.Views = append(result.Views, view)
	}
	// Candidate keys must exist before foreign keys reference them. This order is
	// required by MySQL and is valid for every supported dialect.
	sort.Slice(result.Constraints, func(i, j int) bool {
		left, right := constraintOrder(result.Constraints[i].Kind), constraintOrder(result.Constraints[j].Kind)
		if left != right {
			return left < right
		}
		return result.Constraints[i].Name < result.Constraints[j].Name
	})
	sort.Slice(result.Indexes, func(i, j int) bool { return result.Indexes[i].Name < result.Indexes[j].Name })
	sort.Slice(result.Views, func(i, j int) bool { return result.Views[i].Name < result.Views[j].Name })
	return result, nil
}

// normalizeForeignKeyColumnTypes reconciles legacy pairs accepted by one
// database but rejected by another (notably INTEGER -> BIGINT on MySQL). It
// widens both sides to a lossless common representation.
func normalizeForeignKeyColumnTypes(result *Result, tables map[string]*Table) {
	changed := map[string]bool{}
	for pass := 0; pass < len(result.constraints)+1; pass++ {
		updated := false
		for _, constraint := range result.constraints {
			if constraint.Kind != "foreign" {
				continue
			}
			localTable, referenceTable := tables[constraint.Table], tables[constraint.ReferenceTable]
			if localTable == nil || referenceTable == nil {
				continue
			}
			for index := range constraint.Columns {
				if index >= len(constraint.ReferenceColumns) {
					continue
				}
				local, reference := localTable.byName[constraint.Columns[index]], referenceTable.byName[constraint.ReferenceColumns[index]]
				if local == nil || reference == nil || sameColumnType(local, reference) {
					continue
				}
				if !widenCompatibleColumns(local, reference) {
					result.Unsupported = append(result.Unsupported, fmt.Sprintf(
						"foreign key %s possui tipos incompatíveis: %s.%s (%s) -> %s.%s (%s)",
						constraint.Name, constraint.Table, local.Name, local.Kind,
						constraint.ReferenceTable, reference.Name, reference.Kind,
					))
					continue
				}
				changed[constraint.Table+"."+local.Name] = true
				changed[constraint.ReferenceTable+"."+reference.Name] = true
				updated = true
			}
		}
		if !updated {
			break
		}
	}
	for value := range changed {
		result.NormalizedColumns = append(result.NormalizedColumns, value)
	}
	sort.Strings(result.NormalizedColumns)
}

func sameColumnType(left, right *Column) bool {
	return left.Kind == right.Kind && left.Length == right.Length && left.Precision == right.Precision && left.Scale == right.Scale
}

func widenCompatibleColumns(left, right *Column) bool {
	if isIntegerKind(left.Kind) && isIntegerKind(right.Kind) {
		left.Kind, right.Kind = "integer", "integer"
		return true
	}
	if isTextKind(left.Kind) && isTextKind(right.Kind) {
		length := left.Length
		if right.Length > length {
			length = right.Length
		}
		kind := "char"
		if left.Kind != "char" || right.Kind != "char" {
			kind = "string"
		}
		left.Kind, right.Kind, left.Length, right.Length = kind, kind, length, length
		return true
	}
	if left.Kind == "decimal" && right.Kind == "decimal" {
		precision, scale := left.Precision, left.Scale
		if right.Precision > precision {
			precision = right.Precision
		}
		if right.Scale > scale {
			scale = right.Scale
		}
		left.Precision, right.Precision, left.Scale, right.Scale = precision, precision, scale, scale
		return true
	}
	return false
}

func isIntegerKind(kind string) bool { return kind == "int" || kind == "integer" }
func isTextKind(kind string) bool    { return kind == "char" || kind == "string" }

func constraintOrder(kind string) int {
	switch kind {
	case "primary":
		return 0
	case "unique":
		return 1
	case "check":
		return 2
	case "foreign":
		return 3
	default:
		return 4
	}
}

func ModulePath(projectRoot string) (string, error) {
	data, err := os.ReadFile(filepath.Join(projectRoot, "go.mod"))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "module" {
			return fields[1], nil
		}
	}
	return "", fmt.Errorf("go.mod não declara module")
}

func applyStatement(result *Result, tables map[string]*Table, statement string) error {
	statement = strings.TrimSpace(statement)
	if statement == "" {
		return nil
	}
	upper := strings.ToUpper(statement)
	switch {
	case strings.HasPrefix(upper, "UPDATE "), strings.HasPrefix(upper, "INSERT "), strings.HasPrefix(upper, "DELETE "), strings.HasPrefix(upper, "SELECT "):
		result.IgnoredData++
		return nil
	}
	if match := createTablePattern.FindStringSubmatch(statement); len(match) > 0 {
		name := normalizeName(match[1])
		table := &Table{Name: name, byName: map[string]*Column{}}
		for _, item := range splitTopLevel(match[2], ',') {
			if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(item)), "CONSTRAINT ") {
				constraint, err := parseConstraint(name, item)
				if err != nil {
					return err
				}
				result.constraints[constraintKey(name, constraint.Name)] = constraint
				continue
			}
			column, inline, err := parseColumn(item)
			if err != nil {
				return err
			}
			table.Columns = append(table.Columns, column)
			table.byName[column.Name] = column
			for _, constraint := range inline {
				constraint.Table = name
				constraint.Name = "pk_" + name
				result.constraints[constraintKey(name, constraint.Name)] = constraint
			}
		}
		tables[name] = table
		return nil
	}
	if match := alterTablePattern.FindStringSubmatch(statement); len(match) > 0 {
		return applyAlter(result, tables, normalizeName(match[1]), strings.TrimSpace(match[2]))
	}
	if match := createIndexPattern.FindStringSubmatch(statement); len(match) > 0 {
		index := Index{Unique: strings.TrimSpace(match[1]) != "", Name: normalizeName(match[2]), Table: normalizeName(match[3]), Columns: parseNames(match[4])}
		result.indexes[index.Name] = index
		return nil
	}
	if match := createViewPattern.FindStringSubmatch(statement); len(match) > 0 {
		name := normalizeName(match[1])
		result.views[name] = View{Name: name, Query: strings.TrimSpace(match[2])}
		return nil
	}
	if match := createSequencePattern.FindStringSubmatch(statement); len(match) > 0 {
		result.IgnoredSequences = append(result.IgnoredSequences, normalizeName(match[1]))
		return nil
	}
	return fmt.Errorf("comando SQL ainda não suportado")
}

func applyAlter(result *Result, tables map[string]*Table, tableName, action string) error {
	upper := strings.ToUpper(action)
	if strings.HasPrefix(upper, "RENAME TO ") {
		newName := normalizeName(strings.Fields(action)[2])
		table := tables[tableName]
		if table == nil {
			return nil
		}
		delete(tables, tableName)
		table.Name = newName
		tables[newName] = table
		for key, constraint := range result.constraints {
			changed := false
			if constraint.Table == tableName {
				constraint.Table, changed = newName, true
			}
			if constraint.ReferenceTable == tableName {
				constraint.ReferenceTable, changed = newName, true
			}
			if changed {
				delete(result.constraints, key)
				result.constraints[constraintKey(constraint.Table, constraint.Name)] = constraint
			}
		}
		for name, index := range result.indexes {
			if index.Table == tableName {
				index.Table = newName
				result.indexes[name] = index
			}
		}
		return nil
	}
	if strings.HasPrefix(upper, "RENAME COLUMN ") {
		match := regexp.MustCompile(`(?is)^RENAME\s+COLUMN\s+(\w+)\s+TO\s+(\w+)$`).FindStringSubmatch(action)
		if len(match) == 0 {
			return fmt.Errorf("RenameColumn não reconhecido")
		}
		renameColumn(result, tables[tableName], tableName, normalizeName(match[1]), normalizeName(match[2]))
		return nil
	}
	if strings.HasPrefix(upper, "ADD COLUMN ") || strings.HasPrefix(upper, "ADD ") && !strings.HasPrefix(upper, "ADD CONSTRAINT ") {
		definition := strings.TrimSpace(action[len("ADD "):])
		if strings.HasPrefix(strings.ToUpper(definition), "COLUMN ") {
			definition = strings.TrimSpace(definition[len("COLUMN "):])
		}
		if strings.HasPrefix(strings.ToUpper(definition), "IF NOT EXISTS ") {
			definition = strings.TrimSpace(definition[len("IF NOT EXISTS "):])
		}
		column, _, err := parseColumn(definition)
		if err != nil {
			return err
		}
		table := tables[tableName]
		if table == nil {
			return fmt.Errorf("ADD COLUMN referencia tabela ausente %s", tableName)
		}
		if existing := table.byName[column.Name]; existing != nil {
			*existing = *column
		} else {
			table.Columns = append(table.Columns, column)
			table.byName[column.Name] = column
		}
		return nil
	}
	if strings.HasPrefix(upper, "ADD CONSTRAINT ") {
		constraint, err := parseConstraint(tableName, strings.TrimSpace(action[len("ADD "):]))
		if err != nil {
			return err
		}
		result.constraints[constraintKey(tableName, constraint.Name)] = constraint
		return nil
	}
	if strings.HasPrefix(upper, "DROP CONSTRAINT ") {
		fields := strings.Fields(action)
		name := fields[len(fields)-1]
		if strings.EqualFold(name, "EXISTS") || len(fields) < 3 {
			return fmt.Errorf("DROP CONSTRAINT inválido")
		}
		delete(result.constraints, constraintKey(tableName, normalizeName(name)))
		return nil
	}
	match := regexp.MustCompile(`(?is)^ALTER\s+COLUMN\s+(\w+)\s+(.+)$`).FindStringSubmatch(action)
	if len(match) > 0 {
		table := tables[tableName]
		if table == nil || table.byName[normalizeName(match[1])] == nil {
			return fmt.Errorf("ALTER COLUMN referencia coluna ausente %s.%s", tableName, match[1])
		}
		column := table.byName[normalizeName(match[1])]
		change, changeUpper := strings.TrimSpace(match[2]), strings.ToUpper(strings.TrimSpace(match[2]))
		switch {
		case changeUpper == "SET NOT NULL":
			column.Nullable = false
		case changeUpper == "DROP NOT NULL":
			column.Nullable = true
		case strings.HasPrefix(changeUpper, "SET DEFAULT "):
			setDefault(column, strings.TrimSpace(change[len("SET DEFAULT "):]))
		case changeUpper == "DROP DEFAULT":
			column.Default, column.DefaultRaw = "", false
		case strings.HasPrefix(changeUpper, "TYPE "):
			parsed, _, err := parseColumn(column.Name + " " + strings.TrimSpace(change[len("TYPE "):]))
			if err != nil {
				return err
			}
			parsed.Nullable, parsed.Default, parsed.DefaultRaw = column.Nullable, column.Default, column.DefaultRaw
			*column = *parsed
		default:
			return fmt.Errorf("ALTER COLUMN não suportado: %s", change)
		}
		return nil
	}
	return fmt.Errorf("ALTER TABLE não suportado: %s", preview(action))
}

func parseColumn(item string) (*Column, []Constraint, error) {
	item = strings.TrimSpace(item)
	fields := strings.Fields(item)
	if len(fields) < 2 {
		return nil, nil, fmt.Errorf("coluna inválida: %s", item)
	}
	name := normalizeName(fields[0])
	rest := strings.TrimSpace(item[len(fields[0]):])
	match := typePattern.FindStringSubmatch(rest)
	if len(match) == 0 {
		return nil, nil, fmt.Errorf("tipo de %s não suportado: %s", name, rest)
	}
	typeName, tail := strings.ToUpper(strings.TrimSpace(match[1])), strings.TrimSpace(match[2])
	column := &Column{Name: name, Nullable: !strings.Contains(strings.ToUpper(tail), "NOT NULL")}
	switch {
	case typeName == "BIGINT":
		column.Kind = "integer"
	case typeName == "INTEGER":
		column.Kind = "int"
	case strings.HasPrefix(typeName, "VARCHAR"):
		column.Kind, column.Length = "string", firstNumber(typeName, 255)
	case strings.HasPrefix(typeName, "CHAR"):
		column.Kind, column.Length = "char", firstNumber(typeName, 1)
	case typeName == "TEXT":
		column.Kind = "text"
	case strings.HasPrefix(typeName, "TIMESTAMP"):
		column.Kind = "timestamp"
	case typeName == "BYTEA":
		column.Kind = "binary"
	case strings.HasPrefix(typeName, "NUMERIC"), strings.HasPrefix(typeName, "DECIMAL"):
		column.Kind = "decimal"
		numbers := allNumbers(typeName)
		if len(numbers) == 0 {
			column.Precision, column.Scale = 38, 0
		} else if len(numbers) == 1 {
			column.Precision = numbers[0]
		} else {
			column.Precision, column.Scale = numbers[0], numbers[1]
		}
	}
	upperTail := strings.ToUpper(tail)
	column.AutoIncrement = strings.Contains(upperTail, "IDENTITY")
	if value, ok := defaultValue(tail); ok && !column.AutoIncrement {
		setDefault(column, value)
	}
	if strings.Contains(upperTail, "PRIMARY KEY") {
		column.PrimaryKey, column.Nullable = true, false
	}
	var constraints []Constraint
	if column.PrimaryKey {
		constraints = append(constraints, Constraint{Table: "", Name: "pk_" + name, Kind: "primary", Columns: []string{name}})
	}
	return column, constraints, nil
}

func parseConstraint(table, value string) (Constraint, error) {
	match := constraintPattern.FindStringSubmatch(strings.TrimSpace(value))
	if len(match) == 0 {
		return Constraint{}, fmt.Errorf("constraint inválida: %s", preview(value))
	}
	constraint := Constraint{Table: table, Name: normalizeName(match[1])}
	body, upper := strings.TrimSpace(match[2]), strings.ToUpper(strings.TrimSpace(match[2]))
	switch {
	case strings.HasPrefix(upper, "PRIMARY KEY"):
		constraint.Kind, constraint.Columns = "primary", namesInFirstParentheses(body)
	case strings.HasPrefix(upper, "UNIQUE"):
		constraint.Kind, constraint.Columns = "unique", namesInFirstParentheses(body)
	case strings.HasPrefix(upper, "CHECK"):
		constraint.Kind, constraint.Expression = "check", insideFirstParentheses(body)
	case strings.HasPrefix(upper, "FOREIGN KEY"):
		constraint.Kind, constraint.Columns = "foreign", namesInFirstParentheses(body)
		ref := regexp.MustCompile(`(?is)REFERENCES\s+(\w+)\s*\(([^)]*)\)`).FindStringSubmatch(body)
		if len(ref) == 0 {
			return Constraint{}, fmt.Errorf("referência inválida: %s", preview(body))
		}
		constraint.ReferenceTable, constraint.ReferenceColumns = normalizeName(ref[1]), parseNames(ref[2])
		if strings.Contains(upper, "ON DELETE CASCADE") {
			constraint.OnDelete = "CASCADE"
		}
	default:
		return Constraint{}, fmt.Errorf("tipo de constraint não suportado: %s", preview(body))
	}
	return constraint, nil
}

func renameColumn(result *Result, table *Table, tableName, oldName, newName string) {
	if table == nil || table.byName[oldName] == nil {
		return
	}
	column := table.byName[oldName]
	delete(table.byName, oldName)
	column.Name = newName
	table.byName[newName] = column
	for key, constraint := range result.constraints {
		changed := false
		if constraint.Table == tableName {
			for i := range constraint.Columns {
				if constraint.Columns[i] == oldName {
					constraint.Columns[i], changed = newName, true
				}
			}
		}
		if constraint.ReferenceTable == tableName {
			for i := range constraint.ReferenceColumns {
				if constraint.ReferenceColumns[i] == oldName {
					constraint.ReferenceColumns[i], changed = newName, true
				}
			}
		}
		if changed {
			result.constraints[key] = constraint
		}
	}
	for name, index := range result.indexes {
		if index.Table == tableName {
			for i := range index.Columns {
				if index.Columns[i] == oldName {
					index.Columns[i] = newName
				}
			}
			result.indexes[name] = index
		}
	}
}

func splitStatements(source string) []string {
	var result []string
	var builder strings.Builder
	inString, lineComment := false, false
	for index := 0; index < len(source); index++ {
		char := source[index]
		if lineComment {
			if char == '\n' {
				lineComment = false
				builder.WriteByte(char)
			}
			continue
		}
		if !inString && char == '-' && index+1 < len(source) && source[index+1] == '-' {
			lineComment = true
			index++
			continue
		}
		if char == '\'' {
			builder.WriteByte(char)
			if inString && index+1 < len(source) && source[index+1] == '\'' {
				builder.WriteByte(source[index+1])
				index++
				continue
			}
			inString = !inString
			continue
		}
		if char == ';' && !inString {
			if value := strings.TrimSpace(builder.String()); value != "" {
				result = append(result, value)
			}
			builder.Reset()
			continue
		}
		builder.WriteByte(char)
	}
	if value := strings.TrimSpace(builder.String()); value != "" {
		result = append(result, value)
	}
	return result
}

func splitTopLevel(value string, separator byte) []string {
	var result []string
	start, depth := 0, 0
	inString := false
	for index := 0; index < len(value); index++ {
		char := value[index]
		if char == '\'' {
			if inString && index+1 < len(value) && value[index+1] == '\'' {
				index++
				continue
			}
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if char == '(' {
			depth++
		} else if char == ')' {
			depth--
		} else if char == separator && depth == 0 {
			result = append(result, strings.TrimSpace(value[start:index]))
			start = index + 1
		}
	}
	result = append(result, strings.TrimSpace(value[start:]))
	return result
}

func WriteBaseline(projectRoot, output, modulePath string, result Result) ([]string, error) {
	if len(result.Unsupported) > 0 {
		return nil, fmt.Errorf("importação possui %d comando(s) não suportado(s)", len(result.Unsupported))
	}
	if err := os.MkdirAll(output, 0o755); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(output)
	if err != nil {
		return nil, err
	}
	if len(entries) > 0 {
		return nil, fmt.Errorf("pasta de destino não está vazia: %s", output)
	}
	aliases := map[string]string{}
	for _, table := range result.Tables {
		aliases[aliasName(table.Name)] = table.Name
	}
	if err := migrationgo.WriteCoreCatalog(projectRoot, aliases); err != nil {
		return nil, err
	}
	stamp := time.Date(2026, 7, 31, 20, 0, 0, 0, time.Local)
	var files []string
	write := func(description, body string, importsAlias bool) error {
		id := stamp.Format("2006_01_02_150405")
		stamp = stamp.Add(time.Second)
		function := "Migration" + strings.ReplaceAll(id, "_", "") + pascal(description)
		var source strings.Builder
		source.WriteString("// Code generated by Flexberry SQL importer. DO NOT EDIT.\npackage migrations\n\n")
		source.WriteString("import migrate \"github.com/PhelipeViana/flexberry/migration\"\n")
		if importsAlias {
			fmt.Fprintf(&source, "import alias %q\n", modulePath+"/internal/flexberry/core/migration/alias")
		}
		fmt.Fprintf(&source, "\nfunc %s() migrate.Definition {\n\treturn %s\n}\n", function, body)
		formatted, err := format.Source([]byte(source.String()))
		if err != nil {
			return fmt.Errorf("formatar %s: %w\n%s", description, err, source.String())
		}
		path := filepath.Join(output, id+"_"+description+"_migration.go")
		if err := os.WriteFile(path, formatted, 0o644); err != nil {
			return err
		}
		files = append(files, path)
		return nil
	}
	primary := map[string]Constraint{}
	uniqueColumn := map[string]bool{}
	for _, constraint := range result.Constraints {
		if constraint.Kind == "primary" {
			primary[constraint.Table] = constraint
		}
		if constraint.Kind == "unique" && len(constraint.Columns) == 1 {
			uniqueColumn[constraint.Table+":"+constraint.Columns[0]] = true
		}
	}
	for _, table := range result.Tables {
		pk := primary[table.Name]
		var body strings.Builder
		fmt.Fprintf(&body, "migrate.CreateTable(%q,\n", table.Name)
		for _, column := range table.Columns {
			copyColumn := *column
			if len(pk.Columns) == 1 && pk.Columns[0] == column.Name {
				copyColumn.PrimaryKey = true
			}
			if uniqueColumn[table.Name+":"+column.Name] && !copyColumn.PrimaryKey {
				copyColumn.Unique = true
			}
			fmt.Fprintf(&body, "\t\t%s,\n", renderColumn(copyColumn))
		}
		fmt.Fprintf(&body, "\t).Alias(%q)", aliasName(table.Name))
		if err := write("create_"+table.Name, body.String(), false); err != nil {
			return nil, err
		}
	}
	for _, constraint := range result.Constraints {
		if constraint.Kind == "primary" && len(constraint.Columns) == 1 {
			continue
		}
		if constraint.Kind == "unique" && len(constraint.Columns) == 1 {
			continue
		}
		alias := "alias." + exported(aliasName(constraint.Table))
		var body, description string
		switch constraint.Kind {
		case "primary":
			body = fmt.Sprintf("migrate.AddPrimaryKey(%s, %q%s)", alias, constraint.Name, renderStringArgs(constraint.Columns))
			description = "add_primary_key_" + constraint.Table
		case "unique":
			body = fmt.Sprintf("migrate.AddUnique(%s, %q%s)", alias, constraint.Name, renderStringArgs(constraint.Columns))
			description = "add_unique_" + constraint.Name
		case "check":
			body = fmt.Sprintf("migrate.AddCheck(%s, %q, %q)", alias, constraint.Name, constraint.Expression)
			description = "add_check_" + constraint.Name
		case "foreign":
			if len(constraint.Columns) == 1 && len(constraint.ReferenceColumns) == 1 {
				column := fmt.Sprintf("migrate.Col(%q).References(%q, %q).Constraint(%q)", constraint.Columns[0], constraint.ReferenceTable, constraint.ReferenceColumns[0], constraint.Name)
				if constraint.OnDelete == "CASCADE" {
					column += ".OnDeleteCascade()"
				}
				body = fmt.Sprintf("migrate.AddForeignKey(%s, %s)", alias, column)
			} else {
				mappings := make([]string, len(constraint.Columns))
				for index := range constraint.Columns {
					mappings[index] = constraint.Columns[index] + ":" + constraint.ReferenceColumns[index]
				}
				body = fmt.Sprintf("migrate.AddCompositeForeignKey(%s, %q, %q%s)", alias, constraint.Name, constraint.ReferenceTable, renderStringArgs(mappings))
			}
			description = "add_foreign_key_" + constraint.Name
		}
		if body != "" {
			if err := write(description, body, true); err != nil {
				return nil, err
			}
		}
	}
	for _, index := range result.Indexes {
		method := "CreateIndex"
		if index.Unique {
			method = "CreateUniqueIndex"
		}
		body := fmt.Sprintf("migrate.%s(alias.%s, %q%s)", method, exported(aliasName(index.Table)), index.Name, renderStringArgs(index.Columns))
		if err := write("create_index_"+index.Name, body, true); err != nil {
			return nil, err
		}
	}
	for _, view := range result.Views {
		body := fmt.Sprintf("migrate.CreateView(%q, %q)", view.Name, normalizeViewQuery(view.Query))
		if err := write("create_view_"+view.Name, body, false); err != nil {
			return nil, err
		}
	}
	return files, nil
}

// ClearGeneratedBaseline removes only files carrying this importer's generated
// header. It refuses mixed folders so manually maintained migrations are safe.
func ClearGeneratedBaseline(output string) error {
	entries, err := os.ReadDir(output)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	const header = "// Code generated by Flexberry SQL importer. DO NOT EDIT."
	for _, entry := range entries {
		if entry.IsDir() {
			return fmt.Errorf("pasta de baseline contém subpasta inesperada: %s", entry.Name())
		}
		path := filepath.Join(output, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !strings.HasPrefix(string(data), header) {
			return fmt.Errorf("arquivo não gerado preservado; limpeza recusada: %s", entry.Name())
		}
	}
	for _, entry := range entries {
		if err := os.Remove(filepath.Join(output, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func renderColumn(column Column) string {
	value := fmt.Sprintf("migrate.Col(%q)", column.Name)
	switch column.Kind {
	case "int":
		value += ".Int()"
	case "integer":
		value += ".BigInteger()"
	case "string":
		value += fmt.Sprintf(".Varchar(%d)", column.Length)
	case "char":
		value += fmt.Sprintf(".Char(%d)", column.Length)
	case "text":
		value += ".Text()"
	case "timestamp":
		value += ".Timestamp()"
	case "binary":
		value += ".Binary()"
	case "decimal":
		value += fmt.Sprintf(".Decimal(%d, %d)", column.Precision, column.Scale)
	}
	if column.Nullable {
		value += ".Nullable()"
	}
	if column.PrimaryKey {
		value += ".PrimaryKey()"
	}
	if column.Unique {
		value += ".Unique()"
	}
	if column.AutoIncrement {
		value += ".AutoIncrement()"
	}
	if column.Default != "" {
		if column.DefaultRaw {
			value += fmt.Sprintf(".DefaultExpr(%q)", column.Default)
		} else {
			value += fmt.Sprintf(".Default(%q)", column.Default)
		}
	}
	return value
}

func defaultValue(tail string) (string, bool) {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?is)\bDEFAULT\s+(nextval\s*\([^)]*\))`),
		regexp.MustCompile(`(?is)\bDEFAULT\s+('(?:''|[^'])*')`),
		regexp.MustCompile(`(?is)\bDEFAULT\s+([^\s,]+)`),
	}
	for _, pattern := range patterns {
		if match := pattern.FindStringSubmatch(tail); len(match) > 0 {
			return strings.TrimSpace(match[1]), true
		}
	}
	return "", false
}

func setDefault(column *Column, value string) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(strings.ToLower(value), "nextval(") {
		column.AutoIncrement, column.Default, column.DefaultRaw = true, "", false
		return
	}
	if len(value) >= 2 && value[0] == '\'' && value[len(value)-1] == '\'' {
		column.Default = strings.ReplaceAll(value[1:len(value)-1], "''", "'")
		column.DefaultRaw = false
		return
	}
	if regexp.MustCompile(`^-?\d+(?:\.\d+)?$`).MatchString(value) {
		column.Default, column.DefaultRaw = value, false
		return
	}
	column.Default, column.DefaultRaw = strings.TrimSpace(value), true
}

func namesInFirstParentheses(value string) []string { return parseNames(insideFirstParentheses(value)) }
func insideFirstParentheses(value string) string {
	start := strings.Index(value, "(")
	if start < 0 {
		return ""
	}
	depth, inString := 0, false
	for i := start; i < len(value); i++ {
		c := value[i]
		if c == '\'' {
			inString = !inString
		}
		if inString {
			continue
		}
		if c == '(' {
			depth++
		}
		if c == ')' {
			depth--
			if depth == 0 {
				return strings.TrimSpace(value[start+1 : i])
			}
		}
	}
	return ""
}
func parseNames(value string) []string {
	parts := splitTopLevel(value, ',')
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		result = append(result, normalizeName(strings.TrimSpace(part)))
	}
	return result
}
func normalizeName(value string) string {
	return strings.ToLower(strings.Trim(strings.TrimSpace(value), `"`))
}
func constraintKey(table, name string) string { return table + ":" + name }
func firstNumber(value string, fallback int) int {
	values := allNumbers(value)
	if len(values) == 0 {
		return fallback
	}
	return values[0]
}
func allNumbers(value string) []int {
	matches := regexp.MustCompile(`\d+`).FindAllString(value, -1)
	result := make([]int, 0, len(matches))
	for _, match := range matches {
		number, _ := strconv.Atoi(match)
		result = append(result, number)
	}
	return result
}
func preview(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) > 140 {
		return value[:140] + "..."
	}
	return value
}
func aliasName(value string) string {
	parts := strings.Split(value, "_")
	result := parts[0]
	for _, part := range parts[1:] {
		if part != "" {
			result += strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return result
}
func exported(value string) string {
	if value == "" {
		return "Value"
	}
	return strings.ToUpper(value[:1]) + value[1:]
}
func pascal(value string) string {
	var result strings.Builder
	for _, part := range strings.Split(value, "_") {
		if part != "" {
			result.WriteString(strings.ToUpper(part[:1]))
			result.WriteString(part[1:])
		}
	}
	return result.String()
}
func renderStringArgs(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return ", " + quotedList(values)
}
func quotedList(values []string) string {
	result := make([]string, len(values))
	for i, value := range values {
		result[i] = strconv.Quote(value)
	}
	return strings.Join(result, ", ")
}
func normalizeViewQuery(value string) string {
	return regexp.MustCompile(`(?i)\bCURRENT_TIMESTAMP\b`).ReplaceAllString(strings.TrimSpace(value), "CURRENT_TIMESTAMP")
}
