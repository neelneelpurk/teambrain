package capability

import (
	"bytes"
	"encoding/json"
)

// HookRegistration is the settings.json entry teambrain creates for a hook.
type HookRegistration struct {
	Event   string
	Matcher string
	Command string
}

// hookEntry is a single command hook. Foreign entries are never decoded through
// this type during a merge (they stay as raw JSON), so no unknown fields are
// dropped; it is used only to construct teambrain's own entry and to scan for a
// command during idempotency/removal checks.
type hookEntry struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

// hookGroup is a matcher group under an event.
type hookGroup struct {
	Matcher string      `json:"matcher,omitempty"`
	Hooks   []hookEntry `json:"hooks"`
}

// MergeHook performs a typed read-modify-write that adds reg to a Claude Code
// settings.json. Foreign top-level keys, foreign events, and foreign hook
// entries (including unknown fields) are preserved as raw JSON; only the
// inserted entry is freshly encoded. Whitespace is normalized to two-space
// indent. The merge is idempotent: if reg.Command is already registered under
// the event, no change is made.
func MergeHook(raw []byte, reg HookRegistration) (out []byte, changed bool, err error) {
	top, err := decodeObject(raw)
	if err != nil {
		return nil, false, err
	}
	hooks, err := decodeObject(top["hooks"])
	if err != nil {
		return nil, false, err
	}
	groups, err := decodeArray(hooks[reg.Event])
	if err != nil {
		return nil, false, err
	}

	for _, g := range groups {
		if groupHasCommand(g, reg.Command) {
			return raw, false, nil
		}
	}

	entry, err := json.Marshal(hookGroup{
		Matcher: reg.Matcher,
		Hooks:   []hookEntry{{Type: "command", Command: reg.Command}},
	})
	if err != nil {
		return nil, false, err
	}
	groups = append(groups, entry)

	if err := reassemble(top, hooks, reg.Event, groups); err != nil {
		return nil, false, err
	}
	out, err = marshalSettings(top)
	return out, true, err
}

// UnmergeHook removes every hook entry whose command equals command, pruning
// emptied matcher groups, events, and the hooks object. Foreign content is
// preserved.
func UnmergeHook(raw []byte, command string) (out []byte, changed bool, err error) {
	top, err := decodeObject(raw)
	if err != nil {
		return nil, false, err
	}
	hooks, err := decodeObject(top["hooks"])
	if err != nil {
		return nil, false, err
	}

	for event, rawGroups := range hooks {
		groups, err := decodeArray(rawGroups)
		if err != nil {
			return nil, false, err
		}
		kept := groups[:0]
		for _, g := range groups {
			pruned, removed, perr := removeCommandFromGroup(g, command)
			if perr != nil {
				return nil, false, perr
			}
			if removed {
				changed = true
			}
			if pruned != nil {
				kept = append(kept, pruned)
			}
		}
		if len(kept) == 0 {
			delete(hooks, event)
			continue
		}
		gb, err := json.Marshal(kept)
		if err != nil {
			return nil, false, err
		}
		hooks[event] = gb
	}

	if !changed {
		return raw, false, nil
	}

	if len(hooks) == 0 {
		delete(top, "hooks")
	} else {
		hb, err := json.Marshal(hooks)
		if err != nil {
			return nil, false, err
		}
		top["hooks"] = hb
	}
	out, err = marshalSettings(top)
	return out, true, err
}

// decodeObject parses raw as a JSON object into an ordered-insensitive map of
// raw values, treating empty input as an empty object.
func decodeObject(raw []byte) (map[string]json.RawMessage, error) {
	m := map[string]json.RawMessage{}
	if len(bytes.TrimSpace(raw)) == 0 {
		return m, nil
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// decodeArray parses raw as a JSON array of raw elements, treating empty input
// as an empty array.
func decodeArray(raw []byte) ([]json.RawMessage, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}
	var a []json.RawMessage
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, err
	}
	return a, nil
}

// groupHasCommand reports whether a matcher group contains a hook entry with the
// given command. Unknown fields in the group are ignored, not dropped.
func groupHasCommand(group json.RawMessage, command string) bool {
	var parsed struct {
		Hooks []hookEntry `json:"hooks"`
	}
	if err := json.Unmarshal(group, &parsed); err != nil {
		return false
	}
	for _, h := range parsed.Hooks {
		if h.Command == command {
			return true
		}
	}
	return false
}

// removeCommandFromGroup returns the group with command-matching entries
// removed. It preserves unknown group/entry fields by editing the raw tree. A
// nil pruned result means the group is now empty and should be dropped.
func removeCommandFromGroup(group json.RawMessage, command string) (pruned json.RawMessage, removed bool, err error) {
	obj, err := decodeObject(group)
	if err != nil {
		return nil, false, err
	}
	entries, err := decodeArray(obj["hooks"])
	if err != nil {
		return nil, false, err
	}

	kept := entries[:0]
	for _, e := range entries {
		var probe hookEntry
		if json.Unmarshal(e, &probe) == nil && probe.Command == command {
			removed = true
			continue
		}
		kept = append(kept, e)
	}
	if !removed {
		return group, false, nil
	}
	if len(kept) == 0 {
		return nil, true, nil
	}
	hb, err := json.Marshal(kept)
	if err != nil {
		return nil, false, err
	}
	obj["hooks"] = hb
	out, err := json.Marshal(obj)
	return out, true, err
}

// reassemble writes the updated event array back into hooks and hooks back into
// top.
func reassemble(top, hooks map[string]json.RawMessage, event string, groups []json.RawMessage) error {
	gb, err := json.Marshal(groups)
	if err != nil {
		return err
	}
	hooks[event] = gb
	hb, err := json.Marshal(hooks)
	if err != nil {
		return err
	}
	top["hooks"] = hb
	return nil
}

// marshalSettings renders the settings object with two-space indent and a
// trailing newline.
func marshalSettings(top map[string]json.RawMessage) ([]byte, error) {
	b, err := json.MarshalIndent(top, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}
