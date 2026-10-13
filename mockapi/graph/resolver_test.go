package graph

import (
	"context"
	"errors"
	"slices"
	"testing"

	"mockapi/graph/model"

	"github.com/vektah/gqlparser/v2/gqlerror"
)

func ptr[T any](v T) *T { return &v }

// todoIDs extracts IDs so assertions read as a list.
func todoIDs(ts []*model.Todo) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.ID
	}
	return out
}

// seededWith returns a fresh resolver holding the two seed todos plus n
// created ones (IDs "3".."n+2"), all owned by user "1".
func seededWith(t *testing.T, n int) *Resolver {
	t.Helper()
	r := &Resolver{}
	for range n {
		if _, err := r.Mutation().CreateTodo(context.Background(), model.NewTodo{Text: "extra", UserID: "1"}); err != nil {
			t.Fatal(err)
		}
	}
	return r
}

// TestPaginate is the offset/limit matrix the example SDKs' pagination
// samples depend on. The regression row is "offset past the end": it used to
// fall back to offset 0 and return the first page again.
func TestPaginate(t *testing.T) {
	cases := []struct {
		name       string
		n          int
		p          *model.PaginationInput
		start, end int
	}{
		{"nil input returns all", 5, nil, 0, 5},
		{"zero limit means no limit", 5, &model.PaginationInput{Limit: 0, Offset: 0}, 0, 5},
		{"limit within range", 5, &model.PaginationInput{Limit: 2, Offset: 0}, 0, 2},
		{"limit larger than n", 5, &model.PaginationInput{Limit: 50, Offset: 0}, 0, 5},
		{"offset and limit", 5, &model.PaginationInput{Limit: 2, Offset: 2}, 2, 4},
		{"last partial page", 5, &model.PaginationInput{Limit: 2, Offset: 4}, 4, 5},
		{"offset equal to n is empty", 5, &model.PaginationInput{Limit: 2, Offset: 5}, 5, 5},
		{"offset past n is empty", 5, &model.PaginationInput{Limit: 2, Offset: 99}, 5, 5},
		{"offset past n, no limit", 5, &model.PaginationInput{Offset: 99}, 5, 5},
		{"negative offset treated as zero", 5, &model.PaginationInput{Limit: 2, Offset: -3}, 0, 2},
		{"negative limit treated as no limit", 5, &model.PaginationInput{Limit: -1, Offset: 1}, 1, 5},
		{"empty set", 0, &model.PaginationInput{Limit: 10, Offset: 0}, 0, 0},
		{"empty set with offset", 0, &model.PaginationInput{Limit: 10, Offset: 3}, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start, end := paginate(tc.n, tc.p)
			if start != tc.start || end != tc.end {
				t.Errorf("paginate(%d, %+v) = [%d,%d), want [%d,%d)", tc.n, tc.p, start, end, tc.start, tc.end)
			}
		})
	}
}

// TestTodosFilterMatrix covers each filter field alone, combined, and the
// nil filter, against the seed data.
func TestTodosFilterMatrix(t *testing.T) {
	ctx := context.Background()
	r := &Resolver{}
	if _, err := r.Mutation().UpdateTodo(ctx, "1", model.UpdateTodoInput{Priority: ptr(int32(5))}); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		filter *model.TodoFilter
		want   []string
	}{
		{"nil filter", nil, []string{"1", "2"}},
		{"empty filter", &model.TodoFilter{}, []string{"1", "2"}},
		{"text case-insensitive", &model.TodoFilter{TextContains: ptr("PING")}, []string{"1"}},
		{"empty text matches all", &model.TodoFilter{TextContains: ptr("")}, []string{"1", "2"}},
		{"done=false", &model.TodoFilter{Done: ptr(false)}, []string{"1", "2"}},
		{"done=true", &model.TodoFilter{Done: ptr(true)}, []string{}},
		{"minPriority excludes nil priority", &model.TodoFilter{MinPriority: ptr(int32(1))}, []string{"1"}},
		{"minPriority boundary inclusive", &model.TodoFilter{MinPriority: ptr(int32(5))}, []string{"1"}},
		{"minPriority above all", &model.TodoFilter{MinPriority: ptr(int32(6))}, []string{}},
		{"hasTag exact", &model.TodoFilter{HasTag: ptr("demo")}, []string{"1", "2"}},
		{"hasTag is not substring", &model.TodoFilter{HasTag: ptr("dem")}, []string{}},
		{"combined AND", &model.TodoFilter{HasTag: ptr("demo"), TextContains: ptr("first")}, []string{"2"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := r.Query().Todos(ctx, tc.filter, nil)
			if err != nil {
				t.Fatal(err)
			}
			// A non-null list must never be nil: gqlgen would fail the field.
			if got == nil {
				t.Fatal("Todos returned nil slice")
			}
			if ids := todoIDs(got); !slices.Equal(ids, tc.want) {
				t.Errorf("ids = %v, want %v", ids, tc.want)
			}
		})
	}
}

