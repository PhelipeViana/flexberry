package migraterun

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/PhelipeViana/flexberry/internal/cliui"
	"github.com/PhelipeViana/flexberry/internal/config"
	"github.com/PhelipeViana/flexberry/internal/migrategen"
	"github.com/PhelipeViana/flexberry/internal/migrationgo"
	"github.com/PhelipeViana/flexberry/migration/acao"
)

type migrationFile struct {
	Name, ID, Path, Checksum string
	Provisional              bool
	Plan                     migrategen.Plan
}

var goMigrationName = regexp.MustCompile(`^(\d{4}_\d{2}_\d{2}_\d{6})(?:.*)?\.go$`)

// Validate parses and pre-validates every migration without opening a database.
func Validate(root string, cfg config.MigrateConfig) (int, error) {
	files, err := loadPlans(filepath.Join(root, filepath.FromSlash(cfg.Output.Path)))
	if err != nil {
		return 0, err
	}
	return len(files), nil
}

func Run(root string, base *config.Config, cfg config.MigrateConfig) error {
	files, err := loadPlans(filepath.Join(root, filepath.FromSlash(cfg.Output.Path)))
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return cliui.NewUserError(
			"Nenhuma migration foi gerada.",
			`Execute .\flexberry.exe migrate reload antes de migrate run.`,
		)
	}
	envPath, err := base.EnvironmentFile(root, config.OSLookup)
	if err != nil {
		return err
	}
	if envPath != "" {
		if err := config.LoadEnvFile(envPath); err != nil {
			return err
		}
	}

	cliui.PrintTitle("Flexberry · Migrations")
	name, err := base.DefaultConnection(config.OSLookup)
	if err != nil {
		return err
	}
	connection, err := base.ResolvedConnection(name, config.OSLookup)
	if err != nil {
		return err
	}
	fmt.Printf("%s %s (%s) %s\n", cliui.Info("→"), name, connection.Dialect, cliui.Info("[DEFAULT]"))
	fmt.Printf("  %s %s\n", cliui.Muted("Histórico:"), historyLocation(connection, cfg.History))
	applied, skipped, runErr := runConnection(connection, cfg.History, files)
	if runErr != nil {
		fmt.Printf("  %s %s\n", cliui.Failure("✗ ERRO:"), runErr)
		message, solution := migrationConnectionAdvice(connection, runErr)
		fmt.Printf("  %s %s\n", cliui.Warning("⚠ Diagnóstico:"), message)
		return cliui.NewUserError(
			"A conexão padrão não concluiu as migrations.",
			solution,
		)
	}
	if err := finalizeProvisional(files); err != nil {
		return fmt.Errorf("finalizar migrations executadas: %w", err)
	}
	fmt.Printf("  %s %d aplicada(s), %d já executada(s)\n", cliui.Success("✓ OK"), applied, skipped)
	fmt.Println("\n" + cliui.Success("✓ Migrations atualizadas na conexão padrão."))
	return nil
}

func RunAll(root string, base *config.Config, cfg config.MigrateConfig) error {
	files, err := loadPlans(filepath.Join(root, filepath.FromSlash(cfg.Output.Path)))
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return cliui.NewUserError("Nenhuma migration foi gerada.", `Execute .\flexberry.exe migrate reload antes de migrate run-all.`)
	}
	if err := loadEnvironment(root, base); err != nil {
		return err
	}
	cliui.PrintTitle("Flexberry · Migrations · Todos os bancos")
	var failures []string
	for _, name := range base.ConnectionNames() {
		connection, err := base.ResolvedConnection(name, config.OSLookup)
		if err != nil {
			failures = append(failures, name+": "+err.Error())
			continue
		}
		fmt.Printf("%s %s (%s)\n", cliui.Info("→"), name, connection.Dialect)
		applied, skipped, err := runConnection(connection, cfg.History, files)
		if err != nil {
			fmt.Printf("  %s %s\n", cliui.Failure("✗ ERRO:"), err)
			failures = append(failures, name+": "+err.Error())
			continue
		}
		fmt.Printf("  %s %d aplicada(s), %d já executada(s)\n", cliui.Success("✓ OK"), applied, skipped)
	}
	if len(failures) > 0 {
		return fmt.Errorf("%d conexão(ões) falharam: %s", len(failures), strings.Join(failures, "; "))
	}
	if err := finalizeProvisional(files); err != nil {
		return fmt.Errorf("finalizar migrations executadas: %w", err)
	}
	fmt.Println("\n" + cliui.Success("✓ Migrations executadas em todos os bancos."))
	return nil
}

func Fresh(root string, base *config.Config, cfg config.MigrateConfig) error {
	if err := ValidateFreshEnvironment(root, base); err != nil {
		return err
	}
	files, err := loadPlans(filepath.Join(root, filepath.FromSlash(cfg.Output.Path)))
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return cliui.NewUserError("Nenhuma migration foi gerada.", `Execute .\flexberry.exe migrate reload antes de migrate fresh.`)
	}
	name, err := base.DefaultConnection(config.OSLookup)
	if err != nil {
		return err
	}
	connection, err := base.ResolvedConnection(name, config.OSLookup)
	if err != nil {
		return err
	}
	cliui.PrintTitle("Flexberry · Migrations · Fresh")
	fmt.Printf("%s Recriando %s (%s)\n", cliui.Warning("⚠"), name, connection.Dialect)
	if err := resetConnection(connection, cfg.History, files); err != nil {
		return fmt.Errorf("limpar banco %s: %w", name, err)
	}
	applied, skipped, err := runConnection(connection, cfg.History, files)
	if err != nil {
		return err
	}
	if err := finalizeProvisional(files); err != nil {
		return fmt.Errorf("finalizar migrations executadas: %w", err)
	}
	fmt.Printf("%s %d migration(s) aplicada(s), %d ignorada(s)\n", cliui.Success("✓ Fresh concluído:"), applied, skipped)
	return nil
}

