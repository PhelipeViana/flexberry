package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/PhelipeViana/flexberry"
	"github.com/PhelipeViana/flexberry/internal/cli"
	"github.com/PhelipeViana/flexberry/internal/cliui"
	"github.com/PhelipeViana/flexberry/internal/config"
	"github.com/PhelipeViana/flexberry/internal/connectioncheck"
	"github.com/PhelipeViana/flexberry/internal/factorygen"
	"github.com/PhelipeViana/flexberry/internal/factoryrun"
	"github.com/PhelipeViana/flexberry/internal/generator"
	"github.com/PhelipeViana/flexberry/internal/initializer"
	"github.com/PhelipeViana/flexberry/internal/migrategen"
	"github.com/PhelipeViana/flexberry/internal/migraterun"
	"github.com/PhelipeViana/flexberry/internal/project"
	"github.com/PhelipeViana/flexberry/internal/scanner"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		cliui.PresentError(err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		command, err := cli.Select()
		if err != nil {
			return err
		}
		if command == "exit" {
			return nil
		}
		return run(strings.Fields(command))
	}

	switch args[0] {
	case "connection", "conexao", "conexão":
		if err := ensureFlexberryConfigured(); err != nil {
			return err
		}
		return runConnection(args[1:])
	case "config":
		return runConfig(args[1:])
	case "orm":
		if err := ensureFlexberryConfigured(config.ORMRelativePath); err != nil {
			return err
		}
		return runORM(args[1:])
	case "migrate":
		if err := ensureFlexberryConfigured(config.MigrateRelativePath); err != nil {
			return err
		}
		return runMigrate(args[1:])
	case "factory":
		if err := ensureFlexberryConfigured(config.ORMRelativePath, config.FactoryRelativePath); err != nil {
			return err
		}
		return runFactory(args[1:])
	case "init":
		return runInit(args[1:])
	case "validate":
		if err := ensureFlexberryConfigured(config.ORMRelativePath); err != nil {
			return err
		}
		return runValidate(args[1:])
	case "run":
		if err := ensureFlexberryConfigured(config.ORMRelativePath); err != nil {
			return err
		}
		return runGenerate(args[1:])
	case "version", "--version", "-v":
		fmt.Println("flexberry", flexberry.Version)
		return nil
	case "help", "--help", "-h":
		printHelp()
		return nil
	default:
		return fmt.Errorf("comando %q desconhecido; use flexberry help", args[0])
	}
}

func ensureFlexberryConfigured(required ...string) error {
	root, err := project.FindRoot(".")
	if err != nil {
		return cliui.NewUserError(
			"Este diretório não parece ser um projeto Go.",
			"Execute o Flexberry em uma pasta que contenha um arquivo go.mod.",
		)
	}
	paths := append([]string{config.DefaultRelativePath}, required...)
	var missing []string
	for _, relative := range paths {
		path := filepath.Join(root, filepath.FromSlash(relative))
		content, readErr := os.ReadFile(path)
		if readErr == nil && strings.TrimSpace(string(content)) != "" {
			continue
		}
		if readErr != nil && !os.IsNotExist(readErr) {
			return fmt.Errorf("verificar configuração Flexberry: %w", readErr)
		}
		missing = append(missing, relative)
	}
	if len(missing) == 0 {
		return nil
	}

	if _, initErr := initializer.Run(root, false); initErr != nil {
		return fmt.Errorf("criar configuração inicial: %w", initErr)
	}
	if dependencyErr := installProjectDependency(root); dependencyErr != nil {
		return cliui.UserError{Issues: []cliui.Issue{
			{
				Message:  "A estrutura inicial foi criada, mas a dependência não pôde ser instalada.",
				Solution: dependencyErr.Error(),
			},
		}}
	}
	return cliui.NewUserError(
		"Arquivos de configuração ausentes ou vazios foram criados automaticamente: "+strings.Join(missing, ", ")+".",
		"Revise os arquivos criados e .env; depois execute o comando novamente.",
	)
}

func runConfig(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("use config install, config update ou config remove")
	}
	switch args[0] {
	case "install", "instalar":
		return configureProject("Instalação", args[1:])
	case "update", "atualizar":
		return configureProject("Atualização", args[1:])
	case "remove", "remover":
		return runConfigRemove(args[1:])
	default:
		return fmt.Errorf("ação de configuração %q desconhecida", args[0])
	}
}

