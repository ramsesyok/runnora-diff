package jsondiff

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestNumericStrings(t *testing.T) {
	tests := []struct {
		name, expected, actual string
		opts                   Options
		want                   []string
	}{
		{"strict by default", `{"id":123}`, `{"id":"123"}`, Options{}, []string{"type_mismatch .id"}},
		{"signed max", `{"id":9223372036854775807}`, `{"id":"9223372036854775807"}`, Options{NumericStrings: []string{".id"}}, []string{}},
		{"unsigned max", `{"id":"18446744073709551615"}`, `{"id":18446744073709551615}`, Options{NumericStrings: []string{".id"}}, []string{}},
		{"signed min", `{"id":-9223372036854775808}`, `{"id":"-9223372036854775808"}`, Options{NumericStrings: []string{".id"}}, []string{}},
		{"adjacent large integers", `[9007199254740992]`, `["9007199254740993"]`, Options{NumericStrings: []string{".[]"}}, []string{"changed .[0]"}},
		{"both strings with epsilon", `{"id":"9223372036854775806"}`, `{"id":"9223372036854775807"}`, Options{NumericStrings: []string{".id"}, Default: Tolerance{Abs: 1}}, []string{}},
		{"mixed decimal at boundary", `{"amount":123.01}`, `{"amount":"123"}`, Options{NumericStrings: []string{".amount"}, Tolerances: []ToleranceRule{{Path: ".amount", Abs: .01}}}, []string{}},
		{"relative tolerance", `[1000]`, `["1001"]`, Options{NumericStrings: []string{".[]"}, Default: Tolerance{Rel: .001}}, []string{}},
		{"exponent notation", `["1e3"]`, `[1000]`, Options{NumericStrings: []string{".[]"}}, []string{}},
		{"nested and repeated", `{"items":[{"ids":[1,2]},{"ids":[3]}],"code":"123"}`, `{"items":[{"ids":["1","2"]},{"ids":["3"]}],"code":123}`, Options{NumericStrings: []string{".items[].ids[]"}}, []string{"type_mismatch .code"}},
		{"no implicit descendants", `{"obj":{"id":1}}`, `{"obj":{"id":"1"}}`, Options{NumericStrings: []string{".obj"}}, []string{"type_mismatch .obj.id"}},
		{"unordered", `{"items":[{"id":1},{"id":2}]}`, `{"items":[{"id":"2"},{"id":"1"}]}`, Options{NumericStrings: []string{".items[].id"}, Unordered: []string{".items"}}, []string{}},
		{"missing and null", `{"id":1,"other":null}`, `{"other":"0"}`, Options{NumericStrings: []string{".id", ".other"}}, []string{"removed .id", "type_mismatch .other"}},
		{"ignore wins", `{"id":1}`, `{"id":"invalid"}`, Options{NumericStrings: []string{".id"}, Ignore: []string{".id"}}, []string{}},
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

func TestInvalidNumericStringsRemainStrings(t *testing.T) {
	for _, value := range []string{"", "00123", "+123", " 123", "123 ", "NaN", "Infinity", "0x10", ".5", "1."} {
		b, _ := json.Marshal(value)
		r := mustCompare(t, `123`, string(b), &Options{NumericStrings: []string{"."}, Default: Tolerance{Abs: 1000}})
		if len(r.Differences) != 1 || r.Differences[0].Kind != KindTypeMismatch {
			t.Errorf("%q: %+v", value, r)
		}
	}
}

func TestNumericStringDifferencePreservesInput(t *testing.T) {
	r := mustCompare(t, `{"id":123}`, `{"id":"125"}`, &Options{NumericStrings: []string{".id"}, Default: Tolerance{Abs: 1}})
	d := r.Differences[0]
	if d.Kind != KindChanged || d.Actual != "125" || *d.Diff != 2 || d.Tolerance.Abs != 1 {
		t.Fatalf("difference: %+v", d)
	}
}