// FreshAll recreates every configured development database after a complete
// connectivity preflight, preventing a missing fourth database from being
// discovered only after the first databases were already erased.
func FreshAll(root string, base *config.Config, cfg config.MigrateConfig) error {
	if err := ValidateFreshEnvironment(root, base); err != nil {
		return err
	}
	files, err := loadPlans(filepath.Join(root, filepath.FromSlash(cfg.Output.Path)))
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return cliui.NewUserError("Nenhuma migration foi gerada.", `Execute .\flexberry.exe migrate reload antes de migrate fresh-all.`)
	}
	expected := map[string]bool{"oracle": false, "postgres": false, "mysql": false, "sqlserver": false}
	type namedConnection struct {
		name       string
		connection config.Connection
	}
	connections := make([]namedConnection, 0, 4)
	for _, name := range base.ConnectionNames() {
		connection, err := base.ResolvedConnection(name, config.OSLookup)
		if err != nil {
			return fmt.Errorf("resolver conexão %s: %w", name, err)
		}
		dialect := strings.ToLower(connection.Dialect)
		if _, required := expected[dialect]; required {
			expected[dialect] = true
			connections = append(connections, namedConnection{name: name, connection: connection})
		}
	}
	var missing []string
	for dialect, found := range expected {
		if !found {
			missing = append(missing, dialect)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		return fmt.Errorf("Fresh All exige os quatro dialetos; ausentes: %s", strings.Join(missing, ", "))
	}
	for _, target := range connections {
		if err := pingConnection(target.connection); err != nil {
			return fmt.Errorf("pré-flight %s: %w", target.name, err)
		}
	}
	cliui.PrintTitle("Flexberry · Migrations · Fresh All")
	for _, target := range connections {
		fmt.Printf("%s %s (%s) schema=%s\n", cliui.Info("→"), target.name, target.connection.Dialect, target.connection.Schema)
	}
	for _, target := range connections {
		fmt.Printf("\n%s Recriando %s (%s)\n", cliui.Warning("⚠"), target.name, target.connection.Dialect)
		if err := resetConnection(target.connection, cfg.History, files); err != nil {
			return fmt.Errorf("limpar %s: %w", target.name, err)
		}
		applied, skipped, err := runConnection(target.connection, cfg.History, files)
		if err != nil {
			return fmt.Errorf("executar %s: %w", target.name, err)
		}
		fmt.Printf("  %s %d migration(s) aplicada(s), %d ignorada(s)\n", cliui.Success("✓"), applied, skipped)
	}
	fmt.Println("\n" + cliui.Success("✓ Fresh concluído nos quatro dialetos."))
	return nil
}

func pingConnection(connection config.Connection) error {
	driver := map[string]string{"oracle": "oracle", "postgres": "pgx", "mysql": "mysql", "sqlserver": "sqlserver"}[strings.ToLower(connection.Dialect)]
	if driver == "" {
		return fmt.Errorf("dialeto não suportado: %s", connection.Dialect)
	}
	db, err := sql.Open(driver, connection.URL)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return db.PingContext(ctx)
}

func ValidateFreshEnvironment(root string, base *config.Config) error {
	if err := loadEnvironment(root, base); err != nil {
		return err
	}
	ambient := strings.TrimSpace(base.Environment.Ambient)
	if ambient == "" {
		ambient = "APP_ENV"
	}
	environment := strings.TrimSpace(os.Getenv(ambient))
	if environment == "" {
		environment = strings.TrimSpace(base.Environment.Fallback)
	}
	if !strings.EqualFold(environment, "development") && !strings.EqualFold(environment, "dev") {
		return cliui.NewUserError(
			"Migrate Fresh é permitido somente no ambiente development.",
			fmt.Sprintf("O ambiente atual é %q, definido por %s.", environment, ambient),
		)
	}
	return nil
}

func loadEnvironment(root string, base *config.Config) error {
	envPath, err := base.EnvironmentFile(root, config.OSLookup)
	if err != nil {
		return err
	}
	if envPath != "" {
		return config.LoadEnvFile(envPath)
	}
	return nil
}

func resetConnection(connection config.Connection, historyTable string, files []migrationFile) error {
	dialect := strings.ToLower(connection.Dialect)
	driver := map[string]string{"oracle": "oracle", "postgres": "pgx", "mysql": "mysql", "sqlserver": "sqlserver"}[dialect]
	db, err := sql.Open(driver, connection.URL)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return err
	}
	if dialect == "mysql" {
		if _, err := db.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS=0"); err != nil {
			return err
		}
		defer db.ExecContext(context.Background(), "SET FOREIGN_KEY_CHECKS=1")
	}
	for _, view := range managedViews(files) {
		exists, err := viewExists(ctx, db, dialect, connection.Schema, view)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		if _, err := db.ExecContext(ctx, "DROP VIEW "+qualified(dialect, connection.Schema, view)); err != nil {
			return fmt.Errorf("remover view %s: %w", view, err)
		}
		fmt.Printf("  - view %s removida\n", view)
	}
	tables := managedTables(files)
	for _, table := range tables {
		exists, err := tableExists(ctx, db, dialect, connection.Schema, table)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		if _, err := db.ExecContext(ctx, dropTableSQL(dialect, connection.Schema, table)); err != nil {
			return fmt.Errorf("remover tabela %s: %w", table, err)
		}
		fmt.Printf("  - tabela %s removida\n", table)
	}
	exists, err := tableExists(ctx, db, dialect, connection.Schema, historyTable)
	if err != nil {
		return err
	}
	if exists {
		if _, err := db.ExecContext(ctx, dropTableSQL(dialect, connection.Schema, historyTable)); err != nil {
			return fmt.Errorf("remover histórico %s: %w", historyTable, err)
		}
	}
	return nil
}

