package scanner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PhelipeViana/flexberry/internal/config"
)

func TestInferRelationsAcceptsLegacyIDPrefix(t *testing.T) {
	relations := inferRelations(
		[]Field{{Name: "IDStatus", Column: "id_status", GoType: "int64"}},
		[]Relation{{Name: "Status", Type: "StatusCidade", Kind: "belongsTo"}},
	)
	if len(relations) != 1 || relations[0].ForeignKey != "id_status" {
		t.Fatalf("legacy ID prefix relation was not inferred: %#v", relations)
	}
}

func TestParseMigrateTag(t *testing.T) {
	value, err := parseMigrateTag("primaryKey,autoIncrement,size=150,unique,index,default=true,references=clientes.id")
	if err != nil {
		t.Fatal(err)
	}
	if !value.PrimaryKey || !value.AutoIncrement || !value.Unique || !value.Index || value.Length != 150 || value.Default != "true" || value.ReferenceTable != "clientes" || value.ReferenceColumn != "id" {
		t.Fatalf("unexpected migrate tag: %#v", value)
	}
}

func TestParseMigrateTagRejectsInvalidOptions(t *testing.T) {
	for _, tag := range []string{"unknown", "size=zero", "autoIncrement", "scale=4", "references=clientes"} {
		if _, err := parseMigrateTag(tag); err == nil {
			t.Errorf("tag %q should fail", tag)
		}
	}
}

func TestScanLenientWarnsAndSkipsEntityWithEmptyInternalImport(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "go.mod"), "module example.test/app\n")
	writeTestFile(t, filepath.Join(root, "internal/modules/cidades/domain/.keep"), "")
	writeTestFile(t, filepath.Join(root, "internal/modules/clientes/domain/cliente.go"), `package domain
import cidades "example.test/app/internal/modules/cidades/domain"
type Cliente struct {
	ID int64 `+"`db:\"id\"`"+`
	CidadeID int64 `+"`db:\"cidade_id\"`"+`
	Cidade cidades.Cidade
}`)

	cfg := &config.Config{Entities: config.EntitiesConfig{
		Paths: []string{"internal/modules/**/domain/*.go"},
	}}
	result, err := ScanLenient(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Entities) != 0 {
		t.Fatalf("entidade inválida não foi ignorada: %#v", result.Entities)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "está vazio") {
		t.Fatalf("alerta inesperado: %#v", result.Warnings)
	}

	if _, err := Scan(root, cfg); err == nil || !strings.Contains(err.Error(), "está vazio") {
		t.Fatalf("scan estrito deveria bloquear com mensagem clara: %v", err)
	}
}

func TestPruneUnresolvedRelationsRemovesTransitiveDependents(t *testing.T) {
	result := pruneUnresolvedRelations(Result{Entities: []Entity{
		{Name: "Notificacao", Relations: []Relation{
			{Name: "Cliente", Type: "Cliente", Kind: "belongsTo", ForeignKey: "cliente_id"},
		}},
		{Name: "Evento"},
	}})
	if len(result.Entities) != 1 || result.Entities[0].Name != "Evento" {
		t.Fatalf("entidades restantes inesperadas: %#v", result.Entities)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "Notificacao") {
		t.Fatalf("alertas inesperados: %#v", result.Warnings)
	}
}

func TestScanLenientWarnsAboutEmptyAndUnmappedFiles(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "go.mod"), "module example.test/app\n")
	writeTestFile(t, filepath.Join(root, "internal/modules/vazio/domain/vazio.go"), "")
	writeTestFile(t, filepath.Join(root, "internal/modules/semcampos/domain/tipo.go"), "package domain\ntype Tipo string\n")

	cfg := &config.Config{Entities: config.EntitiesConfig{
		Paths: []string{"internal/modules/**/domain/*.go"},
	}}
	result, err := ScanLenient(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Warnings) != 2 {
		t.Fatalf("esperados 2 alertas, obtidos %#v", result.Warnings)
	}
	joined := strings.Join(result.Warnings, "\n")
	if !strings.Contains(joined, "arquivo vazio") || !strings.Contains(joined, "campos mapeados") {
		t.Fatalf("alertas incompletos: %s", joined)
	}
}

func TestScanLenientTreatsMissingEntityFoldersAsWarning(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "go.mod"), "module example.test/app\n")
	cfg := &config.Config{Entities: config.EntitiesConfig{
		Paths: []string{"internal/modules/**/domain/*.go"},
	}}

	result, err := ScanLenient(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Entities) != 0 || len(result.Warnings) != 1 {
		t.Fatalf("plano tolerante inesperado: %#v", result)
	}
	if !strings.Contains(result.Warnings[0], "preservados") {
		t.Fatalf("alerta deveria registrar preservação: %s", result.Warnings[0])
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