func configureProject(title string, args []string) error {
	flags := flag.NewFlagSet("config", flag.ContinueOnError)
	force := flags.Bool("force", false, "recria configurações editáveis")
	if err := flags.Parse(args); err != nil {
		return err
	}
	root, err := project.FindRoot(".")
	if err != nil {
		return err
	}

	cliui.PrintTitle("Flexberry · " + title)
	fmt.Println(cliui.Info("→ Preparando configurações..."))
	result, err := initializer.Run(root, *force)
	if err != nil {
		return fmt.Errorf("preparar configurações: %w", err)
	}
	for _, path := range result.Created {
		fmt.Printf("  + %s\n", filepath.ToSlash(path))
	}
	for _, path := range result.Repaired {
		fmt.Printf("  ↻ %s (recriado)\n", filepath.ToSlash(path))
	}
	if len(result.Created) == 0 {
		if len(result.Repaired) == 0 {
			fmt.Printf("  ✓ Arquivos existentes preservados (%d)\n", len(result.Skipped))
		}
	} else if len(result.Skipped) > 0 {
		fmt.Printf("  = %d arquivo(s) existente(s) preservado(s)\n", len(result.Skipped))
	}

	fmt.Printf("\n%s\n", cliui.Info(fmt.Sprintf("→ Instalando Flexberry %s...", flexberry.Version)))
	if err := installProjectDependency(root); err != nil {
		return err
	}
	fmt.Println("  " + cliui.Success("✓ Dependência instalada"))
	fmt.Println("\n" + cliui.Success("✓ Operação concluída com sucesso."))
	fmt.Println("\nPróximos passos:")
	fmt.Println("  1. Revise internal/flexberry/connection.yaml")
	fmt.Println("  2. Execute .\\flexberry.exe validate --resolve")
	return nil
}

func installProjectDependency(root string) error {
	version := strings.TrimPrefix(flexberry.Version, "v")
	moduleVersion := "github.com/PhelipeViana/flexberry@v" + version
	output, err := runGoGet(root, moduleVersion, nil)
	if err == nil {
		return nil
	}

	detail := strings.TrimSpace(string(output))
	if strings.Contains(detail, "unknown revision") || strings.Contains(detail, "404 Not Found") {
		directEnv := []string{
			"GOPROXY=direct",
			"GONOSUMDB=github.com/PhelipeViana/flexberry",
		}
		directOutput, directErr := runGoGet(root, moduleVersion, directEnv)
		if directErr == nil {
			return nil
		}
		directDetail := strings.TrimSpace(string(directOutput))
		if directDetail != "" {
			return fmt.Errorf("a versão v%s ainda não está disponível: %s", version, directDetail)
		}
		return fmt.Errorf("a versão v%s ainda não está disponível; tente novamente em alguns minutos", version)
	}
	if detail == "" {
		detail = err.Error()
	}
	return fmt.Errorf("não foi possível instalar a dependência: %s", detail)
}

func runGoGet(root, moduleVersion string, extraEnv []string) ([]byte, error) {
	command := exec.Command("go", "get", moduleVersion)
	command.Dir = root
	if len(extraEnv) > 0 {
		command.Env = append(os.Environ(), extraEnv...)
	}
	return command.CombinedOutput()
}

func runConfigRemove(args []string) error {
	flags := flag.NewFlagSet("config remove", flag.ContinueOnError)
	force := flags.Bool("force", false, "remove sem confirmação")
	if err := flags.Parse(args); err != nil {
		return err
	}
	root, err := project.FindRoot(".")
	if err != nil {
		return err
	}
	target := filepath.Join(root, "internal", "flexberry")
	relative, err := filepath.Rel(root, target)
	if err != nil || filepath.ToSlash(relative) != "internal/flexberry" {
		return fmt.Errorf("caminho de remoção inválido")
	}
	if _, err := os.Stat(target); os.IsNotExist(err) {
		fmt.Println("Flexberry não está configurado neste projeto.")
		return nil
	} else if err != nil {
		return err
	}
	if !*force {
		fmt.Print("Remover integralmente internal/flexberry? [s/N]: ")
		var answer string
		if _, err := fmt.Scanln(&answer); err != nil {
			return fmt.Errorf("confirmação não recebida")
		}
		answer = strings.ToLower(strings.TrimSpace(answer))
		if answer != "s" && answer != "sim" {
			fmt.Println("Remoção cancelada.")
			return nil
		}
	}
	if ormConfig, loadErr := config.LoadORM(filepath.Join(root, filepath.FromSlash(config.ORMRelativePath))); loadErr == nil {
		if err := removeConfiguredPath(root, ormConfig.Output.Path); err != nil {
			return err
		}
	}
	if factoryConfig, loadErr := config.LoadFactory(filepath.Join(root, filepath.FromSlash(config.FactoryRelativePath))); loadErr == nil {
		if err := removeConfiguredPath(root, factoryConfig.Mapper.Path); err != nil {
			return err
		}
	}
	if migrateConfig, loadErr := config.LoadMigrate(filepath.Join(root, filepath.FromSlash(config.MigrateRelativePath))); loadErr == nil {
		if err := removeConfiguredPath(root, migrateConfig.Output.Path); err != nil {
			return err
		}
	}
	if err := os.RemoveAll(target); err != nil {
		return fmt.Errorf("remover configuração Flexberry: %w", err)
	}
	for _, arguments := range [][]string{
		{"mod", "edit", "-droprequire=github.com/PhelipeViana/flexberry"},
		{"mod", "edit", "-dropreplace=github.com/PhelipeViana/flexberry"},
		{"mod", "tidy"},
	} {
		command := exec.Command("go", arguments...)
		command.Dir = root
		if output, commandErr := command.CombinedOutput(); commandErr != nil && !strings.Contains(string(output), "not found") {
			return fmt.Errorf("atualizar go.mod: %s", strings.TrimSpace(string(output)))
		}
	}
	fmt.Println("✓ Configuração Flexberry removida.")
	return nil
}

