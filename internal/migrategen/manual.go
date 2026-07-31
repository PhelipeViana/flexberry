package migrategen

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/PhelipeViana/flexberry/internal/config"
)

func ensureManualAliasImports(root string, cfg config.MigrateConfig) error {
	modulePath, err := readModulePath(root)
	if err != nil {
		return err
	}
	importPath := modulePath + "/internal/flexberry/core/migration/alias"
	entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(cfg.Output.Path)))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || entry.Name() == "dsl.gen.go" {
			continue
		}
		path := filepath.Join(root, filepath.FromSlash(cfg.Output.Path), entry.Name())
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if !bytes.Contains(data, []byte("alias.")) || bytes.Contains(data, []byte(importPath)) {
			continue
		}
		text := string(data)
		const single = `import migrate "github.com/PhelipeViana/flexberry/migration"`
		if strings.Contains(text, single) {
			text = strings.Replace(text, single, "import (\n\tmigrate \"github.com/PhelipeViana/flexberry/migration\"\n\talias \""+importPath+"\"\n)", 1)
		} else if strings.Contains(text, "import (") {
			text = strings.Replace(text, "import (", "import (\n\talias \""+importPath+"\"", 1)
		} else {
			return fmt.Errorf("%s usa alias.* mas o bloco de imports não pôde ser atualizado", entry.Name())
		}
		formatted, formatErr := format.Source([]byte(text))
		if formatErr != nil {
			return fmt.Errorf("atualizar import alias em %s: %w", entry.Name(), formatErr)
		}
		if err := os.WriteFile(path, formatted, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func CreateManual(root string, cfg config.MigrateConfig) (string, error) {
	return createManual(root, cfg, true)
}

func CreateBlank(root string, cfg config.MigrateConfig) (string, error) {
	return createManual(root, cfg, false)
}

func createManual(root string, cfg config.MigrateConfig, examples bool) (string, error) {
	folder := filepath.Join(root, filepath.FromSlash(cfg.Output.Path))
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return "", err
	}
	stamp := time.Now()
	var id, path string
	for {
		id = stamp.Format("2006_01_02_150405")
		path = filepath.Join(folder, id+".go")
		matches, _ := filepath.Glob(filepath.Join(folder, id+"*.go"))
		if len(matches) == 0 {
			break
		}
		stamp = stamp.Add(time.Second)
	}
	function := "Migration" + stamp.Format("20060102150405")
	modulePath, err := readModulePath(root)
	if err != nil {
		return "", err
	}
	comment := ""
	if examples {
		comment = `// Substitua migrate.TODO() por exatamente um método:
		// migrate.CreateTable("nome_da_tabela",
		// 	migrate.Col("id").Integer().PrimaryKey().AutoIncrement(),
		// 	migrate.Col("nome").Varchar(255),
		// ).Alias("nomeDaTabela")
		// migrate.AddColumn(alias.NomeDaTabela, migrate.Col("nova_coluna").Varchar(255))
		// migrate.RenameTable(alias.NomeDaTabela, "novo_nome")`
	}
	source := fmt.Sprintf(`package migrations

import (
	migrate "github.com/PhelipeViana/flexberry/migration"
	alias %q
)

var _ = alias.Catalog

// %s é uma migration manual provisória.
// Adicione as operações e execute Migration Run.
func %s() migrate.Definition {
	%s
	return migrate.TODO()
}
`, modulePath+"/internal/flexberry/core/migration/alias", function, function, comment)
	formatted, err := format.Source([]byte(source))
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, formatted, 0o644); err != nil {
		return "", err
	}
	return path, nil
}
