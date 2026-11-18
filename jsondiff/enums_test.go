package jsondiff

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

var statusEnum = map[string]int64{"STATUS_UNSPECIFIED": 0, "STATUS_ACTIVE": 1, "STATUS_SUSPENDED": 2, "STATUS_ENABLED": 1}

func TestEnums(t *testing.T) {
	status := []EnumRule{{Path: ".user.status", Values: statusEnum}}
	tests := []struct {
		name, expected, actual string
		opts                   Options
		want                   []string
	}{
		{"strict by default", `{"user":{"status":"STATUS_ACTIVE"}}`, `{"user":{"status":1}}`, Options{}, []string{"type_mismatch .user.status"}},
		{"name to number", `{"user":{"status":"STATUS_ACTIVE"}}`, `{"user":{"status":1}}`, Options{Enums: status}, []string{}},
		{"number to name", `{"user":{"status":1}}`, `{"user":{"status":"STATUS_ACTIVE"}}`, Options{Enums: status}, []string{}},
		{"zero value", `{"user":{"status":"STATUS_UNSPECIFIED"}}`, `{"user":{"status":0}}`, Options{Enums: status}, []string{}},
		{"aliases", `{"user":{"status":"STATUS_ACTIVE"}}`, `{"user":{"status":"STATUS_ENABLED"}}`, Options{Enums: status}, []string{}},
		{"integral number literal", `{"user":{"status":"STATUS_SUSPENDED"}}`, `{"user":{"status":2.0}}`, Options{Enums: status}, []string{}},
		{"different values", `{"user":{"status":"STATUS_ACTIVE"}}`, `{"user":{"status":2}}`, Options{Enums: status}, []string{"changed .user.status"}},
		{"different names", `{"user":{"status":"STATUS_ACTIVE"}}`, `{"user":{"status":"STATUS_SUSPENDED"}}`, Options{Enums: status}, []string{"changed .user.status"}},
		{"number missing from values", `{"user":{"status":"STATUS_ACTIVE"}}`, `{"user":{"status":9}}`, Options{Enums: status}, []string{"changed .user.status"}},
		{"name missing from values", `{"user":{"status":"STATUS_DELETED"}}`, `{"user":{"status":1}}`, Options{Enums: status}, []string{"type_mismatch .user.status"}},
		{"non-integral number", `{"user":{"status":"STATUS_ACTIVE"}}`, `{"user":{"status":1.5}}`, Options{Enums: status}, []string{"type_mismatch .user.status"}},
		{"null", `{"user":{"status":"STATUS_ACTIVE"}}`, `{"user":{"status":null}}`, Options{Enums: status}, []string{"type_mismatch .user.status"}},
		{"missing", `{"user":{"status":"STATUS_ACTIVE"}}`, `{"user":{}}`, Options{Enums: status}, []string{"removed .user.status"}},
		{
			name:     "mixed names and numbers in an array",
			expected: `{"members":[{"status":1},{"status":"STATUS_SUSPENDED"},{"status":"STATUS_ACTIVE"}]}`,
			actual:   `{"members":[{"status":1},{"status":2},{"status":"STATUS_ACTIVE"}]}`,
			opts:     Options{Enums: []EnumRule{{Path: ".members[].status", Values: statusEnum}}},
			want:     []string{},
		},
		{
			name:     "repeated enum",
			expected: `{"statuses":["STATUS_ACTIVE",2,"STATUS_UNSPECIFIED"]}`,
			actual:   `{"statuses":[1,"STATUS_SUSPENDED",1]}`,
			opts:     Options{Enums: []EnumRule{{Path: ".statuses[]", Values: statusEnum}}},
			want:     []string{"changed .statuses[2]"},
		},
		{
			name:     "unordered repeated enum",
			expected: `{"statuses":["STATUS_SUSPENDED","STATUS_ACTIVE"]}`,
			actual:   `{"statuses":[1,2]}`,
			opts:     Options{Enums: []EnumRule{{Path: ".statuses[]", Values: statusEnum}}, Unordered: []string{".statuses"}},
			want:     []string{},
		},
		{"no implicit descendants", `{"user":{"status":"STATUS_ACTIVE"}}`, `{"user":{"status":1}}`, Options{Enums: []EnumRule{{Path: ".user", Values: statusEnum}}}, []string{"type_mismatch .user.status"}},
		{"any depth", `{"a":{"status":"STATUS_ACTIVE"},"b":[{"status":2}]}`, `{"a":{"status":1},"b":[{"status":"STATUS_SUSPENDED"}]}`, Options{Enums: []EnumRule{{Path: ".. | .status?", Values: statusEnum}}}, []string{}},
		{"ignore wins", `{"user":{"status":"STATUS_ACTIVE"}}`, `{"user":{"status":2}}`, Options{Enums: status, Ignore: []string{".user.status"}}, []string{}},
		{
			name:     "last rule wins",
			expected: `{"kind":"KIND_BOOK"}`,
			actual:   `{"kind":3}`,
			opts: Options{Enums: []EnumRule{
				{Path: ".kind", Values: map[string]int64{"KIND_BOOK": 1}},
				{Path: ".. | .kind?", Values: map[string]int64{"KIND_BOOK": 3}},
			}},
			want: []string{},
		},
		{"numbers keep their tolerance", `{"user":{"status":1}}`, `{"user":{"status":2}}`, Options{Enums: status, Default: Tolerance{Abs: 1}}, []string{}},
		{"names ignore the tolerance", `{"user":{"status":"STATUS_ACTIVE"}}`, `{"user":{"status":2}}`, Options{Enums: status, Default: Tolerance{Abs: 1}}, []string{"changed .user.status"}},
		{"other strings unaffected", `{"user":{"name":"STATUS_ACTIVE"}}`, `{"user":{"name":1}}`, Options{Enums: status}, []string{"type_mismatch .user.name"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := mustCompare(t, tt.expected, tt.actual, &tt.opts)
			if got := brief(r); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEnumDifference(t *testing.T) {
	r := mustCompare(t,
		`{"a":"STATUS_ACTIVE","b":1,"c":"STATUS_ACTIVE"}`,
		`{"a":2,"b":"STATUS_SUSPENDED","c":9}`,
		&Options{Enums: []EnumRule{{Path: ".a, .b, .c", Values: statusEnum}}})
	if len(r.Differences) != 3 {
		t.Fatalf("got %v", brief(r))
	}
	a := r.Differences[0]
	if a.Expected != "STATUS_ACTIVE" || a.Actual.(interface{ String() string }).String() != "2" {
		t.Errorf("original values not kept: %+v", a)
	}
	if *a.Enum != (EnumMatch{Rule: ".a, .b, .c", Expected: "STATUS_ACTIVE|STATUS_ENABLED", Actual: "STATUS_SUSPENDED"}) {
		t.Errorf("enum: %+v", *a.Enum)
	}
	if c := r.Differences[2].Enum; c.Actual != "" {
		t.Errorf("unknown number has a name: %+v", *c)
	}

	var text bytes.Buffer
	if err := r.WriteText(&text, false); err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{
		`~ .a: "STATUS_ACTIVE" → 2 (STATUS_SUSPENDED) (enum by ".a, .b, .c")`,
		`~ .b: 1 (STATUS_ACTIVE|STATUS_ENABLED) → "STATUS_SUSPENDED" (enum by ".a, .b, .c")`,
		`~ .c: "STATUS_ACTIVE" → 9 (enum by ".a, .b, .c")`,
	} {
		if !strings.Contains(text.String(), line+"\n") {
			t.Errorf("text output lacks %q:\n%s", line, text.String())
		}
	}

	var js bytes.Buffer
	if err := r.WriteJSON(&js); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"enum": {
        "rule": ".a, .b, .c",
        "expected": "STATUS_ACTIVE|STATUS_ENABLED",
        "actual": "STATUS_SUSPENDED"
      }`,
		`"enum": {
        "rule": ".a, .b, .c",
        "expected": "STATUS_ACTIVE|STATUS_ENABLED"
      }`,
	} {
		if !strings.Contains(js.String(), want) {
			t.Errorf("JSON output lacks %s:\n%s", want, js.String())
		}
	}
}

func TestEnumConfig(t *testing.T) {
	// A YAML anchor lets several paths share one table.
	yamlCfg := `enums:
  - path: .user.status
    values: &status {STATUS_UNSPECIFIED: 0, STATUS_ACTIVE: 1}
  - path: .members[].status
    values: *status
`
	opts, err := ParseYAMLConfig([]byte(yamlCfg))
	if err != nil {
		t.Fatal(err)
	}
	want := []EnumRule{
		{Path: ".user.status", Values: map[string]int64{"STATUS_UNSPECIFIED": 0, "STATUS_ACTIVE": 1}},
		{Path: ".members[].status", Values: map[string]int64{"STATUS_UNSPECIFIED": 0, "STATUS_ACTIVE": 1}},
	}
	if !reflect.DeepEqual(opts.Enums, want) {
		t.Errorf("yaml: %+v", opts.Enums)
	}
	jopts, err := ParseJSONConfig([]byte(`{"enums":[{"path":".s","values":{"A":0,"B":-1}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(jopts.Enums, []EnumRule{{Path: ".s", Values: map[string]int64{"A": 0, "B": -1}}}) {
		t.Errorf("json: %+v", jopts.Enums)
	}
}

func TestEnumConfigErrors(t *testing.T) {
	tests := []struct{ name, yaml, want string }{
		{"empty values", "enums:\n  - path: .s\n    values: {}\n", "enums[0]: values must not be empty"},
		{"missing values", "enums:\n  - path: .s\n", "enums[0]: values must not be empty"},
		{"empty name", "enums:\n  - path: .s\n    values: {\"\": 1}\n", "enums[0]: value names must not be empty"},
		{"missing path", "enums:\n  - values: {A: 1}\n", "enums[0]: empty path"},
		{"bad path", "enums:\n  - path: .[\n    values: {A: 1}\n", "enums[0]: invalid path"},
		{"fractional value", "enums:\n  - path: .s\n    values: {A: 1.5}\n", `value of "A" must be an integer`},
		{"integral float value", "enums:\n  - path: .s\n    values: {A: 2.0}\n", `value of "A" must be an integer`},
		{"string value", "enums:\n  - path: .s\n    values: {A: \"1\"}\n", `value of "A" must be an integer`},
		{"overflow", "enums:\n  - path: .s\n    values: {A: 99999999999999999999}\n", `value of "A"`},
		{"duplicate name", "enums:\n  - path: .s\n    values: {A: 1, A: 2}\n", `duplicate value name "A"`},
		{"not a mapping", "enums:\n  - path: .s\n    values: [1, 2]\n", "values must be a mapping"},
		{"unknown key", "enums:\n  - path: .s\n    values: {A: 1}\n    type: Status\n", "field type not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseYAMLConfig([]byte(tt.yaml))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("got %v, want error containing %q", err, tt.want)
			}
		})
	}
}

func TestEnumJSONConfigErrors(t *testing.T) {
	for _, cfg := range []string{
		`{"enums":[{"path":".s","values":{"A":1.5}}]}`,
		`{"enums":[{"path":".s","values":{"A":"1"}}]}`,
		`{"enums":[{"path":".s","values":{}}]}`,
	} {
		if _, err := ParseJSONConfig([]byte(cfg)); err == nil {
			t.Errorf("%s: expected error", cfg)
		}
	}
}

// An enum rule is tried before numericStrings at the same node; values the
// enum rule cannot read (a number string) fall through to numericStrings.
func TestEnumsWithNumericStrings(t *testing.T) {
	opts := Options{Enums: []EnumRule{{Path: ".s", Values: statusEnum}}, NumericStrings: []string{".s"}, Default: Tolerance{Abs: 1}}
	tests := []struct {
		name, expected, actual string
		want                   []string
	}{
		{"name and number", `{"s":"STATUS_ACTIVE"}`, `{"s":1}`, []string{}},
		{"name and number string", `{"s":"STATUS_ACTIVE"}`, `{"s":"1"}`, []string{"changed .s"}},
		{"number string and number", `{"s":"1"}`, `{"s":1}`, []string{}},
		{"numbers keep tolerance", `{"s":"1"}`, `{"s":2}`, []string{}},
		{"name ignores tolerance", `{"s":"STATUS_ACTIVE"}`, `{"s":2}`, []string{"changed .s"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := mustCompare(t, tt.expected, tt.actual, &opts)
			if got := brief(r); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}