// TestTodosConnectionPageInfo pins totalCount as the filtered total (not the
// page size) and the hasNext/hasPrevious flags across the window positions.
func TestTodosConnectionPageInfo(t *testing.T) {
	ctx := context.Background()
	r := seededWith(t, 3) // 5 todos: 1..5
	cases := []struct {
		name             string
		p                *model.PaginationInput
		ids              []string
		hasNext, hasPrev bool
		startCur, endCur string
	}{
		{"no pagination", nil, []string{"1", "2", "3", "4", "5"}, false, false, "cursor-1", "cursor-5"},
		{"first page", &model.PaginationInput{Limit: 2}, []string{"1", "2"}, true, false, "cursor-1", "cursor-2"},
		{"middle page", &model.PaginationInput{Limit: 2, Offset: 2}, []string{"3", "4"}, true, true, "cursor-3", "cursor-4"},
		{"last page", &model.PaginationInput{Limit: 2, Offset: 4}, []string{"5"}, false, true, "cursor-5", "cursor-5"},
		{"past the end", &model.PaginationInput{Limit: 2, Offset: 10}, []string{}, false, true, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := r.Query().TodosConnection(ctx, nil, tc.p)
			if err != nil {
				t.Fatal(err)
			}
			ids := make([]string, len(c.Edges))
			for i, e := range c.Edges {
				ids[i] = e.Node.ID
			}
			if !slices.Equal(ids, tc.ids) {
				t.Errorf("ids = %v, want %v", ids, tc.ids)
			}
			if c.TotalCount != 5 {
				t.Errorf("totalCount = %d, want 5 (filtered total, not page size)", c.TotalCount)
			}
			if c.PageInfo.HasNextPage != tc.hasNext || c.PageInfo.HasPreviousPage != tc.hasPrev {
				t.Errorf("hasNext=%v hasPrev=%v, want %v %v", c.PageInfo.HasNextPage, c.PageInfo.HasPreviousPage, tc.hasNext, tc.hasPrev)
			}
			gotStart, gotEnd := "", ""
			if c.PageInfo.StartCursor != nil {
				gotStart = *c.PageInfo.StartCursor
			}
			if c.PageInfo.EndCursor != nil {
				gotEnd = *c.PageInfo.EndCursor
			}
			if gotStart != tc.startCur || gotEnd != tc.endCur {
				t.Errorf("cursors = %q..%q, want %q..%q", gotStart, gotEnd, tc.startCur, tc.endCur)
			}
		})
	}
}

// TestCreateTodoIDsNeverReused is the regression for IDs derived from len():
// after a delete, the next create handed out an ID that already existed.
func TestCreateTodoIDsNeverReused(t *testing.T) {
	ctx := context.Background()
	r := seededWith(t, 1) // todos 1,2,3
	if ok, err := r.Mutation().DeleteTodo(ctx, "1"); err != nil || !ok {
		t.Fatalf("delete: ok=%v err=%v", ok, err)
	}
	created, err := r.Mutation().CreateTodo(ctx, model.NewTodo{Text: "new", UserID: "2"})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID != "4" {
		t.Errorf("new ID = %q, want 4", created.ID)
	}
	all, _ := r.Query().Todos(ctx, nil, nil)
	seen := map[string]bool{}
	for _, td := range all {
		if seen[td.ID] {
			t.Fatalf("duplicate todo ID %q in %v", td.ID, todoIDs(all))
		}
		seen[td.ID] = true
	}
}

