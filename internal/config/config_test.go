package config

import (
	"strings"
	"testing"
)

const validConfig = `
version: 1
environment:
  file: ./.env
  ambient: APP_ENV
  fallback: development
default:
  dialect: DB_DIALECT
  fallback: postgres
connections:
  postgres:
    dialect: postgres
    url: postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@${POSTGRES_HOST}:${POSTGRES_PORT}/${POSTGRES_DB}?sslmode=${POSTGRES_SSLMODE:-disable}
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
		"POSTGRES_USER":     "app",
		"POSTGRES_PASSWORD": "secret",
		"POSTGRES_HOST":     "localhost",
		"POSTGRES_PORT":     "5433",
		"POSTGRES_DB":       "example",
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
	legacy := strings.Replace(validConfig, "ambient: APP_ENV", "variable: APP_ENV", 1)
	cfg, err := Decode(strings.NewReader(legacy))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Environment.Ambient != "APP_ENV" {
		t.Fatalf("ambient legado não foi normalizado: %#v", cfg.Environment)
	}
}

func TestDefaultConnectionUsesEnvironment(t *testing.T) {
	cfg, err := Decode(strings.NewReader(validConfig))
	if err != nil {
		t.Fatal(err)
	}
	got, err := cfg.DefaultConnection(func(key string) string {
		if key == "DB_DIALECT" {
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
