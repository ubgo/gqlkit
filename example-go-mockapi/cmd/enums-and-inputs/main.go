// Command enums-and-inputs shows how GraphQL enums, input objects, optional
// arguments and nullable fields look in a generated Go SDK: pointer-mode
// optional fields, an empty list sent as [] to clear a field, and Opt[T]
// fields that can also send an explicit null.
//
// Run it against mockapi: go run ./cmd/enums-and-inputs
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"example-go-mockapi/internal/mockapi"
	"example-go-mockapi/sdk/enums"
	"example-go-mockapi/sdk/fields"
	"example-go-mockapi/sdk/graphqlclient"
	"example-go-mockapi/sdk/inputs"
	"example-go-mockapi/sdk/mutations"
	"example-go-mockapi/sdk/queries"
	"example-go-mockapi/sdk/types"
)

// newUserName has no letter "a", so the users this example creates never
// show up in other examples' `search(term: "a")` output.
const newUserName = "Eve"

// seededTodoOwner is the seeded user every todo here is attached to.
const seededTodoOwner = "1"

// seededTodoID is a todo mockapi seeds on start ("Try the ping query").
const seededTodoID = "1"

// ptr returns a pointer to v. Optional GraphQL inputs and arguments are
// pointers in Go, and Go has no literal syntax for &"text" or &true.
func ptr[T any](v T) *T { return &v }

func main() {
	var out strings.Builder
	err := run(context.Background(), &out, mockapi.Endpoint())
	mockapi.Finish(out.String(), err)
}

func run(ctx context.Context, w *strings.Builder, endpoint string) error {
	client := graphqlclient.NewClient(endpoint)
	qr := queries.NewQueryRoot(client)
	mr := mutations.NewMutationRoot(client)

	listEnum(w)
	if err := enumArgument(ctx, w, mr); err != nil {
		return fmt.Errorf("enum argument: %w", err)
	}
	if err := optionalFilter(ctx, w, qr); err != nil {
		return fmt.Errorf("optional filter: %w", err)
	}
	if err := unselectedFields(ctx, w, qr); err != nil {
		return fmt.Errorf("unselected fields: %w", err)
	}
	if err := emptySliceClears(ctx, w, mr); err != nil {
		return fmt.Errorf("empty slice: %w", err)
	}
	if err := optFields(w, mr); err != nil {
		return fmt.Errorf("opt fields: %w", err)
	}
	if err := scalarTypes(ctx, w, qr); err != nil {
		return fmt.Errorf("scalar types: %w", err)
	}
	return nil
}

// listEnum prints the generated constants. An enum is a named string type,
// so a value prints as its GraphQL name and converts with string(role).
func listEnum(w *strings.Builder) {
	fmt.Fprintln(w, "== Enum constants ==")
	for _, r := range []enums.Role{enums.RoleAdmin, enums.RoleUser, enums.RoleGuest} {
		fmt.Fprintf(w, "%s ", r)
	}
	fmt.Fprintln(w)
}

// enumArgument passes an enum as an optional argument. upsertUser declares
// `role: Role = USER`, so the setter takes *enums.Role: leave it unset to get
// the server default, or pass a pointer to choose.
func enumArgument(ctx context.Context, w *strings.Builder, mr *mutations.MutationRoot) error {
	fmt.Fprintln(w, "== Enum as an argument ==")
	sel := func(u *fields.UserFields) { u.ID().Name().Email().Role() }

	guest, err := mr.UpsertUser().
		Name(newUserName).
		Role(ptr(enums.RoleGuest)).
		Select(sel).
		Execute(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "with Role(GUEST): role=%s isGuest=%t\n", guest.Role, guest.Role == enums.RoleGuest)

	// No Role(...) call: the argument is left out of the request, so the
	// schema default (USER) applies. Email is also optional and unset.
	plain, err := mr.UpsertUser().Name(newUserName).Select(sel).Execute(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "without Role:     role=%s email=%v\n", plain.Role, plain.Email)
	return nil
}

// optionalFilter shows that a nil pointer field in an input struct is left
// out of the request entirely, so it does not filter at all: Done: nil means
// "any", Done: ptr(false) means "open only". The variables printed are
// exactly what the builder sends.
func optionalFilter(ctx context.Context, w *strings.Builder, qr *queries.QueryRoot) error {
	fmt.Fprintln(w, "== Optional input fields ==")
	sel := func(f *fields.TodoFields) { f.ID() }

	for _, filter := range []*inputs.TodoFilter{{}, {Done: ptr(false)}} {
		b := qr.Todos().Filter(filter).Select(sel)
		vars, err := json.Marshal(b.GetVariables())
		if err != nil {
			return err
		}
		todos, err := b.Execute(ctx)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "variables %s -> %d todos\n", vars, len(todos))
	}
	return nil
}

