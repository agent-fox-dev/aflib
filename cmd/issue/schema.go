package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/agent-fox-dev/aflib/envelope"
)

// schemaDialect is the JSON Schema dialect every generated document declares.
const schemaDialect = "https://json-schema.org/draft/2020-12/schema"

// selfDescription is the document --schema prints in place of running.
type selfDescription struct {
	Tool        string         `json:"tool"`
	Description string         `json:"description"`
	Input       selfInput      `json:"input"`
	Flags       object         `json:"flags"`
	Result      object         `json:"result"`
	ExitCodes   map[int]string `json:"exit_codes"`
}

type selfInput struct {
	Description string   `json:"description"`
	Examples    []string `json:"examples"`
}

// describe builds the self-description from the flag set and the Go types.
// It reads nothing else: no directory, no environment, no forge.
func describe(fs *flag.FlagSet) selfDescription {
	return selfDescription{
		Tool:        tool,
		Description: description,
		Input: selfInput{
			Description: inputDescription,
			Examples: []string{
				"https://github.com/acme/widgets/issues/42", "acme/widgets#42", "acme/widgets!7", "42",
				"https://gitlab.com/group/sub/project/-/merge_requests/5", "acme/widgets",
			},
		},
		Flags: flagsDocument(fs, map[string][]string{
			"forge": forges, "state": states, "sort": sorts, "direction": directions, "method": methods,
		}),
		Result: documentOf(reflect.TypeFor[envelope.IssueEnvelope]()),
		ExitCodes: map[int]string{
			envelope.ExitOK:     "done: read, or written, or nothing needed to change",
			envelope.ExitFailed: "failed: the stage and category are named in error (auth, input, conflict, api, aborted); side_effects lists the writes made before the failure",
			envelope.ExitUsage:  "usage error: the call is refused before anything is written",
		},
	}
}

// field is one key of an object.
type field struct {
	key   string
	value any
}

// object is a JSON object whose keys keep the order they were set in: in a
// schema a model reads, what a property is comes before what is inside it.
type object []field

func (o object) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, f := range o {
		if i > 0 {
			b.WriteByte(',')
		}
		k, err := json.Marshal(f.key)
		if err != nil {
			return nil, err
		}
		v, err := envelope.Encode(f.value)
		if err != nil {
			return nil, err
		}
		b.Write(k)
		b.WriteByte(':')
		b.Write(bytes.TrimSpace(v))
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

var (
	timeType      = reflect.TypeFor[time.Time]()
	marshalerType = reflect.TypeFor[json.Marshaler]()
)

// documentOf is the standalone JSON Schema document of a type.
func documentOf(t reflect.Type) object {
	return append(object{{"$schema", schemaDialect}}, schemaOf(t, nil)...)
}

// schemaOf describes a Go type as a JSON Schema node the way encoding/json
// writes it: a struct's properties in declaration order, named by their json
// tags and described by their description tags, required unless omitempty or
// omitzero; a slice as an array of its element; a pointer as its pointee.
func schemaOf(t reflect.Type, stack []reflect.Type) object {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == timeType {
		return object{{"type", "string"}, {"format", "date-time"}}
	}
	switch t.Kind() {
	case reflect.Bool:
		return object{{"type", "boolean"}}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return object{{"type", "integer"}}
	case reflect.Float32, reflect.Float64:
		return object{{"type", "number"}}
	case reflect.String:
		return object{{"type", "string"}}
	case reflect.Slice, reflect.Array:
		return object{{"type", "array"}, {"items", schemaOf(t.Elem(), stack)}}
	case reflect.Map:
		return object{{"type", "object"}, {"additionalProperties", schemaOf(t.Elem(), stack)}}
	case reflect.Struct:
		if t.Implements(marshalerType) {
			return object{}
		}
		for _, seen := range stack {
			if seen == t {
				return object{{"type", "object"}}
			}
		}
		return structSchema(t, append(stack, t))
	}
	return object{} // an interface: any value
}

func structSchema(t reflect.Type, stack []reflect.Type) object {
	props := object{}
	required := []string{}
	for _, f := range reflect.VisibleFields(t) {
		if !f.IsExported() || f.Anonymous {
			continue
		}
		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name, opts, _ := strings.Cut(tag, ",")
		if name == "" {
			name = f.Name
		}
		// An error value marshals to an object of its fields, which says
		// nothing; issuex.IssueThread has one, and is not written as is.
		if f.Type.Kind() == reflect.Interface {
			continue
		}
		node := schemaOf(f.Type, stack)
		if d := f.Tag.Get("description"); d != "" {
			node = withDescription(node, d)
		}
		props = append(props, field{name, node})
		if !strings.Contains(","+opts+",", ",omitempty,") && !strings.Contains(","+opts+",", ",omitzero,") {
			required = append(required, name)
		}
	}
	return object{{"type", "object"}, {"properties", props}, {"required", required}}
}

// withDescription puts desc right after the node's type.
func withDescription(node object, desc string) object {
	out := make(object, 0, len(node)+1)
	placed := false
	for _, f := range node {
		out = append(out, f)
		if f.key == "type" && !placed {
			out = append(out, field{"description", desc})
			placed = true
		}
	}
	if !placed {
		out = append(object{{"description", desc}}, out...)
	}
	return out
}

// flagsDocument describes every flag as a standalone JSON Schema document, in
// flag-name order, so a framework can use it as a tool's input schema. enums
// names the flags with a closed set of values. --schema and --version are
// left out: both return before any work is done.
func flagsDocument(fs *flag.FlagSet, enums map[string][]string) object {
	props := object{}
	fs.VisitAll(func(f *flag.Flag) {
		if f.Name == "schema" || f.Name == "version" {
			return
		}
		p := object{}
		switch g := f.Value.(type) {
		case *listFlag:
			p = append(p, field{"type", "array"}, field{"items", object{{"type", "string"}}})
		case flag.Getter:
			switch g.Get().(type) {
			case bool:
				b, _ := strconv.ParseBool(f.DefValue)
				p = append(p, field{"type", "boolean"}, field{"default", b})
			case int:
				n, _ := strconv.Atoi(f.DefValue)
				p = append(p, field{"type", "integer"}, field{"default", n})
			case time.Duration:
				p = append(p, field{"type", "string"}, field{"format", "duration"}, field{"default", f.DefValue})
			default:
				p = append(p, field{"type", "string"})
				if e, ok := enums[f.Name]; ok {
					p = append(p, field{"enum", e})
				}
				p = append(p, field{"default", f.DefValue})
			}
		}
		p = append(p, field{"description", f.Usage})
		props = append(props, field{f.Name, p})
	})
	return object{{"$schema", schemaDialect}, {"type", "object"}, {"properties", props}}
}
