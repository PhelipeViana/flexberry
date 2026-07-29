package factorygen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PhelipeViana/flexberry/internal/config"
	"github.com/PhelipeViana/flexberry/internal/scanner"
)

func TestGenerateAppliesExactRuleAndRelationship(t *testing.T) {
	root := t.TempDir()
	entities := []scanner.Entity{
		{Name: "Categoria", Function: "Categoria", Table: "categorias", PrimaryKey: "id", Fields: []scanner.Field{
			{Name: "ID", Column: "id", GoType: "int64", PrimaryKey: true},
		}},
		{Name: "Produto", Function: "Produto", Table: "produtos", PrimaryKey: "id", Fields: []scanner.Field{
			{Name: "ID", Column: "id", GoType: "int64", PrimaryKey: true},
			{Name: "CategoriaID", Column: "categoria_id", GoType: "int64"},
			{Name: "Ativo", Column: "ativo", GoType: "int64"},
		}, Relations: []scanner.Relation{
			{Name: "Categoria", Type: "Categoria", Kind: "belongsTo", ForeignKey: "categoria_id"},
		}},
	}
	factoryConfig := config.FactoryConfig{
		Version:     1,
		Mapper:      config.FactoryMapper{Path: "internal/database/factories", Package: "factories"},
		Expressions: config.FactoryExpressions{Exact: map[string]string{"ATIVO": "int64(index % 2)"}},
		Defaults:    config.FactoryDefaults{Count: 10, Update: true, Active: true},
	}
	ormConfig := config.ORMConfig{Output: config.ORMOutput{Path: "internal/flexberry/orm", Package: "flexberry"}}
	if _, err := Generate(root, "example.test/app", factoryConfig, ormConfig, entities); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(root, "internal", "database", "factories", "produto_factory.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	if !strings.Contains(text, `flexberry.Vinculo("categorias", "id")`) {
		t.Fatalf("vínculo não gerado:\n%s", text)
	}
	if !strings.Contains(text, `"ativo":`) || !strings.Contains(text, `int64(index % 2)`) {
		t.Fatalf("regra exact não aplicada:\n%s", text)
	}
}
