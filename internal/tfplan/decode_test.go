package tfplan

// testdata/plan_real_capture.json and testdata/plan_opentofu_capture.json
// were captured from real `terraform`/`tofu` runs against no-network
// providers (null_resource/random_id), to confirm hand-written fixtures
// haven't drifted from what the engines actually emit. Regenerate with:
//
//	terraform init && terraform plan -out=plan.bin && terraform show -json plan.bin
//	tofu init && tofu plan -out=plan.bin && tofu show -json plan.bin

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("reading fixture %s: %v", name, err)
	}
	return raw
}

func TestDecode(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
		wantErr bool
		check   func(t *testing.T, p *Plan)
	}{
		{
			name:    "create simple",
			fixture: "plan_create_simple.json",
			check: func(t *testing.T, p *Plan) {
				if len(p.Resources) != 1 {
					t.Fatalf("got %d resources, want 1", len(p.Resources))
				}
				r := p.Resources[0]
				if r.Action != ActionCreate {
					t.Errorf("action = %s, want create", r.Action)
				}
				if p.TotalAdd != 1 || p.TotalChange != 0 || p.TotalDestroy != 0 {
					t.Errorf("totals = add:%d change:%d destroy:%d, want 1/0/0", p.TotalAdd, p.TotalChange, p.TotalDestroy)
				}
				if p.OutputCount != 1 {
					t.Errorf("OutputCount = %d, want 1", p.OutputCount)
				}
				// Regression: id/arn are in after_unknown but never in
				// before or after (Terraform omits an unknown value from
				// "after" entirely), so they used to be invisible to
				// unionKeys and silently vanish from the whole tree.
				for _, name := range []string{"id", "arn"} {
					attr := findAttr(t, r.Attributes, name)
					if attr.Action != ActionCreate {
						t.Errorf("%s.Action = %s, want create", name, attr.Action)
					}
					if !attr.Computed {
						t.Errorf("%s.Computed = false, want true (known after apply)", name)
					}
				}
				// Regression: the output's own attribute Action must stay
				// the real create/update/delete value (here: create, since
				// before is absent and after is unknown) rather than being
				// overwritten with ActionOutput, or renderLeafValue would
				// always show a confusing "old → new" arrow instead of
				// "(known after apply)".
				oc := p.OutputChanges[0]
				if oc.Action != ActionCreate {
					t.Errorf("output attribute Action = %s, want create", oc.Action)
				}
				if !oc.Computed {
					t.Errorf("output attribute should be Computed (after_unknown)")
				}
			},
		},
		{
			name:    "no changes",
			fixture: "plan_no_changes.json",
			check: func(t *testing.T, p *Plan) {
				if len(p.Resources) != 0 {
					t.Errorf("got %d resources, want 0 (no-op filtered out)", len(p.Resources))
				}
				if len(p.OutputChanges) != 0 {
					t.Errorf("got %d output changes, want 0", len(p.OutputChanges))
				}
			},
		},
		{
			name:    "replace patterns",
			fixture: "plan_replace.json",
			check: func(t *testing.T, p *Plan) {
				if len(p.Resources) != 2 {
					t.Fatalf("got %d resources, want 2", len(p.Resources))
				}
				byAddr := map[string]Resource{}
				for _, r := range p.Resources {
					byAddr[r.Address] = r
				}
				dbc := byAddr["aws_instance.destroy_before_create"]
				if dbc.Action != ActionReplace || dbc.ReplacePattern != ReplaceDestroyBeforeCreate {
					t.Errorf("destroy_before_create: action=%s pattern=%s", dbc.Action, dbc.ReplacePattern)
				}
				if len(dbc.ActionReasons) != 1 || dbc.ActionReasons[0] != "requested" {
					t.Errorf("destroy_before_create: ActionReasons = %v, want [requested]", dbc.ActionReasons)
				}
				cbd := byAddr["aws_instance.create_before_destroy"]
				if cbd.Action != ActionReplace || cbd.ReplacePattern != ReplaceCreateBeforeDestroy {
					t.Errorf("create_before_destroy: action=%s pattern=%s", cbd.Action, cbd.ReplacePattern)
				}
				if p.TotalAdd != 2 || p.TotalDestroy != 2 || p.TotalChange != 0 {
					t.Errorf("totals = add:%d change:%d destroy:%d, want 2/0/2", p.TotalAdd, p.TotalChange, p.TotalDestroy)
				}
			},
		},
		{
			name:    "sensitive nested path",
			fixture: "plan_sensitive_nested.json",
			check: func(t *testing.T, p *Plan) {
				if len(p.Resources) != 1 {
					t.Fatalf("got %d resources, want 1", len(p.Resources))
				}
				tags := findAttr(t, p.Resources[0].Attributes, "tags")
				if tags.Sensitive {
					t.Errorf("tags container should not be marked sensitive itself, only its Name child")
				}
				name := findAttr(t, tags.Children, "Name")
				if !name.Sensitive {
					t.Errorf("tags.Name should be sensitive")
				}
				if name.Old != "old-secret-name" || name.New != "new-secret-name" {
					t.Errorf("sensitive leaf should still carry its real Old/New (redaction is a display concern), got Old=%v New=%v", name.Old, name.New)
				}
				env := findAttr(t, tags.Children, "Environment")
				if env.Sensitive {
					t.Errorf("tags.Environment should not be sensitive")
				}
				if env.New != "staging" {
					t.Errorf("tags.Environment.New = %v, want staging", env.New)
				}
			},
		},
		{
			name:    "multiline string value (real capture)",
			fixture: "plan_real_capture.json",
			check: func(t *testing.T, p *Plan) {
				var res *Resource
				for i := range p.Resources {
					if p.Resources[i].Address == "null_resource.example" {
						res = &p.Resources[i]
					}
				}
				if res == nil {
					t.Fatalf("null_resource.example not found")
				}
				triggers := findAttr(t, res.Attributes, "triggers")
				values := findAttr(t, triggers.Children, "values_yaml")
				s, ok := values.New.(string)
				if !ok {
					t.Fatalf("values_yaml.New is %T, want string", values.New)
				}
				if !strings.Contains(s, "\n") {
					t.Errorf("values_yaml should be a multi-line string, got %q", s)
				}
				if !strings.Contains(s, "replicaCount") {
					t.Errorf("values_yaml missing expected content: %q", s)
				}
			},
		},
		{
			// Regression test: an attribute explicitly set to JSON null on a
			// newly-created resource must still be classified as ActionCreate,
			// not ActionNoOp. Both "genuinely absent" and "present with value
			// null" decode to Go nil, so the fix must track key/index
			// existence explicitly rather than comparing decoded values to nil.
			name:    "null-valued attribute on create is still Create, not NoOp (real capture)",
			fixture: "plan_real_capture.json",
			check: func(t *testing.T, p *Plan) {
				var res *Resource
				for i := range p.Resources {
					if p.Resources[i].Address == "random_id.example" {
						res = &p.Resources[i]
					}
				}
				if res == nil {
					t.Fatalf("random_id.example not found")
				}
				if res.Action != ActionCreate {
					t.Fatalf("random_id.example action = %s, want create", res.Action)
				}
				keepers := findAttr(t, res.Attributes, "keepers")
				if keepers.New != nil {
					t.Errorf("keepers.New = %v, want nil (explicit JSON null)", keepers.New)
				}
				if keepers.Action != ActionCreate {
					t.Errorf("keepers.Action = %s, want create (was misclassified as no-op before the existence-tracking fix)", keepers.Action)
				}
			},
		},
		{
			// Regression test: an attribute that existed before and becomes
			// wholly unknown on update (after_unknown: true as a bare bool,
			// not a per-child map) must be classified as ActionUpdate with
			// Computed true -- both for the attribute itself and every one
			// of its children -- not ActionDelete. Terraform's plan JSON
			// omits such an attribute from "after" entirely, the same
			// encoding it uses for a genuine deletion, so existence alone
			// can't distinguish the two cases; the unknown flag must be
			// checked first.
			name:    "attribute becoming unknown on update is Update, not Delete",
			fixture: "plan_update_becomes_unknown.json",
			check: func(t *testing.T, p *Plan) {
				if len(p.Resources) != 1 {
					t.Fatalf("got %d resources, want 1", len(p.Resources))
				}
				res := p.Resources[0]

				id := findAttr(t, res.Attributes, "id")
				if id.Action != ActionUpdate || !id.Computed {
					t.Errorf("id: action=%s computed=%v, want update/true", id.Action, id.Computed)
				}

				metadata := findAttr(t, res.Attributes, "metadata")
				if metadata.Action != ActionUpdate || !metadata.Computed {
					t.Errorf("metadata: action=%s computed=%v, want update/true", metadata.Action, metadata.Computed)
				}
				for _, name := range []string{"app_version", "chart", "revision"} {
					child := findAttr(t, metadata.Children, name)
					if child.Action != ActionUpdate || !child.Computed {
						t.Errorf("metadata.%s: action=%s computed=%v, want update/true", name, child.Action, child.Computed)
					}
				}
			},
		},
		{
			name:    "opentofu capture decodes cleanly",
			fixture: "plan_opentofu_capture.json",
			check: func(t *testing.T, p *Plan) {
				if len(p.Resources) != 1 {
					t.Fatalf("got %d resources, want 1", len(p.Resources))
				}
				if p.Resources[0].Action != ActionCreate {
					t.Errorf("action = %s, want create", p.Resources[0].Action)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			raw := loadFixture(t, tc.fixture)
			p, err := DecodeBytes(raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("DecodeBytes: %v", err)
			}
			if tc.check != nil {
				tc.check(t, p)
			}
		})
	}
}