func managedViews(files []migrationFile) []string {
	set := map[string]bool{}
	for _, file := range files {
		for _, operation := range file.Plan.Operations {
			if operation.Kind == string(acao.CreateView) {
				set[operation.Name] = true
			}
		}
	}
	result := make([]string, 0, len(set))
	for name := range set {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func managedTables(files []migrationFile) []string {
	all, children := map[string]bool{}, map[string][]string{}
	for _, file := range files {
		for _, operation := range file.Plan.Operations {
			if operation.Kind == "create_table" {
				all[operation.Table] = true
			}
			if operation.Kind == "add_foreign_key" && operation.ForeignKey != nil {
				parent := operation.ForeignKey.ReferenceTable
				children[parent] = append(children[parent], operation.Table)
			}
		}
	}
	names := make([]string, 0, len(all))
	for name := range all {
		names = append(names, name)
	}
	sort.Strings(names)
	seen, result := map[string]bool{}, []string{}
	var visit func(string)
	visit = func(table string) {
		if seen[table] {
			return
		}
		seen[table] = true
		for _, child := range children[table] {
			visit(child)
		}
		result = append(result, table)
	}
	for _, name := range names {
		visit(name)
	}
	return result
}

func migrationConnectionAdvice(connection config.Connection, err error) (string, string) {
	detail := strings.ToLower(err.Error())
	host := connectionHost(connection)
	local := isLocalHost(host)

	switch {
	case strings.Contains(detail, "ora-12564"),
		strings.Contains(detail, "connection refused"),
		strings.Contains(detail, "actively refused"),
		strings.Contains(detail, "no connection could be made"):
		if local {
			return fmt.Sprintf(
					"%s é uma conexão local; este erro não depende da internet e indica que a porta, o container ou o listener não aceitou a sessão",
					host,
				),
				"Confirme o container, a porta publicada e o listener Oracle; depois execute Connection e Migrate Run novamente."
		}
		return fmt.Sprintf(
				"%s é um host remoto; o servidor recusou a conexão e pode haver indisponibilidade, firewall, VPN ou falha de internet",
				host,
			),
			"Confira sua internet/VPN, DNS, firewall, host e porta; depois execute Connection e Migrate Run novamente."
	case strings.Contains(detail, "no such host"),
		strings.Contains(detail, "server misbehaving"),
		strings.Contains(detail, "name resolution"):
		return fmt.Sprintf(
				"não foi possível resolver o host %s; a causa pode ser DNS, VPN ou conexão com a internet",
				host,
			),
			"Confira sua internet/VPN e o nome do host no connection.yaml; depois execute Connection novamente."
	case strings.Contains(detail, "timeout"),
		strings.Contains(detail, "deadline exceeded"):
		if local {
			return fmt.Sprintf(
					"%s é local; o serviço não respondeu no tempo esperado e a internet não é necessária",
					host,
				),
				"Confira a saúde do container, a porta e os logs do banco; depois execute Connection novamente."
		}
		return fmt.Sprintf(
				"%s não respondeu; verifique internet, VPN, rota, firewall e disponibilidade do servidor",
				host,
			),
			"Teste sua internet/VPN e a conectividade com o host e a porta antes de repetir Migrate Run."
	case strings.Contains(detail, "ora-12514"):
		return "o listener respondeu, mas o serviço Oracle configurado não foi encontrado",
			"Confira ORACLE_SERVICE no .env e os serviços registrados no listener; depois execute Connection novamente."
	default:
		return "a conexão foi alcançada, mas o banco recusou ou interrompeu a operação",
			"Confira a mensagem técnica acima, execute Connection e corrija a configuração antes de repetir Migrate Run."
	}
}

func connectionHost(connection config.Connection) string {
	if strings.EqualFold(connection.Dialect, "mysql") {
		if start := strings.Index(connection.URL, "@tcp("); start >= 0 {
			remainder := connection.URL[start+5:]
			if end := strings.Index(remainder, ")"); end >= 0 {
				hostPort := remainder[:end]
				if parsed, err := url.Parse("tcp://" + hostPort); err == nil {
					return parsed.Hostname()
				}
			}
		}
	}
	if parsed, err := url.Parse(connection.URL); err == nil && parsed.Hostname() != "" {
		return parsed.Hostname()
	}
	return "host configurado"
}

func isLocalHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func loadPlans(folder string) ([]migrationFile, error) {
	entries, err := os.ReadDir(folder)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var result []migrationFile
	for _, entry := range entries {
		goMatch := goMigrationName.FindStringSubmatch(entry.Name())
		if entry.IsDir() || (!strings.HasSuffix(entry.Name(), "_migration.json") &&
			!strings.HasSuffix(entry.Name(), "_schema.json") &&
			len(goMatch) == 0) {
			continue
		}
		path := filepath.Join(folder, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var plan migrategen.Plan
		checksumData := data
		if strings.HasSuffix(entry.Name(), ".go") {
			operations, err := migrationgo.ParseFile(path)
			if err != nil {
				return nil, fmt.Errorf("ler migration %s: %w", entry.Name(), err)
			}
			plan = migrategen.Plan{Version: 1, Migration: entry.Name(), Operations: operations}
			checksumData, err = json.Marshal(operations)
			if err != nil {
				return nil, err
			}
		} else if err := json.Unmarshal(data, &plan); err != nil {
			return nil, fmt.Errorf("ler migration %s: %w", entry.Name(), err)
		}
		sum := sha256.Sum256(checksumData)
		id := entry.Name()
		provisional := false
		if len(goMatch) > 0 {
			id = goMatch[1]
			provisional = entry.Name() == id+".go"
		}
		result = append(result, migrationFile{Name: entry.Name(), ID: id, Path: path,
			Provisional: provisional, Checksum: hex.EncodeToString(sum[:]), Plan: plan})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	if err := validatePlans(result); err != nil {
		return nil, err
	}
	aliases := map[string]string{}
	for _, file := range result {
		for _, operation := range file.Plan.Operations {
			if operation.Kind == string(acao.CreateTable) {
				aliases[operation.AliasName] = operation.Table
			} else if operation.Kind == string(acao.RenameTable) {
				if _, exists := aliases[operation.Table]; exists {
					aliases[operation.Table] = operation.NewName
				}
			}
		}
	}
	if root := findProjectRoot(folder); root != "" {
		if err := migrationgo.WriteCoreCatalog(root, aliases); err != nil {
			return nil, fmt.Errorf("gerar catálogo de aliases no core: %w", err)
		}
	}
	return result, nil
}

func findProjectRoot(path string) string {
	current, _ := filepath.Abs(path)
	for {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		current = parent
	}
}

func validatePlans(files []migrationFile) error {
	physical := map[string]string{}
	aliases := map[string]string{}
	for _, file := range files {
		for _, operation := range file.Plan.Operations {
			if err := acao.Validar(operation); err != nil {
				return fmt.Errorf("pré-validação de %s: %w", file.Name, err)
			}
			if operation.Kind != string(acao.CreateTable) {
				if requiresTableAlias(operation.Kind) {
					reference := strings.ToLower(operation.Table)
					_, knownAlias := aliases[reference]
					_, knownPhysical := physical[reference]
					if !knownAlias && !knownPhysical {
						return fmt.Errorf("pré-validação de %s: alias de tabela %q não foi declarado por nenhum CreateTable anterior", file.Name, operation.Table)
					}
					if operation.Kind == string(acao.RenameTable) {
						newKey := strings.ToLower(operation.NewName)
						if previous, exists := physical[newKey]; exists {
							return fmt.Errorf("pré-validação de %s: RenameTable usaria o nome físico %q já declarado em %s", file.Name, operation.NewName, previous)
						}
						physical[newKey] = file.Name
					}
				}
				continue
			}
			key := strings.ToLower(operation.Table)
			if previous, exists := physical[key]; exists {
				return fmt.Errorf("pré-validação: CreateTable duplicado para %q em %s e %s", operation.Table, previous, file.Name)
			}
			alias := strings.ToLower(operation.AliasName)
			if previous, exists := aliases[alias]; exists {
				return fmt.Errorf("pré-validação: alias %q duplicado em %s e %s", operation.AliasName, previous, file.Name)
			}
			physical[key], aliases[alias] = file.Name, file.Name
		}
	}
	return nil
}

func requiresTableAlias(kind string) bool {
	switch acao.Tipo(kind) {
	case acao.DropTable, acao.AddColumn, acao.AlterColumn, acao.DropColumn,
		acao.AddForeignKey, acao.DropForeignKey, acao.CreateIndex, acao.DropIndex, acao.RenameTable,
		acao.RenameColumn, acao.AddPrimaryKey, acao.AddUnique, acao.AddCheck, acao.DropConstraint:
		return true
	default:
		return false
	}
}

func runConnection(connection config.Connection, historyTable string, files []migrationFile) (int, int, error) {
	dialect := strings.ToLower(connection.Dialect)
	driver := map[string]string{
		"oracle": "oracle", "postgres": "pgx", "mysql": "mysql", "sqlserver": "sqlserver",
	}[dialect]
	if driver == "" {
		return 0, 0, fmt.Errorf("dialeto não suportado: %s", dialect)
	}
	db, err := sql.Open(driver, connection.URL)
	if err != nil {
		return 0, 0, err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return 0, 0, err
	}
	if err := ensureHistory(ctx, db, dialect, connection.Schema, historyTable); err != nil {
		return 0, 0, fmt.Errorf("criar %s: %w", historyTable, err)
	}
	history, batch, err := loadHistory(ctx, db, dialect, connection.Schema, historyTable)
	if err != nil {
		return 0, 0, err
	}
	batch++
	applied, skipped := 0, 0
	aliases := map[string]string{}
	for _, file := range files {
		checksum, exists := history[file.ID]
		if !exists { // Compatibilidade com históricos antigos baseados no nome do arquivo.
			checksum, exists = history[file.Name]
		}
		if exists {
			if checksum != file.Checksum {
				return applied, skipped, fmt.Errorf("migration %s foi alterada depois de executada", file.Name)
			}
			skipped++
			advanceAliases(aliases, file.Plan.Operations)
			continue
		}
		created := map[string]bool{}
		for _, operation := range file.Plan.Operations {
			resolved := resolveAlias(operation, aliases)
			if err := executeOperation(ctx, db, dialect, connection.Schema, resolved, created); err != nil {
				return applied, skipped, fmt.Errorf("%s: %w", file.Name, err)
			}
		}
		if err := insertHistory(ctx, db, dialect, connection.Schema, historyTable, file, batch); err != nil {
			return applied, skipped, fmt.Errorf("registrar %s: %w", file.Name, err)
		}
		advanceAliases(aliases, file.Plan.Operations)
		applied++
	}
	return applied, skipped, nil
}

func resolveAlias(operation migrategen.Operation, aliases map[string]string) migrategen.Operation {
	if operation.Kind != string(acao.CreateTable) {
		if physical, exists := aliases[operation.Table]; exists {
			operation.Table = physical
		}
	}
	if operation.ForeignKey != nil {
		copyFK := *operation.ForeignKey
		if physical, exists := aliases[copyFK.ReferenceTable]; exists {
			copyFK.ReferenceTable = physical
		}
		operation.ForeignKey = &copyFK
	}
	return operation
}

func advanceAliases(aliases map[string]string, operations []migrategen.Operation) {
	for _, operation := range operations {
		switch operation.Kind {
		case string(acao.CreateTable):
			aliases[operation.AliasName] = operation.Table
		case string(acao.RenameTable):
			if _, exists := aliases[operation.Table]; exists {
				aliases[operation.Table] = operation.NewName
				continue
			}
			for alias, physical := range aliases {
				if physical == operation.Table {
					aliases[alias] = operation.NewName
				}
			}
		}
	}
}

func executeOperation(ctx context.Context, db *sql.DB, dialect, schema string, operation migrategen.Operation, created map[string]bool) error {
	switch operation.Kind {
	case "create_table":
		exists, err := tableExists(ctx, db, dialect, schema, operation.Table)
		if err != nil {
			return err
		}
		if exists {
			return nil
		}
		query, err := createTableSQL(dialect, schema, operation.Table, operation.Columns)
		if err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("criar tabela %s: %w", operation.Table, err)
		}
		created[operation.Table] = true
	case "add_column":
		if operation.Column == nil {
			return fmt.Errorf("operação add_column inválida")
		}
		exists, err := columnExists(ctx, db, dialect, schema, operation.Table, operation.Column.Name)
		if err != nil {
			return err
		}
		if exists {
			return nil
		}
		query := fmt.Sprintf("ALTER TABLE %s ADD %s", qualified(dialect, schema, operation.Table), columnDefinition(dialect, *operation.Column, false))
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("adicionar %s.%s: %w", operation.Table, operation.Column.Name, err)
		}
	case "alter_column":
		if operation.Column == nil {
			return fmt.Errorf("operação alter_column inválida")
		}
		var currentNullable *bool
		if dialect == "oracle" {
			nullable, err := oracleColumnNullable(ctx, db, schema, operation.Table, operation.Column.Name)
			if err != nil {
				return err
			}
			currentNullable = &nullable
		}
		for _, query := range alterColumnSQL(dialect, schema, operation.Table, *operation.Column, currentNullable) {
			if _, err := db.ExecContext(ctx, query); err != nil {
				return fmt.Errorf("alterar %s.%s: %w", operation.Table, operation.Column.Name, err)
			}
		}
	case "drop_column":
		if operation.Column == nil {
			return fmt.Errorf("operação drop_column inválida")
		}
		query := fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", qualified(dialect, schema, operation.Table), quote(dialect, operation.Column.Name))
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("remover %s.%s: %w", operation.Table, operation.Column.Name, err)
		}
	case "drop_table":
		if _, err := db.ExecContext(ctx, dropTableSQL(dialect, schema, operation.Table)); err != nil {
			return fmt.Errorf("remover tabela %s: %w", operation.Table, err)
		}
	case "add_foreign_key":
		if operation.ForeignKey == nil {
			return fmt.Errorf("operação add_foreign_key inválida")
		}
		fk := operation.ForeignKey
		name := fk.ConstraintName
		if name == "" {
			name = foreignKeyName(operation.Table, fk.Column)
		}
		onDelete := ""
		if fk.OnDelete != "" {
			onDelete = " ON DELETE " + fk.OnDelete
		}
		columns, references := fk.Columns, fk.ReferenceColumns
		if len(columns) == 0 {
			columns = []string{fk.Column}
		}
		if len(references) == 0 {
			references = []string{fk.ReferenceColumn}
		}
		query := fmt.Sprintf(
			"ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s)%s",
			qualified(dialect, schema, operation.Table), quote(dialect, name), strings.Join(quotedColumns(dialect, columns), ", "),
			qualified(dialect, schema, fk.ReferenceTable), strings.Join(quotedColumns(dialect, references), ", "), onDelete,
		)
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("criar relacionamento %s.%s: %w", operation.Table, fk.Column, err)
		}
	case "drop_foreign_key":
		if operation.ForeignKey == nil {
			return fmt.Errorf("operação drop_foreign_key inválida")
		}
		name := operation.ForeignKey.ConstraintName
		if name == "" {
			name = foreignKeyName(operation.Table, operation.ForeignKey.Column)
		}
		query := dropForeignKeySQL(dialect, schema, operation.Table, name)
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("remover relacionamento %s.%s: %w", operation.Table, operation.ForeignKey.Column, err)
		}
	case "create_index":
		unique := ""
		if operation.Unique {
			unique = "UNIQUE "
		}
		columns, err := indexColumnsSQL(ctx, db, dialect, schema, operation.Table, operation.IndexColumns)
		if err != nil {
			return fmt.Errorf("planejar índice %s: %w", operation.Name, err)
		}
		query := fmt.Sprintf("CREATE %sINDEX %s ON %s (%s)", unique, quote(dialect, operation.Name), qualified(dialect, schema, operation.Table), strings.Join(columns, ", "))
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("criar índice %s: %w", operation.Name, err)
		}
	case "drop_index":
		query := fmt.Sprintf("DROP INDEX %s", quote(dialect, operation.Name))
		if dialect == "mysql" {
			query = fmt.Sprintf("DROP INDEX %s ON %s", quote(dialect, operation.Name), qualified(dialect, schema, operation.Table))
		}
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("remover índice %s: %w", operation.Name, err)
		}
	case "create_view":
		verb := "CREATE VIEW"
		if dialect == "oracle" || dialect == "postgres" {
			verb = "CREATE OR REPLACE VIEW"
		}
		if _, err := db.ExecContext(ctx, fmt.Sprintf("%s %s AS %s", verb, qualified(dialect, schema, operation.Name), viewQuerySQL(dialect, operation.SQL))); err != nil {
			return fmt.Errorf("criar view %s: %w", operation.Name, err)
		}
	case "drop_view":
		if _, err := db.ExecContext(ctx, "DROP VIEW "+qualified(dialect, schema, operation.Name)); err != nil {
			return fmt.Errorf("remover view %s: %w", operation.Name, err)
		}
	case "create_sequence":
		if dialect == "mysql" {
			return fmt.Errorf("sequences não são suportadas pelo MySQL")
		}
		if _, err := db.ExecContext(ctx, "CREATE SEQUENCE "+qualified(dialect, schema, operation.Name)); err != nil {
			return fmt.Errorf("criar sequence %s: %w", operation.Name, err)
		}
	case "drop_sequence":
		if dialect == "mysql" {
			return fmt.Errorf("sequences não são suportadas pelo MySQL")
		}
		if _, err := db.ExecContext(ctx, "DROP SEQUENCE "+qualified(dialect, schema, operation.Name)); err != nil {
			return fmt.Errorf("remover sequence %s: %w", operation.Name, err)
		}
	case "rename_table":
		query := fmt.Sprintf("ALTER TABLE %s RENAME TO %s", qualified(dialect, schema, operation.Table), quote(dialect, operation.NewName))
		if dialect == "sqlserver" {
			query = fmt.Sprintf("EXEC sp_rename N'%s', N'%s'", objectName(schema, operation.Table), operation.NewName)
		}
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("renomear tabela %s: %w", operation.Table, err)
		}
	case "rename_column":
		if operation.Column == nil {
			return fmt.Errorf("operação rename_column inválida")
		}
		query := fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s", qualified(dialect, schema, operation.Table), quote(dialect, operation.Column.Name), quote(dialect, operation.NewName))
		if dialect == "sqlserver" {
			query = fmt.Sprintf("EXEC sp_rename N'%s.%s', N'%s', N'COLUMN'", objectName(schema, operation.Table), operation.Column.Name, operation.NewName)
		}
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("renomear coluna %s.%s: %w", operation.Table, operation.Column.Name, err)
		}
	case "add_primary_key", "add_unique":
		columns := quotedColumns(dialect, operation.IndexColumns)
		constraintType := "PRIMARY KEY"
		if operation.Kind == string(acao.AddUnique) {
			constraintType = "UNIQUE"
		}
		query := fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s %s (%s)", qualified(dialect, schema, operation.Table), quote(dialect, operation.Name), constraintType, strings.Join(columns, ", "))
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("criar constraint %s: %w", operation.Name, err)
		}
	case "add_check":
		query := fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s CHECK (%s)", qualified(dialect, schema, operation.Table), quote(dialect, operation.Name), checkExpressionSQL(dialect, operation.SQL))
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("criar check %s: %w", operation.Name, err)
		}
	case "drop_constraint":
		query := fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s", qualified(dialect, schema, operation.Table), quote(dialect, operation.Name))
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("remover constraint %s: %w", operation.Name, err)
		}
	case "raw_sql":
		if operation.Dialect != "" && operation.Dialect != "all" && !strings.EqualFold(operation.Dialect, dialect) {
			return nil
		}
		if _, err := db.ExecContext(ctx, operation.SQL); err != nil {
			return fmt.Errorf("executar SQL específico de %s: %w", operation.Dialect, err)
		}
	default:
		return fmt.Errorf("operação desconhecida: %s", operation.Kind)
	}
	return nil
}

