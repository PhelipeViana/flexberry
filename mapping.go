package flexberry

// EntityDefinition connects a generated mapping to its original Go type.
type EntityDefinition[T any] struct {
	Mapping EntityMapping
}

func Define[T any](mapping EntityMapping) EntityDefinition[T] {
	return EntityDefinition[T]{Mapping: mapping}
}

type EntityMapping struct {
	Name           string
	Package        string
	ImportPath     string
	Table          string
	PrimaryKey     string
	Connection     string
	DefaultPerPage int
	MaxPerPage     int
	Fields         []FieldMapping
	Relations      []RelationMapping
}

type FieldMapping struct {
	Name       string
	Column     string
	GoType     string
	Nullable   bool
	PrimaryKey bool
}

type RelationMapping struct {
	Name       string
	Type       string
	Kind       string
	ForeignKey string
}
