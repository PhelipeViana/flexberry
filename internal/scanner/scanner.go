package scanner

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/PhelipeViana/flexberry/internal/config"
)

func Scan(projectRoot string, cfg *config.Config) (Result, error) {
	return scan(projectRoot, cfg, false)
}

// ScanLenient ignores source files whose local imports are missing and reports
// warnings. It is used by factories so one incomplete module does not prevent
// valid factories from being generated and executed.
func ScanLenient(projectRoot string, cfg *config.Config) (Result, error) {
	result, err := scan(projectRoot, cfg, true)
	if err != nil {
		return Result{}, err
	}
	return pruneUnresolvedRelations(result), nil
}

func scan(projectRoot string, cfg *config.Config, lenient bool) (Result, error) {
	modulePath, err := readModulePath(filepath.Join(projectRoot, "go.mod"))
	if err != nil {
		return Result{}, err
	}

	includes, err := compilePatterns(cfg.Entities.Paths)
	if err != nil {
		return Result{}, fmt.Errorf("compilar entities.paths: %w", err)
	}
	excludes, err := compilePatterns(cfg.Entities.Exclude)
	if err != nil {
		return Result{}, fmt.Errorf("compilar entities.exclude: %w", err)
	}

	var files []string
	err = filepath.WalkDir(projectRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			name := entry.Name()
			if path != projectRoot && (name == ".git" || name == "vendor" || name == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") {
			return nil
		}
		relative, err := filepath.Rel(projectRoot, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if matchesAny(relative, includes) && !matchesAny(relative, excludes) {
			files = append(files, relative)
		}
		return nil
	})
	if err != nil {
		return Result{}, fmt.Errorf("procurar entidades: %w", err)
	}
	sort.Strings(files)
	if len(files) == 0 {
		return Result{}, fmt.Errorf("nenhum arquivo encontrado em entities.paths")
	}

	var entities []Entity
	var warnings []string
	for _, relative := range files {
		if problem, err := missingLocalImport(projectRoot, modulePath, relative); err != nil {
			if !lenient {
				return Result{}, err
			}
			warnings = append(warnings, fmt.Sprintf(
				"%s ignorado: arquivo vazio ou código Go inválido: %v",
				relative, err,
			))
			continue
		} else if problem != "" {
			if !lenient {
				return Result{}, fmt.Errorf("%s", problem)
			}
			warnings = append(warnings, problem)
			continue
		}
		found, err := scanFile(projectRoot, modulePath, relative, cfg.Entities.Overrides)
		if err != nil {
			if lenient {
				warnings = append(warnings, fmt.Sprintf(
					"%s ignorado: arquivo vazio ou código Go inválido: %v",
					relative, err,
				))
				continue
			}
			return Result{}, err
		}
		if len(found) == 0 && lenient {
			warnings = append(warnings, fmt.Sprintf(
				"%s ignorado: não contém entidade exportada com campos mapeados pela tag db",
				relative,
			))
			continue
		}
		entities = append(entities, found...)
	}
	if len(entities) == 0 && len(warnings) == 0 {
		return Result{}, fmt.Errorf("nenhuma struct com tags db encontrada nos arquivos mapeados")
	}

	assignGeneratedNames(entities)
	sort.Slice(entities, func(i, j int) bool {
		if entities[i].ImportPath == entities[j].ImportPath {
			return entities[i].Name < entities[j].Name
		}
		return entities[i].ImportPath < entities[j].ImportPath
	})
	return Result{Entities: entities, Files: files, Warnings: warnings}, nil
}

func missingLocalImport(projectRoot, modulePath, relative string) (string, error) {
	path := filepath.Join(projectRoot, filepath.FromSlash(relative))
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		return "", fmt.Errorf("analisar imports de %s: %w", relative, err)
	}
	prefix := strings.TrimSuffix(modulePath, "/") + "/"
	for _, specification := range file.Imports {
		importPath, err := strconv.Unquote(specification.Path.Value)
		if err != nil || !strings.HasPrefix(importPath, prefix) {
			continue
		}
		local := strings.TrimPrefix(importPath, prefix)
		directory := filepath.Join(projectRoot, filepath.FromSlash(local))
		entries, err := os.ReadDir(directory)
		if os.IsNotExist(err) {
			return fmt.Sprintf(
				"%s ignorado: pacote interno %q não existe; crie a entidade ou corrija o import",
				relative, local,
			), nil
		}
		if err != nil {
			return "", fmt.Errorf("ler pacote interno %s: %w", local, err)
		}
		hasSource := false
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") && !strings.HasSuffix(entry.Name(), "_test.go") {
				hasSource = true
				break
			}
		}
		if !hasSource {
			return fmt.Sprintf(
				"%s ignorado: pacote interno %q está vazio; crie a entidade ou corrija o import",
				relative, local,
			), nil
		}
	}
	return "", nil
}

