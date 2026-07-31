package migrategen

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/PhelipeViana/flexberry/internal/config"
	"github.com/PhelipeViana/flexberry/internal/migrationgo"
	"github.com/PhelipeViana/flexberry/internal/scanner"
)

func TestBuildSnapshotUsesMigrateTagMetadata(t *testing.T) {
	snapshot, err := buildSnapshot([]scanner.Entity{{Name: "Pedido", Table: "pedidos", PrimaryKey: "id", Fields: []scanner.Field{
		{Name: "ID", Column: "id", GoType: "int64", PrimaryKey: true, AutoIncrement: true},
		{Name: "Email", Column: "email", GoType: "*string", Nullable: true, Length: 150, Unique: true},
		{Name: "Valor", Column: "valor", GoType: "float64", Precision: 19, Scale: 4},
		{Name: "ClienteID", Column: "cliente_id", GoType: "int64", Index: true, ReferenceTable: "clientes", ReferenceColumn: "id"},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	operations, err := diff(Snapshot{Version: 1}, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var create, index, foreignKey bool
	for _, operation := range operations {
		switch operation.Kind {
		case "create_table":
			columns := columnMap(operation.Columns)
			create = columns["id"].AutoIncrement && columns["email"].Unique && columns["valor"].Precision == 19
		case "create_index":
			index = true
		case "add_foreign_key":
			foreignKey = true
		}
	}
	if !create || !index || !foreignKey {
		t.Fatalf("metadata operations missing: %#v", operations)
	}
}

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
	if !regexp.MustCompile(`^\d{4}_\d{2}_\d{2}_\d{6}\.go$`).MatchString(first.Migration) {
		t.Fatalf("nome de migration inválido: %s", first.Migration)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(first.Path))); err != nil {
		t.Fatal(err)
	}
	catalogPath := filepath.Join(root, "internal", "flexberry", "core", "migration", "alias", "dsl.gen.go")
	if _, err := os.Stat(catalogPath); err != nil {
		t.Fatal("catálogo não foi gerado no core:", err)
	}
	catalog, err := os.ReadFile(catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(catalog), "package alias") || !strings.Contains(string(catalog), `var Pessoas = migrate.Table("pessoas")`) {
		t.Fatalf("catálogo de aliases inesperado:\n%s", catalog)
	}
	if _, err := os.Stat(filepath.Join(root, "migrations", snapshotName)); !os.IsNotExist(err) {
		t.Fatal("snapshot não deve permanecer na pasta de migrations")
	}
	generated, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(first.Path)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(generated), `migrate.CreateTable("pessoas"`) ||
		!strings.Contains(string(generated), `migrate.Col("id").Integer().PrimaryKey()`) {
		t.Fatalf("migration Go inesperada:\n%s", generated)
	}
	second, err := Generate(root, cfg, entities)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Unchanged {
		t.Fatalf("segunda geração deveria ser estável: %#v", second)
	}
}

func TestGenerateCreatesOneFilePerLogicalAction(t *testing.T) {
	root := t.TempDir()
	cfg := config.MigrateConfig{Output: config.MigrateOutput{Path: "migrations"}}
	entities := []scanner.Entity{
		{Name: "Pessoa", Table: "pessoas", Fields: []scanner.Field{{Name: "ID", Column: "id", GoType: "int64", PrimaryKey: true}}},
		{Name: "Produto", Table: "produtos", Fields: []scanner.Field{{Name: "ID", Column: "id", GoType: "int64", PrimaryKey: true}}},
	}
	result, err := Generate(root, cfg, entities)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Paths) != 2 || result.Operations != 2 {
		t.Fatalf("esperava dois arquivos com uma ação cada: %#v", result)
	}
	for _, path := range result.Paths {
		operations, err := migrationgo.ParseFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		if len(operations) != 1 {
			t.Fatalf("migration %s contém %d ações", path, len(operations))
		}
	}
}

func TestGenerateRebuildsMigrationsWhenSnapshotExistsButFilesAreMissing(t *testing.T) {
	root := t.TempDir()
	cfg := config.MigrateConfig{Output: config.MigrateOutput{Path: "migrations"}}
	entities := []scanner.Entity{{Name: "Livro", Table: "livros", Fields: []scanner.Field{{Name: "ID", Column: "id", GoType: "int64", PrimaryKey: true}}}}
	first, err := Generate(root, cfg, entities)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, filepath.FromSlash(first.Path))); err != nil {
		t.Fatal(err)
	}
	second, err := Generate(root, cfg, entities)
	if err != nil {
		t.Fatal(err)
	}
	if second.Unchanged || len(second.Paths) != 1 {
		t.Fatalf("migration ausente deveria ser reconstruída: %#v", second)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(second.Path))); err != nil {
		t.Fatal("arquivo reconstruído não existe:", err)
	}
}

func TestReloadMigratesLegacyTableReferenceToAlias(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(root, "migrations")
	if err := os.MkdirAll(output, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(output, "2026_07_31_100128.go")
	source := `package migrations
import (
	migrate "github.com/PhelipeViana/flexberry/migration"
	table "example.test/project/internal/flexberry/core/migration/table"
)
func Migration20260731100128() migrate.Definition {
	return migrate.AddColumn(table.livros, migrate.Col("autor").Varchar(255))
}`
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := migrateLegacyHelpers(output, "example.test/project"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, `alias "example.test/project/internal/flexberry/core/migration/alias"`) ||
		!strings.Contains(text, "alias.Livros") || strings.Contains(text, "table.") {
		t.Fatalf("migration antiga não foi convertida para alias:\n%s", text)
	}
}

func TestGenerateUsesAliasForExistingTable(t *testing.T) {
	root := t.TempDir()
	cfg := config.MigrateConfig{Output: config.MigrateOutput{Path: "migrations"}}
	entity := scanner.Entity{Name: "Livro", Table: "livros", Fields: []scanner.Field{{Name: "ID", Column: "id", GoType: "int64", PrimaryKey: true}}}
	if _, err := Generate(root, cfg, []scanner.Entity{entity}); err != nil {
		t.Fatal(err)
	}
	entity.Fields = append(entity.Fields, scanner.Field{Name: "Autor", Column: "autor", GoType: "string"})
	result, err := Generate(root, cfg, []scanner.Entity{entity})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(result.Path)))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "/core/migration/alias") || !strings.Contains(text, "alias.Livros") {
		t.Fatalf("alteração não usa o catálogo de aliases:\n%s", text)
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

func TestGenerateFromPartialScanPreservesPreviousSnapshot(t *testing.T) {
	root := t.TempDir()
	cfg := config.MigrateConfig{Output: config.MigrateOutput{Path: "migrations"}}
	entity := scanner.Entity{
		Name: "Cliente", Table: "clientes", PrimaryKey: "id",
		Fields: []scanner.Field{{Name: "ID", Column: "id", GoType: "int64", PrimaryKey: true}},
	}
	if _, err := Generate(root, cfg, []scanner.Entity{entity}); err != nil {
		t.Fatal(err)
	}

	result, err := GenerateFromScan(root, cfg, scanner.Result{
		Warnings: []string{"cliente.go ignorado: import interno ausente"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Unchanged {
		t.Fatalf("leitura parcial não deveria gerar remoções: %#v", result)
	}

	snapshot, err := loadSnapshot(filepath.Join(root, "internal", "flexberry", "core", "migration", snapshotName))
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Tables) != 1 || snapshot.Tables[0].Name != "clientes" {
		t.Fatalf("snapshot válido não foi preservado: %#v", snapshot)
	}
}
