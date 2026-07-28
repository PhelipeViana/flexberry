package initializer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/PhelipeViana/flexberry/internal/config"
)

type Result struct {
	Created []string
	Skipped []string
}

func Run(projectRoot string, force bool) (Result, error) {
	files := []struct {
		relativePath string
		content      string
		secret       bool
	}{
		{config.DefaultRelativePath, ConfigTemplate, false},
		{"internal/flexberry/custom.go", CustomTemplate, true},
		{"internal/flexberry/orm/.gitkeep", "", false},
		{"seguranca/database.env", EnvTemplate, true},
		{"seguranca/database.example.env", EnvTemplate, false},
	}

	var result Result
	for _, file := range files {
		path := filepath.Join(projectRoot, filepath.FromSlash(file.relativePath))
		if err := writeFile(path, file.content, force && !file.secret); err != nil {
			if errors.Is(err, os.ErrExist) {
				result.Skipped = append(result.Skipped, file.relativePath)
				continue
			}
			return result, err
		}
		result.Created = append(result.Created, file.relativePath)
	}

	if err := ensureGitIgnore(projectRoot); err != nil {
		return result, err
	}
	return result, nil
}

func writeFile(path, content string, overwrite bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("criar diretório de %s: %w", path, err)
	}
	flags := os.O_WRONLY | os.O_CREATE
	if overwrite {
		flags |= os.O_TRUNC
	} else {
		flags |= os.O_EXCL
	}
	file, err := os.OpenFile(path, flags, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.WriteString(content); err != nil {
		return fmt.Errorf("escrever %s: %w", path, err)
	}
	return nil
}

func ensureGitIgnore(projectRoot string) error {
	path := filepath.Join(projectRoot, ".gitignore")
	const line = "seguranca/database.env"

	content, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("ler .gitignore: %w", err)
	}
	if containsLine(string(content), line) {
		return nil
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("abrir .gitignore: %w", err)
	}
	defer file.Close()

	prefix := ""
	if len(content) > 0 && content[len(content)-1] != '\n' {
		prefix = "\n"
	}
	_, err = file.WriteString(prefix + "\n# Credenciais locais do Flexberry\n" + line + "\n")
	return err
}

func containsLine(content, expected string) bool {
	start := 0
	for index := 0; index <= len(content); index++ {
		if index == len(content) || content[index] == '\n' {
			line := content[start:index]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			if line == expected {
				return true
			}
			start = index + 1
		}
	}
	return false
}