var postgresAgeYearsPattern = regexp.MustCompile(`(?i)EXTRACT\s*\(\s*YEAR\s+FROM\s+AGE\s*\(\s*CURRENT_DATE\s*,\s*([a-zA-Z_][a-zA-Z0-9_.]*)\s*\)\s*\)`)

func viewQuerySQL(dialect, query string) string {
	switch dialect {
	case "mysql":
		query = postgresAgeYearsPattern.ReplaceAllString(query, `TIMESTAMPDIFF(YEAR, $1, CURRENT_DATE)`)
		query = regexp.MustCompile(`(?i)CAST\s*\(\s*NULL\s+AS\s+VARCHAR\s*\(\s*(\d+)\s*\)\s*\)`).ReplaceAllString(query, `CAST(NULL AS CHAR($1))`)
		query = regexp.MustCompile(`(?i)CAST\s*\(\s*NULL\s+AS\s+TIMESTAMP\s*\)`).ReplaceAllString(query, `CAST(NULL AS DATETIME)`)
	case "oracle":
		query = postgresAgeYearsPattern.ReplaceAllString(query, `TRUNC(MONTHS_BETWEEN(CURRENT_DATE, $1) / 12)`)
		query = regexp.MustCompile(`(?i)CAST\s*\(\s*NULL\s+AS\s+VARCHAR\s*\(`).ReplaceAllString(query, `CAST(NULL AS VARCHAR2(`)
	case "sqlserver":
		query = postgresAgeYearsPattern.ReplaceAllString(query, `DATEDIFF(YEAR, $1, CAST(GETDATE() AS date))`)
		query = regexp.MustCompile(`(?i)CAST\s*\(\s*NULL\s+AS\s+TIMESTAMP\s*\)`).ReplaceAllString(query, `CAST(NULL AS DATETIME2)`)
		query = regexp.MustCompile(`(?i)\bCURRENT_DATE\b`).ReplaceAllString(query, `CAST(GETDATE() AS date)`)
	}
	return query
}

