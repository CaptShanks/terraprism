package tfplan

import (
	"sort"

	tfjson "github.com/hashicorp/terraform-json"
)

func convertPlan(p *tfjson.Plan, raw []byte) (*Plan, error) {
	out := &Plan{
		FormatVersion:    p.FormatVersion,
		TerraformVersion: p.TerraformVersion,
		RawJSON:          raw,
	}

	for _, rc := range p.ResourceChanges {
		res := convertResourceChange(rc)
		if res == nil {
			continue
		}
		out.Resources = append(out.Resources, *res)
		switch res.Action {
		case ActionCreate:
			out.TotalAdd++
		case ActionUpdate:
			out.TotalChange++
		case ActionDelete:
			out.TotalDestroy++
		case ActionReplace:
			out.TotalAdd++
			out.TotalDestroy++
		}
	}

	// Sort output_changes by name for deterministic rendering order —
	// map iteration order is otherwise random.
	names := make([]string, 0, len(p.OutputChanges))
	for name := range p.OutputChanges {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		ch := p.OutputChanges[name]
		if ch == nil || ch.Actions.NoOp() {
			continue
		}
		// Existence for an output's own top-level value is determined by
		// its actions, not by nil-checking Before/After — an output whose
		// value is legitimately `null` must not be confused with one that
		// didn't exist before/after.
		beforeExists := !ch.Actions.Create()
		afterExists := !ch.Actions.Delete()
		// attr.Action keeps the real create/update/delete value from
		// buildAttribute — outputResource() labels the synthetic Resource
		// itself ActionOutput for filtering/display, but overwriting the
		// attribute's own Action here would make renderLeafValue always
		// render a "value → new value" arrow, even for a plain create.
		attr := buildAttribute("", name, ch.Before, ch.After, beforeExists, afterExists, ch.AfterUnknown, ch.BeforeSensitive, ch.AfterSensitive)
		out.OutputChanges = append(out.OutputChanges, OutputChange{Name: name, Attribute: attr})
		out.OutputCount++
	}

	return out, nil
}

func convertResourceChange(rc *tfjson.ResourceChange) *Resource {
	if rc == nil || rc.Change == nil {
		return nil
	}
	actions := rc.Change.Actions
	if actions.NoOp() {
		return nil
	}

	action, replacePattern := deriveResourceAction(actions)

	ch := rc.Change
	beforeMap := asMap(ch.Before)
	afterMap := asMap(ch.After)
	unknownMap := asMap(ch.AfterUnknown)
	beforeSensMap := asMap(ch.BeforeSensitive)
	afterSensMap := asMap(ch.AfterSensitive)

	keys := unionKeys(beforeMap, afterMap, unknownMap)
	attrs := make([]Attribute, 0, len(keys))
	for _, k := range keys {
		bv, bExists := beforeMap[k]
		av, aExists := afterMap[k]
		attrs = append(attrs, buildAttribute(k, k, bv, av, bExists, aExists,
			unknownMap[k], beforeSensMap[k], afterSensMap[k]))
	}

	var actionReasons []string
	if rc.ActionReason != "" {
		actionReasons = []string{string(rc.ActionReason)}
	}

	return &Resource{
		Address:        rc.Address,
		Type:           rc.Type,
		Name:           rc.Name,
		ModuleAddress:  rc.ModuleAddress,
		ProviderName:   rc.ProviderName,
		Action:         action,
		ReplacePattern: replacePattern,
		ActionReasons:  actionReasons,
		IsDataSource:   rc.Mode == tfjson.DataResourceMode,
		Attributes:     attrs,
	}
}

func deriveResourceAction(actions tfjson.Actions) (Action, ReplacePattern) {
	switch {
	case actions.Replace():
		if actions.CreateBeforeDestroy() {
			return ActionReplace, ReplaceCreateBeforeDestroy
		}
		return ActionReplace, ReplaceDestroyBeforeCreate
	case actions.Create():
		return ActionCreate, ""
	case actions.Read():
		return ActionRead, ""
	case actions.Update():
		return ActionUpdate, ""
	case actions.Delete():
		return ActionDelete, ""
	case actions.Forget():
		return ActionForget, ""
	default:
		return ActionNoOp, ""
	}
}
