package flexberry

import (
	"context"
	"fmt"
	"strings"
)

type DynamicQuery struct {
	mapping    EntityMapping
	connection string
	conditions []condition
	orders     []order
	limit      int
	offset     int
	err        error
}

func Dynamic(mapping EntityMapping) *DynamicQuery {
	return &DynamicQuery{mapping: mapping}
}

func InvalidDynamic(err error) *DynamicQuery {
	return &DynamicQuery{err: err}
}

func (query *DynamicQuery) Connection(name string) *DynamicQuery {
	query.connection = strings.ToLower(strings.TrimSpace(name))
	return query
}

func (query *DynamicQuery) Where(column string, value any) *DynamicQuery {
	if !validIdentifier(column) {
		query.err = fmt.Errorf("coluna inválida %q", column)
		return query
	}
	query.conditions = append(query.conditions, condition{column: column, op: "=", value: value})
	return query
}

func (query *DynamicQuery) OrderBy(column string) *DynamicQuery {
	if !validIdentifier(column) {
		query.err = fmt.Errorf("coluna inválida %q", column)
		return query
	}
	query.orders = append(query.orders, order{column: column})
	return query
}

func (query *DynamicQuery) Limit(value int) *DynamicQuery {
	if value < 0 {
		query.err = fmt.Errorf("limit não pode ser negativo")
	} else {
		query.limit = value
	}
	return query
}

func (query *DynamicQuery) Offset(value int) *DynamicQuery {
	if value < 0 {
		query.err = fmt.Errorf("offset não pode ser negativo")
	} else {
		query.offset = value
	}
	return query
}

func (query *DynamicQuery) Exec(ctx context.Context) ([]map[string]any, error) {
	if query.err != nil {
		return nil, query.err
	}
	if !validIdentifier(query.mapping.Table) {
		return nil, fmt.Errorf("tabela inválida %q", query.mapping.Table)
	}
	conn, err := connection(query.connectionOrMapping())
	if err != nil {
		return nil, err
	}
	columns := make([]string, len(query.mapping.Fields))
	for index, field := range query.mapping.Fields {
		if !validIdentifier(field.Column) {
			return nil, fmt.Errorf("coluna inválida %q", field.Column)
		}
		columns[index] = field.Column
	}
	statement := "SELECT " + strings.Join(columns, ", ") + " FROM " + query.mapping.Table
	args := make([]any, len(query.conditions))
	if len(query.conditions) > 0 {
		parts := make([]string, len(query.conditions))
		for index, item := range query.conditions {
			parts[index] = item.column + " = " + placeholder(conn.dialect, index+1)
			args[index] = item.value
		}
		statement += " WHERE " + strings.Join(parts, " AND ")
	}
	if len(query.orders) > 0 {
		values := make([]string, len(query.orders))
		for index, item := range query.orders {
			values[index] = item.column
		}
		statement += " ORDER BY " + strings.Join(values, ", ")
	} else if conn.dialect == "sqlserver" && (query.limit > 0 || query.offset > 0) {
		statement += " ORDER BY (SELECT NULL)"
	}
	statement += paginationSQL(conn.dialect, query.limit, query.offset)

	rows, err := conn.executor.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	names, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var result []map[string]any
	for rows.Next() {
		values := make([]any, len(names))
		destinations := make([]any, len(names))
		for index := range values {
			destinations[index] = &values[index]
		}
		if err := rows.Scan(destinations...); err != nil {
			return nil, err
		}
		item := make(map[string]any, len(names))
		for index, name := range names {
			item[canonicalColumn(query.mapping, name)] = normalizeDynamicValue(values[index])
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func canonicalColumn(mapping EntityMapping, returned string) string {
	for _, field := range mapping.Fields {
		if strings.EqualFold(field.Column, returned) {
			return field.Column
		}
	}
	return returned
}

func (query *DynamicQuery) connectionOrMapping() string {
	if query.connection != "" {
		return query.connection
	}
	return query.mapping.Connection
}

func normalizeDynamicValue(value any) any {
	if bytes, ok := value.([]byte); ok {
		return string(bytes)
	}
	return value
}