// A plan with output changes but no resource changes (e.g. only a
// computed local or data source value feeding an output) must still
// count as "has changes" — this is what DisplayResources is for.
func TestDisplayResourcesIncludesOutputChanges(t *testing.T) {
	raw := loadFixture(t, "plan_create_simple.json")
	p, err := DecodeBytes(raw)
	if err != nil {
		t.Fatalf("DecodeBytes: %v", err)
	}
	if len(p.OutputChanges) == 0 {
		t.Fatalf("fixture should have at least one output change")
	}

	display := p.DisplayResources()
	if len(display) != len(p.Resources)+len(p.OutputChanges) {
		t.Fatalf("DisplayResources len = %d, want %d resources + %d outputs",
			len(display), len(p.Resources), len(p.OutputChanges))
	}

	outputRes := display[len(display)-1]
	if outputRes.Action != ActionOutput {
		t.Errorf("synthetic output resource Action = %s, want output", outputRes.Action)
	}
	if outputRes.Address != "output.instance_id" {
		t.Errorf("synthetic output resource Address = %q, want output.instance_id", outputRes.Address)
	}
	if len(outputRes.Attributes) != 1 || outputRes.Attributes[0].Name != "value" {
		t.Errorf("scalar output should wrap its value in a single 'value' attribute, got %+v", outputRes.Attributes)
	}
	// The Resource itself is labeled ActionOutput for filtering/display,
	// but the wrapped attribute's own Action must stay the real diffed
	// value (create here), not be overwritten to ActionOutput too.
	if outputRes.Attributes[0].Action != ActionCreate {
		t.Errorf("wrapped output attribute Action = %s, want create", outputRes.Attributes[0].Action)
	}
}

