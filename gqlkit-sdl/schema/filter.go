package schema

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// FilterOptions controls which queries/mutations to include or exclude,
// and whether unreferenced types should be pruned from the output.
type FilterOptions struct {
	OnlyQueries      []string // If non-empty, keep only these query fields
	OnlyMutations    []string // If non-empty, keep only these mutation fields
	ExcludeQueries   []string // Remove these query fields
	ExcludeMutations []string // Remove these mutation fields
	RemoveUnused     bool     // Prune types not reachable from root operations
}

// HasFilters reports whether any include/exclude list is set. RemoveUnused is
// deliberately not counted: it prunes types, it does not filter root fields,
// so callers check it separately.
func (o *FilterOptions) HasFilters() bool {
	return len(o.OnlyQueries) > 0 || len(o.OnlyMutations) > 0 ||
		len(o.ExcludeQueries) > 0 || len(o.ExcludeMutations) > 0
}

// ErrEmptyQueryRoot is returned when the filters remove every field of the
// query root. GraphQL requires a query type with at least one field, so the
// result would be SDL that no generator can parse.
var ErrEmptyQueryRoot = errors.New("filters removed every query field; GraphQL requires at least one")

// FilterSchema applies query/mutation field filters and optionally removes
// unreferenced types. It returns a new schema, leaving the original unchanged.
//
// A mutation or subscription root left with no fields is dropped from the
// schema (type and root entry), because an empty `type Mutation {}` is not
// valid SDL; callers can detect that by comparing MutationType before and
// after. An emptied query root is ErrEmptyQueryRoot instead, since the query
// root cannot be dropped. An invalid regex pattern is an error naming it.
func FilterSchema(schema *IntrospectionSchema, opts *FilterOptions) (*IntrospectionSchema, error) {
	if opts == nil {
		return schema, nil
	}

	filtered := copySchema(schema)

	if opts.HasFilters() {
		if err := filterRootTypeFields(filtered, opts); err != nil {
			return nil, err
		}
		if err := dropEmptyRoots(filtered); err != nil {
			return nil, err
		}
	}

	// After dropEmptyRoots, so types only a dropped root used are pruned too.
	if opts.RemoveUnused {
		removeUnusedTypes(filtered)
	}

	return filtered, nil
}

// dropEmptyRoots removes a mutation or subscription root type that has no
// fields left, together with its schema entry, and fails with
// ErrEmptyQueryRoot when the query root has none.
func dropEmptyRoots(schema *IntrospectionSchema) error {
	if schema.QueryType != nil && rootFieldCount(schema, schema.QueryType.Name) == 0 {
		return ErrEmptyQueryRoot
	}
	for _, root := range []**TypeRef{&schema.MutationType, &schema.SubscriptionType} {
		if *root == nil || rootFieldCount(schema, (*root).Name) != 0 {
			continue
		}
		name := (*root).Name
		*root = nil
		schema.Types = slices.DeleteFunc(schema.Types, func(t FullType) bool { return t.Name == name })
	}
	return nil
}

// rootFieldCount returns the number of fields on the named type, or 0 when the
// schema does not contain it.
func rootFieldCount(schema *IntrospectionSchema, name string) int {
	for _, t := range schema.Types {
		if t.Name == name {
			return len(t.Fields)
		}
	}
	return 0
}

// filterRootTypeFields applies the query lists to the query root type and the
// mutation lists to the mutation root type. Subscriptions are never filtered.
func filterRootTypeFields(schema *IntrospectionSchema, opts *FilterOptions) error {
	queryTypeName := ""
	if schema.QueryType != nil {
		queryTypeName = schema.QueryType.Name
	}
	mutationTypeName := ""
	if schema.MutationType != nil {
		mutationTypeName = schema.MutationType.Name
	}

	// Compile every list up front, so a bad pattern fails even when its root
	// type is absent from this schema.
	onlyQ, err := compileMatchers(opts.OnlyQueries)
	if err != nil {
		return err
	}
	exclQ, err := compileMatchers(opts.ExcludeQueries)
	if err != nil {
		return err
	}
	onlyM, err := compileMatchers(opts.OnlyMutations)
	if err != nil {
		return err
	}
	exclM, err := compileMatchers(opts.ExcludeMutations)
	if err != nil {
		return err
	}

	for i, t := range schema.Types {
		if t.Name == queryTypeName && queryTypeName != "" {
			schema.Types[i].Fields = filterFields(t.Fields, onlyQ, exclQ)
		}
		if t.Name == mutationTypeName && mutationTypeName != "" {
			schema.Types[i].Fields = filterFields(t.Fields, onlyM, exclM)
		}
	}
	return nil
}

// filterFields keeps a field when it matches `only` (or `only` is empty) and
// does not match `exclude`; exclude wins when a name matches both.
func filterFields(fields []Field, onlyMatchers, excludeMatchers []matcher) []Field {
	if len(onlyMatchers) == 0 && len(excludeMatchers) == 0 {
		return fields
	}

	var result []Field
	for _, f := range fields {
		if len(onlyMatchers) > 0 && !matchesAny(f.Name, onlyMatchers) {
			continue
		}
		if matchesAny(f.Name, excludeMatchers) {
			continue
		}
		result = append(result, f)
	}
	return result
}

