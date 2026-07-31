package acao

import (
	"strings"
	"testing"
)

func TestValidarRejectsUnsafePhysicalTableNames(t *testing.T) {
	invalid := []string{"phelipe gabriel", "Pessoas", "pessoas-ativas", "pessoas__ativas", "1pessoas"}
	for _, name := range invalid {
		operation := Nova(CreateTable, name, NovaColuna("id").Integer()).Alias("pessoas")
		if err := Validar(operation); err == nil || !strings.Contains(err.Error(), "snake_case") {
			t.Fatalf("nome %q deveria ser recusado, obtido: %v", name, err)
		}
	}
}

func TestValidarAcceptsSnakeCasePhysicalTableName(t *testing.T) {
	operation := Nova(CreateTable, "phelipe_gabriel", NovaColuna("id").Integer()).Alias("phelipeGabriel")
	if err := Validar(operation); err != nil {
		t.Fatalf("nome snake_case deveria ser aceito: %v", err)
	}
}

func TestValidarRejectsUnsafeColumnNames(t *testing.T) {
	tests := []Operacao{
		Nova(CreateTable, "pessoas", NovaColuna("coluna separada tambem vai").Integer()).Alias("pessoas"),
		Nova(AddColumn, "pessoas", NovaColuna("nome completo").Varchar(255)),
		Nova(AlterColumn, "pessoas", NovaColuna("Nome").Varchar(255)),
		Nova(DropColumn, "pessoas", NovaColuna("nome-completo")),
		Nova(AddForeignKey, "pessoas", NovaColuna("cidade id").References("cidades", "id")),
	}
	for _, operation := range tests {
		if err := Validar(operation); err == nil || !strings.Contains(err.Error(), "nome de coluna") {
			t.Fatalf("operação %s deveria recusar coluna inválida, obtido: %v", operation.Kind, err)
		}
	}
}

func TestValidarAcceptsSnakeCaseColumnName(t *testing.T) {
	operation := Nova(AddColumn, "pessoas", NovaColuna("nome_completo").Varchar(255))
	if err := Validar(operation); err != nil {
		t.Fatalf("coluna snake_case deveria ser aceita: %v", err)
	}
}

func TestValidarRejectsUnsafeAliasName(t *testing.T) {
	operation := Nova(CreateTable, "phelipe_gabriel", NovaColuna("id").Integer()).Alias("phelipe gabriel")
	if err := Validar(operation); err == nil || !strings.Contains(err.Error(), "lowerCamelCase") {
		t.Fatalf("alias inválido deveria ser recusado, obtido: %v", err)
	}
}

func TestValidarRejectsUnsafeRenameTableName(t *testing.T) {
	operation := Operacao{Kind: string(RenameTable), Table: "pessoas", NewName: "Phelipe Gabriel"}
	if err := Validar(operation); err == nil || !strings.Contains(err.Error(), "snake_case") {
		t.Fatalf("RenameTable deveria recusar nome inválido, obtido: %v", err)
	}
}