// unselectedFields shows what Go returns for fields you did not select: the
// full types.Todo struct, with zero values where nothing was requested.
// Nothing stops you reading them, so check your Select before trusting one.
func unselectedFields(ctx context.Context, w *strings.Builder, qr *queries.QueryRoot) error {
	fmt.Fprintln(w, "== Unselected fields are zero values ==")
	todo, err := qr.Todo().ID(seededTodoID).
		Select(func(f *fields.TodoFields) { f.ID().Text() }).
		Execute(ctx)
	if err != nil {
		return err
	}
	if todo == nil {
		// todo(id:) returns a nullable Todo, so the result is a pointer and
		// nil means "not found", not an error.
		return fmt.Errorf("seeded todo %s not found", seededTodoID)
	}
	fmt.Fprintf(w, "selected:   id=%s text=%q\n", todo.ID, todo.Text)
	fmt.Fprintf(w, "unselected: tags=%v user=%v createdAt.IsZero=%t\n", todo.Tags, todo.User, todo.CreatedAt.IsZero())
	return nil
}

// emptySliceClears sends an empty list to clear a field. In the default
// pointer mode a nil slice is left out of the request ("leave the tags
// alone") and a non-nil empty slice is sent as [] ("clear the tags").
func emptySliceClears(ctx context.Context, w *strings.Builder, mr *mutations.MutationRoot) error {
	fmt.Fprintln(w, "== Empty slice clears a list ==")
	sel := func(f *fields.TodoFields) { f.ID().Tags() }

	created, err := mr.CreateTodo().
		Input(inputs.NewTodo{Text: "tagged", UserID: seededTodoOwner, Tags: []string{"keep"}}).
		Select(sel).
		Execute(ctx)
	if err != nil {
		return err
	}
	// Delete the todo even if an update fails, so repeated runs leave the
	// shared mockapi store as they found it.
	defer func() { _, _ = mr.DeleteTodo().ID(created.ID).Execute(ctx) }()

	untouched, err := mr.UpdateTodo().ID(created.ID).
		Input(inputs.UpdateTodoInput{Tags: nil}).
		Select(sel).
		Execute(ctx)
	if err != nil {
		return err
	}
	cleared, err := mr.UpdateTodo().ID(created.ID).
		Input(inputs.UpdateTodoInput{Tags: []string{}}).
		Select(sel).
		Execute(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "before=%v after Tags: nil=%v after Tags: []string{}=%v\n", created.Tags, tagsOf(untouched), tagsOf(cleared))
	return nil
}

// optFields shows the three states of an Opt[T] input field. This SDK
// generates UpdateTodoInput.text and .priority as Opt (see the `inputs`
// section of cmd/generate/config.jsonc): the zero value is left out, Some(v)
// sends the value, and Null[T]() sends an explicit null. The variables
// printed are exactly what the builder sends.
func optFields(w *strings.Builder, mr *mutations.MutationRoot) error {
	fmt.Fprintln(w, "== Opt[T]: unset, Some, Null ==")
	for _, in := range []inputs.UpdateTodoInput{
		{},
		{Text: inputs.Some("renamed")},
		{Priority: inputs.Null[int]()},
	} {
		vars, err := json.Marshal(mr.UpdateTodo().ID(seededTodoID).Input(in).GetVariables()["input"])
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "input %s\n", vars)
	}
	return nil
}

// tagsOf reads Tags from a nullable result; updateTodo returns *types.Todo
// and nil when the id does not exist.
func tagsOf(t *types.Todo) []string {
	if t == nil {
		return nil
	}
	return t.Tags
}

// scalarTypes prints the Go type each kind of scalar decodes into:
// DateTime is bound to time.Time in cmd/generate/config.jsonc, JSON is a
// built-in mapping to json.RawMessage, and Metadata has no binding, so it
// falls back to `any` and arrives as whatever encoding/json builds.
func scalarTypes(ctx context.Context, w *strings.Builder, qr *queries.QueryRoot) error {
	fmt.Fprintln(w, "== Scalar types ==")
	todo, err := qr.Todo().ID(seededTodoID).
		Select(func(f *fields.TodoFields) { f.CreatedAt().Metadata() }).
		Execute(ctx)
	if err != nil {
		return err
	}
	if todo == nil || todo.Metadata == nil {
		return fmt.Errorf("seeded todo %s or its metadata is missing", seededTodoID)
	}
	fmt.Fprintf(w, "createdAt: %T year=%d\n", todo.CreatedAt, todo.CreatedAt.Year())
	fmt.Fprintf(w, "metadata:  %T\n", *todo.Metadata)

	info, err := qr.ServerInfo().Execute(ctx)
	if err != nil {
		return err
	}
	// info is a json.RawMessage: the server's JSON, undecoded. (Printing %T
	// would show jsontext.Value on toolchains where RawMessage is an alias.)
	fmt.Fprintf(w, "serverInfo: raw JSON, valid=%t\n", json.Valid(info))
	return nil
}
