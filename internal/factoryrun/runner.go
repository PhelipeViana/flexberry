package factoryrun

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/PhelipeViana/flexberry/internal/config"
	"github.com/PhelipeViana/flexberry/internal/scanner"
)

func Run(root, modulePath string, factoryConfig config.FactoryConfig, entities []scanner.Entity, connectionName string, connection config.Connection) error {
	factoryPath := filepath.Join(root, filepath.FromSlash(factoryConfig.Mapper.Path))
	if err := validateFactoryPackage(factoryPath, factoryConfig.Mapper.Path); err != nil {
		return err
	}
	ordered, err := orderEntities(entities)
	if err != nil {
		return err
	}
	source, err := runnerSource(modulePath, factoryConfig, ordered, connection.Dialect)
	if err != nil {
		return err
	}
	folder := filepath.Join(root, ".flexberry-runner")
	relative, err := filepath.Rel(root, folder)
	if err != nil || filepath.ToSlash(relative) != ".flexberry-runner" {
		return fmt.Errorf("pasta temporária inválida")
	}
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return err
	}
	defer os.RemoveAll(folder)
	if err := os.WriteFile(filepath.Join(folder, "main.go"), source, 0o600); err != nil {
		return err
	}
	runnerName := "flexberry-runner"
	if runtime.GOOS == "windows" {
		runnerName += ".exe"
	}
	runnerPath := filepath.Join(folder, runnerName)
	build := exec.Command("go", "build", "-o", runnerPath, "./.flexberry-runner")
	build.Dir = root
	if output, buildErr := build.CombinedOutput(); buildErr != nil {
		return friendlyBuildError(string(output))
	}

	command := exec.Command(runnerPath)
	command.Dir = root
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Stdin = os.Stdin
	command.Env = append(os.Environ(),
		"FLEXBERRY_RUN_CONNECTION="+connectionName,
		"FLEXBERRY_RUN_DIALECT="+connection.Dialect,
		"FLEXBERRY_RUN_DSN="+connection.URL,
	)
	if err := command.Run(); err != nil {
		return fmt.Errorf("a execução das factories foi interrompida; consulte a mensagem exibida acima")
	}
	return nil
}

func validateFactoryPackage(path, configured string) error {
	entries, err := os.ReadDir(path)
	if os.IsNotExist(err) {
		return fmt.Errorf(
			"factories ainda não foram geradas em %q; execute .\\flexberry.exe factory reload",
			filepath.ToSlash(configured),
		)
	}
	if err != nil {
		return fmt.Errorf("ler pasta das factories: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") && !strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
	}
	return fmt.Errorf(
		"nenhuma factory foi encontrada em %q; execute .\\flexberry.exe factory reload",
		filepath.ToSlash(configured),
	)
}

var missingModule = regexp.MustCompile(`no required module provides package ([^;\s]+)`)

func friendlyBuildError(output string) error {
	detail := strings.TrimSpace(output)
	if match := missingModule.FindStringSubmatch(detail); len(match) == 2 {
		return fmt.Errorf(
			"dependência Go ausente: %s; execute go get %s e tente novamente",
			match[1],
			match[1],
		)
	}
	if detail == "" {
		detail = "o compilador não informou detalhes"
	}
	return fmt.Errorf("não foi possível preparar o executor das factories: %s", detail)
}

func runnerSource(modulePath string, cfg config.FactoryConfig, entities []scanner.Entity, dialect string) ([]byte, error) {
	factoryImport := strings.TrimSuffix(modulePath, "/") + "/" + strings.Trim(filepath.ToSlash(cfg.Mapper.Path), "/")
	driverImports := map[string]string{
		"oracle":    "github.com/sijms/go-ora/v2",
		"postgres":  "github.com/jackc/pgx/v5/stdlib",
		"mysql":     "github.com/go-sql-driver/mysql",
		"sqlserver": "github.com/denisenkom/go-mssqldb",
	}
	driverImport, ok := driverImports[dialect]
	if !ok {
		return nil, fmt.Errorf("dialeto não suportado: %s", dialect)
	}
	var b bytes.Buffer
	b.WriteString("package main\n\nimport (\n\t\"context\"\n\t\"database/sql\"\n\t\"fmt\"\n\t\"os\"\n\n")
	b.WriteString("\tflexberry \"github.com/PhelipeViana/flexberry\"\n")
	fmt.Fprintf(&b, "\tfactories %q\n", factoryImport)
	fmt.Fprintf(&b, "\t_ %q\n)\n\n", driverImport)
	b.WriteString(`func main() {
	ctx := context.Background()
	dialect := os.Getenv("FLEXBERRY_RUN_DIALECT")
	driver := map[string]string{"oracle":"oracle","postgres":"pgx","mysql":"mysql","sqlserver":"sqlserver"}[dialect]
	if driver == "" { fail("dialeto não suportado: " + dialect) }
	fmt.Println("⚡ Testando conexão...")
	db, err := sql.Open(driver, os.Getenv("FLEXBERRY_RUN_DSN"))
	if err != nil { fail("não foi possível abrir a conexão: " + err.Error()) }
	defer db.Close()
	if err := db.PingContext(ctx); err != nil { fail("não foi possível conectar; confira serviço, host, porta e credenciais: " + err.Error()) }
	fmt.Println("✓ Conexão estabelecida:", dialect)
	name := os.Getenv("FLEXBERRY_RUN_CONNECTION")
	if err := flexberry.Register(name, db, dialect); err != nil { fail(err.Error()) }
	if err := flexberry.SetDefault(name); err != nil { fail(err.Error()) }
	values := []flexberry.Factory{
`)
	for _, entity := range entities {
		fmt.Fprintf(&b, "\t\tfactories.%sFactory(),\n", entity.Name)
	}
	b.WriteString(`	}
	for index := len(values)-1; index >= 0; index-- {
		if values[index].Active && values[index].Update {
			if err := values[index].Clean(ctx); err != nil { fail(err.Error()) }
		}
	}
	for _, factory := range values {
		if !factory.Active { continue }
		factory.Update = false
		result, err := factory.Run(ctx)
		if err != nil { fail(err.Error()) }
		fmt.Printf("✓ %s: %d registro(s) em %s\n", result.Name, result.Created, result.Table)
	}
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, "✗", message)
	os.Exit(1)
}
`)
	return format.Source(b.Bytes())
}

func orderEntities(entities []scanner.Entity) ([]scanner.Entity, error) {
	byName := make(map[string]scanner.Entity, len(entities))
	for _, entity := range entities {
		byName[entity.Name] = entity
	}
	var result []scanner.Entity
	visiting, visited := map[string]bool{}, map[string]bool{}
	var visit func(string) error
	visit = func(name string) error {
		if visited[name] {
			return nil
		}
		if visiting[name] {
			return fmt.Errorf("ciclo de relacionamento envolvendo %s", name)
		}
		entity, exists := byName[name]
		if !exists {
			return nil
		}
		visiting[name] = true
		var parents []string
		for _, relation := range entity.Relations {
			if relation.Kind == "belongsTo" {
				parents = append(parents, relationTypeName(relation.Type))
			}
		}
		sort.Strings(parents)
		for _, parent := range parents {
			if err := visit(parent); err != nil {
				return err
			}
		}
		visiting[name] = false
		visited[name] = true
		result = append(result, entity)
		return nil
	}
	var names []string
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := visit(name); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func relationTypeName(value string) string {
	value = strings.TrimLeft(value, "*[]")
	if index := strings.LastIndex(value, "."); index >= 0 {
		return value[index+1:]
	}
	return value
}
