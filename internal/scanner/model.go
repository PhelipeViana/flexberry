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
	Name       string
	Column     string
	GoType     string
	Nullable   bool
	PrimaryKey bool
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
