package migraterun

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/PhelipeViana/flexberry/internal/migrategen"
)

var provisionalFunction = regexp.MustCompile(`func\s+(Migration\d{14})\s*\(`)

func finalizeProvisional(files []migrationFile) error {
	for _, file := range files {
		if !file.Provisional {
			continue
		}
		description := describeMigration(file.Plan.Operations)
		target := filepath.Join(filepath.Dir(file.Path), file.ID+"_"+description+"_migration.go")
		data, err := os.ReadFile(file.Path)
		if err != nil {
			return err
		}
		suffix := exportedDescription(description)
		data = provisionalFunction.ReplaceAll(data, []byte("func ${1}"+suffix+"("))
		temporary := file.Path + ".finalizing"
		if err := os.WriteFile(temporary, data, 0o644); err != nil {
			return err
		}
		if err := os.Rename(temporary, target); err != nil {
			_ = os.Remove(temporary)
			return fmt.Errorf("renomear %s: %w", file.Name, err)
		}
		if err := os.Remove(file.Path); err != nil {
			return fmt.Errorf("remover arquivo provisório %s: %w", file.Name, err)
		}
	}
	return nil
}

func describeMigration(operations []migrategen.Operation) string {
	if len(operations) != 1 {
		if len(operations) > 0 && operations[0].Table != "" {
			return "update_" + snakeName(operations[0].Table)
		}
		return "mixed_changes"
	}
	op := operations[0]
	switch op.Kind {
	case "create_table":
		return "create_" + snakeName(op.Table)
	case "drop_table":
		return "drop_" + snakeName(op.Table)
	case "rename_table":
		return "rename_" + snakeName(op.Table) + "_to_" + snakeName(op.NewName)
	case "add_column", "alter_column", "drop_column":
		column := "column"
		if op.Column != nil {
			column = op.Column.Name
		}
		verb := strings.TrimSuffix(op.Kind, "_column")
		return verb + "_" + snakeName(column) + "_to_" + snakeName(op.Table)
	default:
		name := op.Table
		if name == "" {
			name = op.Name
		}
		return snakeName(op.Kind + "_" + name)
	}
}

func snakeName(value string) string {
	var result []rune
	underscore := false
	for _, char := range strings.ToLower(value) {
		if unicode.IsLetter(char) || unicode.IsDigit(char) {
			result = append(result, char)
			underscore = false
		} else if len(result) > 0 && !underscore {
			result = append(result, '_')
			underscore = true
		}
	}
	return strings.Trim(string(result), "_")
}

func exportedDescription(value string) string {
	var result strings.Builder
	for _, part := range strings.Split(value, "_") {
		if part == "" {
			continue
		}
		result.WriteString(strings.ToUpper(part[:1]))
		result.WriteString(part[1:])
	}
	return result.String()
}
