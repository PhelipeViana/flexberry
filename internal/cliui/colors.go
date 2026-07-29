package cliui

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

const (
	ColorReset  = "\033[0m"
	ColorBlue   = "\033[34m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorCyan   = "\033[36m"
	ColorGray   = "\033[90m"
)

func Paint(color, value string) string {
	if os.Getenv("NO_COLOR") != "" || !term.IsTerminal(int(os.Stdout.Fd())) {
		return value
	}
	return color + value + ColorReset
}

func Title(value string) string   { return Paint(ColorBlue, value) }
func Success(value string) string { return Paint(ColorGreen, value) }
func Failure(value string) string { return Paint(ColorRed, value) }
func Warning(value string) string { return Paint(ColorYellow, value) }
func Info(value string) string    { return Paint(ColorCyan, value) }
func Muted(value string) string   { return Paint(ColorGray, value) }

func PrintTitle(value string) {
	fmt.Printf("\n%s\n\n", Title(value))
}
