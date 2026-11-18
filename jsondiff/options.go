// Package jsondiff compares two JSON documents, treating numbers as equal when
// they are within a configurable absolute or relative tolerance.
//
// Paths used by Options (Ignore, Unordered and Tolerances) are jq path
// expressions, the same syntax runn uses for the ignorePaths argument of its
// compare and diff functions.
package jsondiff

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Tolerance is the allowed difference between two numbers. Two numbers are
// equal when |expected-actual| <= Abs or |expected-actual| <= Rel*|expected|.
type Tolerance struct {
	Abs float64 `json:"abs" yaml:"abs"`
	Rel float64 `json:"rel" yaml:"rel"`
}

// ToleranceRule applies a tolerance to the nodes matched by Path and to all of
// their descendants. An omitted Abs or Rel is zero; it is not inherited from
// Options.Default.
type ToleranceRule struct {
	Path string  `json:"path" yaml:"path"`
	Abs  float64 `json:"abs" yaml:"abs"`
	Rel  float64 `json:"rel" yaml:"rel"`
}

// EnumRule maps enum value names to their numbers at the nodes matched by
// Path, for example protobuf enums that one side holds as names and the other
// as numbers. It is used only when at least one side is a name in Values: two
// numbers are compared as numbers, and a name missing from Values as a string.
// Names that share a number (aliases) compare equal.
type EnumRule struct {
	Path   string     `json:"path" yaml:"path"`
	Values EnumValues `json:"values" yaml:"values"`
}

// EnumValues maps enum value names to their numbers.
type EnumValues map[string]int64

// UnmarshalYAML accepts integers only: decoding YAML into int64 would
// otherwise truncate 1.5 to 1 and make different values compare equal.
func (v *EnumValues) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: values must be a mapping of names to integers", n.Line)
	}
	m := EnumValues{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		key, val := n.Content[i], n.Content[i+1]
		var name string
		if err := key.Decode(&name); err != nil {
			return err
		}
		if _, dup := m[name]; dup {
			return fmt.Errorf("line %d: duplicate value name %q", key.Line, name)
		}
		var num int64
		if val.Kind != yaml.ScalarNode || val.ShortTag() != "!!int" {
			return fmt.Errorf("line %d: value of %q must be an integer", val.Line, name)
		}
		if err := val.Decode(&num); err != nil {
			return fmt.Errorf("line %d: value of %q: %w", val.Line, name, err)
		}
		m[name] = num
	}
	*v = m
	return nil
}

// Options controls how two documents are compared.
type Options struct {
	// Default is the tolerance for numbers matched by no rule in Tolerances.
	Default Tolerance `json:"default" yaml:"default"`
	// Ignore lists paths that are not compared, including their descendants.
	Ignore []string `json:"ignore" yaml:"ignore"`
	// Unordered lists paths of arrays whose element order does not matter.
	// It applies to the matched arrays only, not to arrays nested in them.
	Unordered []string `json:"unordered" yaml:"unordered"`
	// NumericStrings treats valid JSON number strings at the matched nodes
	// as numbers, on either side, including for tolerance comparisons. It
	// does not apply to descendants unless the path expression selects them.
	NumericStrings []string `json:"numericStrings" yaml:"numericStrings"`
	// Enums lists enum value tables. At the nodes a rule matches, an enum name
	// and its number compare equal in either direction. It does not apply to
	// descendants unless the path expression selects them.
	Enums []EnumRule `json:"enums" yaml:"enums"`
	// Tolerances lists per-path tolerances. When several rules match a number,
	// the rule matching the deepest path wins, and among rules matching at
	// the same depth the last one wins.
	Tolerances []ToleranceRule `json:"tolerances" yaml:"tolerances"`
}

// LoadConfig reads Options from a file. Files with a .json extension are read
// as JSON and all others as YAML. Unknown keys are rejected.
func LoadConfig(path string) (*Options, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var opts *Options
	if strings.EqualFold(filepath.Ext(path), ".json") {
		opts, err = ParseJSONConfig(data)
	} else {
		opts, err = ParseYAMLConfig(data)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return opts, nil
}

// ParseYAMLConfig parses Options from YAML. Unknown keys are rejected.
func ParseYAMLConfig(data []byte) (*Options, error) {
	opts := &Options{}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(opts); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	return opts, nil
}

// ParseJSONConfig parses Options from JSON. Unknown keys are rejected.
func ParseJSONConfig(data []byte) (*Options, error) {
	opts := &Options{}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(opts); err != nil {
		return nil, err
	}
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	return opts, nil
}

// Validate reports negative tolerances, empty paths and paths that are not
// valid jq expressions.
func (o *Options) Validate() error {
	_, err := compile(o)
	return err
}

func validateTolerance(name string, abs, rel float64) error {
	if !(abs >= 0) || !(rel >= 0) || math.IsInf(abs, 0) || math.IsInf(rel, 0) {
		return fmt.Errorf("%s: tolerance must be a finite non-negative number (abs=%v, rel=%v)", name, abs, rel)
	}
	return nil
}
