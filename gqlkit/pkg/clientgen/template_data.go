package clientgen

// Typed data for the embedded templates under pkg/templater/template. Each
// struct carries exactly the fields its template reads; text/template
// resolves promoted fields of embedded structs, so headerData's fields are
// visible to every template that includes {{ template "header" . }}.
//
// Invariant: a field a template references MUST exist here, because
// text/template fails at execution time (not parse time) on a missing struct
// field. TestMatrixGeneratedSDKCompiles runs the full Generate path, which
// executes every template below with this data.

// headerData is consumed by header.tmpl.
type headerData struct {
	// Package is the generated sub-package; header.tmpl renders `package
	// <base Package>`.
	Package string
	// Header overrides the generated-code banner line when non-empty. No
	// caller sets it today; header.tmpl falls back to the gqlkit banner.
	Header string
}

// typesTemplateData is consumed by types.tmpl and inputs.tmpl.
type typesTemplateData struct {
	headerData
	Imports []string
	Types   []TypeDef
}

// enumsTemplateData is consumed by enums.tmpl.
type enumsTemplateData struct {
	headerData
	Enums []EnumDef
}

// scalarsTemplateData is consumed by scalar.tmpl. Scalars is a map; the
// template's range visits it in sorted key order, which keeps output
// deterministic.
type scalarsTemplateData struct {
	headerData
	Imports []string
	Scalars SchemaScalarMap
}

// packageTemplateData is consumed by the self-contained runtime templates
// (builder.tmpl, graphqlclient.tmpl), which only need their package clause.
type packageTemplateData struct {
	PackageName string
}

// batchTemplateData is consumed by batch.tmpl. RootPackage is the SDK import
// root the batch package imports builder/ and graphqlclient/ from.
type batchTemplateData struct {
	RootPackage string
}
