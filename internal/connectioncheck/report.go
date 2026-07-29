package connectioncheck

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/denisenkom/go-mssqldb"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/sijms/go-ora/v2"

	"github.com/PhelipeViana/flexberry/internal/cliui"
	"github.com/PhelipeViana/flexberry/internal/config"
)

type Result struct {
	Name, Dialect, Schema, Version, Problem string
	Default                                 bool
	Duration                                time.Duration
}

func Run(root string) error {
	cfg, err := config.Load(filepath.Join(root, filepath.FromSlash(config.DefaultRelativePath)))
	if err != nil {
		return err
	}
	envPath, err := cfg.EnvironmentFile(root, config.OSLookup)
	if err != nil {
		return err
	}
	if envPath != "" {
		if err := config.LoadEnvFile(envPath); err != nil {
			return err
		}
	}
	defaultName, err := cfg.DefaultConnection(config.OSLookup)
	if err != nil {
		return err
	}

	cliui.PrintTitle("Flexberry · Relatório de conexões")
	fmt.Printf("%s %s\n", cliui.Muted("Ambiente:"), cfg.EnvironmentName(config.OSLookup))
	fmt.Printf("%s %s\n\n", cliui.Muted("Conexão padrão:"), cliui.Info(defaultName))

	names := cfg.ConnectionNames()
	results := make([]Result, len(names))
	var wait sync.WaitGroup
	for index, name := range names {
		wait.Add(1)
		go func(index int, name string) {
			defer wait.Done()
			results[index] = check(cfg, name, name == defaultName)
		}(index, name)
	}
	wait.Wait()

	failures := 0
	for _, result := range results {
		printResult(result)
		if result.Problem != "" {
			failures++
		}
	}
	fmt.Println()
	if failures > 0 {
		return fmt.Errorf("%d de %d conexão(ões) apresentaram problema", failures, len(results))
	}
	fmt.Println(cliui.Success(fmt.Sprintf("✓ Todas as %d conexões estão disponíveis.", len(results))))
	return nil
}

func check(cfg *config.Config, name string, isDefault bool) Result {
	result := Result{Name: name, Default: isDefault}
	connection, err := cfg.ResolvedConnection(name, config.OSLookup)
	if err != nil {
		result.Problem = friendlyError(err)
		return result
	}
	result.Dialect = strings.ToLower(connection.Dialect)
	result.Schema = connection.Schema

	driver := map[string]string{
		"oracle":    "oracle",
		"postgres":  "pgx",
		"mysql":     "mysql",
		"sqlserver": "sqlserver",
	}[result.Dialect]
	if driver == "" {
		result.Problem = "dialeto não suportado pelo verificador"
		return result
	}

	started := time.Now()
	db, err := sql.Open(driver, connection.URL)
	if err != nil {
		result.Problem = friendlyError(err)
		return result
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(0)

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		result.Duration = time.Since(started)
		result.Problem = friendlyError(err)
		return result
	}
	result.Duration = time.Since(started)
	result.Version = databaseVersion(ctx, db, result.Dialect)
	return result
}

func databaseVersion(ctx context.Context, db *sql.DB, dialect string) string {
	query := map[string]string{
		"oracle":    "SELECT banner FROM v$version WHERE ROWNUM = 1",
		"postgres":  "SELECT version()",
		"mysql":     "SELECT VERSION()",
		"sqlserver": "SELECT @@VERSION",
	}[dialect]
	var version string
	if query == "" || db.QueryRowContext(ctx, query).Scan(&version) != nil {
		return "versão não informada pelo servidor"
	}
	version = strings.Join(strings.Fields(version), " ")
	if len(version) > 140 {
		version = version[:137] + "..."
	}
	return version
}

func printResult(result Result) {
	defaultLabel := ""
	if result.Default {
		defaultLabel = " " + cliui.Info("[DEFAULT]")
	}
	if result.Problem != "" {
		fmt.Printf("%s %s %s%s\n", cliui.Failure("✗"), result.Name, cliui.Failure("ERRO"), defaultLabel)
		fmt.Printf("  %s\n", cliui.Warning(result.Problem))
		return
	}
	fmt.Printf("%s %s %s%s\n", cliui.Success("✓"), result.Name, cliui.Success("OK"), defaultLabel)
	fmt.Printf("  %s %s", cliui.Muted("Dialeto:"), result.Dialect)
	if result.Schema != "" {
		fmt.Printf("  %s %s", cliui.Muted("Schema:"), result.Schema)
	}
	fmt.Printf("  %s %s\n", cliui.Muted("Tempo:"), result.Duration.Round(time.Millisecond))
	fmt.Printf("  %s %s\n", cliui.Muted("Banco:"), result.Version)
}

func friendlyError(err error) string {
	detail := strings.TrimSpace(err.Error())
	lower := strings.ToLower(detail)
	switch {
	case strings.Contains(lower, "connection refused"), strings.Contains(lower, "actively refused"),
		strings.Contains(lower, "no connection could be made"):
		return "serviço indisponível; confirme se o container está ativo e confira host e porta"
	case strings.Contains(lower, "password authentication failed"), strings.Contains(lower, "access denied"),
		strings.Contains(lower, "login failed"), strings.Contains(lower, "ora-01017"):
		return "credenciais recusadas; confira usuário e senha no arquivo de ambiente"
	case strings.Contains(lower, "unknown database"), strings.Contains(lower, "does not exist"),
		strings.Contains(lower, "ora-12514"):
		return "banco, schema ou serviço não encontrado; confira o nome configurado"
	case strings.Contains(lower, "deadline exceeded"), strings.Contains(lower, "i/o timeout"),
		strings.Contains(lower, "connection timed out"):
		return "tempo de conexão esgotado; confira a rede, o host e a porta"
	case strings.Contains(lower, "missing environment variable"):
		return detail + "; preencha a variável no arquivo de ambiente"
	default:
		return detail
	}
}
