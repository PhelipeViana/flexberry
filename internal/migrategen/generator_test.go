package migrategen

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/PhelipeViana/flexberry/internal/config"
	"github.com/PhelipeViana/flexberry/internal/scanner"
)

func TestGenerateCreatesInitialPlanAndThenRemainsStable(t *testing.T) {
	root := t.TempDir()
	cfg := config.MigrateConfig{Output: config.MigrateOutput{Path: "migrations"}}
	entities := []scanner.Entity{{
		Name: "Pessoa", Table: "pessoas", PrimaryKey: "id",
		Fields: []scanner.Field{
			{Name: "ID", Column: "id", GoType: "int64", PrimaryKey: true},
			{Name: "Nome", Column: "nome", GoType: "string"},
		},
	}}
	first, err := Generate(root, cfg, entities)
	if err != nil {
		t.Fatal(err)
	}
	if first.Unchanged || first.Operations != 1 {
		t.Fatalf("resultado inicial inesperado: %#v", first)
	}
	if !regexp.MustCompile(`^\d{4}_\d{2}_\d{2}_\d{6}_migration\.json$`).MatchString(first.Migration) {
		t.Fatalf("nome de migration inválido: %s", first.Migration)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(first.Path))); err != nil {
		t.Fatal(err)
	}
	second, err := Generate(root, cfg, entities)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Unchanged {
		t.Fatalf("segunda geração deveria ser estável: %#v", second)
	}
}

func TestDiffCreatesColumnRemovalOperation(t *testing.T) {
	previous := Snapshot{Version: 1, Tables: []Table{{
		Name: "pessoas", Columns: []Column{{Name: "id", Type: "integer"}, {Name: "nome", Type: "string"}},
	}}}
	current := Snapshot{Version: 1, Tables: []Table{{
		Name: "pessoas", Columns: []Column{{Name: "id", Type: "integer"}},
	}}}
	operations, err := diff(previous, current)
	if err != nil {
		t.Fatal(err)
	}
	if len(operations) != 1 || operations[0].Kind != "drop_column" {
		t.Fatalf("operações inesperadas: %#v", operations)
	}
}
