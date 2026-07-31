package cliui

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestNormalizeFirstName(t *testing.T) {
	tests := map[string]string{
		"joao-da-silva": "Joao",
		"PHELipe Viana": "Phelipe",
		"maria_souza":   "Maria",
		"":              "",
	}
	for input, expected := range tests {
		if actual := normalizeFirstName(input); actual != expected {
			t.Fatalf("%q: recebido %q, esperado %q", input, actual, expected)
		}
	}
}

func TestFriendlyIssuesSeparatesMigrationLocationAndProblem(t *testing.T) {
	err := errors.New(`ler migration 2026_07_31_152537.go: C:\projeto\teste\migrations\2026_07_31_152537.go:10: nome de tabela "phelipe gabriel" inválido; use snake_case`)
	issues := friendlyIssues(err)
	if len(issues) != 1 {
		t.Fatalf("esperava um erro, obtido: %#v", issues)
	}
	issue := issues[0]
	if issue.Message != "O nome físico da tabela é inválido." ||
		!strings.Contains(issue.Location, "2026_07_31_152537.go:10") ||
		!strings.Contains(issue.Solution, "phelipe_gabriel") {
		t.Fatalf("erro de migration mal estruturado: %#v", issue)
	}
}

func TestPresentIssuesUsesReadableBlocks(t *testing.T) {
	var output bytes.Buffer
	presentIssues(&output, "Phelipe", []Issue{{
		Message: "O nome físico da tabela é inválido.", Location: `teste\migrations\migration.go:10`,
		Detail: `nome de tabela "phelipe gabriel" inválido`, Solution: `Use "phelipe_gabriel".`,
	}})
	text := output.String()
	for _, expected := range []string{"Problema:\n", "Local:\n", "Detalhes:\n", "Como corrigir:\n"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("campo %q ausente no layout:\n%s", expected, text)
		}
	}
	if strings.Contains(text, "Como corrigir: Use") {
		t.Fatalf("rótulo e instrução não devem ficar espremidos na mesma linha:\n%s", text)
	}
}

func TestFriendlyIssuesExplainsColumnAndAliasNames(t *testing.T) {
	tests := []struct {
		detail  string
		message string
		example string
	}{
		{`nome de coluna "nome completo" inválido; use snake_case`, "O nome físico da coluna é inválido.", "coluna_separada_tambem_vai"},
		{`alias "Phelipe Gabriel" inválido; use lowerCamelCase`, "O alias da tabela é inválido.", "phelipeGabriel"},
	}
	for _, test := range tests {
		err := errors.New(`ler migration migration.go: C:\projeto\migration.go:7: ` + test.detail)
		issue := friendlyIssues(err)[0]
		if issue.Message != test.message || !strings.Contains(issue.Solution, test.example) {
			t.Fatalf("orientação inesperada para %q: %#v", test.detail, issue)
		}
	}
}
