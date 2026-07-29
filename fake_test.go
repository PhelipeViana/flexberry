package flexberry

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestFakeTextHelpersRespectRuneLimit(t *testing.T) {
	tests := []struct {
		name  string
		value string
		limit int
	}{
		{"string", FakeString(10, 8), 8},
		{"name", FakeName(10, 12), 12},
		{"email", FakeEmail(10, 12), 12},
		{"phone", FakePhone(10, 11), 11},
		{"city", FakeCity(1, 6), 6},
		{"unicode", LimitText("Várzea Grande", 6), 6},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !utf8.ValidString(test.value) {
				t.Fatalf("texto UTF-8 inválido: %q", test.value)
			}
			if got := utf8.RuneCountInString(test.value); got > test.limit {
				t.Fatalf("tamanho %d ultrapassa limite %d: %q", got, test.limit, test.value)
			}
		})
	}
}

func TestFakeUniqueValuesKeepIndexWithinLimit(t *testing.T) {
	first := FakeName(0, 12)
	second := FakeName(1, 12)
	if first == second {
		t.Fatalf("valores perderam unicidade após limite: %q", first)
	}
	if !strings.HasSuffix(first, "-001") || !strings.HasSuffix(second, "-002") {
		t.Fatalf("sufixos determinísticos não foram preservados: %q, %q", first, second)
	}
}

func TestFakeChoiceAndRangeVaryByIndex(t *testing.T) {
	if FakeChoice(0, "A", "B") == FakeChoice(1, "A", "B") {
		t.Fatal("FakeChoice não variou pelo index")
	}
	if FakeIntRange(0, 3, 4) != 3 || FakeIntRange(1, 3, 4) != 4 || FakeIntRange(2, 3, 4) != 3 {
		t.Fatal("FakeIntRange não respeitou intervalo e ciclo")
	}
}

func TestFakeDocumentsRespectFormattedAndNumericLengths(t *testing.T) {
	if got := FakeCPF(0, 14); utf8.RuneCountInString(got) != 14 {
		t.Fatalf("CPF formatado inválido: %q", got)
	}
	if got := FakeCPF(0, 11); len(got) != 11 || strings.ContainsAny(got, ".-/") {
		t.Fatalf("CPF numérico inválido: %q", got)
	}
	if got := FakeCNPJ(0, 18); utf8.RuneCountInString(got) != 18 {
		t.Fatalf("CNPJ formatado inválido: %q", got)
	}
}
