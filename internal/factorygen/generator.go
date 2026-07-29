package factorygen

import (
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/PhelipeViana/flexberry/internal/config"
	"github.com/PhelipeViana/flexberry/internal/scanner"
)

type Result struct {
	Created, Updated, Preserved, Disabled int
}

var fieldExpression = regexp.MustCompile(`(?m)^\s*"([^"]+)"\s*:\s*(.+),\s*$`)

func Generate(root, modulePath string, cfg config.FactoryConfig, orm config.ORMConfig, entities []scanner.Entity) (Result, error) {
	if err := validateRelationTargets(entities); err != nil {
		return Result{}, err
	}
	output := filepath.Join(root, filepath.FromSlash(cfg.Mapper.Path))
	relative, err := filepath.Rel(root, output)
	if err != nil || relative == "." || relative == ".." || filepath.IsAbs(relative) {
		return Result{}, fmt.Errorf("factory mapper.path precisa ficar dentro do projeto")
	}
	if err := os.MkdirAll(output, 0o755); err != nil {
		return Result{}, err
	}
	var result Result
	expected := make(map[string]bool, len(entities))
	for _, entity := range entities {
		filename := snake(entity.Name) + "_factory.go"
		expected[filename] = true
		path := filepath.Join(output, filename)
		existing := make(map[string]string)
		current, readErr := os.ReadFile(path)
		if readErr == nil {
			for _, match := range fieldExpression.FindAllStringSubmatch(string(current), -1) {
				existing[normalize(match[1])] = strings.TrimSpace(match[2])
			}
		} else if !os.IsNotExist(readErr) {
			return result, readErr
		}
		rendered, err := render(modulePath, cfg, orm, entity, entities, existing)
		if err != nil {
			return result, err
		}
		if readErr == nil && string(current) == string(rendered) {
			result.Preserved++
			continue
		}
		if err := os.WriteFile(path, rendered, 0o644); err != nil {
			return result, err
		}
		if readErr == nil {
			result.Updated++
		} else {
			result.Created++
		}
	}
	entries, err := os.ReadDir(output)
	if err != nil {
		return result, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_factory.go") || expected[entry.Name()] {
			continue
		}
		source := filepath.Join(output, entry.Name())
		target := nextDisabledPath(source)
		if err := os.Rename(source, target); err != nil {
			return result, fmt.Errorf("desativar factory sem entidade %s: %w", entry.Name(), err)
		}
		result.Disabled++
	}
	return result, nil
}

func nextDisabledPath(source string) string {
	target := source + ".disabled"
	for index := 2; ; index++ {
		if _, err := os.Stat(target); os.IsNotExist(err) {
			return target
		}
		target = fmt.Sprintf("%s.disabled.%d", source, index)
	}
}

func validateRelationTargets(entities []scanner.Entity) error {
	available := make(map[string]bool, len(entities))
	for _, entity := range entities {
		available[entity.Name] = true
	}
	for _, entity := range entities {
		for _, relation := range entity.Relations {
			if relation.Kind != "belongsTo" || relation.ForeignKey == "" {
				continue
			}
			target := typeName(relation.Type)
			if !available[target] {
				return fmt.Errorf(
					"%s.%s referencia a entidade %s, mas ela não foi encontrada; verifique entities.paths e o arquivo da entidade antes de executar Factory Reload",
					entity.Name,
					relation.Name,
					target,
				)
			}
		}
	}
	return nil
}

