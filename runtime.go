package flexberry

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"sync"
)

type Executor interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

type registeredConnection struct {
	executor Executor
	dialect  string
}

var connections = struct {
	sync.RWMutex
	defaultName string
	values      map[string]registeredConnection
}{values: make(map[string]registeredConnection)}

func Register(name string, executor Executor, dialect string) error {
	name = strings.ToLower(strings.TrimSpace(name))
	dialect = strings.ToLower(strings.TrimSpace(dialect))
	if name == "" || executor == nil {
		return fmt.Errorf("nome e executor da conexão são obrigatórios")
	}
	switch dialect {
	case "postgres", "oracle", "mysql", "sqlserver":
	default:
		return fmt.Errorf("dialeto %q não suportado", dialect)
	}
	connections.Lock()
	defer connections.Unlock()
	connections.values[name] = registeredConnection{executor: executor, dialect: dialect}
	if connections.defaultName == "" {
		connections.defaultName = name
	}
	return nil
}

func SetDefault(name string) error {
	name = strings.ToLower(strings.TrimSpace(name))
	connections.Lock()
	defer connections.Unlock()
	if _, ok := connections.values[name]; !ok {
		return fmt.Errorf("conexão %q não registrada", name)
	}
	connections.defaultName = name
	return nil
}

func ResetConnections() {
	connections.Lock()
	defer connections.Unlock()
	connections.values = make(map[string]registeredConnection)
	connections.defaultName = ""
}

func connection(name string) (registeredConnection, error) {
	connections.RLock()
	defer connections.RUnlock()
	if name == "" {
		name = connections.defaultName
	}
	value, ok := connections.values[strings.ToLower(name)]
	if !ok {
		return registeredConnection{}, fmt.Errorf("conexão %q não registrada", name)
	}
	return value, nil
}

type condition struct {
	column string
	op     string
	value  any
}

type order struct {
	column string
	desc   bool
}

type Query[T any] struct {
	mapping    EntityMapping
	connection string
	conditions []condition
	orders     []order
	limit      int
	offset     int
	err        error
}

type PageRequest struct {
	Page    int
	PerPage int
}

type Page[T any] struct {
	Data       []T `json:"data"`
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

func (definition EntityDefinition[T]) Query() *Query[T] {
	return &Query[T]{mapping: definition.Mapping}
}

func (definition EntityDefinition[T]) Filter() *Query[T] {
	return definition.Query()
}

func (definition EntityDefinition[T]) Where(column string, value any) *Query[T] {
	return definition.Query().Where(column, value)
}

func (definition EntityDefinition[T]) Connection(name string) *Query[T] {
	return definition.Query().Connection(name)
}

func (query *Query[T]) Connection(name string) *Query[T] {
	query.connection = strings.ToLower(strings.TrimSpace(name))
	return query
}

func (query *Query[T]) Where(column string, value any) *Query[T] {
	return query.WhereOp(column, "=", value)
}

func (query *Query[T]) WhereOp(column, operator string, value any) *Query[T] {
	if !validIdentifier(column) {
		query.err = fmt.Errorf("coluna inválida %q", column)
		return query
	}
	operator = strings.ToUpper(strings.TrimSpace(operator))
	switch operator {
	case "=", "!=", "<>", ">", ">=", "<", "<=", "LIKE":
	default:
		query.err = fmt.Errorf("operador %q não permitido", operator)
		return query
	}
	query.conditions = append(query.conditions, condition{column: column, op: operator, value: value})
	return query
}

func (query *Query[T]) OrderBy(column string) *Query[T] {
	return query.addOrder(column, false)
}

func (query *Query[T]) OrderByDesc(column string) *Query[T] {
	return query.addOrder(column, true)
}

func (query *Query[T]) addOrder(column string, desc bool) *Query[T] {
	if !validIdentifier(column) {
		query.err = fmt.Errorf("coluna inválida %q", column)
		return query
	}
	query.orders = append(query.orders, order{column: column, desc: desc})
	return query
}

func (query *Query[T]) Limit(value int) *Query[T] {
	if value < 0 {
		query.err = fmt.Errorf("limit não pode ser negativo")
	} else {
		query.limit = value
	}
	return query
}

func (query *Query[T]) Offset(value int) *Query[T] {
	if value < 0 {
		query.err = fmt.Errorf("offset não pode ser negativo")
	} else {
		query.offset = value
	}
	return query
}

func (query *Query[T]) Get(ctx context.Context) ([]T, error) {
	conn, err := query.resolve()
	if err != nil {
		return nil, err
	}
	statement, args := query.selectSQL(conn.dialect)
	rows, err := conn.executor.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAll[T](rows)
}

func (query *Query[T]) Exec(ctx context.Context) ([]T, error) {
	return query.Get(ctx)
}

func (query *Query[T]) First(ctx context.Context) (*T, error) {
	copy := *query
	copy.limit = 1
	items, err := copy.Get(ctx)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, sql.ErrNoRows
	}
	return &items[0], nil
}

