package flexberry

import (
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"
)

// FakeString generates deterministic text and optionally limits its rune count.
func FakeString(index int, maxLength ...int) string {
	return limitUnique("Flexberry", index, optionalLimit(maxLength))
}

// FakeText generates a longer deterministic text.
func FakeText(index int, maxLength ...int) string {
	value := fmt.Sprintf("Texto Flexberry gerado para o registro %03d", positiveIndex(index)+1)
	return LimitText(value, optionalLimit(maxLength))
}

// FakeName generates a deterministic person name.
func FakeName(index int, maxLength ...int) string {
	return limitUnique("Pessoa Flexberry", index, optionalLimit(maxLength))
}

func FakeEmail(index int, maxLength ...int) string {
	value := fmt.Sprintf("usuario.%03d@flexberry.dev", positiveIndex(index)+1)
	limit := optionalLimit(maxLength)
	if limit <= 0 || utf8.RuneCountInString(value) <= limit {
		return value
	}
	compact := fmt.Sprintf("u%d@f.dev", positiveIndex(index)+1)
	return LimitText(compact, limit)
}

func FakeUsername(index int, maxLength ...int) string {
	return limitUnique("usuario", index, optionalLimit(maxLength))
}

func FakePhone(index int, maxLength ...int) string {
	number := 900000000 + positiveIndex(index)%99999999
	value := fmt.Sprintf("(65) %05d-%04d", number/10000, number%10000)
	return limitDocument(value, optionalLimit(maxLength))
}

func FakeCEP(index int, maxLength ...int) string {
	value := fmt.Sprintf("78%03d-%03d", positiveIndex(index)%1000, (positiveIndex(index)+100)%1000)
	return limitDocument(value, optionalLimit(maxLength))
}

func FakeCPF(index int, maxLength ...int) string {
	base := digitsFromNumber(100000000+positiveIndex(index), 9)
	first := cpfDigit(base, 10)
	second := cpfDigit(append(append([]int(nil), base...), first), 11)
	value := fmt.Sprintf("%d%d%d.%d%d%d.%d%d%d-%d%d",
		base[0], base[1], base[2], base[3], base[4], base[5],
		base[6], base[7], base[8], first, second)
	return limitDocument(value, optionalLimit(maxLength))
}

func FakeCNPJ(index int, maxLength ...int) string {
	base := digitsFromNumber(112223330001+positiveIndex(index), 12)
	first := weightedDigit(base, []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2})
	second := weightedDigit(append(append([]int(nil), base...), first), []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2})
	value := fmt.Sprintf("%d%d.%d%d%d.%d%d%d/%d%d%d%d-%d%d",
		base[0], base[1], base[2], base[3], base[4], base[5], base[6],
		base[7], base[8], base[9], base[10], base[11], first, second)
	return limitDocument(value, optionalLimit(maxLength))
}

func FakeUUID(index int, maxLength ...int) string {
	value := fmt.Sprintf("00000000-0000-4000-8000-%012d", positiveIndex(index)+1)
	return LimitText(value, optionalLimit(maxLength))
}

func FakeHash(index int, maxLength ...int) string {
	limit := optionalLimit(maxLength)
	if limit <= 0 {
		limit = 60
	}
	return limitUnique("$2y$12$flexberry", index, limit)
}

func FakePasswordHash(maxLength ...int) string {
	const value = "$2y$12$3YZte70BSGA0rDmtnRH1t.8M696/MOUR940JfvjeanBfGY/TTI6Ve"
	return LimitText(value, optionalLimit(maxLength))
}

func FakeCode(index int, prefix string, maxLength ...int) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "COD"
	}
	return limitUnique(prefix, index, optionalLimit(maxLength))
}

func FakeFileName(index int, maxLength ...int) string {
	value := fmt.Sprintf("arquivo_%03d.pdf", positiveIndex(index)+1)
	return LimitText(value, optionalLimit(maxLength))
}

func FakeUF(index int) string {
	values := []string{"MT", "SP", "RJ", "GO", "PR", "SC"}
	return values[positiveIndex(index)%len(values)]
}