func indexColumnsSQL(ctx context.Context, db *sql.DB, dialect, schema, table string, columns []string) ([]string, error) {
	result := quotedColumns(dialect, columns)
	if dialect != "mysql" || len(columns) == 0 {
		return result, nil
	}

	// InnoDB permits 3072 bytes per index. Four bytes per utf8mb4 character
	// gives a safe portable character budget, shared across composite columns.
	characterBudget := 768 / len(columns)
	if characterBudget < 1 {
		characterBudget = 1
	}
	for index, column := range columns {
		var dataType string
		var length sql.NullInt64
		err := db.QueryRowContext(ctx, `
			SELECT data_type, character_maximum_length
			FROM information_schema.columns
			WHERE table_schema = COALESCE(NULLIF(?, ''), DATABASE())
			  AND table_name = ? AND column_name = ?`, schema, table, column).Scan(&dataType, &length)
		if err != nil {
			return nil, err
		}
		dataType = strings.ToLower(dataType)
		isText := strings.Contains(dataType, "char") || strings.Contains(dataType, "text")
		if isText && (!length.Valid || length.Int64 > int64(characterBudget)) {
			result[index] = fmt.Sprintf("%s(%d)", quote(dialect, column), characterBudget)
		}
	}
	return result, nil
}

func quotedColumns(dialect string, columns []string) []string {
	result := make([]string, len(columns))
	for index, column := range columns {
		result[index] = quote(dialect, column)
	}
	return result
}

