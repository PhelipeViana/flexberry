package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const MigrateRelativePath = "internal/flexberry/migrate.yaml"

type MigrateConfig struct {
	Version  int            `yaml:"version"`
	Entities EntitiesConfig `yaml:"entities"`
	Output   MigrateOutput  `yaml:"output"`
	History  string         `yaml:"history_table"`
}

type MigrateOutput struct {
	Path string `yaml:"path"`
}

func LoadMigrate(path string) (MigrateConfig, error) {
	file, err := os.Open(path)
	if err != nil {
		return MigrateConfig{}, fmt.Errorf("abrir migrate.yaml: %w", err)
	}
	defer file.Close()
	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	var cfg MigrateConfig
	if err := decoder.Decode(&cfg); err != nil {
		return MigrateConfig{}, fmt.Errorf("ler migrate.yaml: %w", err)
	}
	if cfg.Version != 1 {
		return MigrateConfig{}, fmt.Errorf("migrate.yaml: version precisa ser 1")
	}
	if len(cfg.Entities.Paths) == 0 {
		return MigrateConfig{}, fmt.Errorf("migrate.yaml: entities.paths precisa conter ao menos um caminho")
	}
	if strings.TrimSpace(cfg.Output.Path) == "" {
		cfg.Output.Path = "internal/database/migrations"
	}
	if strings.TrimSpace(cfg.History) == "" {
		cfg.History = "migrations_flex"
	}
	if !validName(cfg.History) {
		return MigrateConfig{}, fmt.Errorf("migrate.yaml: history_table possui nome inválido")
	}
	return cfg, nil
}