// removeUnusedTypes walks the type graph starting from root operation types
// and directives, collecting all reachable type names. Any type not reachable
// is removed from the schema.
func removeUnusedTypes(schema *IntrospectionSchema) {
	reachable := make(map[string]bool)

	// Always keep built-in scalars reachable
	for _, name := range builtInScalars {
		reachable[name] = true
	}

	typeIndex := make(map[string]*FullType, len(schema.Types))
	for i := range schema.Types {
		typeIndex[schema.Types[i].Name] = &schema.Types[i]
	}

	// Seed from root operation types
	if schema.QueryType != nil {
		walkType(schema.QueryType.Name, typeIndex, reachable)
	}
	if schema.MutationType != nil {
		walkType(schema.MutationType.Name, typeIndex, reachable)
	}
	if schema.SubscriptionType != nil {
		walkType(schema.SubscriptionType.Name, typeIndex, reachable)
	}

	// Seed from directive argument types
	for _, d := range schema.Directives {
		for _, arg := range d.Args {
			walkTypeInfo(arg.Type, typeIndex, reachable)
		}
	}

	var kept []FullType
	for _, t := range schema.Types {
		if strings.HasPrefix(t.Name, metaTypePrefix) || reachable[t.Name] {
			kept = append(kept, t)
		}
	}
	schema.Types = kept
}

// walkType marks a named type as reachable and recursively walks its fields,
// input fields, interfaces, possible types, and enum values.
func walkType(name string, index map[string]*FullType, reachable map[string]bool) {
	if reachable[name] {
		return
	}
	reachable[name] = true

	t, ok := index[name]
	if !ok {
		return
	}

	for _, f := range t.Fields {
		walkTypeInfo(f.Type, index, reachable)
		for _, arg := range f.Args {
			walkTypeInfo(arg.Type, index, reachable)
		}
	}

	for _, f := range t.InputFields {
		walkTypeInfo(f.Type, index, reachable)
	}

	for _, iface := range t.Interfaces {
		if iface.Name != nil {
			walkType(*iface.Name, index, reachable)
		}
	}

	for _, pt := range t.PossibleTypes {
		if pt.Name != nil {
			walkType(*pt.Name, index, reachable)
		}
	}
}

// walkTypeInfo unwraps NON_NULL/LIST wrappers to find the named type, then walks it.
func walkTypeInfo(ti TypeInfo, index map[string]*FullType, reachable map[string]bool) {
	switch ti.Kind {
	case kindNonNull, kindList:
		if ti.OfType != nil {
			walkTypeInfo(*ti.OfType, index, reachable)
		}
	default:
		if ti.Name != nil {
			walkType(*ti.Name, index, reachable)
		}
	}
}

// copySchema copies the Types and Directives slices so FilterSchema can
// replace entries without mutating the caller's schema. Nested slices inside a
// FullType are shared; filtering only ever reassigns them, never edits in place.
func copySchema(s *IntrospectionSchema) *IntrospectionSchema {
	cp := *s
	cp.Types = make([]FullType, len(s.Types))
	copy(cp.Types, s.Types)
	cp.Directives = make([]Directive, len(s.Directives))
	copy(cp.Directives, s.Directives)
	return &cp
}

// matcher is either a compiled regex or a plain string for exact matching.
type matcher struct {
	exact string
	re    *regexp.Regexp
}

// matches reports whether name satisfies the regex, or equals the exact string.
func (m matcher) matches(name string) bool {
	if m.re != nil {
		return m.re.MatchString(name)
	}
	return m.exact == name
}

// regexMeta detects a filter entry that should be compiled as a regex rather
// than compared as an exact field name.
var regexMeta = regexp.MustCompile(`[\\.*+?^${}()|[\]]`)

// compileMatchers turns filter entries into matchers. Plain strings become
// exact matches; entries containing regex metacharacters are compiled as
// whole-name regexes, `^(?:pattern)$`. The group matters: anchoring
// `todo|echo` without it gives `^todo|echo$`, which also keeps "todos". An
// entry that is not a valid regex is an error naming it, never a silent
// exact match that keeps nothing.
func compileMatchers(patterns []string) ([]matcher, error) {
	if len(patterns) == 0 {
		return nil, nil
	}
	matchers := make([]matcher, 0, len(patterns))
	for _, p := range patterns {
		if !regexMeta.MatchString(p) {
			matchers = append(matchers, matcher{exact: p})
			continue
		}
		re, err := regexp.Compile("^(?:" + p + ")$")
		if err != nil {
			return nil, fmt.Errorf("invalid filter pattern %q: %w", p, err)
		}
		matchers = append(matchers, matcher{re: re})
	}
	return matchers, nil
}

// matchesAny reports whether any matcher accepts name; false for no matchers.
func matchesAny(name string, matchers []matcher) bool {
	for _, m := range matchers {
		if m.matches(name) {
			return true
		}
	}
	return false
}
