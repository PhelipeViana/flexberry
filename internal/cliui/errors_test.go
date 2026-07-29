package cliui

import "testing"

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