func removeConfiguredPath(root, configured string) error {
	target := filepath.Clean(filepath.Join(root, filepath.FromSlash(configured)))
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == "." || relative == ".." || filepath.IsAbs(relative) {
		return fmt.Errorf("caminho configurado inseguro para remoção: %s", configured)
	}
	return os.RemoveAll(target)
}

func runORM(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("use orm reload ou orm run")
	}
	switch args[0] {
	case "reload":
		return reloadORM(args[1:])
	case "run", "sync":
		return runGenerate(args[1:])
	default:
		return fmt.Errorf("ação de ORM %q desconhecida; use orm reload ou orm run", args[0])
	}
}

func runFactory(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("use factory reload ou factory run")
	}
	switch args[0] {
	case "reload", "create", "sync":
		return createFactories()
	case "run":
		return executeFactories()
	default:
		return fmt.Errorf("ação de factory %q desconhecida", args[0])
	}
}

func runMigrate(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("use migrate reload ou migrate run")
	}
	root, err := project.FindRoot(".")
	if err != nil {
		return err
	}
	base, err := config.Load(filepath.Join(root, filepath.FromSlash(config.DefaultRelativePath)))
	if err != nil {
		return err
	}
	migrateConfig, err := config.LoadMigrate(filepath.Join(root, filepath.FromSlash(config.MigrateRelativePath)))
	if err != nil {
		return err
	}
	switch args[0] {
	case "reload":
		base.Entities = migrateConfig.Entities
		scanned, err := scanner.Scan(root, base)
		if err != nil {
			return err
		}
		result, err := migrategen.Generate(root, migrateConfig, scanned.Entities)
		if err != nil {
			return err
		}
		if result.Unchanged {
			fmt.Println(cliui.Muted("✓ Migrate Reload: não houve modificação nas entidades monitoradas."))
			return nil
		}
		fmt.Printf("%s %s (%d operação(ões))\n", cliui.Success("✓ Migration gerada:"), result.Path, result.Operations)
		return nil
	case "run":
		return migraterun.Run(root, base, migrateConfig)
	default:
		return fmt.Errorf("ação de migrate %q desconhecida; use migrate reload ou migrate run", args[0])
	}
}

func runConnection(args []string) error {
	if len(args) > 0 && args[0] != "report" {
		return fmt.Errorf("use connection report")
	}
	root, err := project.FindRoot(".")
	if err != nil {
		return err
	}
	return connectioncheck.Run(root)
}

func reloadORM(args []string) error {
	root, err := project.FindRoot(".")
	if err != nil {
		return err
	}
	ormConfig, err := config.LoadORM(filepath.Join(root, filepath.FromSlash(config.ORMRelativePath)))
	if err != nil {
		return err
	}
	if err := removeConfiguredPath(root, ormConfig.Output.Path); err != nil {
		return fmt.Errorf("recriar ORM: %w", err)
	}
	return runGenerate(args)
}

type factoryProject struct {
	root       string
	modulePath string
	base       *config.Config
	orm        config.ORMConfig
	factory    config.FactoryConfig
	scan       scanner.Result
}

