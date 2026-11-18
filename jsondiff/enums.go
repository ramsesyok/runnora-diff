package jsondiff

import (
	"fmt"
	"math/big"
	"sort"
	"strings"
)

// EnumMatch tells how an enum rule read the values of a KindChanged
// difference: the rule's Path and the names of both sides. A side holding a
// number missing from the rule's values has no name.
type EnumMatch struct {
	Rule     string `json:"rule"`
	Expected string `json:"expected,omitempty"`
	Actual   string `json:"actual,omitempty"`
}

type compiledEnum struct {
	path     string
	q        *pathQuery
	byName   map[string]int64
	byNumber map[int64]string // alias names joined with "|"
}

func compileEnum(i int, r EnumRule) (*compiledEnum, error) {
	name := fmt.Sprintf("enums[%d]", i)
	if len(r.Values) == 0 {
		return nil, fmt.Errorf("%s: values must not be empty", name)
	}
	q, err := compilePathQuery(r.Path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	c := &compiledEnum{path: r.Path, q: q, byName: map[string]int64{}, byNumber: map[int64]string{}}
	aliases := map[int64][]string{}
	for n, v := range r.Values {
		if n == "" {
			return nil, fmt.Errorf("%s: value names must not be empty", name)
		}
		c.byName[n] = v
		aliases[v] = append(aliases[v], n)
	}
	for v, names := range aliases {
		sort.Strings(names)
		c.byNumber[v] = strings.Join(names, "|")
	}
	return c, nil
}

type enumSides struct {
	rule *compiledEnum
	sides
}

// enumAt returns the rule for the node; the last rule matching it wins.
func (m *matcher) enumAt(pe, pa Path) *compiledEnum {
	var rule *compiledEnum
	for _, r := range m.enums {
		if r.has(pe, pa) {
			rule = r.rule
		}
	}
	return rule
}

// value reads v as an enum number: a name in the table, or an integer.
func (c *compiledEnum) value(v any) (n int64, isName, ok bool) {
	if s, isString := v.(string); isString {
		n, ok = c.byName[s]
		return n, true, ok
	}
	lit, isNumber := numberLiteral(v)
	if !isNumber {
		return 0, false, false
	}
	r, valid := new(big.Rat).SetString(lit)
	if !valid || !r.IsInt() || !r.Num().IsInt64() {
		return 0, false, false
	}
	return r.Num().Int64(), false, true
}

// compareEnum compares e and a with the rule when at least one side is a
// known name and the other is a name or an integer. It reports false, leaving
// the node to the other comparisons, otherwise; in particular two numbers keep
// their numeric comparison and tolerance.
func (w *walker) compareEnum(rule *compiledEnum, e, a any, pe Path) bool {
	ne, nameE, okE := rule.value(e)
	na, nameA, okA := rule.value(a)
	if !okE || !okA || (!nameE && !nameA) {
		return false
	}
	w.compared++
	if ne != na {
		w.add(Difference{Path: pe.String(), Kind: KindChanged, Expected: e, Actual: a,
			Enum: &EnumMatch{Rule: rule.path, Expected: rule.byNumber[ne], Actual: rule.byNumber[na]}})
	}
	return true
}
