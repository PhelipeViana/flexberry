package factoryrun

import (
	"strings"
	"testing"

	"github.com/PhelipeViana/flexberry/internal/config"
)

func TestValidateFactoryPackageExplainsHowToGenerateMissingFolder(t *testing.T) {
	err := validateFactoryPackage(t.TempDir()+"/inexistente", "internal/database/factories")
	if err == nil {
		t.Fatal("era esperado um erro")
	}
	message := err.Error()
	if !strings.Contains(message, "factory reload") || !strings.Contains(message, "internal/database/factories") {
		t.Fatalf("mensagem pouco amigável: %s", message)
	}
}

func TestFriendlyBuildErrorExplainsMissingDependency(t *testing.T) {
	err := friendlyBuildError("no required module provides package example.com/driver; to add it:", "example.com/app")
	if !strings.Contains(err.Error(), "go get example.com/driver") {
		t.Fatalf("mensagem inesperada: %s", err)
	}
}

func TestFriendlyBuildErrorDoesNotSuggestGoGetForInternalPackage(t *testing.T) {
	const missing = "example.com/app/internal/modules/cidades/domain"
	err := friendlyBuildError(
		"no required module provides package "+missing+"; to add it:",
		"example.com/app",
	)
	message := err.Error()
	if strings.Contains(message, "go get") {
		t.Fatalf("não deve sugerir go get para pacote interno: %s", message)
	}
	if !strings.Contains(message, "ORM Reload") || !strings.Contains(message, "internal/modules/cidades/domain") {
		t.Fatalf("mensagem não orienta a correção local: %s", message)
	}
}

func TestRunnerContinuesAfterIndividualFactoryFailure(t *testing.T) {
	source, err := runnerSource(
		"example.com/app",
		config.FactoryConfig{Mapper: config.FactoryMapper{Path: "internal/factories"}},
		nil,
		"postgres",
	)
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if !strings.Contains(text, "failures = append(failures, message)") ||
		!strings.Contains(text, "as demais continuaram normalmente") {
		t.Fatalf("runner não acumula alertas:\n%s", text)
	}
	if strings.Contains(text, "if err != nil {\n\t\t\tfail(err.Error())") {
		t.Fatal("runner ainda encerra na primeira falha individual")
	}
}
