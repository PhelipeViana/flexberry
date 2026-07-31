// Package migrate is the public, autocomplete-friendly migration DSL.
package migrate

import (
	"strings"

	"github.com/PhelipeViana/flexberry/migration/acao"
)

type Table string
type Column = acao.Coluna
type Operation = acao.Operacao
type Definition = Operation

// Define mantém compatibilidade com migrations antigas.
func Define(operations ...Operation) Definition {
	if len(operations) == 1 {
		return operations[0]
	}
	return TODO()
}

func TODO() Definition { return Operation{Kind: string(acao.Todo)} }

func Col(name string) Column { return acao.NovaColuna(name) }

func CreateTable(name string, columns ...Column) Operation {
	return withColumns(acao.CreateTable, Table(name), columns)
}
func DropTable(table Table) Operation { return acao.Nova(acao.DropTable, string(table)) }
func AddColumn(table Table, column Column) Operation {
	return withColumns(acao.AddColumn, table, []Column{column})
}
func AlterColumn(table Table, column Column) Operation {
	return withColumns(acao.AlterColumn, table, []Column{column})
}
func DropColumn(table Table, column Column) Operation {
	return withColumns(acao.DropColumn, table, []Column{column})
}
func AddForeignKey(table Table, column Column) Operation {
	return withColumns(acao.AddForeignKey, table, []Column{column})
}
func AddCompositeForeignKey(table Table, name, referenceTable string, mappings ...string) Operation {
	foreignKey := &acao.ForeignKey{ConstraintName: name, ReferenceTable: referenceTable}
	for _, mapping := range mappings {
		parts := strings.SplitN(mapping, ":", 2)
		if len(parts) == 2 {
			foreignKey.Columns = append(foreignKey.Columns, parts[0])
			foreignKey.ReferenceColumns = append(foreignKey.ReferenceColumns, parts[1])
		}
	}
	return Operation{Kind: string(acao.AddForeignKey), Table: string(table), ForeignKey: foreignKey}
}
func DropForeignKey(table Table, column Column) Operation {
	return withColumns(acao.DropForeignKey, table, []Column{column})
}

func CreateIndex(table Table, name string, columns ...string) Operation {
	return Operation{Kind: string(acao.CreateIndex), Table: string(table), Name: name, IndexColumns: columns}
}
func CreateUniqueIndex(table Table, name string, columns ...string) Operation {
	op := CreateIndex(table, name, columns...)
	op.Unique = true
	return op
}
func DropIndex(table Table, name string) Operation {
	return Operation{Kind: string(acao.DropIndex), Table: string(table), Name: name}
}
func CreateView(name, query string) Operation {
	return Operation{Kind: string(acao.CreateView), Name: name, SQL: query}
}
func DropView(name string) Operation { return Operation{Kind: string(acao.DropView), Name: name} }
func CreateSequence(name string) Operation {
	return Operation{Kind: string(acao.CreateSequence), Name: name}
}
func DropSequence(name string) Operation {
	return Operation{Kind: string(acao.DropSequence), Name: name}
}
func RenameTable(table Table, newName string) Operation {
	return Operation{Kind: string(acao.RenameTable), Table: string(table), NewName: newName}
}
func RenameColumn(table Table, oldName, newName string) Operation {
	return Operation{Kind: string(acao.RenameColumn), Table: string(table), Column: &acao.ColunaDefinicao{Name: oldName}, NewName: newName}
}
func AddPrimaryKey(table Table, name string, columns ...string) Operation {
	return Operation{Kind: string(acao.AddPrimaryKey), Table: string(table), Name: name, IndexColumns: columns}
}
func AddUnique(table Table, name string, columns ...string) Operation {
	return Operation{Kind: string(acao.AddUnique), Table: string(table), Name: name, IndexColumns: columns}
}
func AddCheck(table Table, name, expression string) Operation {
	return Operation{Kind: string(acao.AddCheck), Table: string(table), Name: name, SQL: expression}
}
func DropConstraint(table Table, name string) Operation {
	return Operation{Kind: string(acao.DropConstraint), Table: string(table), Name: name}
}
func SQL(dialect, statement string) Operation {
	return Operation{Kind: string(acao.RawSQL), Dialect: dialect, SQL: statement}
}

func withColumns(kind acao.Tipo, table Table, columns []Column) Operation {
	items := make([]acao.Item, len(columns))
	for i := range columns {
		items[i] = columns[i]
	}
	return acao.Nova(kind, string(table), items...)
}
