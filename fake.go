package flexberry

import "fmt"

func FakeString(index int) string { return fmt.Sprintf("Flexberry %03d", index+1) }
func FakeName(index int) string   { return fmt.Sprintf("Pessoa Flexberry %03d", index+1) }
func FakeInt(index int) int64     { return int64(index + 1) }
func FakeBool(index int) bool     { return index%2 == 0 }
