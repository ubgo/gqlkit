package schema

import (
	"testing"
)

// fieldNames extracts the names of the fields on the type with the given name.
func fieldNames(s *IntrospectionSchema, typeName string) []string {
	for _, t := range s.Types {
		if t.Name == typeName {
			names := make([]string, len(t.Fields))
			for i, f := range t.Fields {
				names[i] = f.Name
			}
			return names
		}
	}
	return nil
}

// typeNames returns the names of all types in the schema.
func typeNames(s *IntrospectionSchema) []string {
	names := make([]string, len(s.Types))
	for i, t := range s.Types {
		names[i] = t.Name
	}
	return names
}

func hasType(s *IntrospectionSchema, name string) bool {
	for _, n := range typeNames(s) {
		if n == name {
			return true
		}
	}
	return false
}

// sampleSchema builds a small but representative schema:
//
//	Query { users: [User!]!, posts: [Post!]!, taskList: [Task!]! }
//	Mutation { createUser(input: CreateUserInput!): User, deleteTask(id: ID!): Boolean }
//	User, Post, Task objects; CreateUserInput input; Orphan object (unreferenced)
func sampleSchema() *IntrospectionSchema {
	return &IntrospectionSchema{
		QueryType:    &TypeRef{Name: "Query"},
		MutationType: &TypeRef{Name: "Mutation"},
		Types: []FullType{
			{Kind: "OBJECT", Name: "Query", Fields: []Field{
				{Name: "users", Type: nonNull(list(nonNull(named("OBJECT", "User"))))},
				{Name: "posts", Type: nonNull(list(nonNull(named("OBJECT", "Post"))))},
				{Name: "taskList", Type: nonNull(list(nonNull(named("OBJECT", "Task"))))},
			}},
			{Kind: "OBJECT", Name: "Mutation", Fields: []Field{
				{Name: "createUser", Type: named("OBJECT", "User"), Args: []InputValue{
					{Name: "input", Type: nonNull(named("INPUT_OBJECT", "CreateUserInput"))},
				}},
				{Name: "deleteTask", Type: named("SCALAR", "Boolean"), Args: []InputValue{
					{Name: "id", Type: nonNull(named("SCALAR", "ID"))},
				}},
			}},
			{Kind: "OBJECT", Name: "User", Fields: []Field{
				{Name: "id", Type: nonNull(named("SCALAR", "ID"))},
				{Name: "name", Type: named("SCALAR", "String")},
			}},
			{Kind: "OBJECT", Name: "Post", Fields: []Field{
				{Name: "id", Type: nonNull(named("SCALAR", "ID"))},
				{Name: "author", Type: named("OBJECT", "User")},
			}},
			{Kind: "OBJECT", Name: "Task", Fields: []Field{
				{Name: "id", Type: nonNull(named("SCALAR", "ID"))},
			}},
			{Kind: "INPUT_OBJECT", Name: "CreateUserInput", InputFields: []InputValue{
				{Name: "name", Type: nonNull(named("SCALAR", "String"))},
			}},
			{Kind: "OBJECT", Name: "Orphan", Fields: []Field{
				{Name: "id", Type: nonNull(named("SCALAR", "ID"))},
			}},
		},
	}
}