// checkExpressionSQL keeps migration CHECK expressions portable. Authors use
// plain logical column names; identifiers are quoted only for the target DB.
func checkExpressionSQL(dialect, expression string) string {
	keywords := map[string]bool{
		"AND": true, "OR": true, "NOT": true, "NULL": true, "IS": true,
		"IN": true, "BETWEEN": true, "LIKE": true, "TRUE": true, "FALSE": true,
		"CASE": true, "WHEN": true, "THEN": true, "ELSE": true, "END": true,
		"CURRENT_DATE": true, "CURRENT_TIMESTAMP": true, "CURRENT_USER": true,
	}

	var result strings.Builder
	for index := 0; index < len(expression); {
		if expression[index] == '\'' {
			start := index
			index++
			for index < len(expression) {
				if expression[index] != '\'' {
					index++
					continue
				}
				index++
				if index < len(expression) && expression[index] == '\'' {
					index++
					continue
				}
				break
			}
			result.WriteString(expression[start:index])
			continue
		}

		if isSQLIdentifierStart(expression[index]) {
			start := index
			index++
			for index < len(expression) && isSQLIdentifierPart(expression[index]) {
				index++
			}
			token := expression[start:index]
			lookahead := index
			for lookahead < len(expression) && strings.ContainsRune(" \t\r\n", rune(expression[lookahead])) {
				lookahead++
			}
			if keywords[strings.ToUpper(token)] || (lookahead < len(expression) && expression[lookahead] == '(') {
				result.WriteString(token)
			} else {
				result.WriteString(quote(dialect, token))
			}
			continue
		}

		result.WriteByte(expression[index])
		index++
	}
	return result.String()
}

func isSQLIdentifierStart(value byte) bool {
	return value == '_' || value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
}

func isSQLIdentifierPart(value byte) bool {
	return isSQLIdentifierStart(value) || value >= '0' && value <= '9'
}

func foreignKeyName(table, column string) string {
	name := "fk_" + table + "_" + column
	if len(name) > 30 {
		name = name[:30]
	}
	return name
}

func dropForeignKeySQL(dialect, schema, table, name string) string {
	target, constraint := qualified(dialect, schema, table), quote(dialect, name)
	switch dialect {
	case "mysql":
		return fmt.Sprintf("ALTER TABLE %s DROP FOREIGN KEY %s", target, constraint)
	case "postgres":
		return fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT IF EXISTS %s", target, constraint)
	default:
		return fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s", target, constraint)
	}
}

func ensureHistory(ctx context.Context, db *sql.DB, dialect, schema, table string) error {
	if dialect == "oracle" {
		if err := repairLegacyOracleHistory(ctx, db, schema, table); err != nil {
			return err
		}
	}
	target := qualified(dialect, schema, table)
	var query string
	switch dialect {
	case "postgres":
		query = fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
			id BIGSERIAL PRIMARY KEY, migration VARCHAR(255) NOT NULL UNIQUE,
			checksum VARCHAR(64) NOT NULL, batch INTEGER NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP)`, target)
	case "mysql":
		query = fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
			id BIGINT AUTO_INCREMENT PRIMARY KEY, migration VARCHAR(255) NOT NULL UNIQUE,
			checksum VARCHAR(64) NOT NULL, batch INT NOT NULL,
			applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP)`, target)
	case "sqlserver":
		query = fmt.Sprintf(`IF OBJECT_ID(N'%s', N'U') IS NULL CREATE TABLE %s (
			id BIGINT IDENTITY(1,1) PRIMARY KEY, migration NVARCHAR(255) NOT NULL UNIQUE,
			checksum VARCHAR(64) NOT NULL, batch INT NOT NULL,
			applied_at DATETIME2 NOT NULL DEFAULT SYSUTCDATETIME())`,
			objectName(schema, table), target)
	case "oracle":
		query = fmt.Sprintf(`BEGIN EXECUTE IMMEDIATE 'CREATE TABLE %s (
			id NUMBER(19) GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
			migration VARCHAR2(255) NOT NULL UNIQUE, checksum VARCHAR2(64) NOT NULL,
			batch NUMBER(10) NOT NULL, applied_at TIMESTAMP DEFAULT SYSTIMESTAMP NOT NULL)';
			EXCEPTION WHEN OTHERS THEN IF SQLCODE != -955 THEN RAISE; END IF; END;`, target)
	default:
		return fmt.Errorf("dialeto não suportado: %s", dialect)
	}
	_, err := db.ExecContext(ctx, query)
	return err
}

func loadHistory(ctx context.Context, db *sql.DB, dialect, schema, table string) (map[string]string, int, error) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf("SELECT migration, checksum, batch FROM %s ORDER BY id", qualified(dialect, schema, table)))
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	history, maxBatch := map[string]string{}, 0
	for rows.Next() {
		var migration, checksum string
		var batch int
		if err := rows.Scan(&migration, &checksum, &batch); err != nil {
			return nil, 0, err
		}
		history[migration] = checksum
		if batch > maxBatch {
			maxBatch = batch
		}
	}
	return history, maxBatch, rows.Err()
}