// TestCreateTodoDefaults covers the create path's defaults and user linking.
func TestCreateTodoDefaults(t *testing.T) {
	ctx := context.Background()
	r := &Resolver{}
	got, err := r.Mutation().CreateTodo(ctx, model.NewTodo{Text: "x", UserID: "2", Priority: ptr(int32(3)), Tags: []string{"a"}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Done || got.CreatedAt == "" || got.User == nil || got.User.Name != "Bob" || *got.Priority != 3 {
		t.Errorf("unexpected todo: %+v user=%+v", got, got.User)
	}
}

// TestCreateTodoUnknownUserRejected: an unknown owner is a NOT_FOUND GraphQL
// error and stores nothing, so later `todos { user }` queries stay valid.
func TestCreateTodoUnknownUserRejected(t *testing.T) {
	ctx := context.Background()
	r := &Resolver{}
	for _, userID := range []string{"user-1", "", "999"} {
		got, err := r.Mutation().CreateTodo(ctx, model.NewTodo{Text: "orphan", UserID: userID})
		if got != nil {
			t.Errorf("userId %q: created %+v", userID, got)
		}
		var gqlErr *gqlerror.Error
		if !errors.As(err, &gqlErr) || gqlErr.Extensions[errorCodeExtension] != errorCodeNotFound {
			t.Errorf("userId %q: err = %#v, want NOT_FOUND gqlerror", userID, err)
		}
	}
	all, _ := r.Query().Todos(ctx, nil, nil)
	if ids := todoIDs(all); !slices.Equal(ids, []string{"1", "2"}) {
		t.Errorf("rejected creates were stored: %v", ids)
	}
	// The ID counter is not consumed by a rejected create.
	ok, err := r.Mutation().CreateTodo(ctx, model.NewTodo{Text: "real", UserID: "1"})
	if err != nil || ok.ID != "3" {
		t.Errorf("next valid create: %+v %v", ok, err)
	}
}

// TestUpdateTodoPatchSemantics: absent fields are untouched, present ones
// applied; an explicit empty tag list clears tags; unknown ID returns nil.
func TestUpdateTodoPatchSemantics(t *testing.T) {
	ctx := context.Background()
	r := &Resolver{}
	m := r.Mutation()

	got, err := m.UpdateTodo(ctx, "1", model.UpdateTodoInput{Done: ptr(true)})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Done || got.Text != "Try the ping query" || !slices.Equal(got.Tags, []string{"demo", "ping"}) {
		t.Errorf("partial update touched other fields: %+v", got)
	}

	got, _ = m.UpdateTodo(ctx, "1", model.UpdateTodoInput{Text: ptr("renamed"), Priority: ptr(int32(9)), Tags: []string{"x"}})
	if got.Text != "renamed" || *got.Priority != 9 || !slices.Equal(got.Tags, []string{"x"}) || !got.Done {
		t.Errorf("full update: %+v", got)
	}

	got, _ = m.UpdateTodo(ctx, "1", model.UpdateTodoInput{Tags: []string{}})
	if len(got.Tags) != 0 {
		t.Errorf("explicit empty tags should clear, got %v", got.Tags)
	}

	if missing, err := m.UpdateTodo(ctx, "999", model.UpdateTodoInput{Done: ptr(true)}); err != nil || missing != nil {
		t.Errorf("unknown id: got %+v err %v, want nil nil", missing, err)
	}
}

// TestDeleteTodo covers found, not-found, and repeat delete.
func TestDeleteTodo(t *testing.T) {
	ctx := context.Background()
	r := &Resolver{}
	m := r.Mutation()
	for _, step := range []struct {
		id   string
		want bool
	}{{"2", true}, {"2", false}, {"999", false}, {"1", true}} {
		if got, err := m.DeleteTodo(ctx, step.id); err != nil || got != step.want {
			t.Errorf("DeleteTodo(%q) = %v, %v; want %v", step.id, got, err, step.want)
		}
	}
	// Deleting everything must leave an empty list, not re-seed.
	all, _ := r.Query().Todos(ctx, nil, nil)
	if len(all) != 0 {
		t.Errorf("store re-seeded after deleting all: %v", todoIDs(all))
	}
}

// TestUpsertUser covers update, create-without-id, create-with-unknown-id,
// default role and explicit role.
func TestUpsertUser(t *testing.T) {
	ctx := context.Background()
	r := &Resolver{}
	m := r.Mutation()

	u, _ := m.UpsertUser(ctx, ptr("1"), "Alicia", ptr("a@x.test"), nil)
	if u.ID != "1" || u.Name != "Alicia" || u.Role != model.RoleUser || *u.Email != "a@x.test" {
		t.Errorf("update kept role? %+v", u)
	}
	u, _ = m.UpsertUser(ctx, ptr("1"), "Alicia", nil, ptr(model.RoleGuest))
	if u.Role != model.RoleGuest || u.Email != nil {
		t.Errorf("update with role/nil email: %+v", u)
	}
	u, _ = m.UpsertUser(ctx, nil, "Carol", nil, nil)
	if u.ID != "3" || u.Role != model.RoleUser {
		t.Errorf("create default: %+v", u)
	}
	u, _ = m.UpsertUser(ctx, ptr("42"), "Dan", nil, ptr(model.RoleAdmin))
	if u.ID != "4" || u.Role != model.RoleAdmin {
		t.Errorf("unknown id creates with next id: %+v", u)
	}
	users, _ := r.Query().Users(ctx)
	if len(users) != 4 {
		t.Errorf("users = %d, want 4", len(users))
	}
}

// TestCompleteAllTodosCountsOnlyChanges: second run changes nothing.
func TestCompleteAllTodosCountsOnlyChanges(t *testing.T) {
	ctx := context.Background()
	r := &Resolver{}
	if _, err := r.Mutation().UpdateTodo(ctx, "1", model.UpdateTodoInput{Done: ptr(true)}); err != nil {
		t.Fatal(err)
	}
	for i, want := range []int32{1, 0} {
		if got, _ := r.Mutation().CompleteAllTodos(ctx); got != want {
			t.Errorf("run %d: changed %d, want %d", i+1, got, want)
		}
	}
}

// TestLookupsNotFound: single-item queries answer nil, not an error.
func TestLookupsNotFound(t *testing.T) {
	ctx := context.Background()
	q := (&Resolver{}).Query()
	if td, err := q.Todo(ctx, "nope"); td != nil || err != nil {
		t.Errorf("Todo: %+v %v", td, err)
	}
	if u, err := q.User(ctx, "nope"); u != nil || err != nil {
		t.Errorf("User: %+v %v", u, err)
	}
	if td, _ := q.Todo(ctx, "1"); td == nil || td.User == nil {
		t.Errorf("seeded todo 1 missing or without user: %+v", td)
	}
}

// TestSearch covers empty term (everything), mixed hits, case-insensitivity
// and no hits (an empty, non-nil list).
func TestSearch(t *testing.T) {
	ctx := context.Background()
	q := (&Resolver{}).Query()
	cases := []struct {
		term       string
		todos, ppl int
	}{
		{"", 2, 2},
		{"BOB", 0, 1},
		{"o", 1, 1}, // only "Create your first todo" and "Bob" contain "o"
		{"zzz", 0, 0},
	}
	for _, tc := range cases {
		res, err := q.Search(ctx, tc.term)
		if err != nil {
			t.Fatal(err)
		}
		if res == nil {
			t.Fatalf("Search(%q) returned nil slice", tc.term)
		}
		var todos, ppl int
		for _, r := range res {
			switch r.(type) {
			case *model.Todo:
				todos++
			case *model.User:
				ppl++
			default:
				t.Fatalf("unexpected result type %T", r)
			}
		}
		if todos != tc.todos || ppl != tc.ppl {
			t.Errorf("Search(%q): %d todos %d users, want %d %d", tc.term, todos, ppl, tc.todos, tc.ppl)
		}
	}
}

// TestScalarQueries pins the fixed answers the samples print.
func TestScalarQueries(t *testing.T) {
	ctx := context.Background()
	q := (&Resolver{}).Query()
	if s, _ := q.Ping(ctx); s != pingResponse {
		t.Errorf("Ping = %q", s)
	}
	for _, msg := range []string{"", "héllo", "line\nbreak"} {
		if s, _ := q.Echo(ctx, msg); s != msg {
			t.Errorf("Echo(%q) = %q", msg, s)
		}
	}
	if n, _ := q.Sum(ctx, -2, 5); n != 3 {
		t.Errorf("Sum = %d", n)
	}
	info, _ := q.ServerInfo(ctx)
	if info["version"] == nil || info["features"] == nil {
		t.Errorf("ServerInfo missing keys: %v", info)
	}
}

// TestReturnedValuesAreSnapshots pins the clone invariant: a caller (gqlgen
// marshalling) holding a returned todo must not see later mutations, and
// editing a returned value must not write through to the store.
func TestReturnedValuesAreSnapshots(t *testing.T) {
	ctx := context.Background()
	r := &Resolver{}
	before, _ := r.Query().Todo(ctx, "1")
	if _, err := r.Mutation().UpdateTodo(ctx, "1", model.UpdateTodoInput{Done: ptr(true), Tags: []string{"changed"}}); err != nil {
		t.Fatal(err)
	}
	if before.Done || before.Tags[0] != "demo" {
		t.Errorf("earlier result changed under the caller: %+v", before)
	}
	before.Tags[0] = "hacked"
	before.User.Name = "hacked"
	after, _ := r.Query().Todo(ctx, "1")
	if after.Tags[0] == "hacked" || after.User.Name == "hacked" {
		t.Errorf("editing a returned value wrote through to the store: %+v %+v", after, after.User)
	}
}
