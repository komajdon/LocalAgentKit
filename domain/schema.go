package domain

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// SchemaProvider is a tool that describes its arguments with a JSON schema.
type SchemaProvider interface {
	Schema() json.RawMessage
}

// SchemaOf returns the JSON schema of a tool's arguments. Tools without one,
// and whose parameter hint is not itself a schema, accept any object.
func SchemaOf(t Tool) json.RawMessage {
	if p, ok := t.(SchemaProvider); ok {
		return p.Schema()
	}
	var probe struct {
		Type string `json:"type"`
	}
	hint := t.Parameters()
	if json.Unmarshal([]byte(hint), &probe) == nil && probe.Type == "object" {
		return json.RawMessage(hint)
	}
	return json.RawMessage(`{"type":"object"}`)
}

// ValidateArgs checks arguments against the top level of a schema: required
// properties and property types. Models often quote numbers, booleans and
// nested values, so a string is converted when the schema wants another type.
// Constructs it does not understand are accepted.
func ValidateArgs(schema json.RawMessage, args Args) (Args, error) {
	var s struct {
		Properties map[string]struct {
			Type any `json:"type"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if json.Unmarshal(schema, &s) != nil {
		return args, nil
	}
	out := Args{}
	for k, v := range args {
		out[k] = v
	}
	var problems []string
	for _, name := range s.Required {
		if _, ok := out[name]; !ok {
			problems = append(problems, fmt.Sprintf("missing required argument %q", name))
		}
	}
	for name, prop := range s.Properties {
		v, present := out[name]
		types := typeNames(prop.Type)
		if !present || len(types) == 0 {
			continue
		}
		converted, ok := conform(v, types)
		if !ok {
			problems = append(problems, fmt.Sprintf("argument %q must be %s", name, strings.Join(types, " or ")))
			continue
		}
		out[name] = converted
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return args, fmt.Errorf("invalid arguments: %s", strings.Join(problems, "; "))
	}
	return out, nil
}

func typeNames(t any) []string {
	switch v := t.(type) {
	case string:
		return []string{v}
	case []any:
		var out []string
		for _, e := range v {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func is(v any, typ string) bool {
	switch typ {
	case "string":
		_, ok := v.(string)
		return ok
	case "number", "integer":
		n, ok := v.(json.Number)
		if !ok {
			return false
		}
		if typ == "number" {
			return true
		}
		_, err := strconv.ParseInt(n.String(), 10, 64)
		return err == nil
	case "boolean":
		_, ok := v.(bool)
		return ok
	case "array":
		_, ok := v.([]any)
		return ok
	case "object":
		_, ok := v.(map[string]any)
		return ok
	case "null":
		return v == nil
	}
	return true // unknown type keyword
}

func conform(v any, types []string) (any, bool) {
	for _, t := range types {
		if is(v, t) {
			return v, true
		}
	}
	text, isText := v.(string)
	for _, t := range types {
		switch {
		case t == "string":
			// Numbers and booleans have an obvious text form.
			switch v.(type) {
			case json.Number, bool:
				return Args{"v": v}.String("v"), true
			case map[string]any, []any:
				return encode(v), true
			}
		case isText:
			var parsed any
			if DecodeJSON([]byte(strings.TrimSpace(text)), &parsed) == nil && is(parsed, t) {
				return parsed, true
			}
		}
	}
	return v, false
}
