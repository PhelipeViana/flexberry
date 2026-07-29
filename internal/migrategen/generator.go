package migrategen

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/PhelipeViana/flexberry/internal/config"
	"github.com/PhelipeViana/flexberry/internal/scanner"
)

const snapshotName = "schema.snapshot.json"

type Column struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Nullable   bool   `json:"nullable"`
	PrimaryKey bool   `json:"primary_key,omitempty"`
}

type ForeignKey struct {
	Column          string `json:"column"`
	ReferenceTable  string `json:"reference_table"`
	ReferenceColumn string `json:"reference_column"`
}

type Table struct {
	Name        string       `json:"name"`
	Columns     []Column     `json:"columns"`
	ForeignKeys []ForeignKey `json:"foreign_keys,omitempty"`
}

type Snapshot struct {
	Version int     `json:"version"`
	Tables  []Table `json:"tables"`
}

type Operation struct {
	Kind       string      `json:"kind"`
	Table      string      `json:"table"`
	Columns    []Column    `json:"columns,omitempty"`
	Column     *Column     `json:"column,omitempty"`
	ForeignKey *ForeignKey `json:"foreign_key,omitempty"`
}

type Plan struct {
	Version    int         `json:"version"`
	Migration  string      `json:"migration"`
	CreatedAt  time.Time   `json:"created_at"`
	Operations []Operation `json:"operations"`
}

type Result struct {
	Path       string
	Migration  string
	Operations int
	Unchanged  bool
}

func Generate(root string, cfg config.MigrateConfig, entities []scanner.Entity) (Result, error) {
	output := filepath.Join(root, filepath.FromSlash(cfg.Output.Path))
	relative, err := filepath.Rel(root, output)
	if err != nil || relative == "." || relative == ".." || filepath.IsAbs(relative) {
		return Result{}, fmt.Errorf("migrate output.path precisa ficar dentro do projeto")
	}
	if err := os.MkdirAll(output, 0o755); err != nil {
		return Result{}, fmt.Errorf("criar pasta de migrations: %w", err)
	}

	current, err := buildSnapshot(entities)
	if err != nil {
		return Result{}, err
	}
	previous, err := loadSnapshot(filepath.Join(output, snapshotName))
	if err != nil {
		return Result{}, err
	}
	operations, err := diff(previous, current)
	if err != nil {
		return Result{}, err
	}
	if len(operations) == 0 {
		return Result{Unchanged: true}, nil
	}

	migration := nextMigrationName(output, time.Now())
	plan := Plan{Version: 1, Migration: migration, CreatedAt: time.Now().UTC(), Operations: operations}
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return Result{}, err
	}
	data = append(data, '\n')
	path := filepath.Join(output, migration)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return Result{}, fmt.Errorf("escrever migration: %w", err)
	}
	snapshotData, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return Result{}, err
	}
	if err := os.WriteFile(filepath.Join(output, snapshotName), append(snapshotData, '\n'), 0o644); err != nil {
		return Result{}, fmt.Errorf("escrever snapshot: %w", err)
	}
	return Result{
		Path:       filepath.ToSlash(filepath.Join(cfg.Output.Path, migration)),
		Migration:  migration,
		Operations: len(operations),
	}, nil
}

func buildSnapshot(entities []scanner.Entity) (Snapshot, error) {
	byEntity := make(map[string]scanner.Entity, len(entities))
	for _, entity := range entities {
		byEntity[entity.Name] = entity
	}
	result := Snapshot{Version: 1}
	for _, entity := range entities {
		table := Table{Name: entity.Table}
		for _, field := range entity.Fields {
			kind, err := neutralType(field.GoType)
			if err != nil {
				return Snapshot{}, fmt.Errorf("%s.%s: %w", entity.Name, field.Name, err)
			}
			table.Columns = append(table.Columns, Column{
				Name: field.Column, Type: kind, Nullable: field.Nullable, PrimaryKey: field.PrimaryKey,
			})
		}
		for _, relation := range entity.Relations {
			if relation.Kind != "belongsTo" || relation.ForeignKey == "" {
				continue
			}
			target, exists := byEntity[relationTypeName(relation.Type)]
			if !exists {
				continue
			}
			table.ForeignKeys = append(table.ForeignKeys, ForeignKey{
				Column: relation.ForeignKey, ReferenceTable: target.Table, ReferenceColumn: target.PrimaryKey,
			})
		}
		sort.Slice(table.Columns, func(i, j int) bool { return table.Columns[i].Name < table.Columns[j].Name })
		sort.Slice(table.ForeignKeys, func(i, j int) bool { return table.ForeignKeys[i].Column < table.ForeignKeys[j].Column })
		result.Tables = append(result.Tables, table)
	}
	sort.Slice(result.Tables, func(i, j int) bool { return result.Tables[i].Name < result.Tables[j].Name })
	return result, nil
}