func loadFactoryProject() (factoryProject, error) {
	root, err := project.FindRoot(".")
	if err != nil {
		return factoryProject{}, err
	}
	base, err := config.Load(filepath.Join(root, filepath.FromSlash(config.DefaultRelativePath)))
	if err != nil {
		return factoryProject{}, err
	}
	ormConfig, err := config.LoadORM(filepath.Join(root, filepath.FromSlash(config.ORMRelativePath)))
	if err != nil {
		return factoryProject{}, err
	}
	factoryConfig, err := config.LoadFactory(filepath.Join(root, filepath.FromSlash(config.FactoryRelativePath)))
	if err != nil {
		return factoryProject{}, err
	}
	base.Entities = ormConfig.Entities
	base.Generate.Output = ormConfig.Output.Path
	base.Generate.Package = ormConfig.Output.Package
	scanned, err := scanner.ScanLenient(root, base)
	if err != nil {
		return factoryProject{}, err
	}
	modulePath, err := project.ModulePath(root)
	if err != nil {
		return factoryProject{}, err
	}
	return factoryProject{root: root, modulePath: modulePath, base: base, orm: ormConfig, factory: factoryConfig, scan: scanned}, nil
}

func createFactories() error {
	value, err := loadFactoryProject()
	if err != nil {
		return err
	}
	result, err := syncFactories(value)
	if err != nil {
		return err
	}
	fmt.Printf("✓ Factories: %d criada(s), %d atualizada(s), %d preservada(s), %d desativada(s)\n",
		result.Created, result.Updated, result.Preserved, result.Disabled)
	printScanWarnings(value.scan.Warnings)
	return nil
}

func syncFactories(value factoryProject) (factorygen.Result, error) {
	files, err := generator.Build(value.base, value.scan)
	if err != nil {
		return factorygen.Result{}, err
	}
	if _, err := generator.Write(value.root, value.base, files); err != nil {
		return factorygen.Result{}, err
	}
	result, err := factorygen.Generate(value.root, value.modulePath, value.factory, value.orm, value.scan.Entities)
	if err != nil {
		return factorygen.Result{}, err
	}
	return result, nil
}

func executeFactories() error {
	value, err := loadFactoryProject()
	if err != nil {
		return err
	}
	if _, err := syncFactories(value); err != nil {
		return err
	}
	envPath, err := value.base.EnvironmentFile(value.root, config.OSLookup)
	if err != nil {
		return err
	}
	if envPath != "" {
		if err := config.LoadEnvFile(envPath); err != nil {
			return err
		}
	}
	name, err := value.base.DefaultConnection(config.OSLookup)
	if err != nil {
		return err
	}
	connection, err := value.base.ResolvedConnection(name, config.OSLookup)
	if err != nil {
		return err
	}
	err = factoryrun.Run(value.root, value.modulePath, value.factory, value.scan.Entities, name, connection)
	printScanWarnings(value.scan.Warnings)
	return err
}

func printScanWarnings(warnings []string) {
	if len(warnings) == 0 {
		return
	}
	fmt.Println()
	fmt.Println(cliui.Warning("⚠ Entidades ignoradas:"))
	for _, warning := range warnings {
		fmt.Printf("  → %s\n", warning)
	}
	fmt.Println(cliui.Muted("  As demais entidades continuaram normalmente."))
}

func runGenerate(args []string) error {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	configPath := flags.String("config", "", "caminho alternativo para connection.yaml")
	dryRun := flags.Bool("dry-run", false, "analisa sem escrever arquivos")
	if err := flags.Parse(args); err != nil {
		return err
	}
	root, err := project.FindRoot(".")
	if err != nil {
		return err
	}
	path := *configPath
	if path == "" {
		path = filepath.Join(root, filepath.FromSlash(config.DefaultRelativePath))
	} else if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	ormConfig, err := config.LoadORM(filepath.Join(root, filepath.FromSlash(config.ORMRelativePath)))
	if err != nil {
		return err
	}
	cfg.Entities = ormConfig.Entities
	cfg.Generate.Output = ormConfig.Output.Path
	cfg.Generate.Package = ormConfig.Output.Package
	result, err := scanner.ScanLenient(root, cfg)
	if err != nil {
		return err
	}
	files, err := generator.Build(cfg, result)
	if err != nil {
		return err
	}
	fmt.Printf("Mapeadas %d entidade(s) em %d arquivo(s) fonte\n", len(result.Entities), len(result.Files))
	if *dryRun {
		for _, entity := range result.Entities {
			fmt.Printf("  - %s.%s -> %s\n", entity.Package, entity.Name, entity.Table)
		}
		fmt.Printf("Dry-run: %d arquivo(s) seriam gerados em %s\n", len(files), cfg.Generate.Output)
		printScanWarnings(result.Warnings)
		return nil
	}
	written, err := generator.Write(root, cfg, files)
	if err != nil {
		return err
	}
	for _, generatedPath := range written {
		fmt.Printf("  + %s\n", generatedPath)
	}
	printScanWarnings(result.Warnings)
	return nil
}

