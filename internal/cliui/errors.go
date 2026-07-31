package cliui

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Issue struct {
	Message  string
	Location string
	Detail   string
	Solution string
}

var migrationErrorPattern = regexp.MustCompile(`^(.*\.go):(\d+)(?::(\d+))?:\s*(.+)$`)

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

	presentIssues(os.Stderr, greeting, issues)
}

func presentIssues(writer io.Writer, greeting string, issues []Issue) {
	fmt.Fprintf(writer, "\n%s\n", Failure("❌ "+greeting+", encontramos alguns erros."))
	for index, issue := range issues {
		fmt.Fprintf(writer, "\n%s\n", Warning(fmt.Sprintf("⚠ Erro %d", index+1)))
		printIssueField(writer, "Problema", issue.Message)
		if issue.Location != "" {
			printIssueField(writer, "Local", issue.Location)
		}
		if issue.Detail != "" && issue.Detail != issue.Message {
			printIssueField(writer, "Detalhes", issue.Detail)
		}
		if issue.Solution != "" {
			printIssueField(writer, "Como corrigir", issue.Solution)
		}
	}
	fprintln(writer)
}

func printIssueField(writer io.Writer, label, value string) {
	fmt.Fprintf(writer, "\n  %s\n", Info(label+":"))
	for _, line := range strings.Split(strings.TrimSpace(value), "\n") {
		fmt.Fprintf(writer, "    %s\n", line)
	}
}

func fprintln(writer io.Writer) {
	fmt.Fprintln(writer)
}

func friendlyIssues(err error) []Issue {
	var userError UserError
	if errors.As(err, &userError) && len(userError.Issues) > 0 {
		return userError.Issues
	}

	detail := strings.TrimSpace(err.Error())
	if issue, ok := migrationIssue(detail); ok {
		return []Issue{issue}
	}
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
			Message: "Não foi possível concluir a operação.",
			Detail:  detail,
		}}
	}
}

func migrationIssue(detail string) (Issue, bool) {
	const prefix = "ler migration "
	if !strings.HasPrefix(detail, prefix) {
		return Issue{}, false
	}
	remainder := strings.TrimPrefix(detail, prefix)
	if separator := strings.Index(remainder, ": "); separator >= 0 {
		remainder = remainder[separator+2:]
	}
	match := migrationErrorPattern.FindStringSubmatch(remainder)
	if len(match) == 0 {
		return Issue{}, false
	}
	location := shortLocation(match[1]) + ":" + match[2]
	if match[3] != "" {
		location += ":" + match[3]
	}
	problem := strings.TrimSpace(match[4])
	issue := Issue{
		Message:  "A migration contém uma definição inválida.",
		Location: location,
		Detail:   problem,
		Solution: "Corrija a definição indicada e execute Migration Run novamente.",
	}
	lower := strings.ToLower(problem)
	if strings.Contains(lower, "nome de tabela") && strings.Contains(lower, "inválido") {
		issue.Message = "O nome físico da tabela é inválido."
		issue.Solution = `Use snake_case, sem espaços. Exemplo: migrate.CreateTable("phelipe_gabriel", ...).`
	} else if strings.Contains(lower, "nome de coluna") && strings.Contains(lower, "inválido") {
		issue.Message = "O nome físico da coluna é inválido."
		issue.Solution = `Use snake_case, sem espaços. Exemplo: migrate.Col("coluna_separada_tambem_vai").`
	} else if strings.Contains(lower, "alias") && strings.Contains(lower, "inválido") {
		issue.Message = "O alias da tabela é inválido."
		issue.Solution = `Use lowerCamelCase, sem espaços. Exemplo: .Alias("phelipeGabriel").`
	}
	return issue, true
}

func shortLocation(path string) string {
	clean := filepath.Clean(path)
	workingDirectory, err := os.Getwd()
	if err != nil {
		return clean
	}
	relative, err := filepath.Rel(workingDirectory, clean)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return clean
	}
	return relative
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