func TestDisplayResourcesNoOutputsReturnsResourcesUnchanged(t *testing.T) {
	p := &Plan{Resources: []Resource{{Address: "a.b"}}}
	display := p.DisplayResources()
	if len(display) != 1 || display[0].Address != "a.b" {
		t.Fatalf("DisplayResources with no outputs = %+v, want unchanged Resources", display)
	}
}

func findAttr(t *testing.T, attrs []Attribute, name string) Attribute {
	t.Helper()
	for _, a := range attrs {
		if a.Name == name {
			return a
		}
	}
	t.Fatalf("attribute %q not found among %d attributes", name, len(attrs))
	return Attribute{}
}

// A whole-subtree-sensitive attribute (before_sensitive/after_sensitive
// is the bare bool true at a container path, not a per-child map) still
// carries its real value on Old/New -- just not recursed into Children
// -- the same "redaction is a display concern, not a decode-time one"
// contract as a sensitive leaf.
func TestSensitiveWholeSubtreeCarriesRealValue(t *testing.T) {
	raw := []byte(`{
		"format_version": "1.2",
		"resource_changes": [{
			"address": "aws_db_instance.main",
			"mode": "managed",
			"type": "aws_db_instance",
			"name": "main",
			"change": {
				"actions": ["update"],
				"before": {"credentials": {"user": "old-user", "pass": "old-pass"}},
				"after": {"credentials": {"user": "new-user", "pass": "new-pass"}},
				"after_unknown": {},
				"before_sensitive": {"credentials": true},
				"after_sensitive": {"credentials": true}
			}
		}],
		"output_changes": {}
	}`)

	p, err := DecodeBytes(raw)
	if err != nil {
		t.Fatalf("DecodeBytes: %v", err)
	}
	if len(p.Resources) != 1 {
		t.Fatalf("got %d resources, want 1", len(p.Resources))
	}

	creds := findAttr(t, p.Resources[0].Attributes, "credentials")
	if !creds.Sensitive {
		t.Errorf("credentials should be marked sensitive")
	}
	if len(creds.Children) != 0 {
		t.Errorf("sensitive container should not recurse into Children, got %+v", creds.Children)
	}
	wantOld := map[string]interface{}{"user": "old-user", "pass": "old-pass"}
	wantNew := map[string]interface{}{"user": "new-user", "pass": "new-pass"}
	if !reflect.DeepEqual(creds.Old, wantOld) {
		t.Errorf("credentials.Old = %#v, want %#v", creds.Old, wantOld)
	}
	if !reflect.DeepEqual(creds.New, wantNew) {
		t.Errorf("credentials.New = %#v, want %#v", creds.New, wantNew)
	}
}

