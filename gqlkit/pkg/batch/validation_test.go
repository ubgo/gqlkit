package batch

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/ubgo/gqlkit/gqlkit/pkg/builder"
	"github.com/ubgo/gqlkit/gqlkit/pkg/graphqlclient"

	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"
)

// argsQueryBuilder is a query stand-in with an arbitrary argument set, so the
// alias × argument-name space can be enumerated.
type argsQueryBuilder struct {
	*builder.BaseBuilder
	builder.QueryMarker
}

// argsMutationBuilder is the mutation dual of argsQueryBuilder.
type argsMutationBuilder struct {
	*builder.BaseBuilder
	builder.MutationMarker
}

// newArgsBase builds a BaseBuilder with one Int! argument per name.
func newArgsBase(c builder.GraphQLClient, op string, args []string) *builder.BaseBuilder {
	b := builder.NewBaseBuilder(c, op, "Op", "field")
	b.GetSelection().AddField("id")
	for i, a := range args {
		b.SetArg(a, i, "Int!")
	}
	return b
}

// TestIsGraphQLName enumerates the GraphQL Name production boundaries.
func TestIsGraphQLName(t *testing.T) {
	cases := map[string]bool{
		"":      false,
		"a":     true,
		"Z":     true,
		"_":     true,
		"__x":   true,
		"a1":    true,
		"a_B_9": true,
		"1a":    false,
		"9":     false,
		"a-b":   false,
		"a b":   false,
		"a.b":   false,
		"a$":    false,
		"é":     false, // Names are ASCII-only by spec
		"aé":    false,
		"a\n":   false,
	}
	for in, want := range cases {
		if got := isGraphQLName(in); got != want {
			t.Errorf("isGraphQLName(%q) = %v, want %v", in, got, want)
		}
	}
}

// TestRunRejectsInvalidAlias pins that an alias that cannot be spliced into
// the document is refused before any request, on both entry points.
func TestRunRejectsInvalidAlias(t *testing.T) {
	for _, alias := range []string{"", "1st", "my-alias", "a b", "a:b", "ünï"} {
		fc := &fakeClient{}
		err := RunQueries(context.Background(), nil, QueryItems{alias: &argsQueryBuilder{BaseBuilder: newArgsBase(fc, "query", nil)}})
		if !errors.Is(err, ErrInvalidAlias) {
			t.Errorf("RunQueries alias %q: err = %v, want ErrInvalidAlias", alias, err)
		}
		err = RunMutations(context.Background(), nil, MutationItems{alias: &argsMutationBuilder{BaseBuilder: newArgsBase(fc, "mutation", nil)}})
		if !errors.Is(err, ErrInvalidAlias) {
			t.Errorf("RunMutations alias %q: err = %v, want ErrInvalidAlias", alias, err)
		}
		if fc.query != "" {
			t.Errorf("alias %q: a request was sent despite the invalid alias", alias)
		}
	}
}

// TestRunRejectsNilBuilder pins the nil-entry guard (it used to panic on a
// nil interface method call).
func TestRunRejectsNilBuilder(t *testing.T) {
	fc := &fakeClient{}
	ok := &argsQueryBuilder{BaseBuilder: newArgsBase(fc, "query", nil)}
	if err := RunQueries(context.Background(), nil, QueryItems{"a": ok, "b": nil}); !errors.Is(err, ErrNilBuilder) {
		t.Errorf("RunQueries: err = %v, want ErrNilBuilder", err)
	}
	if err := RunMutations(context.Background(), nil, MutationItems{"a": nil}); !errors.Is(err, ErrNilBuilder) {
		t.Errorf("RunMutations: err = %v, want ErrNilBuilder", err)
	}
	if fc.query != "" {
		t.Error("a request was sent despite a nil builder")
	}
}

// TestRunVariableNamespacingMatrix enumerates alias × argument-name layouts.
// Namespacing is "<alias>_<arg>" and both parts may contain "_", so some
// layouts map two different (alias, arg) pairs onto one variable. Those must
// be refused with ErrVariableCollision (merging them would silently send one
// value for both); every other layout must merge with one declaration and
// one value per (alias, arg) pair.
func TestRunVariableNamespacingMatrix(t *testing.T) {
	cases := []struct {
		name      string
		items     map[string][]string // alias → argument names
		collision bool
	}{
		{"no args", map[string][]string{"a": nil, "b": nil}, false},
		{"same arg, different aliases", map[string][]string{"a": {"x"}, "b": {"x"}}, false},
		{"many args one alias", map[string][]string{"a": {"x", "y", "z_w"}}, false},
		{"underscore alias, no overlap", map[string][]string{"a_b": {"c"}, "a": {"d"}}, false},
		{"prefix alias, no args on longer", map[string][]string{"a": {"b"}, "a_b": nil}, false},
		{"alias+arg split differently", map[string][]string{"a": {"b_c"}, "a_b": {"c"}}, true},
		{"three-way with bystander", map[string][]string{"x": {"y_z"}, "x_y": {"z"}, "q": {"z"}}, true},
		{"trailing/leading underscore", map[string][]string{"a_": {"b"}, "a": {"_b"}}, true},
		{"deep split", map[string][]string{"a": {"b_c_d"}, "a_b_c": {"d"}}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for _, op := range []string{"query", "mutation"} {
				fc := &fakeClient{data: []byte(`{}`)}
				var err error
				if op == "query" {
					items := QueryItems{}
					for alias, args := range c.items {
						items[alias] = &argsQueryBuilder{BaseBuilder: newArgsBase(fc, op, args)}
					}
					err = RunQueries(context.Background(), nil, items)
				} else {
					items := MutationItems{}
					for alias, args := range c.items {
						items[alias] = &argsMutationBuilder{BaseBuilder: newArgsBase(fc, op, args)}
					}
					err = RunMutations(context.Background(), nil, items)
				}

				if c.collision {
					if !errors.Is(err, ErrVariableCollision) {
						t.Fatalf("%s: err = %v, want ErrVariableCollision", op, err)
					}
					if fc.query != "" {
						t.Fatalf("%s: request sent despite collision", op)
					}
					continue
				}
				if err != nil {
					t.Fatalf("%s: unexpected err %v", op, err)
				}

				// One value and one declaration per (alias, arg) pair,
				// each named <alias>_<arg>.
				var want []string
				for alias, args := range c.items {
					for _, a := range args {
						want = append(want, alias+"_"+a)
					}
				}
				sort.Strings(want)
				got := make([]string, 0, len(fc.vars))
				for k := range fc.vars {
					got = append(got, k)
				}
				sort.Strings(got)
				if strings.Join(got, ",") != strings.Join(want, ",") {
					t.Fatalf("%s: variables %v, want %v", op, got, want)
				}
				for _, v := range want {
					if n := strings.Count(fc.query, "$"+v+": "); n != 1 {
						t.Errorf("%s: $%s declared %d times in\n%s", op, v, n, fc.query)
					}
				}
				if _, perr := parser.ParseQuery(&ast.Source{Input: fc.query}); perr != nil {
					t.Errorf("%s: merged document does not parse: %v\n%s", op, perr, fc.query)
				}
				if !strings.HasPrefix(fc.query, op+" "+batchOpName) {
					t.Errorf("%s: document does not start with %q:\n%s", op, op+" "+batchOpName, fc.query)
				}
			}
		})
	}
}

