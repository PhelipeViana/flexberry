package cliui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Issue struct {
	Message  string
	Solution string
}

type UserError struct {
	Issues []Issue
}

func NewUserError(message, solution string) error {
	return UserError{Issues: []Issue{{Message: message, Solution: solution}}}
}

func (e UserError) Error() string {
	if len(e.Issues) == 0 {
		return "operação não concluída"
	}
	return e.Issues[0].Message
}

func PresentError(err error) {
	issues := friendlyIssues(err)
	name := gitFirstName()
	greeting := "Olá"
	if name != "" {
		greeting = name
	}

	fmt.Fprintf(os.Stderr, "\n%s\n\n", Failure("❌ "+greeting+", encontramos alguns erros."))
	for index, issue := range issues {
		fmt.Fprintf(os.Stderr, "%s\n", Warning(fmt.Sprintf("⚠️  Erro %d", index+1)))
		fmt.Fprintf(os.Stderr, "   %s\n", issue.Message)
		if issue.Solution != "" {
			fmt.Fprintf(os.Stderr, "   %s %s\n", Info("→ Como corrigir:"), issue.Solution)
		}
		if index+1 < len(issues) {
			fmt.Fprintln(os.Stderr)
		}
	}
	fmt.Fprintln(os.Stderr)
}

func friendlyIssues(err error) []Issue {
	var userError UserError
	if errors.As(err, &userError) && len(userError.Issues) > 0 {
		return userError.Issues
	}

	detail := strings.TrimSpace(err.Error())
	lower := strings.ToLower(detail)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return []Issue{{
			Message:  "A configuração necessária não foi encontrada.",
			Solution: `Execute .\flexberry.exe e escolha "Flexberry Init".`,
		}}
	case strings.Contains(lower, "field") && strings.Contains(lower, "not found"),
		strings.Contains(lower, "cannot unmarshal"),
		strings.Contains(lower, "ler configuração"):
		return []Issue{{
			Message:  "O arquivo connection.yaml possui uma configuração inválida.",
			Solution: "Revise os campos e a indentação do YAML e execute novamente.",
		}}
	case strings.Contains(lower, "missing environment variable"):
		return []Issue{{
			Message:  "Uma variável obrigatória não foi preenchida no arquivo .env.",
			Solution: detail,
		}}
	case strings.Contains(lower, "exit status"):
		return []Issue{{
			Message:  "Uma ferramenta interna encerrou a execução antes de concluir.",
			Solution: "Confira a mensagem exibida imediatamente acima e tente novamente.",
		}}
	default:
		return []Issue{{
			Message:  "Não foi possível concluir a operação.",
			Solution: detail,
		}}
	}
}

func gitFirstName() string {
	for _, local := range []bool{true, false} {
		command := exec.Command("git", "config", "user.name")
		if !local {
			command.Args = []string{"git", "config", "--global", "user.name"}
		}
		output, err := command.Output()
		if err == nil {
			if name := normalizeFirstName(string(output)); name != "" {
				return name
			}
		}
	}
	return ""
}

func normalizeFirstName(value string) string {
	parts := strings.FieldsFunc(strings.TrimSpace(value), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	if len(parts) == 0 {
		return ""
	}
	first := strings.ToLower(parts[0])
	head, size := utf8.DecodeRuneInString(first)
	if head == utf8.RuneError && size == 0 {
		return ""
	}
	return string(unicode.ToUpper(head)) + first[size:]
}