func TestHasFilters(t *testing.T) {
	tests := []struct {
		name string
		opts FilterOptions
		want bool
	}{
		{"empty", FilterOptions{}, false},
		{"only remove-unused is not a field filter", FilterOptions{RemoveUnused: true}, false},
		{"only-queries", FilterOptions{OnlyQueries: []string{"users"}}, true},
		{"only-mutations", FilterOptions{OnlyMutations: []string{"createUser"}}, true},
		{"exclude-queries", FilterOptions{ExcludeQueries: []string{"posts"}}, true},
		{"exclude-mutations", FilterOptions{ExcludeMutations: []string{"deleteTask"}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.opts.HasFilters(); got != tt.want {
				t.Errorf("HasFilters() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFilterSchemaNilOptsReturnsSame(t *testing.T) {
	s := sampleSchema()
	if got := FilterSchema(s, nil); got != s {
		t.Errorf("nil opts should return the original schema pointer")
	}
}

func TestFilterSchemaLeavesOriginalUnchanged(t *testing.T) {
	s := sampleSchema()
	origQueryFields := len(fieldNames(s, "Query"))
	opts := &FilterOptions{OnlyQueries: []string{"users"}}
	FilterSchema(s, opts)
	if got := len(fieldNames(s, "Query")); got != origQueryFields {
		t.Errorf("original schema mutated: Query fields = %d, want %d", got, origQueryFields)
	}
}

func TestFilterOnlyQueries(t *testing.T) {
	s := sampleSchema()
	out := FilterSchema(s, &FilterOptions{OnlyQueries: []string{"users"}})
	got := fieldNames(out, "Query")
	if len(got) != 1 || got[0] != "users" {
		t.Errorf("only-queries: got %v, want [users]", got)
	}
	// Mutations untouched.
	if len(fieldNames(out, "Mutation")) != 2 {
		t.Errorf("mutations should be untouched, got %v", fieldNames(out, "Mutation"))
	}
}

func TestFilterOnlyMutations(t *testing.T) {
	s := sampleSchema()
	out := FilterSchema(s, &FilterOptions{OnlyMutations: []string{"createUser"}})
	got := fieldNames(out, "Mutation")
	if len(got) != 1 || got[0] != "createUser" {
		t.Errorf("only-mutations: got %v, want [createUser]", got)
	}
}

func TestFilterExcludeQueries(t *testing.T) {
	s := sampleSchema()
	out := FilterSchema(s, &FilterOptions{ExcludeQueries: []string{"posts"}})
	got := fieldNames(out, "Query")
	for _, n := range got {
		if n == "posts" {
			t.Errorf("exclude-queries: posts should be removed, got %v", got)
		}
	}
	if len(got) != 2 {
		t.Errorf("exclude-queries: expected 2 remaining, got %v", got)
	}
}

func TestFilterExcludeByRegex(t *testing.T) {
	s := sampleSchema()
	// "task.*" contains regex metacharacters -> compiled as anchored regex.
	out := FilterSchema(s, &FilterOptions{ExcludeQueries: []string{"task.*"}})
	got := fieldNames(out, "Query")
	for _, n := range got {
		if n == "taskList" {
			t.Errorf("regex exclude should remove taskList, got %v", got)
		}
	}
}

func TestFilterOnlyByRegex(t *testing.T) {
	s := sampleSchema()
	out := FilterSchema(s, &FilterOptions{OnlyQueries: []string{"user.*"}})
	got := fieldNames(out, "Query")
	if len(got) != 1 || got[0] != "users" {
		t.Errorf("regex only: got %v, want [users]", got)
	}
}

func TestFilterRemoveUnused(t *testing.T) {
	s := sampleSchema()
	// Keep only the users query, then prune. Post, Task, CreateUserInput, Orphan
	// become unreachable (createUser mutation is still present, so User + input stay).
	out := FilterSchema(s, &FilterOptions{
		OnlyQueries:  []string{"users"},
		RemoveUnused: true,
	})
	if !hasType(out, "User") {
		t.Errorf("User is reachable via users query, should be kept")
	}
	if hasType(out, "Orphan") {
		t.Errorf("Orphan is unreferenced, should be pruned")
	}
	if !hasType(out, "CreateUserInput") {
		t.Errorf("CreateUserInput reachable via createUser mutation arg, should be kept")
	}
}

func TestFilterRemoveUnusedPrunesEverythingUnreachable(t *testing.T) {
	s := sampleSchema()
	// Exclude all mutations and keep only users query. Post, Task, CreateUserInput,
	// Orphan all become unreachable.
	out := FilterSchema(s, &FilterOptions{
		OnlyQueries:      []string{"users"},
		ExcludeMutations: []string{"createUser", "deleteTask"},
		RemoveUnused:     true,
	})
	for _, unreachable := range []string{"Post", "Task", "CreateUserInput", "Orphan"} {
		if hasType(out, unreachable) {
			t.Errorf("%s should be pruned as unreachable, types = %v", unreachable, typeNames(out))
		}
	}
	if !hasType(out, "User") {
		t.Errorf("User should remain, types = %v", typeNames(out))
	}
}

func TestRemoveUnusedKeepsMetaTypes(t *testing.T) {
	s := &IntrospectionSchema{
		QueryType: &TypeRef{Name: "Query"},
		Types: []FullType{
			{Kind: "OBJECT", Name: "Query", Fields: []Field{{Name: "ok", Type: named("SCALAR", "Boolean")}}},
			{Kind: "OBJECT", Name: "__Type"}, // introspection meta-type kept by "__" prefix rule
			{Kind: "OBJECT", Name: "Orphan"},
		},
	}
	out := FilterSchema(s, &FilterOptions{RemoveUnused: true})
	if !hasType(out, "__Type") {
		t.Errorf("__ prefixed meta-types should be retained")
	}
	if hasType(out, "Orphan") {
		t.Errorf("Orphan should be pruned")
	}
}

func TestRemoveUnusedFollowsInterfacesUnionsEnums(t *testing.T) {
	s := &IntrospectionSchema{
		QueryType: &TypeRef{Name: "Query"},
		Types: []FullType{
			{Kind: "OBJECT", Name: "Query", Fields: []Field{
				{Name: "node", Type: named("INTERFACE", "Node")},
				{Name: "result", Type: named("UNION", "Result")},
				{Name: "status", Type: named("ENUM", "Status")},
			}},
			{Kind: "INTERFACE", Name: "Node",
				Fields:        []Field{{Name: "id", Type: nonNull(named("SCALAR", "ID"))}},
				PossibleTypes: []TypeInfo{named("OBJECT", "Widget")},
			},
			{Kind: "OBJECT", Name: "Widget",
				Interfaces: []TypeInfo{named("INTERFACE", "Node")},
				Fields:     []Field{{Name: "id", Type: nonNull(named("SCALAR", "ID"))}},
			},
			{Kind: "UNION", Name: "Result", PossibleTypes: []TypeInfo{named("OBJECT", "Hit")}},
			{Kind: "OBJECT", Name: "Hit", Fields: []Field{{Name: "score", Type: named("SCALAR", "Int")}}},
			{Kind: "ENUM", Name: "Status", EnumValues: []EnumValue{{Name: "OK"}}},
			{Kind: "OBJECT", Name: "Dangling"},
		},
	}
	out := FilterSchema(s, &FilterOptions{RemoveUnused: true})
	for _, reachable := range []string{"Node", "Widget", "Result", "Hit", "Status"} {
		if !hasType(out, reachable) {
			t.Errorf("%s should be reachable, types = %v", reachable, typeNames(out))
		}
	}
	if hasType(out, "Dangling") {
		t.Errorf("Dangling should be pruned, types = %v", typeNames(out))
	}
}

func TestRemoveUnusedSeedsFromDirectiveArgs(t *testing.T) {
	s := &IntrospectionSchema{
		QueryType: &TypeRef{Name: "Query"},
		Types: []FullType{
			{Kind: "OBJECT", Name: "Query", Fields: []Field{{Name: "ok", Type: named("SCALAR", "Boolean")}}},
			{Kind: "ENUM", Name: "Role", EnumValues: []EnumValue{{Name: "ADMIN"}}},
			{Kind: "OBJECT", Name: "Orphan"},
		},
		Directives: []Directive{
			{Name: "auth", Locations: []string{"FIELD_DEFINITION"}, Args: []InputValue{
				{Name: "requires", Type: named("ENUM", "Role")},
			}},
		},
	}
	out := FilterSchema(s, &FilterOptions{RemoveUnused: true})
	if !hasType(out, "Role") {
		t.Errorf("Role is referenced by a directive arg, should be kept")
	}
	if hasType(out, "Orphan") {
		t.Errorf("Orphan should be pruned")
	}
}

func TestFilterSubscriptionRootWalked(t *testing.T) {
	s := &IntrospectionSchema{
		QueryType:        &TypeRef{Name: "Query"},
		SubscriptionType: &TypeRef{Name: "Subscription"},
		Types: []FullType{
			{Kind: "OBJECT", Name: "Query", Fields: []Field{{Name: "ok", Type: named("SCALAR", "Boolean")}}},
			{Kind: "OBJECT", Name: "Subscription", Fields: []Field{
				{Name: "onEvent", Type: named("OBJECT", "Event")},
			}},
			{Kind: "OBJECT", Name: "Event", Fields: []Field{{Name: "id", Type: nonNull(named("SCALAR", "ID"))}}},
			{Kind: "OBJECT", Name: "Orphan"},
		},
	}
	out := FilterSchema(s, &FilterOptions{RemoveUnused: true})
	if !hasType(out, "Event") {
		t.Errorf("Event reachable via subscription, should be kept, types = %v", typeNames(out))
	}
	if hasType(out, "Orphan") {
		t.Errorf("Orphan should be pruned")
	}
}

func TestRemoveUnusedDanglingReference(t *testing.T) {
	// Query references a type name that has no definition in Types. walkType
	// marks it reachable but hits the `!ok` early-return since it's not indexed.
	s := &IntrospectionSchema{
		QueryType: &TypeRef{Name: "Query"},
		Types: []FullType{
			{Kind: "OBJECT", Name: "Query", Fields: []Field{
				{Name: "ghost", Type: named("OBJECT", "Missing")},
			}},
			{Kind: "OBJECT", Name: "Orphan"},
		},
	}
	out := FilterSchema(s, &FilterOptions{RemoveUnused: true})
	if hasType(out, "Orphan") {
		t.Errorf("Orphan should be pruned, types = %v", typeNames(out))
	}
	if !hasType(out, "Query") {
		t.Errorf("Query should remain")
	}
}

func TestCompileMatchersInvalidRegexFallsBackToExact(t *testing.T) {
	// "[" has a regex metacharacter but is an invalid pattern; compileMatchers
	// should fall back to an exact matcher.
	ms := compileMatchers([]string{"["})
	if len(ms) != 1 {
		t.Fatalf("expected 1 matcher, got %d", len(ms))
	}
	if ms[0].re != nil {
		t.Errorf("invalid regex should fall back to exact matcher, got regex")
	}
	if !ms[0].matches("[") {
		t.Errorf("exact fallback matcher should match the literal string")
	}
}

func TestCompileMatchersAlreadyAnchored(t *testing.T) {
	// Pattern already anchored with ^ and $ should compile as-is.
	ms := compileMatchers([]string{"^users$"})
	if len(ms) != 1 || ms[0].re == nil {
		t.Fatalf("expected a compiled regex matcher")
	}
	if !ms[0].matches("users") || ms[0].matches("usersX") {
		t.Errorf("anchored regex matched incorrectly")
	}
}

func TestCompileMatchersEmpty(t *testing.T) {
	if ms := compileMatchers(nil); ms != nil {
		t.Errorf("compileMatchers(nil) = %v, want nil", ms)
	}
}

func TestMatchesAny(t *testing.T) {
	ms := compileMatchers([]string{"a", "b.*"})
	if !matchesAny("a", ms) {
		t.Errorf("expected exact match on 'a'")
	}
	if !matchesAny("bcd", ms) {
		t.Errorf("expected regex match on 'bcd'")
	}
	if matchesAny("z", ms) {
		t.Errorf("did not expect match on 'z'")
	}
}