// TestBatchTemplateMirrorsPackage enforces the "two faces of batch" invariant
// from the project rules: the generated SDK's batch package (batch.tmpl) must
// carry the same merge/validation logic as this package. Everything from the
// batchOpName declaration to the end of run() is compared verbatim.
func TestBatchTemplateMirrorsPackage(t *testing.T) {
	pkgSrc, err := os.ReadFile("batch.go")
	if err != nil {
		t.Fatal(err)
	}
	tmplSrc, err := os.ReadFile("../templater/template/batch.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	const marker = "// batchOpName is"
	tail := func(name string, src []byte) string {
		s := string(src)
		i := strings.Index(s, marker)
		if i < 0 {
			t.Fatalf("%s: marker %q not found", name, marker)
		}
		s = strings.TrimSpace(s[i:])
		return strings.TrimSpace(strings.TrimSuffix(s, "{{ end }}"))
	}
	if p, tm := tail("batch.go", pkgSrc), tail("batch.tmpl", tmplSrc); p != tm {
		t.Errorf("batch.tmpl has drifted from pkg/batch/batch.go (from %q to the end); mirror the change", marker)
	}
}

// lyingQueryBuilder satisfies QueryBatchable but reports a configurable op
// type — a hand-written builder the compile-time gate cannot see through.
type lyingQueryBuilder struct {
	builder.QueryMarker
	op     string
	client builder.GraphQLClient
}

func (l lyingQueryBuilder) GetOpFragment(alias string) builder.OpFragment {
	return builder.OpFragment{OpType: l.op, AliasedField: alias + ": f"}
}

func (l lyingQueryBuilder) GetClient() builder.GraphQLClient { return l.client }

// clientlessQueryBuilder has no GetClient, so batch cannot find a transport.
type clientlessQueryBuilder struct{ builder.QueryMarker }

func (clientlessQueryBuilder) GetOpFragment(alias string) builder.OpFragment {
	return builder.OpFragment{OpType: "query", AliasedField: alias + ": f"}
}

// transportErrClient fails at the transport layer on the partial-data path.
type transportErrClient struct{ err error }

func (c transportErrClient) Execute(context.Context, string, map[string]any, any) error {
	return c.err
}

func (c transportErrClient) ExecuteWithPartialData(context.Context, string, map[string]any) (json.RawMessage, graphqlclient.GraphQLErrors, error) {
	return nil, nil, c.err
}

// TestRunRuntimeGuards covers run()'s defensive decisions for inputs the type
// system cannot rule out: mixed op types from custom builders, a builder with
// no transport, a nil transport, and a transport failure.
func TestRunRuntimeGuards(t *testing.T) {
	ctx := context.Background()
	fc := &fakeClient{}

	err := RunQueries(ctx, nil, QueryItems{
		"a": lyingQueryBuilder{op: "query", client: fc},
		"b": lyingQueryBuilder{op: "mutation", client: fc},
	})
	if err == nil || !strings.Contains(err.Error(), "cannot mix operation types") {
		t.Errorf("mixed op types: err = %v, want a mix error", err)
	}
	if fc.query != "" {
		t.Error("mixed op types: request was sent")
	}

	if err := RunQueries(ctx, nil, QueryItems{"a": clientlessQueryBuilder{}}); err == nil || !strings.Contains(err.Error(), "GetClient") {
		t.Errorf("no GetClient: err = %v", err)
	}

	if err := RunQueries(ctx, nil, QueryItems{"a": lyingQueryBuilder{op: "query"}}); err == nil || !strings.Contains(err.Error(), "nil GraphQLClient") {
		t.Errorf("nil client: err = %v", err)
	}

	boom := errors.New("connection refused")
	if err := RunQueries(ctx, nil, QueryItems{"a": lyingQueryBuilder{op: "query", client: transportErrClient{boom}}}); !errors.Is(err, boom) {
		t.Errorf("transport error: err = %v, want %v", err, boom)
	}
}