func pruneUnresolvedRelations(result Result) Result {
	for {
		available := make(map[string]bool, len(result.Entities))
		for _, entity := range result.Entities {
			available[entity.Name] = true
		}
		removed := false
		filtered := make([]Entity, 0, len(result.Entities))
		for _, entity := range result.Entities {
			missing := ""
			for _, relation := range entity.Relations {
				if relation.Kind != "belongsTo" || relation.ForeignKey == "" {
					continue
				}
				target := relationTypeName(relation.Type)
				if !available[target] {
					missing = target
					break
				}
			}
			if missing != "" {
				result.Warnings = append(result.Warnings, fmt.Sprintf(
					"entidade %s ignorada: depende da entidade ausente %s",
					entity.Name, missing,
				))
				removed = true
				continue
			}
			filtered = append(filtered, entity)
		}
		result.Entities = filtered
		if !removed {
			return result
		}
	}
}

func relationTypeName(value string) string {
	value = strings.TrimLeft(value, "*[]")
	if index := strings.LastIndex(value, "."); index >= 0 {
		return value[index+1:]
	}
	return value
}

func scanFile(projectRoot, modulePath, relative string, overrides map[string]config.EntityOverride) ([]Entity, error) {
	path := filepath.Join(projectRoot, filepath.FromSlash(relative))
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("analisar %s: %w", relative, err)
	}

	directory := filepath.ToSlash(filepath.Dir(relative))
	importPath := strings.TrimSuffix(modulePath, "/")
	if directory != "." {
		importPath += "/" + strings.TrimPrefix(directory, "./")
	}

	var entities []Entity
	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.TYPE {
			continue
		}
		for _, specification := range general.Specs {
			typeSpec, ok := specification.(*ast.TypeSpec)
			if !ok || !typeSpec.Name.IsExported() {
				continue
			}
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				continue
			}

			entity, ok, err := mapStruct(fileSet, file.Name.Name, importPath, relative, typeSpec.Name.Name, structType)
			if err != nil {
				return nil, fmt.Errorf("%s: entidade %s: %w", relative, typeSpec.Name.Name, err)
			}
			if !ok {
				continue
			}
			applyOverride(&entity, overrides)
			entities = append(entities, entity)
		}
	}
	return entities, nil
}

func mapStruct(fileSet *token.FileSet, packageName, importPath, relative, name string, value *ast.StructType) (Entity, bool, error) {
	entity := Entity{
		Name:       name,
		Package:    packageName,
		ImportPath: importPath,
		SourceFile: relative,
		Table:      pluralTable(snakeCase(name)),
	}

	var relationCandidates []Relation
	for _, field := range value.Fields.List {
		if len(field.Names) == 0 {
			continue
		}
		goType, err := formatExpression(fileSet, field.Type)
		if err != nil {
			return Entity{}, false, err
		}
		dbColumn := structTag(field, "db")

		for _, fieldName := range field.Names {
			if !fieldName.IsExported() {
				continue
			}
			if dbColumn != "" && dbColumn != "-" {
				entity.Fields = append(entity.Fields, Field{
					Name:     fieldName.Name,
					Column:   strings.Split(dbColumn, ",")[0],
					GoType:   goType,
					Nullable: isPointer(field.Type),
				})
				continue
			}
			if relation, ok := relationCandidate(fieldName.Name, field.Type, goType); ok {
				relationCandidates = append(relationCandidates, relation)
			}
		}
	}
	if len(entity.Fields) == 0 {
		return Entity{}, false, nil
	}

	entity.PrimaryKey = inferPrimaryKey(name, entity.Fields)
	for index := range entity.Fields {
		entity.Fields[index].PrimaryKey = entity.Fields[index].Column == entity.PrimaryKey
	}
	entity.Relations = inferRelations(entity.Fields, relationCandidates)
	return entity, true, nil
}

func inferPrimaryKey(entityName string, fields []Field) string {
	expectedField := strings.ToLower(entityName + "Id")
	expectedColumn := strings.ToUpper(snakeCase(entityName) + "_id")
	for _, field := range fields {
		if strings.ToLower(field.Name) == expectedField || strings.ToUpper(field.Column) == expectedColumn {
			return field.Column
		}
	}
	for _, field := range fields {
		if strings.EqualFold(field.Name, "ID") || strings.EqualFold(field.Column, "ID") {
			return field.Column
		}
	}
	return ""
}

func relationCandidate(name string, expression ast.Expr, goType string) (Relation, bool) {
	switch typed := expression.(type) {
	case *ast.StarExpr:
		if isNamedType(typed.X) {
			return Relation{Name: name, Type: strings.TrimPrefix(goType, "*"), Kind: "belongsTo"}, true
		}
	case *ast.ArrayType:
		if isNamedType(typed.Elt) {
			return Relation{Name: name, Type: strings.TrimPrefix(goType, "[]"), Kind: "hasMany"}, true
		}
	case *ast.Ident, *ast.SelectorExpr:
		if isNamedType(expression) && goType != "time.Time" {
			return Relation{Name: name, Type: goType, Kind: "belongsTo"}, true
		}
	}
	return Relation{}, false
}

