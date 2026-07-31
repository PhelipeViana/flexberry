package scanner

type Entity struct {
	Name       string
	Package    string
	ImportPath string
	SourceFile string
	Table      string
	PrimaryKey string
	Connection string
	Fields     []Field
	Relations  []Relation
	Function   string
	Alias      string
}

type Field struct {
	Name, Column, GoType                     string
	Nullable, PrimaryKey, AutoIncrement      bool
	Unique, Index                            bool
	Length, Precision, Scale                 int
	Default, ReferenceTable, ReferenceColumn string
}

type Relation struct {
	Name       string
	Type       string
	Kind       string
	ForeignKey string
}

type Result struct {
	Entities []Entity
	Files    []string
	Warnings []string
}