func FakeCity(index int, maxLength ...int) string {
	values := []string{"Cuiabá", "Várzea Grande", "Rondonópolis", "Sinop", "São Paulo", "Goiânia"}
	return LimitText(values[positiveIndex(index)%len(values)], optionalLimit(maxLength))
}

func FakeStreet(index int, maxLength ...int) string {
	values := []string{"Rua das Flores", "Avenida Brasil", "Rua São José", "Avenida Mato Grosso"}
	return LimitText(values[positiveIndex(index)%len(values)], optionalLimit(maxLength))
}

func FakeInt(index int) int64 {
	return int64(positiveIndex(index) + 1)
}

func FakeIntRange(index int, min, max int64) int64 {
	if max < min {
		min, max = max, min
	}
	span := max - min + 1
	if span <= 0 {
		return min
	}
	return min + int64(positiveIndex(index))%span
}

func FakeDecimal(index, precision, scale int) float64 {
	if precision < 1 {
		precision = 10
	}
	if precision > 18 {
		precision = 18
	}
	if scale < 0 {
		scale = 0
	}
	if scale >= precision {
		scale = precision - 1
	}
	integerDigits := precision - scale
	maxInteger := int64(math.Pow10(integerDigits)) - 1
	integer := FakeIntRange(index, 1, maxInteger)
	factor := math.Pow10(scale)
	fraction := float64(positiveIndex(index)%int(factor)) / factor
	return math.Round((float64(integer)+fraction)*factor) / factor
}

func FakeBool(index int) bool {
	return positiveIndex(index)%2 == 0
}

func FakeChoice(index int, values ...string) string {
	if len(values) == 0 {
		return ""
	}
	return values[positiveIndex(index)%len(values)]
}

func FakeDate(index int) time.Time {
	return time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC).
		AddDate(0, 0, positiveIndex(index)%365)
}

func FakeDateTime(index int) time.Time {
	return FakeDate(index).Add(time.Duration(positiveIndex(index)%24) * time.Hour)
}

func FakeBytes(index, maxLength int) []byte {
	return []byte(FakeString(index, maxLength))
}

// LimitText safely limits text by Unicode characters, never in the middle of UTF-8.
// A non-positive limit means that no explicit limit was supplied.
func LimitText(value string, maxLength int) string {
	if maxLength <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= maxLength {
		return value
	}
	return string(runes[:maxLength])
}

func optionalLimit(values []int) int {
	if len(values) == 0 {
		return 0
	}
	return values[0]
}

func limitUnique(prefix string, index, maxLength int) string {
	suffix := fmt.Sprintf("-%03d", positiveIndex(index)+1)
	if maxLength <= 0 {
		return strings.TrimSuffix(prefix, "-") + suffix
	}
	if maxLength <= utf8.RuneCountInString(suffix) {
		return LimitText(fmt.Sprintf("%d", positiveIndex(index)+1), maxLength)
	}
	prefix = strings.TrimSuffix(strings.TrimSpace(prefix), "-")
	return LimitText(prefix, maxLength-utf8.RuneCountInString(suffix)) + suffix
}

func limitDocument(value string, maxLength int) string {
	if maxLength <= 0 || utf8.RuneCountInString(value) <= maxLength {
		return value
	}
	var digits strings.Builder
	for _, char := range value {
		if char >= '0' && char <= '9' {
			digits.WriteRune(char)
		}
	}
	return LimitText(digits.String(), maxLength)
}

func positiveIndex(index int) int {
	if index < 0 {
		return -index
	}
	return index
}

func digitsFromNumber(value, length int) []int {
	if value < 0 {
		value = -value
	}
	result := make([]int, length)
	for index := length - 1; index >= 0; index-- {
		result[index] = value % 10
		value /= 10
	}
	return result
}

func cpfDigit(values []int, weight int) int {
	sum := 0
	for _, value := range values {
		sum += value * weight
		weight--
	}
	remainder := sum % 11
	if remainder < 2 {
		return 0
	}
	return 11 - remainder
}

func weightedDigit(values, weights []int) int {
	sum := 0
	for index, value := range values {
		sum += value * weights[index]
	}
	remainder := sum % 11
	if remainder < 2 {
		return 0
	}
	return 11 - remainder
}
