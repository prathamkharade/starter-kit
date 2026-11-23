// Package listquery turns list-endpoint query strings into safe, parameterised
// SQL with search, filtering, sorting and cursor (keyset) pagination.
//
// Query string contract (identical for every list endpoint, mirrored by the
// Flutter `ListQuery` class):
//
//	?q=free text                     full-text (websearch syntax) + fuzzy title match
//	&filter[archived]=false          equality
//	&filter[tags][in]=a,b            ops: eq ne in gt gte lt lte
//	&sort=-updated_at                "-" = descending; sort=relevance when q is set
//	&limit=20&cursor=<opaque>
//
// Only fields whitelisted in a Spec are accepted, values are type-checked
// before they reach SQL, and all values are bound as parameters — there is
// no string interpolation of user input.
package listquery

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Type int

const (
	Text Type = iota
	Bool
	Int
	Time
	UUID
	TextArray // e.g. tags text[]
)

type Field struct {
	Column string
	Type   Type
}

type Spec struct {
	Filters      map[string]Field
	Sorts        map[string]Field // sort columns must be NOT NULL
	DefaultSort  string           // e.g. "-updated_at"
	SearchColumn string           // tsvector column; "" disables ?q
	FuzzyColumn  string           // text column with a pg_trgm index; optional
	IDColumn     string           // tie-breaker, default "id" (uuidv7)
	DefaultLimit int
	MaxLimit     int
}

type Query struct {
	spec      Spec
	filters   []filter
	search    string
	sortField Field
	desc      bool
	relevance bool
	limit     int
	cur       cursor
}

type filter struct {
	field Field
	op    string
	val   any
}

type cursor struct {
	V  string `json:"v,omitempty"`
	ID string `json:"id,omitempty"`
	O  int    `json:"o,omitempty"`
}

var ops = []string{"eq", "ne", "in", "gt", "gte", "lt", "lte"}

type Error struct{ Msg string }

func (e *Error) Error() string { return e.Msg }

func bad(f string, a ...any) error { return &Error{Msg: fmt.Sprintf(f, a...)} }

// Parse reads parameters through get (pass fiber's c.Query).
func Parse(get func(key string, def ...string) string, spec Spec) (Query, error) {
	if spec.IDColumn == "" {
		spec.IDColumn = "id"
	}
	if spec.DefaultLimit == 0 {
		spec.DefaultLimit = 20
	}
	if spec.MaxLimit == 0 {
		spec.MaxLimit = 100
	}
	q := Query{spec: spec, limit: spec.DefaultLimit}

	if l := get("limit"); l != "" {
		n, err := strconv.Atoi(l)
		if err != nil || n < 1 {
			return q, bad("limit must be a positive integer")
		}
		q.limit = min(n, spec.MaxLimit)
	}

	q.search = strings.TrimSpace(get("q"))
	if len(q.search) > 200 {
		return q, bad("q is too long")
	}
	if q.search != "" && spec.SearchColumn == "" {
		return q, bad("search is not supported on this endpoint")
	}

	sort := get("sort")
	if sort == "" {
		sort = spec.DefaultSort
	}
	if sort == "relevance" {
		if q.search == "" {
			sort = spec.DefaultSort
		} else {
			q.relevance = true
		}
	}
	if !q.relevance {
		name, desc := strings.TrimPrefix(sort, "-"), strings.HasPrefix(sort, "-")
		f, ok := spec.Sorts[name]
		if !ok {
			return q, bad("cannot sort by %q", name)
		}
		q.sortField, q.desc = f, desc
	}

	for name, field := range spec.Filters {
		for _, op := range ops {
			key := "filter[" + name + "]"
			if op != "eq" {
				key += "[" + op + "]"
			}
			raw := get(key)
			if raw == "" {
				continue
			}
			v, err := parseValue(field.Type, op, raw)
			if err != nil {
				return q, bad("filter %s: %v", name, err)
			}
			q.filters = append(q.filters, filter{field: field, op: op, val: v})
		}
	}

	if c := get("cursor"); c != "" {
		b, err := base64.RawURLEncoding.DecodeString(c)
		if err != nil || json.Unmarshal(b, &q.cur) != nil {
			return q, bad("invalid cursor")
		}
	}
	return q, nil
}

func parseValue(t Type, op, raw string) (any, error) {
	if op == "in" {
		parts := strings.Split(raw, ",")
		if len(parts) > 50 {
			return nil, fmt.Errorf("too many values")
		}
		for _, p := range parts {
			if _, err := parseValue(t, "eq", p); err != nil {
				return nil, err
			}
		}
		return parts, nil
	}
	switch t {
	case Bool:
		if op != "eq" && op != "ne" {
			return nil, fmt.Errorf("unsupported operator")
		}
		return strconv.ParseBool(raw)
	case Int:
		return strconv.ParseInt(raw, 10, 64)
	case Time:
		if ts, err := time.Parse(time.RFC3339, raw); err == nil {
			return ts, nil
		}
		return time.Parse("2006-01-02", raw)
	case UUID:
		return uuid.Parse(raw)
	default:
		if len(raw) > 200 {
			return nil, fmt.Errorf("value too long")
		}
		return raw, nil
	}
}