func inferRelations(fields []Field, candidates []Relation) []Relation {
	var relations []Relation
	for _, relation := range candidates {
		expected := strings.ToLower(relation.Name + "Id")
		for _, field := range fields {
			if strings.ToLower(field.Name) == expected {
				relation.ForeignKey = field.Column
				break
			}
		}
		if relation.Kind == "hasMany" || relation.ForeignKey != "" {
			relations = append(relations, relation)
		}
	}
	return relations
}

func applyOverride(entity *Entity, overrides map[string]config.EntityOverride) {
	if len(overrides) == 0 {
		return
	}
	keys := []string{
		entity.ImportPath + "." + entity.Name,
		entity.Package + "." + entity.Name,
		entity.Name,
	}
	for _, key := range keys {
		override, ok := overrides[key]
		if !ok {
			continue
		}
		if override.Table != "" {
			entity.Table = override.Table
		}
		if override.PrimaryKey != "" {
			entity.PrimaryKey = override.PrimaryKey
		}
		if override.Connection != "" {
			entity.Connection = override.Connection
		}
		break
	}
	for index := range entity.Fields {
		entity.Fields[index].PrimaryKey = entity.Fields[index].Column == entity.PrimaryKey
	}
}

func assignGeneratedNames(entities []Entity) {
	counts := make(map[string]int)
	aliases := make(map[string]string)
	usedAliases := make(map[string]int)
	for _, entity := range entities {
		counts[entity.Name]++
		if _, ok := aliases[entity.ImportPath]; !ok {
			base := importAlias(entity.ImportPath)
			usedAliases[base]++
			if usedAliases[base] > 1 {
				base += strconv.Itoa(usedAliases[base])
			}
			aliases[entity.ImportPath] = base
		}
	}
	for index := range entities {
		entities[index].Alias = aliases[entities[index].ImportPath]
		entities[index].Function = entities[index].Name
		if counts[entities[index].Name] > 1 {
			entities[index].Function = exportedName(filepath.Base(filepath.Dir(entities[index].ImportPath))) + entities[index].Name
		}
	}
}

func compilePatterns(values []string) ([]*regexp.Regexp, error) {
	patterns := make([]*regexp.Regexp, 0, len(values))
	for _, value := range values {
		pattern, err := globRegex(value)
		if err != nil {
			return nil, err
		}
		patterns = append(patterns, pattern)
	}
	return patterns, nil
}

func readModulePath(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("abrir go.mod: %w", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && fields[0] == "module" {
			return fields[1], nil
		}
	}
	return "", fmt.Errorf("module não encontrado em go.mod")
}

func formatExpression(fileSet *token.FileSet, expression ast.Expr) (string, error) {
	var builder strings.Builder
	if err := format.Node(&builder, fileSet, expression); err != nil {
		return "", err
	}
	return builder.String(), nil
}

func structTag(field *ast.Field, key string) string {
	if field.Tag == nil {
		return ""
	}
	unquoted, err := strconv.Unquote(field.Tag.Value)
	if err != nil {
		return ""
	}
	return reflect.StructTag(unquoted).Get(key)
}

func isPointer(value ast.Expr) bool {
	_, ok := value.(*ast.StarExpr)
	return ok
}

func isNamedType(value ast.Expr) bool {
	switch value.(type) {
	case *ast.Ident, *ast.SelectorExpr:
		return true
	default:
		return false
	}
}

func snakeCase(value string) string {
	var result []rune
	runes := []rune(value)
	for index, current := range runes {
		if unicode.IsUpper(current) {
			if index > 0 && (unicode.IsLower(runes[index-1]) || index+1 < len(runes) && unicode.IsLower(runes[index+1])) {
				result = append(result, '_')
			}
			result = append(result, unicode.ToLower(current))
		} else {
			result = append(result, current)
		}
	}
	return string(result)
}

func pluralTable(value string) string {
	if strings.HasSuffix(value, "s") {
		return value
	}
	return value + "s"
}

func importAlias(importPath string) string {
	base := filepath.Base(importPath)
	parent := filepath.Base(filepath.Dir(importPath))
	if base == "domain" && parent != "." {
		base = parent + "domain"
	}
	var cleaned []rune
	for _, char := range base {
		if unicode.IsLetter(char) || unicode.IsDigit(char) || char == '_' {
			cleaned = append(cleaned, char)
		}
	}
	return string(cleaned)
}

func exportedName(value string) string {
	parts := strings.FieldsFunc(value, func(char rune) bool {
		return char == '_' || char == '-' || char == ' '
	})
	var builder strings.Builder
	for _, part := range parts {
		runes := []rune(part)
		if len(runes) == 0 {
			continue
		}
		builder.WriteRune(unicode.ToUpper(runes[0]))
		builder.WriteString(string(runes[1:]))
	}
	return builder.String()
}