func neutralType(goType string) (string, error) {
	value := strings.TrimLeft(strings.TrimSpace(goType), "*")
	switch value {
	case "string":
		return "string", nil
	case "bool":
		return "boolean", nil
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64":
		return "integer", nil
	case "float32", "float64":
		return "decimal", nil
	case "time.Time":
		return "datetime", nil
	case "[]byte":
		return "binary", nil
	default:
		return "", fmt.Errorf("tipo Go %q ainda não é suportado por migrations", goType)
	}
}

func loadSnapshot(path string) (Snapshot, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Snapshot{Version: 1}, nil
	}
	if err != nil {
		return Snapshot{}, err
	}
	var value Snapshot
	if err := json.Unmarshal(data, &value); err != nil {
		return Snapshot{}, fmt.Errorf("ler snapshot de migrations: %w", err)
	}
	return value, nil
}

func diff(previous, current Snapshot) ([]Operation, error) {
	oldTables := tableMap(previous.Tables)
	newTables := tableMap(current.Tables)
	var operations []Operation
	for name, oldTable := range oldTables {
		newTable, exists := newTables[name]
		if !exists {
			operations = append(operations, Operation{Kind: "drop_table", Table: name})
			continue
		}
		oldColumns, newColumns := columnMap(oldTable.Columns), columnMap(newTable.Columns)
		for column, oldValue := range oldColumns {
			newValue, exists := newColumns[column]
			if !exists {
				if foreignKeyColumn(oldTable, column) {
					return nil, fmt.Errorf("remoção de %s.%s foi bloqueada porque a coluna participa de um relacionamento", name, column)
				}
				copyColumn := oldValue
				operations = append(operations, Operation{Kind: "drop_column", Table: name, Column: &copyColumn})
				continue
			}
			if oldValue.Type != newValue.Type || oldValue.Nullable != newValue.Nullable {
				if oldValue.PrimaryKey != newValue.PrimaryKey {
					return nil, fmt.Errorf("alteração automática de chave primária em %s.%s foi bloqueada", name, column)
				}
				copyColumn := newValue
				operations = append(operations, Operation{Kind: "alter_column", Table: name, Column: &copyColumn})
			}
		}
	}

	for _, table := range current.Tables {
		oldTable, exists := oldTables[table.Name]
		if !exists {
			operations = append(operations, Operation{Kind: "create_table", Table: table.Name, Columns: table.Columns})
			continue
		}
		oldColumns := columnMap(oldTable.Columns)
		for index := range table.Columns {
			column := table.Columns[index]
			if _, exists := oldColumns[column.Name]; !exists {
				copyColumn := column
				operations = append(operations, Operation{Kind: "add_column", Table: table.Name, Column: &copyColumn})
			}
		}
	}
	if len(previous.Tables) == 0 {
		for _, table := range current.Tables {
			for index := range table.ForeignKeys {
				fk := table.ForeignKeys[index]
				operations = append(operations, Operation{Kind: "add_foreign_key", Table: table.Name, ForeignKey: &fk})
			}
		}
	}
	return operations, nil
}

func nextMigrationName(output string, now time.Time) string {
	for {
		name := now.Format("2006_01_02_150405") + "_migration.json"
		if _, err := os.Stat(filepath.Join(output, name)); os.IsNotExist(err) {
			return name
		}
		now = now.Add(time.Second)
	}
}

func tableMap(values []Table) map[string]Table {
	result := make(map[string]Table, len(values))
	for _, value := range values {
		result[value.Name] = value
	}
	return result
}

func columnMap(values []Column) map[string]Column {
	result := make(map[string]Column, len(values))
	for _, value := range values {
		result[value.Name] = value
	}
	return result
}

func foreignKeyColumn(table Table, column string) bool {
	for _, foreignKey := range table.ForeignKeys {
		if foreignKey.Column == column {
			return true
		}
	}
	return false
}

func relationTypeName(value string) string {
	value = strings.TrimLeft(value, "*[]")
	if index := strings.LastIndex(value, "."); index >= 0 {
		return value[index+1:]
	}
	return value
}
