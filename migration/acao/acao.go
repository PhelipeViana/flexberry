// Package acao defines the declarative DSL used by Flexberry migrations.
package acao

import (
	"fmt"
	"regexp"
)

var physicalNamePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:_[a-z0-9]+)*$`)

type Tipo string

const (
	CreateTable    Tipo = "create_table"
	DropTable      Tipo = "drop_table"
	AddColumn      Tipo = "add_column"
	AlterColumn    Tipo = "alter_column"
	DropColumn     Tipo = "drop_column"
	AddForeignKey  Tipo = "add_foreign_key"
	DropForeignKey Tipo = "drop_foreign_key"
	CreateIndex    Tipo = "create_index"
	DropIndex      Tipo = "drop_index"
	CreateView     Tipo = "create_view"
	DropView       Tipo = "drop_view"
	CreateSequence Tipo = "create_sequence"
	DropSequence   Tipo = "drop_sequence"
	RenameTable    Tipo = "rename_table"
	RenameColumn   Tipo = "rename_column"
	AddPrimaryKey  Tipo = "add_primary_key"
	AddUnique      Tipo = "add_unique"
	AddCheck       Tipo = "add_check"
	DropConstraint Tipo = "drop_constraint"
	RawSQL         Tipo = "raw_sql"
	Todo           Tipo = "todo"
)

type ColunaDefinicao struct {
	Name            string `json:"name"`
	Type            string `json:"type,omitempty"`
	Nullable        bool   `json:"nullable"`
	PrimaryKey      bool   `json:"primary_key,omitempty"`
	AutoIncrement   bool   `json:"auto_increment,omitempty"`
	Length          int    `json:"length,omitempty"`
	Precision       int    `json:"precision,omitempty"`
	Scale           int    `json:"scale,omitempty"`
	Unique          bool   `json:"unique,omitempty"`
	Index           bool   `json:"index,omitempty"`
	Default         string `json:"default,omitempty"`
	ReferenceTable  string `json:"reference_table,omitempty"`
	ReferenceColumn string `json:"reference_column,omitempty"`
	ConstraintName  string `json:"constraint_name,omitempty"`
	OnDelete        string `json:"on_delete,omitempty"`
	DefaultRaw      bool   `json:"default_raw,omitempty"`
}

type ForeignKey struct {
	Column           string   `json:"column"`
	Columns          []string `json:"columns,omitempty"`
	ReferenceTable   string   `json:"reference_table"`
	ReferenceColumn  string   `json:"reference_column"`
	ReferenceColumns []string `json:"reference_columns,omitempty"`
	ConstraintName   string   `json:"constraint_name,omitempty"`
	OnDelete         string   `json:"on_delete,omitempty"`
}

type Operacao struct {
	Kind         string            `json:"kind"`
	Table        string            `json:"table"`
	AliasName    string            `json:"alias,omitempty"`
	Columns      []ColunaDefinicao `json:"columns,omitempty"`
	Column       *ColunaDefinicao  `json:"column,omitempty"`
	ForeignKey   *ForeignKey       `json:"foreign_key,omitempty"`
	Name         string            `json:"name,omitempty"`
	NewName      string            `json:"new_name,omitempty"`
	IndexColumns []string          `json:"index_columns,omitempty"`
	Unique       bool              `json:"unique,omitempty"`
	SQL          string            `json:"sql,omitempty"`
	Dialect      string            `json:"dialect,omitempty"`
}

func (o Operacao) Alias(name string) Operacao { o.AliasName = name; return o }

type Item interface{ migrationItem() }

type Coluna struct{ value ColunaDefinicao }

func NovaColuna(nome string) Coluna { return Coluna{value: ColunaDefinicao{Name: nome}} }
func (c Coluna) migrationItem()     {}
func (c Coluna) Integer() Coluna    { c.value.Type = "integer"; return c }
func (c Coluna) BigInteger() Coluna { c.value.Type = "integer"; return c }
func (c Coluna) Int() Coluna        { c.value.Type = "int"; return c }
func (c Coluna) Varchar(length int) Coluna {
	c.value.Type, c.value.Length = "string", length
	return c
}
func (c Coluna) Char(length int) Coluna { c.value.Type, c.value.Length = "char", length; return c }
func (c Coluna) Text() Coluna           { c.value.Type = "text"; return c }
func (c Coluna) Boolean() Coluna        { c.value.Type = "boolean"; return c }
func (c Coluna) Decimal(precision, scale int) Coluna {
	c.value.Type, c.value.Precision, c.value.Scale = "decimal", precision, scale
	return c
}
func (c Coluna) DateTime() Coluna            { c.value.Type = "datetime"; return c }
func (c Coluna) Timestamp() Coluna           { c.value.Type = "timestamp"; return c }
func (c Coluna) Binary() Coluna              { c.value.Type = "binary"; return c }
func (c Coluna) PrimaryKey() Coluna          { c.value.PrimaryKey = true; return c }
func (c Coluna) AutoIncrement() Coluna       { c.value.AutoIncrement = true; return c }
func (c Coluna) Nullable() Coluna            { c.value.Nullable = true; return c }
func (c Coluna) Unique() Coluna              { c.value.Unique = true; return c }
func (c Coluna) Index() Coluna               { c.value.Index = true; return c }
func (c Coluna) Default(value string) Coluna { c.value.Default = value; return c }
func (c Coluna) DefaultExpr(value string) Coluna {
	c.value.Default, c.value.DefaultRaw = value, true
	return c
}
func (c Coluna) References(tabela, coluna string) Coluna {
	c.value.ReferenceTable, c.value.ReferenceColumn = tabela, coluna
	return c
}
func (c Coluna) Constraint(name string) Coluna { c.value.ConstraintName = name; return c }
func (c Coluna) OnDeleteCascade() Coluna       { c.value.OnDelete = "CASCADE"; return c }
func (c Coluna) Definition() ColunaDefinicao   { return c.value }

func Nova(tipo Tipo, tabela string, itens ...Item) Operacao {
	columns := make([]ColunaDefinicao, 0, len(itens))
	for _, item := range itens {
		if column, ok := item.(Coluna); ok {
			columns = append(columns, column.Definition())
		}
	}
	result := Operacao{Kind: string(tipo), Table: tabela}
	switch tipo {
	case CreateTable:
		result.Columns = columns
	case AddColumn, AlterColumn, DropColumn:
		if len(columns) == 1 {
			result.Column = &columns[0]
		}
	case AddForeignKey, DropForeignKey:
		if len(columns) == 1 {
			result.ForeignKey = &ForeignKey{
				Column: columns[0].Name, ReferenceTable: columns[0].ReferenceTable,
				ReferenceColumn: columns[0].ReferenceColumn, ConstraintName: columns[0].ConstraintName,
				OnDelete: columns[0].OnDelete,
			}
		}
	case CreateIndex, DropIndex, CreateView, DropView, CreateSequence, DropSequence, RenameTable,
		RenameColumn, AddPrimaryKey, AddUnique, AddCheck, DropConstraint, RawSQL:
		// These operations are assembled by the public migration package.
	}
	return result
}

func Validar(operation Operacao) error {
	switch Tipo(operation.Kind) {
	case CreateTable:
		if !nomeFisicoValido(operation.Table) {
			return fmt.Errorf("nome de tabela %q inválido; use snake_case, começando por letra minúscula e sem espaços ou símbolos", operation.Table)
		}
		if operation.AliasName == "" {
			return fmt.Errorf("CreateTable exige .Alias(\"apelido\")")
		}
		if !aliasValido(operation.AliasName) {
			return fmt.Errorf("alias %q inválido; use lowerCamelCase, começando por letra minúscula e sem espaços ou símbolos", operation.AliasName)
		}
		if len(operation.Columns) == 0 {
			return fmt.Errorf("CreateTable exige ao menos uma coluna")
		}
		for _, column := range operation.Columns {
			if column.Name == "" || column.Type == "" {
				return fmt.Errorf("CreateTable exige nome e tipo em todas as colunas")
			}
			if !nomeFisicoValido(column.Name) {
				return nomeColunaInvalido(column.Name)
			}
		}
	case DropTable:
		if operation.Column != nil || len(operation.Columns) > 0 {
			return fmt.Errorf("DropTable não aceita colunas")
		}
	case AddColumn, AlterColumn:
		if operation.Column == nil || operation.Column.Name == "" || operation.Column.Type == "" {
			return fmt.Errorf("%s exige exatamente uma coluna com tipo", operation.Kind)
		}
		if !nomeFisicoValido(operation.Column.Name) {
			return nomeColunaInvalido(operation.Column.Name)
		}
	case DropColumn:
		if operation.Column == nil || operation.Column.Name == "" {
			return fmt.Errorf("DropColumn exige exatamente uma coluna")
		}
		if !nomeFisicoValido(operation.Column.Name) {
			return nomeColunaInvalido(operation.Column.Name)
		}
	case AddForeignKey, DropForeignKey:
		if operation.ForeignKey == nil || operation.ForeignKey.ReferenceTable == "" {
			return fmt.Errorf("%s exige uma coluna com References", operation.Kind)
		}
		columns, references := operation.ForeignKey.Columns, operation.ForeignKey.ReferenceColumns
		if len(columns) == 0 && operation.ForeignKey.Column != "" {
			columns = []string{operation.ForeignKey.Column}
		}
		if len(references) == 0 && operation.ForeignKey.ReferenceColumn != "" {
			references = []string{operation.ForeignKey.ReferenceColumn}
		}
		if len(columns) == 0 || len(columns) != len(references) {
			return fmt.Errorf("%s exige a mesma quantidade de colunas locais e referenciadas", operation.Kind)
		}
		for _, column := range columns {
			if !nomeFisicoValido(column) {
				return nomeColunaInvalido(column)
			}
		}
		if !nomeFisicoValido(operation.ForeignKey.ReferenceTable) {
			return fmt.Errorf("nome de tabela referenciada %q inválido; use snake_case, sem espaços", operation.ForeignKey.ReferenceTable)
		}
		for _, column := range references {
			if !nomeFisicoValido(column) {
				return fmt.Errorf("nome de coluna referenciada %q inválido; use snake_case, sem espaços", column)
			}
		}
	case CreateIndex:
		if operation.Name == "" || len(operation.IndexColumns) == 0 {
			return fmt.Errorf("CreateIndex exige nome e colunas")
		}
		if !nomeFisicoValido(operation.Name) {
			return fmt.Errorf("nome de índice %q inválido; use snake_case, sem espaços", operation.Name)
		}
		for _, column := range operation.IndexColumns {
			if !nomeFisicoValido(column) {
				return nomeColunaInvalido(column)
			}
		}
	case DropIndex, DropView, DropSequence:
		if operation.Name == "" {
			return fmt.Errorf("%s exige um nome", operation.Kind)
		}
	case CreateView:
		if operation.Name == "" || operation.SQL == "" {
			return fmt.Errorf("CreateView exige nome e consulta")
		}
	case CreateSequence:
		if operation.Name == "" {
			return fmt.Errorf("CreateSequence exige nome")
		}
	case RenameTable:
		if operation.NewName == "" {
			return fmt.Errorf("RenameTable exige o novo nome")
		}
		if !nomeFisicoValido(operation.NewName) {
			return fmt.Errorf("novo nome de tabela %q inválido; use snake_case, começando por letra minúscula e sem espaços ou símbolos", operation.NewName)
		}
	case RenameColumn:
		if operation.Column == nil || !nomeFisicoValido(operation.Column.Name) || !nomeFisicoValido(operation.NewName) {
			return fmt.Errorf("RenameColumn exige nomes de coluna válidos em snake_case")
		}
	case AddPrimaryKey, AddUnique:
		if !nomeFisicoValido(operation.Name) || len(operation.IndexColumns) == 0 {
			return fmt.Errorf("%s exige nome de constraint e colunas em snake_case", operation.Kind)
		}
		for _, column := range operation.IndexColumns {
			if !nomeFisicoValido(column) {
				return nomeColunaInvalido(column)
			}
		}
	case AddCheck:
		if !nomeFisicoValido(operation.Name) || operation.SQL == "" {
			return fmt.Errorf("AddCheck exige nome da constraint em snake_case e expressão")
		}
	case DropConstraint:
		if !nomeFisicoValido(operation.Name) {
			return fmt.Errorf("DropConstraint exige nome em snake_case")
		}
	case RawSQL:
		if operation.SQL == "" {
			return fmt.Errorf("SQL exige conteúdo")
		}
	case Todo:
		return fmt.Errorf("migration ainda não foi preenchida; substitua migrate.TODO() por uma ação")
	default:
		return fmt.Errorf("ação %q desconhecida", operation.Kind)
	}
	if exigeTabela(Tipo(operation.Kind)) && operation.Table == "" {
		return fmt.Errorf("a tabela é obrigatória")
	}
	return nil
}

func exigeTabela(tipo Tipo) bool {
	switch tipo {
	case CreateTable, DropTable, AddColumn, AlterColumn, DropColumn, AddForeignKey, DropForeignKey,
		CreateIndex, DropIndex, RenameTable, RenameColumn, AddPrimaryKey, AddUnique, AddCheck, DropConstraint:
		return true
	default:
		return false
	}
}

func nomeFisicoValido(name string) bool {
	return physicalNamePattern.MatchString(name)
}

func nomeColunaInvalido(name string) error {
	return fmt.Errorf("nome de coluna %q inválido; use snake_case, começando por letra minúscula e sem espaços ou símbolos", name)
}

func aliasValido(alias string) bool {
	for index, char := range alias {
		if index == 0 {
			if char < 'a' || char > 'z' {
				return false
			}
			continue
		}
		if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9') {
			return false
		}
	}
	return alias != ""
}
