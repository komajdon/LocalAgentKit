package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateArgs(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{
		"s":{"type":"string"},"n":{"type":"integer"},"f":{"type":"number"},
		"b":{"type":"boolean"},"a":{"type":"array"},"o":{"type":"object"},
		"m":{"type":["string","null"]}},"required":["s"]}`)

	got, err := ValidateArgs(schema, Args{
		"s": json.Number("7"), "n": "5", "f": "1.5", "b": "true",
		"a": `[1]`, "o": `{"k":1}`, "m": nil, "extra": "kept",
	})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(got)
	want := `{"a":[1],"b":true,"extra":"kept","f":1.5,"m":null,"n":5,"o":{"k":1},"s":"7"}`
	if string(data) != want {
		t.Fatalf("got %s", data)
	}

	_, err = ValidateArgs(schema, Args{"n": "five", "b": json.Number("1")})
	for _, part := range []string{`missing required argument "s"`, `"n" must be integer`, `"b" must be boolean`} {
		if err == nil || !strings.Contains(err.Error(), part) {
			t.Errorf("error %v lacks %s", err, part)
		}
	}
	if _, err := ValidateArgs(json.RawMessage(`not a schema`), Args{"x": 1}); err != nil {
		t.Fatal("unreadable schemas must not block calls")
	}
}
