package config

import (
	"strings"
	"testing"
)

const validConfig = `
version: 1
environment:
  file: ./.env
  ambient: APPLICATION_ENV
  fallback: development
default:
  variable: DATABASE_CONNECTION
  fallback: postgres
connections:
  postgres:
    dialect: postgres
    url: postgres://${POSTGRES_USERNAME}:${POSTGRES_PASSWORD}@${POSTGRES_HOST}:${POSTGRES_PORT}/${POSTGRES_DATABASE}?sslmode=${POSTGRES_SSLMODE:-disable}
    schema: ${POSTGRES_SCHEMA:-public}
entities:
  paths:
    - internal/modules/**/domain/*.go
`

func TestDecodeAndResolveConnection(t *testing.T) {
	cfg, err := Decode(strings.NewReader(validConfig))
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]string{
		"POSTGRES_USERNAME": "app",
		"POSTGRES_PASSWORD": "secret",
		"POSTGRES_HOST":     "localhost",
		"POSTGRES_PORT":     "5433",
		"POSTGRES_DATABASE": "example",
	}
	connection, err := cfg.ResolvedConnection("postgres", func(key string) string {
		return values[key]
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "postgres://app:secret@localhost:5433/example?sslmode=disable"
	if connection.URL != want {
		t.Fatalf("URL = %q, esperado %q", connection.URL, want)
	}
	if connection.Schema != "public" {
		t.Fatalf("Schema = %q, esperado public", connection.Schema)
	}
	if cfg.Generate.Output != DefaultGenerateOutput || cfg.Generate.Package != DefaultGeneratePackage {
		t.Fatalf("convenções de geração não aplicadas: %#v", cfg.Generate)
	}
	if cfg.Pagination.DefaultPerPage != DefaultPerPage || cfg.Pagination.MaxPerPage != MaxPerPage {
		t.Fatalf("convenções de paginação não aplicadas: %#v", cfg.Pagination)
	}
}

func TestDecodeRejectsUnknownField(t *testing.T) {
	_, err := Decode(strings.NewReader(validConfig + "\nunknown: true\n"))
	if err == nil {
		t.Fatal("esperava erro para campo desconhecido")
	}
}

func TestDecodeAcceptsLegacyEnvironmentVariable(t *testing.T) {
	legacy := strings.Replace(validConfig, "ambient: APPLICATION_ENV", "variable: APPLICATION_ENV", 1)
	cfg, err := Decode(strings.NewReader(legacy))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Environment.Ambient != "APPLICATION_ENV" {
		t.Fatalf("ambient legado não foi normalizado: %#v", cfg.Environment)
	}
}

func TestDefaultConnectionUsesEnvironment(t *testing.T) {
	cfg, err := Decode(strings.NewReader(validConfig))
	if err != nil {
		t.Fatal(err)
	}
	got, err := cfg.DefaultConnection(func(key string) string {
		if key == "DATABASE_CONNECTION" {
			return "postgres"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "postgres" {
		t.Fatalf("DefaultConnection() = %q", got)
	}
}
