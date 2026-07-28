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
	if len(result.Created) != 5 {
		t.Fatalf("foram criados %d arquivos, esperado 5", len(result.Created))
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(config.DefaultRelativePath))); err != nil {
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
