package initializer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/PhelipeViana/flexberry/internal/config"
)

func TestRunCreatesInitialStructureAndPreservesExistingFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Run(root, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Created) != 6 {
		t.Fatalf("foram criados %d arquivos, esperado 6", len(result.Created))
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(config.DefaultRelativePath))); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "internal", "flexberry", "factory.yaml")); err != nil {
		t.Fatal(err)
	}

	customPath := filepath.Join(root, "internal", "flexberry", "custom.go")
	if err := os.WriteFile(customPath, []byte("package flexberry\n// usuário\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(root, false); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(customPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "package flexberry\n// usuário\n" {
		t.Fatal("custom.go foi sobrescrito")
	}

	if _, err := Run(root, true); err != nil {
		t.Fatal(err)
	}
	content, err = os.ReadFile(customPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "package flexberry\n// usuário\n" {
		t.Fatal("custom.go foi sobrescrito com --force")
	}
}

func TestRunRepairsBlankConfiguration(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, filepath.FromSlash(config.ORMRelativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(" \n\t"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Run(root, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Repaired) != 1 || result.Repaired[0] != config.ORMRelativePath {
		t.Fatalf("reparos inesperados: %#v", result.Repaired)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != ORMConfigTemplateV2 {
		t.Fatal("orm.yaml vazio não foi recriado")
	}
}

func TestRunPreservesExistingRootEnv(t *testing.T) {
	root := t.TempDir()
	const custom = "DB_DIALECT=legacy\n"
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte(custom), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := Run(root, false)
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(root, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != custom {
		t.Fatal(".env existente foi sobrescrito")
	}
	if _, err := os.Stat(filepath.Join(root, ".env.example")); err != nil {
		t.Fatal(".env.example não foi criado")
	}
	if len(result.Skipped) == 0 {
		t.Fatal(".env existente deveria constar como preservado")
	}
	gitignore, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if !containsLine(string(gitignore), ".env") {
		t.Fatal(".env não foi incluído no .gitignore")
	}
}
