package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const DefaultRelativePath = "internal/flexberry/flexberry.yaml"
const (
	DefaultGenerateOutput  = "internal/flexberry/orm"
	DefaultGeneratePackage = "flexberry"
	DefaultPerPage         = 15
	MaxPerPage             = 100
)

var (
	variablePattern   = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(?::-(.*?))?\}`)
	supportedDialects = map[string]struct{}{
		"oracle":    {},
		"postgres":  {},
		"mysql":     {},
		"sqlserver": {},
	}
)

type Config struct {
	Version     int                   `yaml:"version"`
	Environment EnvironmentConfig     `yaml:"environment"`
	Default     DefaultConfig         `yaml:"default"`
	Connections map[string]Connection `yaml:"connections"`
	Entities    EntitiesConfig        `yaml:"entities"`
	Generate    GenerateConfig        `yaml:"generate,omitempty"`
	Pagination  PaginationConfig      `yaml:"pagination,omitempty"`
}

type EnvironmentConfig struct {
	File     string `yaml:"file"`
	Variable string `yaml:"variable"`
	Fallback string `yaml:"fallback"`
}

type DefaultConfig struct {
	Variable string `yaml:"variable"`
	Fallback string `yaml:"fallback"`
}

type Connection struct {
	Dialect string `yaml:"dialect"`
	URL     string `yaml:"url"`
	Schema  string `yaml:"schema,omitempty"`
}

type EntitiesConfig struct {
	Paths     []string                  `yaml:"paths"`
	Exclude   []string                  `yaml:"exclude,omitempty"`
	Overrides map[string]EntityOverride `yaml:"overrides,omitempty"`
}

type EntityOverride struct {
	Table      string `yaml:"table,omitempty"`
	PrimaryKey string `yaml:"primary_key,omitempty"`
	Connection string `yaml:"connection,omitempty"`
}

type GenerateConfig struct {
	Output  string `yaml:"output"`
	Package string `yaml:"package"`
}

type PaginationConfig struct {
	DefaultPerPage int `yaml:"default_per_page"`
	MaxPerPage     int `yaml:"max_per_page"`
}

func Load(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("abrir configuração %s: %w", path, err)
	}
	defer file.Close()

	cfg, err := Decode(file)
	if err != nil {
		return nil, fmt.Errorf("ler configuração %s: %w", path, err)
	}
	return cfg, nil
}

func Decode(reader io.Reader) (*Config, error) {
	decoder := yaml.NewDecoder(reader)
	decoder.KnownFields(true)

	var cfg Config
	if err := decoder.Decode(&cfg); err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.Generate.Output) == "" {
		cfg.Generate.Output = DefaultGenerateOutput
	}
	if strings.TrimSpace(cfg.Generate.Package) == "" {
		cfg.Generate.Package = DefaultGeneratePackage
	}
	if cfg.Pagination.DefaultPerPage == 0 {
		cfg.Pagination.DefaultPerPage = DefaultPerPage
	}
	if cfg.Pagination.MaxPerPage == 0 {
		cfg.Pagination.MaxPerPage = MaxPerPage
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) Validate() error {
	var problems []string

	if c.Version != 1 {
		problems = append(problems, fmt.Sprintf("version deve ser 1, recebido %d", c.Version))
	}
	if strings.TrimSpace(c.Environment.Variable) == "" {
		problems = append(problems, "environment.variable é obrigatório")
	}
	if strings.TrimSpace(c.Default.Variable) == "" {
		problems = append(problems, "default.variable é obrigatório")
	}
	if strings.TrimSpace(c.Default.Fallback) == "" {
		problems = append(problems, "default.fallback é obrigatório")
	}
	if len(c.Connections) == 0 {
		problems = append(problems, "ao menos uma conexão deve ser configurada")
	}

	for name, connection := range c.Connections {
		if !validName(name) {
			problems = append(problems, fmt.Sprintf("connections.%s possui nome inválido", name))
		}
		dialect := strings.ToLower(strings.TrimSpace(connection.Dialect))
		if _, ok := supportedDialects[dialect]; !ok {
			problems = append(problems, fmt.Sprintf("connections.%s.dialect %q não é suportado", name, connection.Dialect))
		}
		if strings.TrimSpace(connection.URL) == "" {
			problems = append(problems, fmt.Sprintf("connections.%s.url é obrigatória", name))
		}
	}

	if _, ok := c.Connections[c.Default.Fallback]; !ok {
		problems = append(problems, fmt.Sprintf(
			"default.fallback %q não existe em connections",
			c.Default.Fallback,
		))
	}
	if len(c.Entities.Paths) == 0 {
		problems = append(problems, "entities.paths precisa conter ao menos um caminho")
	}
	if strings.TrimSpace(c.Generate.Output) == "" {
		problems = append(problems, "generate.output é obrigatório")
	}
	if !validName(c.Generate.Package) {
		problems = append(problems, "generate.package deve ser um identificador Go simples")
	}
	if c.Pagination.DefaultPerPage < 1 {
		problems = append(problems, "pagination.default_per_page deve ser maior que zero")
	}
	if c.Pagination.MaxPerPage < c.Pagination.DefaultPerPage {
		problems = append(problems, "pagination.max_per_page deve ser maior ou igual ao padrão")
	}

	if len(problems) > 0 {
		sort.Strings(problems)
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

func (c *Config) EnvironmentName(lookup func(string) string) string {
	return valueOrFallback(lookup(c.Environment.Variable), c.Environment.Fallback)
}

func (c *Config) DefaultConnection(lookup func(string) string) (string, error) {
	name := strings.ToLower(valueOrFallback(lookup(c.Default.Variable), c.Default.Fallback))
	if _, ok := c.Connections[name]; !ok {
		return "", fmt.Errorf(
			"conexão padrão %q não configurada; disponíveis: %s",
			name,
			strings.Join(c.ConnectionNames(), ", "),
		)
	}
	return name, nil
}

func (c *Config) ConnectionNames() []string {
	names := make([]string, 0, len(c.Connections))
	for name := range c.Connections {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (c *Config) ResolvedConnection(name string, lookup func(string) string) (Connection, error) {
	connection, ok := c.Connections[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return Connection{}, fmt.Errorf("conexão %q não configurada", name)
	}

	var err error
	connection.URL, err = Expand(connection.URL, lookup)
	if err != nil {
		return Connection{}, fmt.Errorf("resolver URL da conexão %s: %w", name, err)
	}
	if connection.Schema != "" {
		connection.Schema, err = Expand(connection.Schema, lookup)
		if err != nil {
			return Connection{}, fmt.Errorf("resolver schema da conexão %s: %w", name, err)
		}
	}
	return connection, nil
}

func (c *Config) EnvironmentFile(projectRoot string, lookup func(string) string) (string, error) {
	if strings.TrimSpace(c.Environment.File) == "" {
		return "", nil
	}
	resolved, err := Expand(c.Environment.File, lookup)
	if err != nil {
		return "", fmt.Errorf("resolver environment.file: %w", err)
	}
	if resolved == "" {
		return "", nil
	}
	if filepath.IsAbs(resolved) {
		return filepath.Clean(resolved), nil
	}
	return filepath.Join(projectRoot, filepath.FromSlash(resolved)), nil
}

func Expand(template string, lookup func(string) string) (string, error) {
	var missing []string
	result := variablePattern.ReplaceAllStringFunc(template, func(match string) string {
		parts := variablePattern.FindStringSubmatch(match)
		if value := lookup(parts[1]); value != "" {
			return value
		}
		if parts[2] != "" {
			return parts[2]
		}
		missing = append(missing, parts[1])
		return match
	})
	if len(missing) > 0 {
		sort.Strings(missing)
		return "", fmt.Errorf("variáveis obrigatórias ausentes: %s", strings.Join(missing, ", "))
	}
	return result, nil
}

func validName(value string) bool {
	if value == "" {
		return false
	}
	for index, char := range value {
		if !(char == '_' || char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || index > 0 && char >= '0' && char <= '9') {
			return false
		}
	}
	return true
}

func valueOrFallback(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	return strings.TrimSpace(fallback)
}

func OSLookup(key string) string {
	return os.Getenv(key)
}
