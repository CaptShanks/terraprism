package tfplan

// Action represents the type of change being made to a resource,
// output, or individual attribute node.
type Action string

const (
	ActionNoOp    Action = "no-op"
	ActionCreate  Action = "create"
	ActionRead    Action = "read"
	ActionUpdate  Action = "update"
	ActionDelete  Action = "delete"
	ActionReplace Action = "replace"
	ActionForget  Action = "forget"
	ActionOutput  Action = "output"
)

// ReplacePattern distinguishes the ordering of a replace action. It is
// only meaningful when Resource.Action == ActionReplace.
type ReplacePattern string

const (
	ReplaceCreateBeforeDestroy ReplacePattern = "create-before-destroy"
	ReplaceDestroyBeforeCreate ReplacePattern = "destroy-before-create"
)

// ValueKind classifies the shape of an Attribute node so internal/tui can
// decide how to render it (scalar row vs. foldable container) without
// inspecting the value itself.
type ValueKind string

const (
	KindNull   ValueKind = "null"
	KindString ValueKind = "string"
	KindNumber ValueKind = "number"
	KindBool   ValueKind = "bool"
	KindList   ValueKind = "list"
	KindMap    ValueKind = "map"
)

// Attribute is one node in a resource's (or output's) before/after diff
// tree. A leaf node carries Old/New scalar values; a container node
// (Kind == KindList or KindMap) carries Children instead, each already
// diffed the same way. This is the sole rendering input for internal/tui.
type Attribute struct {
	Name string // path segment: attribute name, map key, or list index
	Path string // full path, e.g. "tags.Environment", "ingress[0].cidr_blocks[1]"
	Kind ValueKind

	// Action is this node's own diff action, aggregated up from Children
	// for containers ("update" if any descendant changed).
	Action Action

	// Old and New are decoded JSON scalars (string, json.Number, bool, or
	// nil for absent/null) for an ordinary leaf, or the full decoded
	// container value (map[string]interface{}/[]interface{}) for a
	// whole-subtree-sensitive node -- see Sensitive below. Both are nil
	// for an ordinary (non-sensitive) container node, which carries its
	// values via Children instead.
	Old any
	New any

	// Children is non-nil only for a non-sensitive container node (Kind
	// == KindList or KindMap), and empty for a sensitive container (not
	// recursed into; its value is carried on Old/New as a whole instead).
	Children []Attribute

	Computed  bool // from after_unknown at this exact path
	Sensitive bool // from before_sensitive || after_sensitive at this exact path
}

// Resource represents one resource_changes[] entry with a real change
// (no-op entries are filtered out during conversion).
type Resource struct {
	Address       string
	Type          string
	Name          string
	ModuleAddress string // "" for root module resources
	ProviderName  string

	Action         Action
	ReplacePattern ReplacePattern // meaningful only when Action == ActionReplace
	ActionReasons  []string       // e.g. "tainted"; display/debug only, never used for logic
	IsDataSource   bool

	Attributes []Attribute
}

// ChangedAttributeCount returns the number of leaf attributes (recursively)
// whose Action is not ActionNoOp, for display purposes (e.g. "(3 changes)"
// next to a resource's address).
func (r Resource) ChangedAttributeCount() int {
	n := 0
	var walk func(attrs []Attribute)
	walk = func(attrs []Attribute) {
		for _, a := range attrs {
			if len(a.Children) > 0 {
				walk(a.Children)
				continue
			}
			if a.Action != ActionNoOp {
				n++
			}
		}
	}
	walk(r.Attributes)
	return n
}

// OutputChange represents one output_changes[] entry with a real change.
type OutputChange struct {
	Name string
	Attribute
}

// Plan is the terraprism view model built from a decoded
// `terraform show -json` (or `tofu show -json`) payload.
type Plan struct {
	FormatVersion    string
	TerraformVersion string

	Resources     []Resource
	OutputChanges []OutputChange

	TotalAdd     int
	TotalChange  int
	TotalDestroy int
	OutputCount  int

	// RawJSON holds the original decoded bytes, kept for history
	// persistence. internal/tui must never re-parse this.
	RawJSON []byte
}

// DisplayResources returns Resources with each OutputChange appended as a
// synthetic Resource (Action ActionOutput), giving callers that want a
// single unified, navigable list — as Terraform's own CLI text output
// interleaves output changes alongside resource changes — one to render,
// count, or check for emptiness against, instead of having to remember to
// check OutputChanges separately (a plan with output-only changes and no
// resource changes must still count as "has changes").
func (p *Plan) DisplayResources() []Resource {
	if len(p.OutputChanges) == 0 {
		return p.Resources
	}
	resources := make([]Resource, 0, len(p.Resources)+len(p.OutputChanges))
	resources = append(resources, p.Resources...)
	for _, oc := range p.OutputChanges {
		resources = append(resources, outputResource(oc))
	}
	return resources
}

// outputResource converts an OutputChange into a synthetic Resource for
// display purposes. Container-shaped outputs (map/list) spread their
// Children directly as top-level attributes; scalar outputs are wrapped
// in a single "value" attribute, since a bare output value has no
// attribute name of its own the way a resource attribute does.
func outputResource(oc OutputChange) Resource {
	attrs := oc.Children
	if len(attrs) == 0 {
		value := oc.Attribute
		value.Name = "value"
		attrs = []Attribute{value}
	}
	return Resource{
		Address:    "output." + oc.Name,
		Type:       "output",
		Name:       oc.Name,
		Action:     ActionOutput,
		Attributes: attrs,
	}
}
