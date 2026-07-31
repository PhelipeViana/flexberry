package migrationgo

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/PhelipeViana/flexberry/migration/acao"
)

var tableEntry = regexp.MustCompile(`(?m)^\s*(?:var\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*(?::|=)\s*migrate\.Table\("([^"]+)"\),?`)

func ParseFile(path string) ([]acao.Operacao, error) {
	catalog := loadCatalog(filepath.Join(filepath.Dir(path), "dsl.gen.go"))
	if root := projectRoot(filepath.Dir(path)); root != "" {
		core := loadCatalog(filepath.Join(root, "internal", "flexberry", "core", "migration", "alias", "dsl.gen.go"))
		legacyCore := loadCatalog(filepath.Join(root, "internal", "flexberry", "core", "migration", "table", "dsl.gen.go"))
		for name, alias := range legacyCore {
			catalog[name] = alias
		}
		for name, alias := range core {
			catalog[name] = alias
		}
	}
	set := token.NewFileSet()
	file, err := parser.ParseFile(set, path, nil, 0)
	if err != nil {
		return nil, err
	}
	var operations []acao.Operacao
	for _, declaration := range file.Decls {
		if function, ok := declaration.(*ast.FuncDecl); ok {
			found, err := evalDefinitionFunction(set, function, catalog)
			if err != nil {
				return nil, err
			}
			operations = append(operations, found...)
			continue
		}
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.VAR {
			continue
		}
		for _, spec := range general.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, expression := range value.Values {
				literal, ok := expression.(*ast.CompositeLit)
				if !ok {
					continue
				}
				for _, element := range literal.Elts {
					operation, err := evalOperation(element, catalog)
					if err != nil {
						return nil, positionError(set, element, err)
					}
					operations = append(operations, operation)
				}
			}
		}
	}
	if len(operations) == 0 {
		return nil, fmt.Errorf("nenhuma operação migrate.* encontrada")
	}
	for index := range operations {
		if operations[index].Kind == string(acao.CreateTable) && operations[index].AliasName == "" {
			if strings.HasSuffix(filepath.Base(path), "_migration.go") {
				// Compatibilidade: migrations finalizadas antes da exigência do alias.
				operations[index].AliasName = tableIdentifier(operations[index].Table)
			} else {
				return nil, fmt.Errorf("CreateTable exige .Alias(\"apelido\")")
			}
		}
	}
	return operations, nil
}

func projectRoot(path string) string {
	current, _ := filepath.Abs(path)
	for {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		current = parent
	}
}

