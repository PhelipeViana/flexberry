package config

import (
	"fmt"
	"path/filepath"
	"strings"
)

func packageFromPath(value string) (string, error) {
	cleaned := filepath.Clean(filepath.FromSlash(strings.TrimSpace(value)))
	name := filepath.Base(cleaned)
	if name == "." || name == string(filepath.Separator) || !validName(name) {
		return "", fmt.Errorf("o último diretório precisa ser um nome válido de package Go")
	}
	return name, nil
}
