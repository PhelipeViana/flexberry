package flexberry

import (
	"strings"
	"testing"
)

type runtimePerson struct {
	ID   int64  `db:"ID"`
	Name string `db:"NAME"`
}

func runtimeDefinition() EntityDefinition[runtimePerson] {
	return Define[runtimePerson](EntityMapping{
		Name:  "runtimePerson",
		Table: "people",
		Fields: []FieldMapping{
			{Name: "ID", Column: "ID"},
			{Name: "Name", Column: "NAME"},
		},
	})
}

func TestSelectSQLByDialect(t *testing.T) {
	query := runtimeDefinition().Where("ACTIVE", true).OrderByDesc("NAME").Limit(10).Offset(20)
	statement, args := query.selectSQL("postgres")
	expected := "SELECT ID, NAME FROM people WHERE ACTIVE = $1 ORDER BY NAME DESC LIMIT 10 OFFSET 20"
	if statement != expected {
		t.Fatalf("SQL inesperado:\n%s\nesperado:\n%s", statement, expected)
	}
	if len(args) != 1 || args[0] != true {
		t.Fatalf("argumentos inesperados: %#v", args)
	}
}

func TestOraclePaginationAndPlaceholder(t *testing.T) {
	query := runtimeDefinition().Where("ID", int64(7)).Limit(1)
	statement, _ := query.selectSQL("oracle")
	if !strings.Contains(statement, "ID = :1") || !strings.HasSuffix(statement, "OFFSET 0 ROWS FETCH NEXT 1 ROWS ONLY") {
		t.Fatalf("SQL Oracle inesperado: %s", statement)
	}
}

func TestSQLServerPaginationAndPlaceholder(t *testing.T) {
	query := runtimeDefinition().Where("ID", int64(7)).Limit(1)
	statement, _ := query.selectSQL("sqlserver")
	if !strings.Contains(statement, "ID = @p1") ||
		!strings.Contains(statement, "ORDER BY (SELECT NULL)") ||
		!strings.HasSuffix(statement, "OFFSET 0 ROWS FETCH NEXT 1 ROWS ONLY") {
		t.Fatalf("SQL Server inesperado: %s", statement)
	}
}

func TestInvalidIdentifierIsRejected(t *testing.T) {
	query := runtimeDefinition().Where("ID; DROP TABLE people", 1)
	if query.err == nil {
		t.Fatal("identificador perigoso deveria ser rejeitado")
	}
}
