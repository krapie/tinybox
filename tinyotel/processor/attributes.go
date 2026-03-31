package processor

import "github.com/krapi0314/tinybox/tinyotel/model"

// AttributeRule describes a single transformation to apply to span and
// resource attributes.
type AttributeRule struct {
	Action string // "insert" | "update" | "delete" | "rename"
	Key    string
	Value  string // for insert/update
	NewKey string // for rename
}

// Attributes applies a list of AttributeRules to every span's attributes
// and resource attributes.
type Attributes struct {
	rules []AttributeRule
}

// NewAttributes creates an Attributes processor with the given rules.
func NewAttributes(rules []AttributeRule) *Attributes {
	return &Attributes{rules: rules}
}

func (a *Attributes) Process(spans []model.Span) []model.Span {
	for i := range spans {
		applyRules(spans[i].Attributes, a.rules)
		applyRules(spans[i].Resource.Attributes, a.rules)
	}
	return spans
}

func applyRules(attrs map[string]string, rules []AttributeRule) {
	for _, r := range rules {
		switch r.Action {
		case "insert":
			if _, exists := attrs[r.Key]; !exists {
				attrs[r.Key] = r.Value
			}
		case "update":
			if _, exists := attrs[r.Key]; exists {
				attrs[r.Key] = r.Value
			}
		case "delete":
			delete(attrs, r.Key)
		case "rename":
			if v, exists := attrs[r.Key]; exists {
				attrs[r.NewKey] = v
				delete(attrs, r.Key)
			}
		}
	}
}