func cast(t Type) string {
	switch t {
	case Int:
		return "bigint"
	case Time:
		return "timestamptz"
	case UUID:
		return "uuid"
	case Bool:
		return "boolean"
	default:
		return "text"
	}
}

var sqlOp = map[string]string{"eq": "=", "ne": "<>", "gt": ">", "gte": ">=", "lt": "<", "lte": "<="}

// SQL appends WHERE/ORDER/LIMIT to selectFrom ("SELECT … FROM notes").
// baseWhere (e.g. "owner_id = $1") is ANDed in and uses baseArgs.
func (q Query) SQL(selectFrom, baseWhere string, baseArgs ...any) (string, []any) {
	args := append([]any{}, baseArgs...)
	ph := func(v any) string { args = append(args, v); return "$" + strconv.Itoa(len(args)) }
	var conds []string
	if baseWhere != "" {
		conds = append(conds, baseWhere)
	}
	for _, f := range q.filters {
		col := f.field.Column
		switch {
		case f.field.Type == TextArray && f.op == "in":
			conds = append(conds, fmt.Sprintf("%s && %s::text[]", col, ph(f.val)))
		case f.field.Type == TextArray:
			conds = append(conds, fmt.Sprintf("%s = ANY(%s)", ph(f.val), col))
		case f.op == "in":
			conds = append(conds, fmt.Sprintf("%s = ANY(%s::%s[])", col, ph(f.val), cast(f.field.Type)))
		default:
			conds = append(conds, fmt.Sprintf("%s %s %s", col, sqlOp[f.op], ph(f.val)))
		}
	}
	if q.search != "" {
		s := q.spec
		fts := fmt.Sprintf("%s @@ websearch_to_tsquery('simple', %s)", s.SearchColumn, ph(q.search))
		if s.FuzzyColumn != "" {
			fts = fmt.Sprintf("(%s OR %s ILIKE %s)", fts, s.FuzzyColumn, ph("%"+escapeLike(q.search)+"%"))
		}
		conds = append(conds, fts)
	}
	id := q.spec.IDColumn
	if !q.relevance && q.cur.ID != "" {
		cmp := ">"
		if q.desc {
			cmp = "<"
		}
		conds = append(conds, fmt.Sprintf("(%s, %s) %s (%s::%s, %s::uuid)",
			q.sortField.Column, id, cmp, ph(q.cur.V), cast(q.sortField.Type), ph(q.cur.ID)))
	}

	var sb strings.Builder
	sb.WriteString(selectFrom)
	if len(conds) > 0 {
		sb.WriteString(" WHERE " + strings.Join(conds, " AND "))
	}
	if q.relevance {
		fmt.Fprintf(&sb, " ORDER BY ts_rank(%s, websearch_to_tsquery('simple', %s)) DESC, %s DESC LIMIT %d OFFSET %d",
			q.spec.SearchColumn, ph(q.search), id, q.limit+1, q.cur.O)
	} else {
		dir := "ASC"
		if q.desc {
			dir = "DESC"
		}
		fmt.Fprintf(&sb, " ORDER BY %s %s, %s %s LIMIT %d", q.sortField.Column, dir, id, dir, q.limit+1)
	}
	return sb.String(), args
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// Page is the JSON shape of every list response.
type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
}

// MakePage trims the look-ahead row and builds the next cursor.
// key must return the current sort column's value and the row id.
func MakePage[T any](q Query, rows []T, key func(T) (sortValue any, id string)) Page[T] {
	p := Page[T]{Items: rows}
	if p.Items == nil {
		p.Items = []T{}
	}
	if len(rows) <= q.limit {
		return p
	}
	p.Items, p.HasMore = rows[:q.limit], true
	var c cursor
	if q.relevance {
		c.O = q.cur.O + q.limit
	} else {
		v, id := key(p.Items[len(p.Items)-1])
		c.ID = id
		switch t := v.(type) {
		case time.Time:
			c.V = t.UTC().Format(time.RFC3339Nano)
		default:
			c.V = fmt.Sprint(t)
		}
	}
	b, _ := json.Marshal(c)
	p.NextCursor = base64.RawURLEncoding.EncodeToString(b)
	return p
}

// SortName reports the active sort column (useful inside key funcs).
func (q Query) SortColumn() string { return q.sortField.Column }
