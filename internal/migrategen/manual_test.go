package migrategen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PhelipeViana/flexberry/internal/config"
)

func TestCreateManualWritesProvisionalScaffold(t *testing.T) {
	root := t.TempDir()
	path, err := CreateManual(root, config.MigrateConfig{Output: config.MigrateOutput{Path: "migrations"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasSuffix(path, "_migration.go") || filepath.Ext(path) != ".go" {
		t.Fatalf("nome provisório inesperado: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "return migrate.TODO()") || !strings.Contains(string(data), "func Migration") {
		t.Fatalf("scaffold inesperado: %s", data)
	}
	if !strings.Contains(string(data), "/core/migration/alias") || !strings.Contains(string(data), "var _ = alias.Catalog") {
		t.Fatalf("scaffold não prepara o autocomplete de alias: %s", data)
	}
}
