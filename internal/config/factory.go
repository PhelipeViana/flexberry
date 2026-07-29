package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const FactoryRelativePath = "internal/flexberry/factory.yaml"

type FactoryConfig struct {
	Version     int                `yaml:"version"`
	Mapper      FactoryMapper      `yaml:"mapper"`
	Expressions FactoryExpressions `yaml:"expressions"`
	Defaults    FactoryDefaults    `yaml:"defaults"`
}

type FactoryMapper struct {
	Path    string `yaml:"path"`
	Package string `yaml:"package"`
}

type FactoryExpressions struct {
	Exact    map[string]string     `yaml:"exact"`
	Contains []FactoryContainsRule `yaml:"contains"`
}

type FactoryContainsRule struct {
	Pattern    string `yaml:"pattern"`
	Expression string `yaml:"expression"`
}

type FactoryDefaults struct {
	Count  int  `yaml:"count"`
	Update bool `yaml:"update"`
	Active bool `yaml:"active"`
}

func LoadFactory(path string) (FactoryConfig, error) {
	file, err := os.Open(path)
	if err != nil {
		return FactoryConfig{}, fmt.Errorf("abrir factory.yaml: %w", err)
	}
	defer file.Close()
	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	var cfg FactoryConfig
	if err := decoder.Decode(&cfg); err != nil {
		return FactoryConfig{}, fmt.Errorf("ler factory.yaml: %w", err)
	}
	if cfg.Version != 1 {
		return FactoryConfig{}, fmt.Errorf("factory.yaml: version precisa ser 1")
	}
	if strings.TrimSpace(cfg.Mapper.Path) == "" {
		return FactoryConfig{}, fmt.Errorf("factory.yaml: mapper.path é obrigatório")
	}
	if !validName(cfg.Mapper.Package) {
		return FactoryConfig{}, fmt.Errorf("factory.yaml: mapper.package precisa ser um identificador Go")
	}
	if cfg.Defaults.Count < 1 {
		return FactoryConfig{}, fmt.Errorf("factory.yaml: defaults.count precisa ser maior que zero")
	}
	for _, rule := range cfg.Expressions.Contains {
		if strings.TrimSpace(rule.Pattern) == "" || strings.TrimSpace(rule.Expression) == "" {
			return FactoryConfig{}, fmt.Errorf("factory.yaml: regras contains exigem pattern e expression")
		}
	}
	return cfg, nil
}
