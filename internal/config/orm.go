package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const ORMRelativePath = "internal/flexberry/orm.yaml"

type ORMConfig struct {
	Version   int                       `yaml:"version"`
	Entities  EntitiesConfig            `yaml:"entities"`
	Output    ORMOutput                 `yaml:"output"`
	Overrides map[string]EntityOverride `yaml:"overrides,omitempty"`
}

type ORMOutput struct {
	Path    string `yaml:"path"`
	Package string `yaml:"package,omitempty"`
}

func LoadORM(path string) (ORMConfig, error) {
	file, err := os.Open(path)
	if err != nil {
		return ORMConfig{}, fmt.Errorf("abrir orm.yaml: %w", err)
	}
	defer file.Close()
	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	var cfg ORMConfig
	if err := decoder.Decode(&cfg); err != nil {
		return ORMConfig{}, fmt.Errorf("ler orm.yaml: %w", err)
	}
	if cfg.Version != 1 {
		return ORMConfig{}, fmt.Errorf("orm.yaml: version precisa ser 1")
	}
	if len(cfg.Entities.Paths) == 0 {
		return ORMConfig{}, fmt.Errorf("orm.yaml: entities.paths precisa conter ao menos um caminho")
	}
	if strings.TrimSpace(cfg.Output.Path) == "" {
		cfg.Output.Path = DefaultGenerateOutput
	}
	cfg.Output.Package, err = packageFromPath(cfg.Output.Path)
	if err != nil {
		return ORMConfig{}, fmt.Errorf("orm.yaml: output.path: %w", err)
	}
	cfg.Entities.Overrides = cfg.Overrides
	return cfg, nil
}
