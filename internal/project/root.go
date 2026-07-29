package project

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FindRoot walks from start towards the filesystem root until it finds go.mod.
func FindRoot(start string) (string, error) {
	current, err := filepath.Abs(start)
	if err != nil {
		return "", fmt.Errorf("resolver diretório atual: %w", err)
	}

	for {
		info, statErr := os.Stat(filepath.Join(current, "go.mod"))
		if statErr == nil && !info.IsDir() {
			return current, nil
		}
		if statErr != nil && !os.IsNotExist(statErr) {
			return "", fmt.Errorf("verificar go.mod em %s: %w", current, statErr)
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("go.mod não encontrado a partir de %s", start)
		}
		current = parent
	}
}

func ModulePath(root string) (string, error) {
	file, err := os.Open(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && fields[0] == "module" {
			return fields[1], nil
		}
	}
	return "", fmt.Errorf("module não encontrado em go.mod")
}