func render(modulePath string, cfg config.FactoryConfig, orm config.ORMConfig, entity scanner.Entity, entities []scanner.Entity, existing map[string]string) ([]byte, error) {
	ormImport := strings.TrimSuffix(modulePath, "/") + "/" + strings.Trim(filepath.ToSlash(orm.Output.Path), "/")
	var b strings.Builder
	fmt.Fprintf(&b, "package %s\n\n", cfg.Mapper.Package)
	b.WriteString("import (\n")
	b.WriteString("\tflexberry \"github.com/PhelipeViana/flexberry\"\n")
	fmt.Fprintf(&b, "\torm %q\n", ormImport)
	b.WriteString(")\n\n")
	fmt.Fprintf(&b, "// %sFactory gera dados para %s.\n", entity.Name, entity.Table)
	fmt.Fprintf(&b, "func %sFactory() flexberry.Factory {\n", entity.Name)
	b.WriteString("\treturn flexberry.Factory{\n")
	fmt.Fprintf(&b, "\t\tName: %q,\n\t\tMapping: orm.%s().Mapping,\n", entity.Name+"Factory", entity.Function)
	fmt.Fprintf(&b, "\t\tCount: %d,\n\t\tUpdate: %t,\n\t\tActive: %t,\n", cfg.Defaults.Count, cfg.Defaults.Update, cfg.Defaults.Active)
	fmt.Fprintf(&b, "\t\tDependencies: %#v,\n", dependencies(entity, entities))
	b.WriteString("\t\tData: func(index int) map[string]any {\n\t\t\treturn map[string]any{\n")
	fields := append([]scanner.Field(nil), entity.Fields...)
	sort.Slice(fields, func(i, j int) bool { return fields[i].Column < fields[j].Column })
	links := relationLinks(entity, entities)
	for _, field := range fields {
		if field.PrimaryKey {
			continue
		}
		expression := ""
		if link, ok := links[normalize(field.Column)]; ok {
			expression = fmt.Sprintf("flexberry.Vinculo(%q, %q)", link.table, link.column)
		} else if configured := configuredExpression(cfg, entity.Table, field.Column); configured != "" {
			// factory.yaml intercepta qualquer expressão gerada anteriormente.
			// Dessa forma, alterar exact/contains e executar Reload atualiza a factory.
			expression = configured
		} else {
			expression = existing[normalize(field.Column)]
		}
		if expression == "" {
			expression = defaultExpression(field)
		}
		fmt.Fprintf(&b, "\t\t\t\t%q: %s,\n", field.Column, expression)
	}
	b.WriteString("\t\t\t}\n\t\t},\n\t}\n}\n")
	return format.Source([]byte(b.String()))
}

type link struct{ table, column string }

func relationLinks(entity scanner.Entity, entities []scanner.Entity) map[string]link {
	result := make(map[string]link)
	for _, relation := range entity.Relations {
		if relation.Kind != "belongsTo" || relation.ForeignKey == "" {
			continue
		}
		name := typeName(relation.Type)
		for _, parent := range entities {
			if parent.Name == name && parent.PrimaryKey != "" {
				result[normalize(relation.ForeignKey)] = link{parent.Table, parent.PrimaryKey}
			}
		}
	}
	return result
}

func dependencies(entity scanner.Entity, entities []scanner.Entity) []string {
	links := relationLinks(entity, entities)
	seen := make(map[string]bool)
	var result []string
	for _, value := range links {
		if !seen[value.table] && value.table != entity.Table {
			seen[value.table] = true
			result = append(result, value.table)
		}
	}
	sort.Strings(result)
	return result
}

func configuredExpression(cfg config.FactoryConfig, table, column string) string {
	tableKey, columnKey := normalize(table), normalize(column)
	for key, expression := range cfg.Expressions.Exact {
		left, right, specific := strings.Cut(key, ".")
		if specific && normalize(left) == tableKey && normalize(right) == columnKey {
			return strings.TrimSpace(expression)
		}
	}
	for key, expression := range cfg.Expressions.Exact {
		if !strings.Contains(key, ".") && normalize(key) == columnKey {
			return strings.TrimSpace(expression)
		}
	}
	for _, rule := range cfg.Expressions.Contains {
		if strings.Contains(columnKey, normalize(rule.Pattern)) {
			return strings.TrimSpace(rule.Expression)
		}
	}
	return ""
}

func defaultExpression(field scanner.Field) string {
	value := strings.TrimPrefix(field.GoType, "*")
	switch value {
	case "string":
		return "flexberry.FakeString(index)"
	case "bool":
		return "flexberry.FakeBool(index)"
	case "float32", "float64":
		return "flexberry.FakeDecimal(index, 10, 2)"
	case "time.Time":
		return "flexberry.FakeDateTime(index)"
	case "[]byte":
		return "flexberry.FakeBytes(index, 128)"
	default:
		return "flexberry.FakeInt(index)"
	}
}

func normalize(value string) string {
	var result strings.Builder
	for _, char := range strings.ToUpper(strings.TrimSpace(value)) {
		if unicode.IsLetter(char) || unicode.IsDigit(char) {
			result.WriteRune(char)
		}
	}
	return result.String()
}

func snake(value string) string {
	var result []rune
	for index, char := range value {
		if index > 0 && unicode.IsUpper(char) {
			result = append(result, '_')
		}
		result = append(result, unicode.ToLower(char))
	}
	return string(result)
}

func typeName(value string) string {
	value = strings.TrimLeft(value, "*[]")
	if index := strings.LastIndex(value, "."); index >= 0 {
		return value[index+1:]
	}
	return value
}
