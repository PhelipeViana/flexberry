package migrategen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PhelipeViana/flexberry/internal/config"
	"github.com/PhelipeViana/flexberry/internal/scanner"
)

func TestWriteEntityDocumentationReplaysMigrationHistory(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/app\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "migrations")
	if err := os.MkdirAll(output, 0o755); err != nil {
		t.Fatal(err)
	}
	writeMigrationTestFile(t, output, "2026_01_01_100000.go", `package migrations
import migrate "github.com/PhelipeViana/flexberry/migration"
func Definition() migrate.Definition { return migrate.Define(
 migrate.CreateTable("leis",
  migrate.Col("id").Integer().PrimaryKey().AutoIncrement(),
  migrate.Col("titulo").Varchar(100),
 ).Alias("lei"),
) }
`)
	writeMigrationTestFile(t, output, "2026_01_01_100001.go", `package migrations
import migrate "github.com/PhelipeViana/flexberry/migration"
func Definition() migrate.Definition { return migrate.Define(
 migrate.AddColumn(alias.leis, migrate.Col("publicado_em").DateTime().Nullable()),
) }
`)
	// The parser resolves table aliases through the generated catalog.
	if err := os.WriteFile(filepath.Join(output, "dsl.gen.go"), []byte("package migrations\nvar leis = migrate.Table(\"leis\")\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path, count, err := WriteEntityDocumentation(root, config.MigrateConfig{Output: config.MigrateOutput{Path: "migrations"}})
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("count = %d", count)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, expected := range []string{"type Lei struct", "ID", "migrate:\"primaryKey,autoIncrement\"", "*time.Time"} {
		if !strings.Contains(text, expected) {
			t.Errorf("documentation does not contain %q:\n%s", expected, text)
		}
	}
	if strings.Contains(text, "// primary key") {
		t.Errorf("struct documentation must not contain migration modifiers:\n%s", text)
	}
}

func TestRenderEntityDocumentationPreservesDomainNames(t *testing.T) {
	tables := map[string]*Table{"cidades": {Name: "cidades", Columns: []Column{{Name: "id", Type: "integer"}, {Name: "nome", Type: "string"}, {Name: "id_status", Type: "integer"}}}}
	entities := map[string]scanner.Entity{"cidades": {Name: "Cidade", Table: "cidades", Fields: []scanner.Field{{Name: "ID", Column: "id", GoType: "int64"}, {Name: "NomeCidade", Column: "nome", GoType: "string"}, {Name: "IDStatus", Column: "id_status", GoType: "int64"}}, Relations: []scanner.Relation{{Name: "Status", Type: "StatusCidade"}}}}
	text := renderEntityDocumentation(tables, nil, entities)
	for _, expected := range []string{"type Cidade struct", "NomeCidade", "`db:\"nome\" json:\"nome\"`", "StatusCidade", "`json:\"status\"`"} {
		if !strings.Contains(text, expected) {
			t.Errorf("documentation does not contain %q:\n%s", expected, text)
		}
	}
}

func writeMigrationTestFile(t *testing.T, dir, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
