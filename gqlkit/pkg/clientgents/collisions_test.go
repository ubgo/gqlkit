package clientgents

import (
	"fmt"
	"strings"
	"testing"
)

// collisionNames are every name a generated class defines for itself, plus
// the names of the runtime state it used to keep in ordinary private fields
// (selection, builder, client). The schema below uses each one as a field, an
// argument, an operation and an enum value.
var collisionNames = []string{"constructor", "select", "execute", "executeRaw", "getOpFragment", "selection", "builder", "client"}

// TestGenerateMemberNameCollisions pins the collision rule: a schema name that
// equals a member of the generated class is suffixed with "_", every other
// name — including the old private-field names, now `#private` — is kept, and
// the result compiles and is callable.
func TestGenerateMemberNameCollisions(t *testing.T) {
	var fields, args, ops, enum strings.Builder
	for _, n := range collisionNames {
		fmt.Fprintf(&fields, "  %s: String\n", n)
		fmt.Fprintf(&args, "%s: Int, ", n)
		fmt.Fprintf(&ops, "  %s: Boolean\n", n)
		fmt.Fprintf(&enum, "%s ", n)
	}
	sdk := generateSDK(t, fmt.Sprintf(`
enum Clash { %s }
type Thing {
%s  select_: Int
  kind: Clash
  self: Thing
}
type Query {
  thing(%sselect_: Int): Thing
  plain(select: Int, execute: Int): Int
%s}
`, enum.String(), fields.String(), args.String(), ops.String()), "")

	sel := read(t, sdk, "fields/thing.ts")
	mustContain(t, sel, "selector members",
		"constructor_(): ThingFields<T & { constructor: string | null }>",
		`this.#selection.addField("constructor");`,
		"select(): ThingFields<T & { select: string | null }>",
		"selection(): ThingFields<T & { selection: string | null }>",
		"select_(): ThingFields<T & { select_: number | null }>",
	)

	thing := read(t, sdk, "queries/thing.ts")
	mustContain(t, thing, "builder setters",
		"constructor_(v: number | null): this",
		"select_(v: number | null): this",
		`this.#builder.setArg("select", v, "Int");`,
		"execute_(v: number | null): this",
		"executeRaw_(v: number | null): this",
		"getOpFragment_(v: number | null): this",
		"builder(v: number | null): this",
		"client(v: number | null): this",
		"selection(v: number | null): this",
		// the schema's own select_ arg comes after the suffixed select
		"select__(v: number | null): this",
		`this.#builder.setArg("select_", v, "Int");`,
		"select<T extends object>(selector: (f: ThingFields) => ThingFields<T>): ThingBuilder<T | null>",
	)
	mustContain(t, read(t, sdk, "queries/plain.ts"), "setters on a builder without select()", "select_(v: number | null): this", "execute_(v: number | null): this")

	root := read(t, sdk, "queries/root.ts")
	mustContain(t, root, "root factory methods",
		"constructor_(): ConstructorBuilder",
		"select(): SelectBuilder",
		"client(): ClientBuilder",
		"execute(): ExecuteBuilder",
	)
	mustContain(t, read(t, sdk, "enums/index.ts"), "enum values", `constructor = "constructor",`, `select = "select",`)

	for _, f := range []string{sel, thing, root} {
		if strings.Contains(f, "as any") || strings.Contains(f, "private ") {
			t.Errorf("generated class still uses `as any` or a TS private field:\n%s", f)
		}
	}

	typecheckSDK(t, sdk, map[string]string{"usage.ts": `
import { GraphQLClient } from "gqlkit-ts";
import { QueryRoot } from "./sdk/queries";

export function build(client: GraphQLClient) {
  const root = new QueryRoot(client);
  const b = root.thing().constructor_(1).select_(2).select__(3).execute_(4).client(5)
    .select((f) => f.constructor_().select().self((s) => s.selection()));
  const typed: Promise<{ constructor: string | null; select: string | null; self: { selection: string | null } | null } | null> = b.execute();
  return [typed, root.constructor_().execute(), root.client().execute(), root.plain().select_(1).execute()];
}
`})
}
