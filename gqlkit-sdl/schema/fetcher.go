package schema

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	http2curl "moul.io/http2curl/v2"
)

// IntrospectionQuery is the standard GraphQL introspection query that retrieves
// the full schema definition. It uses fragments to recursively resolve type
// references up to 7 levels deep (sufficient for most real-world schemas with
// nested NON_NULL/LIST wrappers like [String!]!).
const IntrospectionQuery = `
query IntrospectionQuery {
  __schema {
    queryType { name }
    mutationType { name }
    subscriptionType { name }
    types {
      ...FullType
    }
    directives {
      name
      description
      locations
      args {
        ...InputValue
      }
    }
  }
}

fragment FullType on __Type {
  kind
  name
  description
  fields(includeDeprecated: true) {
    name
    description
    args {
      ...InputValue
    }
    type {
      ...TypeRef
    }
    isDeprecated
    deprecationReason
  }
  inputFields {
    ...InputValue
  }
  interfaces {
    ...TypeRef
  }
  enumValues(includeDeprecated: true) {
    name
    description
    isDeprecated
    deprecationReason
  }
  possibleTypes {
    ...TypeRef
  }
}

fragment InputValue on __InputValue {
  name
  description
  type {
    ...TypeRef
  }
  defaultValue
}

fragment TypeRef on __Type {
  kind
  name
  ofType {
    kind
    name
    ofType {
      kind
      name
      ofType {
        kind
        name
        ofType {
          kind
          name
          ofType {
            kind
            name
            ofType {
              kind
              name
            }
          }
        }
      }
    }
  }
}
`

// DefaultFetchTimeout bounds the whole introspection round trip when
// FetchOptions.Timeout is zero. Without a bound, an endpoint that accepts the
// connection but never answers hangs the CLI forever (the stdlib client has no
// default timeout). It is generous because introspection of a large schema
// can legitimately take a while to produce and transfer.
const DefaultFetchTimeout = 2 * time.Minute

// hostHeader is special-cased: net/http ignores a "Host" entry in
// Request.Header and sends Request.Host instead, so a user-supplied Host
// header would otherwise be dropped silently.
const hostHeader = "Host"

// maxErrorBodyBytes caps how much of a non-200 response body is quoted in the
// error. Gateways and login walls answer with whole HTML pages; past this many
// bytes the excerpt stops helping and only buries the status code.
const maxErrorBodyBytes = 512

// truncatedSuffix marks an error body that was cut at maxErrorBodyBytes, so a
// reader knows the server said more.
const truncatedSuffix = "... (truncated)"

// introspectionRequest is the JSON body of a GraphQL-over-HTTP POST.
type introspectionRequest struct {
	Query string `json:"query"`
}

// ErrNoSchema is returned when the endpoint answers 200 with valid JSON but no
// `data.__schema.queryType` — for example `{"data":null}` from a proxy, or a
// non-GraphQL JSON API. Every GraphQL schema has a query root, so treating
// this as success would write an empty schema file and report "Done".
var ErrNoSchema = errors.New("response contains no introspection schema (data.__schema.queryType missing)")

// FetchOptions holds optional configuration for the schema fetch request.
type FetchOptions struct {
	// Headers are sent with the request (Authorization, Referer, Origin...).
	// A "Host" entry sets the request's Host, since net/http ignores it as a
	// header.
	Headers map[string]string
	// Debug prints the equivalent curl command to DebugOutput instead of
	// sending the request; FetchSchema then returns (nil, nil).
	Debug bool
	// DebugOutput receives the Debug curl command. Nil means os.Stdout.
	DebugOutput io.Writer
	// Timeout bounds the whole request including reading the body. Zero
	// means DefaultFetchTimeout.
	Timeout time.Duration
}

// FetchSchema sends the introspection query to the given GraphQL endpoint URL
// via HTTP POST and returns the parsed IntrospectionSchema. It applies any
// custom headers from opts. Returns an error if the HTTP request fails or
// times out, the response status is not 200, JSON parsing fails, the GraphQL
// response contains errors, or the response carries no schema (ErrNoSchema).
// Returns (nil, nil) only in Debug mode, where nothing is sent.
func FetchSchema(url string, opts *FetchOptions) (*IntrospectionSchema, error) {
	if opts == nil {
		opts = &FetchOptions{}
	}

	jsonBody, err := json.Marshal(introspectionRequest{Query: IntrospectionQuery})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create the HTTP POST request to the GraphQL endpoint.
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Apply any user-supplied headers (auth tokens, referer, etc.).
	for key, value := range opts.Headers {
		if http.CanonicalHeaderKey(key) == hostHeader {
			req.Host = value
			continue
		}
		req.Header.Set(key, value)
	}

	if opts.Debug {
		curlCmd, err := http2curl.GetCurlCommand(req)
		if err != nil {
			return nil, fmt.Errorf("failed to build curl command: %w", err)
		}
		out := opts.DebugOutput
		if out == nil {
			out = os.Stdout
		}
		if _, err := fmt.Fprintln(out, curlCmd); err != nil {
			return nil, fmt.Errorf("failed to write curl command: %w", err)
		}
		return nil, nil
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultFetchTimeout
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	// Body is read in full below; a Close error cannot change that.
	defer func() { _ = resp.Body.Close() }()

	// Reject non-200 responses with the response body for debugging.
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, errorBodyExcerpt(resp.Body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse the JSON response into the IntrospectionResponse structure.
	var introspectionResp IntrospectionResponse
	if err := json.Unmarshal(body, &introspectionResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// If the GraphQL response contains errors, aggregate and return them.
	if len(introspectionResp.Errors) > 0 {
		messages := make([]string, len(introspectionResp.Errors))
		for i, e := range introspectionResp.Errors {
			messages[i] = e.Message
		}
		return nil, fmt.Errorf("GraphQL errors: %s", strings.Join(messages, "; "))
	}

	if introspectionResp.Data.Schema.QueryType == nil {
		return nil, ErrNoSchema
	}

	return &introspectionResp.Data.Schema, nil
}

// errorBodyExcerpt reads at most maxErrorBodyBytes of a failed response's body
// for the error message, appending truncatedSuffix when there was more. A read
// error just ends the excerpt: the status code is the real error.
func errorBodyExcerpt(body io.Reader) string {
	b, _ := io.ReadAll(io.LimitReader(body, maxErrorBodyBytes+1))
	if len(b) > maxErrorBodyBytes {
		return string(b[:maxErrorBodyBytes]) + truncatedSuffix
	}
	return string(b)
}