func insertHistory(ctx context.Context, db *sql.DB, dialect, schema, table string, file migrationFile, batch int) error {
	target := qualified(dialect, schema, table)
	query := map[string]string{
		"postgres":  fmt.Sprintf("INSERT INTO %s (migration, checksum, batch) VALUES ($1, $2, $3)", target),
		"oracle":    fmt.Sprintf("INSERT INTO %s (migration, checksum, batch) VALUES (:1, :2, :3)", target),
		"mysql":     fmt.Sprintf("INSERT INTO %s (migration, checksum, batch) VALUES (?, ?, ?)", target),
		"sqlserver": fmt.Sprintf("INSERT INTO %s (migration, checksum, batch) VALUES (@p1, @p2, @p3)", target),
	}[dialect]
	_, err := db.ExecContext(ctx, query, file.ID, file.Checksum, batch)
	return err
}

func createTableSQL(dialect, schema, table string, columns []migrategen.Column) (string, error) {
	if len(columns) == 0 {
		return "", fmt.Errorf("tabela %s não possui colunas", table)
	}
	definitions := make([]string, 0, len(columns))
	for _, column := range columns {
		definitions = append(definitions, columnDefinition(dialect, column, true))
	}
	return fmt.Sprintf("CREATE TABLE %s (%s)", qualified(dialect, schema, table), strings.Join(definitions, ", ")), nil
}