// Regression for a real bug: a map key that's brand new *and* not known
// until apply (e.g. an ARN derived from a resource being created in the
// same plan) is absent from both "before" (genuinely new) and "after"
// (Terraform omits an unknown value from "after" entirely) -- present
// only in the parallel after_unknown structure. unionKeys used to only
// look at before/after, so such a key was never discovered at all and
// silently vanished from the whole tree instead of showing up as
// "(known after apply)", alongside ordinary sibling keys that were
// either updated or genuinely untouched.
func TestNewUnknownMapKeyIsNotDroppedFromTree(t *testing.T) {
	raw := []byte(`{
		"format_version": "1.2",
		"resource_changes": [{
			"address": "aws_iam_role.example",
			"mode": "managed",
			"type": "aws_iam_role",
			"name": "example",
			"change": {
				"actions": ["update"],
				"before": {"config": {"existing_key": "old", "unchanged_key": "same"}},
				"after": {"config": {"existing_key": "new", "unchanged_key": "same"}},
				"after_unknown": {"config": {"new_unknown_key": true}},
				"before_sensitive": {},
				"after_sensitive": {}
			}
		}],
		"output_changes": {}
	}`)

	p, err := DecodeBytes(raw)
	if err != nil {
		t.Fatalf("DecodeBytes: %v", err)
	}
	if len(p.Resources) != 1 {
		t.Fatalf("got %d resources, want 1", len(p.Resources))
	}

	config := findAttr(t, p.Resources[0].Attributes, "config")
	newKey := findAttr(t, config.Children, "new_unknown_key")
	if newKey.Action != ActionCreate {
		t.Errorf("new_unknown_key.Action = %s, want create", newKey.Action)
	}
	if !newKey.Computed {
		t.Errorf("new_unknown_key.Computed = false, want true (known after apply)")
	}

	// Sibling keys must be unaffected by the fix.
	existing := findAttr(t, config.Children, "existing_key")
	if existing.Action != ActionUpdate || existing.Old != "old" || existing.New != "new" {
		t.Errorf("existing_key = %+v, want an update from \"old\" to \"new\"", existing)
	}
	unchanged := findAttr(t, config.Children, "unchanged_key")
	if unchanged.Action != ActionNoOp {
		t.Errorf("unchanged_key.Action = %s, want no-op", unchanged.Action)
	}
}

func TestFormatVersionCompatibility(t *testing.T) {
	tests := []struct {
		name          string
		formatVersion string
		wantErr       bool
	}{
		{name: "1.0 accepted", formatVersion: "1.0", wantErr: false},
		{name: "1.2 accepted", formatVersion: "1.2", wantErr: false},
		{name: "0.1 accepted (lower bound)", formatVersion: "0.1", wantErr: false},
		{name: "2.0 rejected (upper bound)", formatVersion: "2.0", wantErr: true},
		{name: "malformed rejected", formatVersion: "not-a-version", wantErr: true},
		{name: "missing rejected", formatVersion: "", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			raw := []byte(`{"format_version":"` + tc.formatVersion + `","resource_changes":[],"output_changes":{}}`)
			_, err := DecodeBytes(raw)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for format_version %q, got nil", tc.formatVersion)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error for format_version %q: %v", tc.formatVersion, err)
			}
		})
	}
}
