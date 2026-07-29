package factoryrun

import (
	"strings"
	"testing"
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
	err := friendlyBuildError("no required module provides package example.com/driver; to add it:")
	if !strings.Contains(err.Error(), "go get example.com/driver") {
		t.Fatalf("mensagem inesperada: %s", err)
	}
}