func (query *Query[T]) Count(ctx context.Context) (int, error) {
	conn, err := query.resolve()
	if err != nil {
		return 0, err
	}
	where, args := query.whereSQL(conn.dialect)
	statement := "SELECT COUNT(*) FROM " + query.mapping.Table + where
	var total int
	if err := conn.executor.QueryRowContext(ctx, statement, args...).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

func (query *Query[T]) Exists(ctx context.Context) (bool, error) {
	total, err := query.Count(ctx)
	return total > 0, err
}

func (query *Query[T]) Delete(ctx context.Context) (sql.Result, error) {
	if len(query.conditions) == 0 {
		return nil, fmt.Errorf("delete sem Where foi bloqueado")
	}
	conn, err := query.resolve()
	if err != nil {
		return nil, err
	}
	where, args := query.whereSQL(conn.dialect)
	return conn.executor.ExecContext(ctx, "DELETE FROM "+query.mapping.Table+where, args...)
}

func (query *Query[T]) Paginate(ctx context.Context, request PageRequest) (Page[T], error) {
	if request.Page < 1 {
		request.Page = 1
	}
	if request.PerPage < 1 {
		request.PerPage = query.mapping.DefaultPerPage
		if request.PerPage < 1 {
			request.PerPage = 15
		}
	}
	if query.mapping.MaxPerPage > 0 && request.PerPage > query.mapping.MaxPerPage {
		request.PerPage = query.mapping.MaxPerPage
	}
	total, err := query.Count(ctx)
	if err != nil {
		return Page[T]{}, err
	}
	copy := *query
	copy.limit = request.PerPage
	copy.offset = (request.Page - 1) * request.PerPage
	data, err := copy.Get(ctx)
	if err != nil {
		return Page[T]{}, err
	}
	pages := 0
	if total > 0 {
		pages = (total + request.PerPage - 1) / request.PerPage
	}
	return Page[T]{Data: data, Page: request.Page, PerPage: request.PerPage, Total: total, TotalPages: pages}, nil
}

func (query *Query[T]) resolve() (registeredConnection, error) {
	if query.err != nil {
		return registeredConnection{}, query.err
	}
	if !validIdentifier(query.mapping.Table) {
		return registeredConnection{}, fmt.Errorf("tabela inválida %q", query.mapping.Table)
	}
	for _, field := range query.mapping.Fields {
		if !validIdentifier(field.Column) {
			return registeredConnection{}, fmt.Errorf("coluna mapeada inválida %q", field.Column)
		}
	}
	name := query.connection
	if name == "" {
		name = query.mapping.Connection
	}
	return connection(name)
}

func (query *Query[T]) selectSQL(dialect string) (string, []any) {
	columns := make([]string, len(query.mapping.Fields))
	for index, field := range query.mapping.Fields {
		columns[index] = field.Column
	}
	where, args := query.whereSQL(dialect)
	statement := "SELECT " + strings.Join(columns, ", ") + " FROM " + query.mapping.Table + where
	if len(query.orders) > 0 {
		values := make([]string, len(query.orders))
		for index, value := range query.orders {
			values[index] = value.column
			if value.desc {
				values[index] += " DESC"
			}
		}
		statement += " ORDER BY " + strings.Join(values, ", ")
	} else if dialect == "sqlserver" && (query.limit > 0 || query.offset > 0) {
		statement += " ORDER BY (SELECT NULL)"
	}
	return statement + paginationSQL(dialect, query.limit, query.offset), args
}

func (query *Query[T]) whereSQL(dialect string) (string, []any) {
	if len(query.conditions) == 0 {
		return "", nil
	}
	parts := make([]string, len(query.conditions))
	args := make([]any, len(query.conditions))
	for index, value := range query.conditions {
		parts[index] = value.column + " " + value.op + " " + placeholder(dialect, index+1)
		args[index] = value.value
	}
	return " WHERE " + strings.Join(parts, " AND "), args
}

func placeholder(dialect string, index int) string {
	switch dialect {
	case "postgres":
		return fmt.Sprintf("$%d", index)
	case "oracle":
		return fmt.Sprintf(":%d", index)
	case "sqlserver":
		return fmt.Sprintf("@p%d", index)
	default:
		return "?"
	}
}

func paginationSQL(dialect string, limit, offset int) string {
	if limit == 0 && offset == 0 {
		return ""
	}
	if limit == 0 {
		limit = 1<<31 - 1
	}
	switch dialect {
	case "oracle", "sqlserver":
		return fmt.Sprintf(" OFFSET %d ROWS FETCH NEXT %d ROWS ONLY", offset, limit)
	default:
		return fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)
	}
}

var identifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.$]*$`)

func validIdentifier(value string) bool {
	return identifierPattern.MatchString(value)
}

func scanAll[T any](rows *sql.Rows) ([]T, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var result []T
	for rows.Next() {
		var item T
		destinations, err := scanDestinations(&item, columns)
		if err != nil {
			return nil, err
		}
		if err := rows.Scan(destinations...); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func scanDestinations(target any, columns []string) ([]any, error) {
	value := reflect.ValueOf(target)
	if value.Kind() != reflect.Pointer || value.Elem().Kind() != reflect.Struct {
		return nil, fmt.Errorf("destino precisa ser ponteiro para struct")
	}
	value = value.Elem()
	typed := value.Type()
	fields := make(map[string]int)
	for index := 0; index < typed.NumField(); index++ {
		column := strings.Split(typed.Field(index).Tag.Get("db"), ",")[0]
		if column != "" && column != "-" {
			fields[strings.ToLower(column)] = index
		}
	}
	destinations := make([]any, len(columns))
	for index, column := range columns {
		fieldIndex, ok := fields[strings.ToLower(column)]
		if !ok {
			var discard any
			destinations[index] = &discard
			continue
		}
		destinations[index] = value.Field(fieldIndex).Addr().Interface()
	}
	return destinations, nil
}
