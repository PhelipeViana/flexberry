package migrationgo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFileReadsFluentMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "2026_01_01_000001_migration.go")
	source := `package migrations
import "github.com/PhelipeViana/flexberry/migration/acao"
var Migration = []acao.Operacao{
	nova(acao.CreateTable, "teste",
		coluna("id").Integer().PrimaryKey().AutoIncrement(),
		coluna("nome").Varchar(255).Nullable(),
	),
	nova(acao.AddForeignKey, "teste",
		coluna("usuario_id").References("usuarios", "id"),
	),
}`
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	operations, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(operations) != 2 || len(operations[0].Columns) != 2 {
		t.Fatalf("operações inesperadas: %#v", operations)
	}
	if !operations[0].Columns[0].PrimaryKey || !operations[0].Columns[1].Nullable {
		t.Fatalf("modificadores não interpretados: %#v", operations[0].Columns)
	}
	if operations[1].ForeignKey.ReferenceTable != "usuarios" {
		t.Fatalf("referência inesperada: %#v", operations[1].ForeignKey)
	}
}

func TestParseFileRejectsUnknownCalls(t *testing.T) {
	path := filepath.Join(t.TempDir(), "migration.go")
	if err := os.WriteFile(path, []byte(`package migrations
var Migration = []any{ executarComando("rm") }`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseFile(path); err == nil {
		t.Fatal("chamada arbitrária deveria ser recusada")
	}
}

func TestRefreshCatalogAndParseAliasReference(t *testing.T) {
	folder := t.TempDir()
	createPath := filepath.Join(folder, "2026_01_01_000001_migration.go")
	createSource := `package migrations
import migrate "github.com/PhelipeViana/flexberry/migration"
var Migration = []migrate.Operation{
	migrate.CreateTable("pedido_itens", col("id").Integer().PrimaryKey()),
}`
	if err := os.WriteFile(createPath, []byte(createSource), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RefreshCatalog(folder); err != nil {
		t.Fatal(err)
	}
	alterPath := filepath.Join(folder, "2026_01_01_000002_migration.go")
	alterSource := `package migrations
import migrate "github.com/PhelipeViana/flexberry/migration"
var Migration = []migrate.Operation{
	migrate.AddColumn(alias.pedidoItens, col("quantidade").Integer()),
}`
	if err := os.WriteFile(alterPath, []byte(alterSource), 0o644); err != nil {
		t.Fatal(err)
	}
	operations, err := ParseFile(alterPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(operations) != 1 || operations[0].Table != "pedido_itens" {
		t.Fatalf("referência de tabela inesperada: %#v", operations)
	}
}

func TestParseFileReadsDefinitionFunction(t *testing.T) {
	path := filepath.Join(t.TempDir(), "2026_07_31_100128.go")
	source := `package migrations
import migrate "github.com/PhelipeViana/flexberry/migration"
func Migration20260731100128() migrate.Definition {
	return migrate.Define(
		migrate.CreateTable("auditorias", col("id").Integer().PrimaryKey()).Alias("auditoria"),
	)
}`
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	operations, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(operations) != 1 || operations[0].Table != "auditorias" {
		t.Fatalf("operações inesperadas: %#v", operations)
	}
}

func TestParseFileRejectsMultipleActionsInDefinition(t *testing.T) {
	path := filepath.Join(t.TempDir(), "2026_07_31_100128.go")
	source := `package migrations
import migrate "github.com/PhelipeViana/flexberry/migration"
func Migration20260731100128() migrate.Definition {
	return migrate.Define(
		migrate.CreateTable("a", col("id").Integer()),
		migrate.CreateTable("b", col("id").Integer()),
	)
}`
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseFile(path); err == nil || !strings.Contains(err.Error(), "exatamente uma ação") {
		t.Fatalf("esperava bloqueio de múltiplas ações, obtido: %v", err)
	}
}
