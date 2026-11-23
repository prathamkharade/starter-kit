package listquery

import (
	"strings"
	"testing"
	"time"
)

var spec = Spec{
	Filters: map[string]Field{
		"archived": {Column: "archived", Type: Bool},
		"tags":     {Column: "tags", Type: TextArray},
	},
	Sorts:        map[string]Field{"updated_at": {Column: "updated_at", Type: Time}, "title": {Column: "title", Type: Text}},
	DefaultSort:  "-updated_at",
	SearchColumn: "search",
	FuzzyColumn:  "title",
}

func getter(m map[string]string) func(string, ...string) string {
	return func(k string, _ ...string) string { return m[k] }
}

func TestParseAndBuild(t *testing.T) {
	q, err := Parse(getter(map[string]string{
		"q": "hello", "filter[archived]": "false", "filter[tags][in]": "a,b", "limit": "500",
	}), spec)
	if err != nil {
		t.Fatal(err)
	}
	sql, args := q.SQL("SELECT id FROM notes", "owner_id = $1", "u1")
	for _, want := range []string{"owner_id = $1", "archived = $", "tags && $", "websearch_to_tsquery", "ILIKE", "ORDER BY updated_at DESC, id DESC LIMIT 101"} {
		if !strings.Contains(sql, want) {
			t.Errorf("sql missing %q: %s", want, sql)
		}
	}
	if args[0] != "u1" || len(args) != 5 {
		t.Errorf("args = %#v", args)
	}
}

func TestRejectsUnknownSortAndBadValues(t *testing.T) {
	if _, err := Parse(getter(map[string]string{"sort": "password_hash"}), spec); err == nil {
		t.Error("unknown sort accepted")
	}
	if _, err := Parse(getter(map[string]string{"filter[archived]": "maybe"}), spec); err == nil {
		t.Error("bad bool accepted")
	}
	if _, err := Parse(getter(map[string]string{"cursor": "!!"}), spec); err == nil {
		t.Error("bad cursor accepted")
	}
}

type row struct {
	ID string
	U  time.Time
}

func TestCursorRoundTrip(t *testing.T) {
	q, _ := Parse(getter(map[string]string{"limit": "2"}), spec)
	now := time.Now()
	rows := []row{{"00000000-0000-0000-0000-000000000003", now}, {"00000000-0000-0000-0000-000000000002", now}, {"00000000-0000-0000-0000-000000000001", now}}
	p := MakePage(q, rows, func(r row) (any, string) { return r.U, r.ID })
	if !p.HasMore || len(p.Items) != 2 || p.NextCursor == "" {
		t.Fatalf("page = %+v", p)
	}
	q2, err := Parse(getter(map[string]string{"limit": "2", "cursor": p.NextCursor}), spec)
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := q2.SQL("SELECT id FROM notes", "")
	if !strings.Contains(sql, "(updated_at, id) < ($1::timestamptz, $2::uuid)") {
		t.Errorf("keyset missing: %s", sql)
	}
}