func alterColumnSQL(dialect, schema, table string, column migrategen.Column, currentNullable *bool) []string {
	target, name := qualified(dialect, schema, table), quote(dialect, column.Name)
	definition := columnDefinition(dialect, column, false)
	switch dialect {
	case "postgres":
		dataType := strings.TrimSpace(strings.TrimPrefix(definition, name))
		dataType = strings.TrimSuffix(strings.TrimSuffix(dataType, " NOT NULL"), " NULL")
		nullability := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET NOT NULL", target, name)
		if column.Nullable {
			nullability = fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP NOT NULL", target, name)
		}
		return []string{
			fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s", target, name, dataType),
			nullability,
		}
	case "oracle":
		dataType := strings.TrimSpace(strings.TrimPrefix(definition, name))
		dataType = strings.TrimSuffix(strings.TrimSuffix(dataType, " NOT NULL"), " NULL")
		queries := []string{fmt.Sprintf("ALTER TABLE %s MODIFY (%s %s)", target, name, dataType)}
		if currentNullable != nil && *currentNullable != column.Nullable {
			nullability := "NOT NULL"
			if column.Nullable {
				nullability = "NULL"
			}
			queries = append(queries, fmt.Sprintf("ALTER TABLE %s MODIFY (%s %s)", target, name, nullability))
		}
		return queries
	case "mysql":
		return []string{fmt.Sprintf("ALTER TABLE %s MODIFY COLUMN %s", target, definition)}
	default:
		return []string{fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s", target, definition)}
	}
}

func dropTableSQL(dialect, schema, table string) string {
	target := qualified(dialect, schema, table)
	switch dialect {
	case "postgres":
		return "DROP TABLE " + target + " CASCADE"
	case "oracle":
		return "DROP TABLE " + target + " CASCADE CONSTRAINTS PURGE"
	default:
		return "DROP TABLE " + target
	}
}

func columnDefinition(dialect string, column migrategen.Column, allowIdentity bool) string {
	name := quote(dialect, column.Name)
	if column.AutoIncrement && allowIdentity {
		identity := map[string]string{
			"postgres": "BIGSERIAL", "oracle": "NUMBER(19) GENERATED BY DEFAULT AS IDENTITY",
			"mysql": "BIGINT AUTO_INCREMENT", "sqlserver": "BIGINT IDENTITY(1,1)",
		}[dialect]
		definition := name + " " + identity
		if column.PrimaryKey {
			definition += " PRIMARY KEY"
		}
		if column.Unique && !column.PrimaryKey {
			definition += " UNIQUE"
		}
		if column.Default != "" {
			definition += " DEFAULT " + columnDefaultSQL(dialect, column)
		}
		return definition
	}
	stringLength := column.Length
	if stringLength == 0 {
		stringLength = 255
	}
	precision, scale := column.Precision, column.Scale
	if precision == 0 {
		precision, scale = 19, 4
	}
	dataType := map[string]map[string]string{
		"int":       {"postgres": "INTEGER", "oracle": "NUMBER(10)", "mysql": "INT", "sqlserver": "INT"},
		"integer":   {"postgres": "BIGINT", "oracle": "NUMBER(19)", "mysql": "BIGINT", "sqlserver": "BIGINT"},
		"string":    {"postgres": fmt.Sprintf("VARCHAR(%d)", stringLength), "oracle": fmt.Sprintf("VARCHAR2(%d)", stringLength), "mysql": fmt.Sprintf("VARCHAR(%d)", stringLength), "sqlserver": fmt.Sprintf("NVARCHAR(%d)", stringLength)},
		"char":      {"postgres": fmt.Sprintf("CHAR(%d)", stringLength), "oracle": fmt.Sprintf("CHAR(%d)", stringLength), "mysql": fmt.Sprintf("CHAR(%d)", stringLength), "sqlserver": fmt.Sprintf("NCHAR(%d)", stringLength)},
		"text":      {"postgres": "TEXT", "oracle": "CLOB", "mysql": "LONGTEXT", "sqlserver": "NVARCHAR(MAX)"},
		"boolean":   {"postgres": "BOOLEAN", "oracle": "NUMBER(1)", "mysql": "BOOLEAN", "sqlserver": "BIT"},
		"decimal":   {"postgres": fmt.Sprintf("DECIMAL(%d,%d)", precision, scale), "oracle": fmt.Sprintf("NUMBER(%d,%d)", precision, scale), "mysql": fmt.Sprintf("DECIMAL(%d,%d)", precision, scale), "sqlserver": fmt.Sprintf("DECIMAL(%d,%d)", precision, scale)},
		"datetime":  {"postgres": "TIMESTAMPTZ", "oracle": "TIMESTAMP", "mysql": "DATETIME", "sqlserver": "DATETIME2"},
		"timestamp": {"postgres": "TIMESTAMP", "oracle": "TIMESTAMP", "mysql": "DATETIME", "sqlserver": "DATETIME2"},
		"binary":    {"postgres": "BYTEA", "oracle": "BLOB", "mysql": "LONGBLOB", "sqlserver": "VARBINARY(MAX)"},
	}[column.Type][dialect]
	nullability := " NOT NULL"
	if column.Nullable {
		nullability = " NULL"
	}
	definition := name + " " + dataType
	if column.PrimaryKey {
		definition += " PRIMARY KEY"
	} else if column.Unique {
		definition += " UNIQUE"
	}
	if column.Default != "" {
		definition += " DEFAULT " + columnDefaultSQL(dialect, column)
	}
	return definition + nullability
}

func columnDefaultSQL(dialect string, column migrategen.Column) string {
	if !column.DefaultRaw {
		return defaultSQL(dialect, column.Default)
	}
	lower := strings.ToLower(strings.TrimSpace(column.Default))
	switch lower {
	case "current_user", "user":
		if dialect == "oracle" {
			return "USER"
		}
		if dialect == "mysql" {
			return "(CURRENT_USER())"
		}
		return "CURRENT_USER"
	case "current_timestamp", "sysdate", "systimestamp":
		if dialect == "oracle" {
			return "SYSTIMESTAMP"
		}
		return "CURRENT_TIMESTAMP"
	default:
		return column.Default
	}
}

func defaultSQL(dialect, value string) string {
	lower := strings.ToLower(strings.TrimSpace(value))
	if lower == "true" {
		if dialect == "oracle" {
			return "1"
		}
		return "TRUE"
	}
	if lower == "false" {
		if dialect == "oracle" {
			return "0"
		}
		return "FALSE"
	}
	if lower == "null" || lower == "current_timestamp" || regexp.MustCompile(`^-?\d+(\.\d+)?$`).MatchString(lower) {
		return strings.ToUpper(lower)
	}
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func tableExists(ctx context.Context, db *sql.DB, dialect, schema, table string) (bool, error) {
	var count int
	switch dialect {
	case "postgres":
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=$1 AND table_name=$2", schemaOr(schema, "public"), table).Scan(&count)
		return count > 0, err
	case "mysql":
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=?", table).Scan(&count)
		return count > 0, err
	case "sqlserver":
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=@p1 AND table_name=@p2", schemaOr(schema, "dbo"), table).Scan(&count)
		return count > 0, err
	default:
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM all_tables WHERE owner=:1 AND table_name=:2", strings.ToUpper(schema), strings.ToUpper(table)).Scan(&count)
		return count > 0, err
	}
}

func viewExists(ctx context.Context, db *sql.DB, dialect, schema, view string) (bool, error) {
	var count int
	switch dialect {
	case "postgres":
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.views WHERE table_schema=$1 AND table_name=$2", schemaOr(schema, "public"), view).Scan(&count)
		return count > 0, err
	case "mysql":
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.views WHERE table_schema=DATABASE() AND table_name=?", view).Scan(&count)
		return count > 0, err
	case "sqlserver":
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.views WHERE table_schema=@p1 AND table_name=@p2", schemaOr(schema, "dbo"), view).Scan(&count)
		return count > 0, err
	default:
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM all_views WHERE owner=:1 AND view_name=:2", strings.ToUpper(schema), strings.ToUpper(view)).Scan(&count)
		return count > 0, err
	}
}

func columnExists(ctx context.Context, db *sql.DB, dialect, schema, table, column string) (bool, error) {
	var count int
	switch dialect {
	case "postgres":
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=$1 AND table_name=$2 AND column_name=$3", schemaOr(schema, "public"), table, column).Scan(&count)
		return count > 0, err
	case "mysql":
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name=? AND column_name=?", table, column).Scan(&count)
		return count > 0, err
	case "sqlserver":
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=@p1 AND table_name=@p2 AND column_name=@p3", schemaOr(schema, "dbo"), table, column).Scan(&count)
		return count > 0, err
	default:
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM all_tab_columns WHERE owner=:1 AND table_name=:2 AND column_name=:3", strings.ToUpper(schema), strings.ToUpper(table), strings.ToUpper(column)).Scan(&count)
		return count > 0, err
	}
}

func oracleColumnNullable(ctx context.Context, db *sql.DB, schema, table, column string) (bool, error) {
	var nullable string
	err := db.QueryRowContext(ctx,
		"SELECT nullable FROM all_tab_columns WHERE owner=:1 AND table_name=:2 AND column_name=:3",
		strings.ToUpper(schema), strings.ToUpper(table), strings.ToUpper(column),
	).Scan(&nullable)
	if err != nil {
		return false, err
	}
	return strings.EqualFold(nullable, "Y"), nil
}

func qualified(dialect, schema, table string) string {
	if strings.TrimSpace(schema) == "" || dialect == "mysql" {
		return quote(dialect, table)
	}
	return quote(dialect, schema) + "." + quote(dialect, table)
}

func quote(dialect, value string) string {
	switch dialect {
	case "mysql":
		return "`" + strings.ReplaceAll(value, "`", "``") + "`"
	case "sqlserver":
		return "[" + strings.ReplaceAll(value, "]", "]]") + "]"
	case "oracle":
		return `"` + strings.ReplaceAll(strings.ToUpper(value), `"`, `""`) + `"`
	default:
		return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
	}
}

func repairLegacyOracleHistory(ctx context.Context, db *sql.DB, schema, table string) error {
	owner := strings.ToUpper(schema)
	var current, legacy int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM all_tables WHERE owner=:1 AND table_name=:2",
		owner, strings.ToUpper(table),
	).Scan(&current); err != nil {
		return err
	}
	if current > 0 {
		return nil
	}
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM all_tables WHERE owner=:1 AND table_name=:2",
		owner, table,
	).Scan(&legacy); err != nil {
		return err
	}
	if legacy == 0 {
		return nil
	}
	legacyTarget := `"` + strings.ReplaceAll(owner, `"`, `""`) + `"."` + strings.ReplaceAll(table, `"`, `""`) + `"`
	_, err := db.ExecContext(ctx, fmt.Sprintf(
		"ALTER TABLE %s RENAME TO %s",
		legacyTarget,
		quote("oracle", table),
	))
	return err
}

func historyLocation(connection config.Connection, table string) string {
	dialect := strings.ToLower(connection.Dialect)
	schema := connection.Schema
	host, database := "", ""
	if dialect == "mysql" {
		value := connection.URL
		if start := strings.Index(value, "@tcp("); start >= 0 {
			remainder := value[start+5:]
			if end := strings.Index(remainder, ")"); end >= 0 {
				host = remainder[:end]
				after := strings.TrimPrefix(remainder[end+1:], "/")
				database = strings.SplitN(after, "?", 2)[0]
			}
		}
	} else if parsed, err := url.Parse(connection.URL); err == nil {
		host = parsed.Host
		database = strings.TrimPrefix(parsed.Path, "/")
		if dialect == "sqlserver" {
			database = parsed.Query().Get("database")
		}
	}
	location := host
	if database != "" {
		location += "/" + database
	}
	if schema != "" {
		location += "." + schema
	}
	if location != "" {
		location += "."
	}
	return location + table
}

func objectName(schema, table string) string {
	return schemaOr(schema, "dbo") + "." + table
}

func schemaOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