func evalDefinitionFunction(set *token.FileSet, function *ast.FuncDecl, catalog map[string]string) ([]acao.Operacao, error) {
	if function.Body == nil || function.Name == nil || len(function.Body.List) != 1 {
		return nil, nil
	}
	statement, ok := function.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(statement.Results) != 1 {
		return nil, nil
	}
	call, ok := statement.Results[0].(*ast.CallExpr)
	if !ok {
		return nil, nil
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if ok && identName(selector.X) == "migrate" && selector.Sel.Name == "Define" {
		if len(call.Args) != 1 {
			return nil, positionError(set, call, fmt.Errorf("cada migration deve conter exatamente uma ação"))
		}
		operation, err := evalOperation(call.Args[0], catalog)
		if err != nil {
			return nil, positionError(set, call.Args[0], err)
		}
		return []acao.Operacao{operation}, nil
	}
	operation, err := evalOperation(statement.Results[0], catalog)
	if err != nil {
		return nil, positionError(set, statement.Results[0], err)
	}
	return []acao.Operacao{operation}, nil
}

func loadCatalog(path string) map[string]string {
	result := map[string]string{}
	data, err := os.ReadFile(path)
	if err != nil {
		return result
	}
	for _, match := range tableEntry.FindAllStringSubmatch(string(data), -1) {
		result[match[1]] = match[2]
	}
	return result
}

func evalOperation(expression ast.Expr, catalog map[string]string) (acao.Operacao, error) {
	call, ok := expression.(*ast.CallExpr)
	if !ok {
		return acao.Operacao{}, fmt.Errorf("esperado migrate.Metodo(...)")
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if ok && selector.Sel.Name == "Alias" {
		if len(call.Args) != 1 {
			return acao.Operacao{}, fmt.Errorf("Alias exige exatamente um apelido")
		}
		operation, err := evalOperation(selector.X, catalog)
		if err != nil {
			return acao.Operacao{}, err
		}
		if operation.Kind != string(acao.CreateTable) {
			return acao.Operacao{}, fmt.Errorf("Alias só pode ser usado em CreateTable")
		}
		operation.AliasName, err = stringLiteral(call.Args[0])
		if err == nil {
			err = acao.Validar(operation)
		}
		return operation, err
	}
	if !ok || identName(selector.X) != "migrate" {
		return evalLegacyOperation(expression)
	}
	method := selector.Sel.Name
	if method == "TODO" {
		if err := expectArgs(call, 0); err != nil {
			return acao.Operacao{}, err
		}
		return acao.Operacao{Kind: string(acao.Todo)}, nil
	}
	if method == "CreateTable" {
		if len(call.Args) < 2 {
			return acao.Operacao{}, fmt.Errorf("CreateTable exige nome e colunas")
		}
		table, err := stringLiteral(call.Args[0])
		if err != nil {
			return acao.Operacao{}, err
		}
		return columnOperation(acao.CreateTable, table, call.Args[1:])
	}
	if method == "CreateView" {
		if err := expectArgs(call, 2); err != nil {
			return acao.Operacao{}, err
		}
		name, err := stringLiteral(call.Args[0])
		if err != nil {
			return acao.Operacao{}, err
		}
		query, err := stringLiteral(call.Args[1])
		if err != nil {
			return acao.Operacao{}, err
		}
		return acao.Operacao{Kind: string(acao.CreateView), Name: name, SQL: query}, nil
	}
	if method == "DropView" || method == "CreateSequence" || method == "DropSequence" {
		if err := expectArgs(call, 1); err != nil {
			return acao.Operacao{}, err
		}
		name, err := stringLiteral(call.Args[0])
		if err != nil {
			return acao.Operacao{}, err
		}
		kind := map[string]acao.Tipo{"DropView": acao.DropView, "CreateSequence": acao.CreateSequence, "DropSequence": acao.DropSequence}[method]
		return acao.Operacao{Kind: string(kind), Name: name}, nil
	}
	if method == "SQL" {
		if err := expectArgs(call, 2); err != nil {
			return acao.Operacao{}, err
		}
		dialect, err := stringLiteral(call.Args[0])
		if err != nil {
			return acao.Operacao{}, err
		}
		statement, err := stringLiteral(call.Args[1])
		if err != nil {
			return acao.Operacao{}, err
		}
		return acao.Operacao{Kind: string(acao.RawSQL), Dialect: dialect, SQL: statement}, nil
	}
	if len(call.Args) == 0 {
		return acao.Operacao{}, fmt.Errorf("%s exige alias.*", method)
	}
	table, err := tableReference(call.Args[0], catalog)
	if err != nil {
		return acao.Operacao{}, err
	}
	switch method {
	case "DropTable":
		return acao.Nova(acao.DropTable, table), nil
	case "AddColumn":
		return columnOperation(acao.AddColumn, table, call.Args[1:])
	case "AlterColumn":
		return columnOperation(acao.AlterColumn, table, call.Args[1:])
	case "DropColumn":
		return columnOperation(acao.DropColumn, table, call.Args[1:])
	case "AddForeignKey":
		return columnOperation(acao.AddForeignKey, table, call.Args[1:])
	case "DropForeignKey":
		return columnOperation(acao.DropForeignKey, table, call.Args[1:])
	case "RenameTable":
		if len(call.Args) != 2 {
			return acao.Operacao{}, fmt.Errorf("RenameTable exige alias.* e novo nome")
		}
		name, err := stringLiteral(call.Args[1])
		return acao.Operacao{Kind: string(acao.RenameTable), Table: table, NewName: name}, err
	case "RenameColumn":
		if len(call.Args) != 3 {
			return acao.Operacao{}, fmt.Errorf("RenameColumn exige alias.*, nome atual e novo nome")
		}
		oldName, err := stringLiteral(call.Args[1])
		if err != nil {
			return acao.Operacao{}, err
		}
		newName, err := stringLiteral(call.Args[2])
		return acao.Operacao{Kind: string(acao.RenameColumn), Table: table, Column: &acao.ColunaDefinicao{Name: oldName}, NewName: newName}, err
	case "AddPrimaryKey", "AddUnique":
		if len(call.Args) < 3 {
			return acao.Operacao{}, fmt.Errorf("%s exige alias.*, nome e colunas", method)
		}
		name, err := stringLiteral(call.Args[1])
		if err != nil {
			return acao.Operacao{}, err
		}
		columns, err := stringArguments(call.Args[2:])
		kind := acao.AddPrimaryKey
		if method == "AddUnique" {
			kind = acao.AddUnique
		}
		return acao.Operacao{Kind: string(kind), Table: table, Name: name, IndexColumns: columns}, err
	case "AddCompositeForeignKey":
		if len(call.Args) < 4 {
			return acao.Operacao{}, fmt.Errorf("AddCompositeForeignKey exige alias.*, nome, tabela referenciada e mapeamentos")
		}
		name, err := stringLiteral(call.Args[1])
		if err != nil {
			return acao.Operacao{}, err
		}
		referenceTable, err := stringLiteral(call.Args[2])
		if err != nil {
			return acao.Operacao{}, err
		}
		mappings, err := stringArguments(call.Args[3:])
		if err != nil {
			return acao.Operacao{}, err
		}
		foreignKey := &acao.ForeignKey{ConstraintName: name, ReferenceTable: referenceTable}
		for _, mapping := range mappings {
			parts := strings.SplitN(mapping, ":", 2)
			if len(parts) != 2 {
				return acao.Operacao{}, fmt.Errorf("mapeamento %q deve usar coluna:referência", mapping)
			}
			foreignKey.Columns = append(foreignKey.Columns, parts[0])
			foreignKey.ReferenceColumns = append(foreignKey.ReferenceColumns, parts[1])
		}
		return acao.Operacao{Kind: string(acao.AddForeignKey), Table: table, ForeignKey: foreignKey}, nil
	case "AddCheck":
		if len(call.Args) != 3 {
			return acao.Operacao{}, fmt.Errorf("AddCheck exige alias.*, nome e expressão")
		}
		name, err := stringLiteral(call.Args[1])
		if err != nil {
			return acao.Operacao{}, err
		}
		expression, err := stringLiteral(call.Args[2])
		return acao.Operacao{Kind: string(acao.AddCheck), Table: table, Name: name, SQL: expression}, err
	case "DropConstraint":
		if len(call.Args) != 2 {
			return acao.Operacao{}, fmt.Errorf("DropConstraint exige alias.* e nome")
		}
		name, err := stringLiteral(call.Args[1])
		return acao.Operacao{Kind: string(acao.DropConstraint), Table: table, Name: name}, err
	case "CreateIndex", "CreateUniqueIndex":
		if len(call.Args) < 3 {
			return acao.Operacao{}, fmt.Errorf("%s exige alias.*, nome e colunas", method)
		}
		name, err := stringLiteral(call.Args[1])
		if err != nil {
			return acao.Operacao{}, err
		}
		columns := []string{}
		for _, arg := range call.Args[2:] {
			value, err := stringLiteral(arg)
			if err != nil {
				return acao.Operacao{}, err
			}
			columns = append(columns, value)
		}
		return acao.Operacao{Kind: string(acao.CreateIndex), Table: table, Name: name, IndexColumns: columns, Unique: method == "CreateUniqueIndex"}, nil
	case "DropIndex":
		if len(call.Args) != 2 {
			return acao.Operacao{}, fmt.Errorf("DropIndex exige alias.* e nome")
		}
		name, err := stringLiteral(call.Args[1])
		return acao.Operacao{Kind: string(acao.DropIndex), Table: table, Name: name}, err
	default:
		return acao.Operacao{}, fmt.Errorf("método migrate.%s não suportado", method)
	}
}

func columnOperation(kind acao.Tipo, table string, expressions []ast.Expr) (acao.Operacao, error) {
	items := make([]acao.Item, 0, len(expressions))
	for _, expression := range expressions {
		column, err := evalColumn(expression)
		if err != nil {
			return acao.Operacao{}, err
		}
		items = append(items, column)
	}
	op := acao.Nova(kind, table, items...)
	if kind != acao.CreateTable {
		if err := acao.Validar(op); err != nil {
			return acao.Operacao{}, err
		}
	}
	return op, nil
}

func tableReference(expression ast.Expr, catalog map[string]string) (string, error) {
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok || (identName(selector.X) != "alias" && identName(selector.X) != "table") {
		return "", fmt.Errorf("use uma referência alias.*")
	}
	name, ok := catalog[selector.Sel.Name]
	if !ok {
		return "", fmt.Errorf("referência alias.%s não existe no catálogo; execute migrate reload", selector.Sel.Name)
	}
	return name, nil
}

func evalLegacyOperation(expression ast.Expr) (acao.Operacao, error) {
	call, ok := expression.(*ast.CallExpr)
	if !ok || identName(call.Fun) != "nova" || len(call.Args) < 2 {
		return acao.Operacao{}, fmt.Errorf("esperado migrate.Metodo(...)")
	}
	selector, ok := call.Args[0].(*ast.SelectorExpr)
	if !ok || identName(selector.X) != "acao" {
		return acao.Operacao{}, fmt.Errorf("ação antiga inválida")
	}
	table, err := stringLiteral(call.Args[1])
	if err != nil {
		return acao.Operacao{}, err
	}
	kind := map[string]acao.Tipo{"CreateTable": acao.CreateTable, "DropTable": acao.DropTable, "AddColumn": acao.AddColumn, "AlterColumn": acao.AlterColumn, "DropColumn": acao.DropColumn, "AddForeignKey": acao.AddForeignKey, "DropForeignKey": acao.DropForeignKey}[selector.Sel.Name]
	return columnOperation(kind, table, call.Args[2:])
}

func evalColumn(expression ast.Expr) (acao.Coluna, error) {
	call, ok := expression.(*ast.CallExpr)
	if !ok {
		return acao.Coluna{}, fmt.Errorf("esperado col(...) ou método de coluna")
	}
	if name := identName(call.Fun); name == "col" || name == "coluna" {
		if len(call.Args) != 1 {
			return acao.Coluna{}, fmt.Errorf("col exige um nome")
		}
		value, err := stringLiteral(call.Args[0])
		return acao.NovaColuna(value), err
	}
	if selector, ok := call.Fun.(*ast.SelectorExpr); ok && identName(selector.X) == "migrate" && selector.Sel.Name == "Col" {
		if len(call.Args) != 1 {
			return acao.Coluna{}, fmt.Errorf("migrate.Col exige um nome")
		}
		value, err := stringLiteral(call.Args[0])
		return acao.NovaColuna(value), err
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return acao.Coluna{}, fmt.Errorf("método de coluna inválido")
	}
	column, err := evalColumn(selector.X)
	if err != nil {
		return column, err
	}
	switch selector.Sel.Name {
	case "Integer":
		return column.Integer(), expectArgs(call, 0)
	case "BigInteger":
		return column.BigInteger(), expectArgs(call, 0)
	case "Int":
		return column.Int(), expectArgs(call, 0)
	case "Varchar":
		size, err := intArgument(call, 0, 1)
		return column.Varchar(size), err
	case "Char":
		size, err := intArgument(call, 0, 1)
		return column.Char(size), err
	case "Text":
		return column.Text(), expectArgs(call, 0)
	case "Boolean":
		return column.Boolean(), expectArgs(call, 0)
	case "Decimal":
		p, err := intArgument(call, 0, 2)
		if err != nil {
			return column, err
		}
		s, err := intArgument(call, 1, 2)
		return column.Decimal(p, s), err
	case "DateTime":
		return column.DateTime(), expectArgs(call, 0)
	case "Timestamp":
		return column.Timestamp(), expectArgs(call, 0)
	case "Binary":
		return column.Binary(), expectArgs(call, 0)
	case "PrimaryKey":
		return column.PrimaryKey(), expectArgs(call, 0)
	case "AutoIncrement":
		return column.AutoIncrement(), expectArgs(call, 0)
	case "Nullable":
		return column.Nullable(), expectArgs(call, 0)
	case "Unique":
		return column.Unique(), expectArgs(call, 0)
	case "Index":
		return column.Index(), expectArgs(call, 0)
	case "Default":
		if err := expectArgs(call, 1); err != nil {
			return column, err
		}
		value, err := stringLiteral(call.Args[0])
		return column.Default(value), err
	case "DefaultExpr":
		if err := expectArgs(call, 1); err != nil {
			return column, err
		}
		value, err := stringLiteral(call.Args[0])
		return column.DefaultExpr(value), err
	case "References":
		if err := expectArgs(call, 2); err != nil {
			return column, err
		}
		table, err := stringLiteral(call.Args[0])
		if err != nil {
			return column, err
		}
		target, err := stringLiteral(call.Args[1])
		return column.References(table, target), err
	case "Constraint":
		if err := expectArgs(call, 1); err != nil {
			return column, err
		}
		name, err := stringLiteral(call.Args[0])
		return column.Constraint(name), err
	case "OnDeleteCascade":
		return column.OnDeleteCascade(), expectArgs(call, 0)
	default:
		return column, fmt.Errorf("método de coluna %s não suportado", selector.Sel.Name)
	}
}

func stringArguments(expressions []ast.Expr) ([]string, error) {
	values := make([]string, 0, len(expressions))
	for _, expression := range expressions {
		value, err := stringLiteral(expression)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func identName(e ast.Expr) string {
	if i, ok := e.(*ast.Ident); ok {
		return i.Name
	}
	return ""
}
func stringLiteral(e ast.Expr) (string, error) {
	v, ok := e.(*ast.BasicLit)
	if !ok || v.Kind != token.STRING {
		return "", fmt.Errorf("esperado texto entre aspas")
	}
	return strconv.Unquote(v.Value)
}
func expectArgs(c *ast.CallExpr, n int) error {
	if len(c.Args) != n {
		return fmt.Errorf("%s exige %d argumento(s)", callName(c), n)
	}
	return nil
}
func intArgument(c *ast.CallExpr, index, count int) (int, error) {
	if err := expectArgs(c, count); err != nil {
		return 0, err
	}
	v, ok := c.Args[index].(*ast.BasicLit)
	if !ok || v.Kind != token.INT {
		return 0, fmt.Errorf("%s exige número inteiro", callName(c))
	}
	return strconv.Atoi(v.Value)
}
func callName(c *ast.CallExpr) string {
	if s, ok := c.Fun.(*ast.SelectorExpr); ok {
		return s.Sel.Name
	}
	return identName(c.Fun)
}
func positionError(set *token.FileSet, node ast.Node, err error) error {
	p := set.Position(node.Pos())
	return fmt.Errorf("%s:%d: %w", p.Filename, p.Line, err)
}
