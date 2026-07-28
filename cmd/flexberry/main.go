package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/PhelipeViana/flexberry"
	"github.com/PhelipeViana/flexberry/internal/config"
	"github.com/PhelipeViana/flexberry/internal/generator"
	"github.com/PhelipeViana/flexberry/internal/initializer"
	"github.com/PhelipeViana/flexberry/internal/project"
	"github.com/PhelipeViana/flexberry/internal/scanner"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "flexberry: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		printHelp()
		return nil
	}

	switch args[0] {
	case "init":
		return runInit(args[1:])
	case "validate":
		return runValidate(args[1:])
	case "run":
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

func runGenerate(args []string) error {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	configPath := flags.String("config", "", "caminho alternativo para flexberry.yaml")
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
	result, err := scanner.Scan(root, cfg)
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
		return nil
	}
	written, err := generator.Write(root, cfg, files)
	if err != nil {
		return err
	}
	for _, generatedPath := range written {
		fmt.Printf("  + %s\n", generatedPath)
	}
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
	fmt.Println("\nPróximo passo: revise internal/flexberry/flexberry.yaml")
	fmt.Println("Depois execute: flexberry validate")
	return nil
}

func runValidate(args []string) error {
	flags := flag.NewFlagSet("validate", flag.ContinueOnError)
	configPath := flags.String("config", "", "caminho alternativo para flexberry.yaml")
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

	fmt.Printf("✓ configuração válida: %s\n", filepath.ToSlash(path))
	fmt.Printf("✓ conexões: %s\n", strings.Join(cfg.ConnectionNames(), ", "))
	fmt.Printf("✓ entidades: %d caminho(s)\n", len(cfg.Entities.Paths))
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

func printHelp() {
	fmt.Printf(`Flexberry %s

Uso:
  flexberry init [--force]       cria a estrutura inicial no projeto
  flexberry validate            valida internal/flexberry/flexberry.yaml
  flexberry validate --resolve  valida também o arquivo de ambiente
  flexberry version             exibe a versão instalada

  flexberry run [--dry-run]     mapeia entidades e atualiza o código gerado
`, flexberry.Version)
}
