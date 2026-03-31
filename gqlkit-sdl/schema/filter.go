package schema

import (
	"regexp"
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

func (o *FilterOptions) HasFilters() bool {
	return len(o.OnlyQueries) > 0 || len(o.OnlyMutations) > 0 ||
		len(o.ExcludeQueries) > 0 || len(o.ExcludeMutations) > 0
}

// FilterSchema applies query/mutation field filters and optionally removes
// unreferenced types. It returns a new schema, leaving the original unchanged.
func FilterSchema(schema *IntrospectionSchema, opts *FilterOptions) *IntrospectionSchema {
	if opts == nil {
		return schema
	}

	filtered := copySchema(schema)

	if opts.HasFilters() {
		filterRootTypeFields(filtered, opts)
	}

	if opts.RemoveUnused {
		removeUnusedTypes(filtered)
	}

	return filtered
}

func filterRootTypeFields(schema *IntrospectionSchema, opts *FilterOptions) {
	queryTypeName := ""
	if schema.QueryType != nil {
		queryTypeName = schema.QueryType.Name
	}
	mutationTypeName := ""
	if schema.MutationType != nil {
		mutationTypeName = schema.MutationType.Name
	}

	for i, t := range schema.Types {
		if t.Name == queryTypeName && queryTypeName != "" {
			schema.Types[i].Fields = filterFields(t.Fields, opts.OnlyQueries, opts.ExcludeQueries)
		}
		if t.Name == mutationTypeName && mutationTypeName != "" {
			schema.Types[i].Fields = filterFields(t.Fields, opts.OnlyMutations, opts.ExcludeMutations)
		}
	}
}

func filterFields(fields []Field, only []string, exclude []string) []Field {
	if len(only) == 0 && len(exclude) == 0 {
		return fields
	}

	onlyMatchers := compileMatchers(only)
	excludeMatchers := compileMatchers(exclude)

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
	for _, name := range []string{"Int", "Float", "String", "Boolean", "ID"} {
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
		if strings.HasPrefix(t.Name, "__") || reachable[t.Name] {
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
	case "NON_NULL", "LIST":
		if ti.OfType != nil {
			walkTypeInfo(*ti.OfType, index, reachable)
		}
	default:
		if ti.Name != nil {
			walkType(*ti.Name, index, reachable)
		}
	}
}

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

func (m matcher) matches(name string) bool {
	if m.re != nil {
		return m.re.MatchString(name)
	}
	return m.exact == name
}

var regexMeta = regexp.MustCompile(`[\\.*+?^${}()|[\]]`)

// compileMatchers turns filter entries into matchers. Plain strings become
// exact matches; entries containing regex metacharacters are compiled as
// anchored regexes (^pattern$).
func compileMatchers(patterns []string) []matcher {
	if len(patterns) == 0 {
		return nil
	}
	matchers := make([]matcher, 0, len(patterns))
	for _, p := range patterns {
		if regexMeta.MatchString(p) {
			anchored := p
			if !strings.HasPrefix(anchored, "^") {
				anchored = "^" + anchored
			}
			if !strings.HasSuffix(anchored, "$") {
				anchored = anchored + "$"
			}
			if re, err := regexp.Compile(anchored); err == nil {
				matchers = append(matchers, matcher{re: re})
			} else {
				matchers = append(matchers, matcher{exact: p})
			}
		} else {
			matchers = append(matchers, matcher{exact: p})
		}
	}
	return matchers
}

func matchesAny(name string, matchers []matcher) bool {
	for _, m := range matchers {
		if m.matches(name) {
			return true
		}
	}
	return false
}
