package migraterun

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PhelipeViana/flexberry/internal/migrategen"
)

func TestFinalizeProvisionalRenamesFileAndFunction(t *testing.T) {
	folder := t.TempDir()
	path := filepath.Join(folder, "2026_07_31_100128.go")
	source := "package migrations\nfunc Migration20260731100128() {}\n"
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	files := []migrationFile{{Name: filepath.Base(path), ID: "2026_07_31_100128", Path: path, Provisional: true,
		Plan: migrategen.Plan{Operations: []migrategen.Operation{{Kind: "create_table", Table: "pessoas"}}}}}
	if err := finalizeProvisional(files); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(folder, "2026_07_31_100128_create_pessoas_migration.go")
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Migration20260731100128CreatePessoas") {
		t.Fatalf("função não foi finalizada: %s", data)
	}
}