func runInit(args []string) error {
	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	force := flags.Bool("force", false, "recria arquivos gerados pelo init, preservando credenciais")
	if err := flags.Parse(args); err != nil {
		return err
	}

	root, err := project.FindRoot(".")
	if err != nil {
		return err
	}
	result, err := initializer.Run(root, *force)
	if err != nil {
		return err
	}

	fmt.Printf("Flexberry inicializado em %s\n", root)
	for _, path := range result.Created {
		fmt.Printf("  + %s\n", filepath.ToSlash(path))
	}
	for _, path := range result.Skipped {
		fmt.Printf("  = %s (preservado)\n", filepath.ToSlash(path))
	}
	fmt.Println("\nPróximo passo: revise internal/flexberry/connection.yaml")
	fmt.Println("Depois execute: flexberry validate")
	return nil
}

func runValidate(args []string) error {
	flags := flag.NewFlagSet("validate", flag.ContinueOnError)
	configPath := flags.String("config", "", "caminho alternativo para connection.yaml")
	resolve := flags.Bool("resolve", false, "carrega o .env e valida a conexão padrão")
	if err := flags.Parse(args); err != nil {
		return err
	}

	root, err := project.FindRoot(".")
	if err != nil {
		return err
	}
	path := *configPath
	if path == "" {
		path = filepath.Join(root, filepath.FromSlash(config.DefaultRelativePath))
	} else if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}

	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	ormConfig, err := config.LoadORM(filepath.Join(root, filepath.FromSlash(config.ORMRelativePath)))
	if err != nil {
		return err
	}
	cfg.Entities = ormConfig.Entities
	cfg.Generate.Output = ormConfig.Output.Path
	cfg.Generate.Package = ormConfig.Output.Package

	fmt.Printf("✓ configuração válida: %s\n", filepath.ToSlash(path))
	fmt.Printf("✓ conexões: %s\n", strings.Join(cfg.ConnectionNames(), ", "))
	fmt.Printf("✓ entidades: %d caminho(s)\n", len(cfg.Entities.Paths))
	fmt.Printf("✓ ORM: %s\n", filepath.ToSlash(filepath.Join(root, filepath.FromSlash(config.ORMRelativePath))))
	fmt.Printf("✓ saída: %s\n", cfg.Generate.Output)

	if !*resolve {
		fmt.Println("ℹ use flexberry validate --resolve para validar as variáveis")
		return nil
	}

	envPath, err := cfg.EnvironmentFile(root, config.OSLookup)
	if err != nil {
		return err
	}
	if envPath != "" {
		if err := config.LoadEnvFile(envPath); err != nil {
			return err
		}
		fmt.Printf("✓ ambiente carregado: %s\n", filepath.ToSlash(envPath))
	}
	fmt.Printf("✓ profile: %s\n", cfg.EnvironmentName(config.OSLookup))

	defaultName, err := cfg.DefaultConnection(config.OSLookup)
	if err != nil {
		return err
	}
	if _, err := cfg.ResolvedConnection(defaultName, config.OSLookup); err != nil {
		return err
	}
	fmt.Printf("✓ conexão padrão resolvida: %s\n", defaultName)
	return nil
}

func printHelpLegacy() {
	fmt.Printf(`Flexberry %s

Uso:
  flexberry init [--force]       cria a estrutura inicial no projeto
  flexberry validate            valida internal/flexberry/connection.yaml
  flexberry validate --resolve  valida também o arquivo de ambiente
  flexberry version             exibe a versão instalada

  flexberry run [--dry-run]     mapeia entidades e atualiza o código gerado
`, flexberry.Version)
}

func printHelp() {
	fmt.Printf(`Flexberry %s

Uso:
  flexberry                         abre o menu online/interativo
  flexberry config install          cria os três YAMLs
  flexberry config update           cria configurações ausentes
  flexberry config remove --force   remove internal/flexberry
  flexberry connection report       exibe o relatório das conexões
  flexberry migrate reload          gera migrations pelas entidades
  flexberry migrate run             aplica migrations em todas as conexões
  flexberry orm reload              recria completamente o ORM
  flexberry orm run                 atualiza o ORM
  flexberry factory reload          cria ou atualiza factories
  flexberry factory run             executa factories
  flexberry validate --resolve      valida configurações e ambiente
  flexberry version                 exibe a versão instalada
`, flexberry.Version)
}
