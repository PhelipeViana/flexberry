package connectioncheck

import (
	"errors"
	"strings"
	"testing"
)

func TestFriendlyError(t *testing.T) {
	tests := map[string]string{
		"connect: connection refused":                 "serviço indisponível",
		"password authentication failed for user app": "credenciais recusadas",
		"context deadline exceeded":                   "tempo de conexão esgotado",
		"unknown database flexberry":                  "não encontrado",
	}
	for input, expected := range tests {
		actual := friendlyError(errors.New(input))
		if !strings.Contains(actual, expected) {
			t.Fatalf("%q: recebido %q, esperado conter %q", input, actual, expected)
		}
	}
}
