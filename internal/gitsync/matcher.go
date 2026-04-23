package gitsync

import (
	"path"
	"strings"

	"github.com/storl0rd/otel-hive/internal/gitsync/providers"
)

// ParseRules derives label-selector rules from file paths under configRoot.
//
// Supported path conventions (all relative to repo root):
//
//	<root>/environments/<env>/<role>.yaml
//	  → selectors: deployment.environment=<env>, collector.role=<role>
//
//	<root>/environments/<env>/default.yaml  (or base.yaml)
//	  → selectors: deployment.environment=<env>
//	  (matches any collector in that environment regardless of role)
//
//	<root>/environments/default/<role>.yaml
//	  → selectors: collector.role=<role>  (matches any env)
//
//	<root>/environments/default/base.yaml  (or default.yaml)
//	  → selectors: {}  (matches ALL agents — fallback config)
//
//	<root>/groups/<group-id>.yaml
//	  → selectors: group.id=<group-id>
//	  (also matches agents whose group_id field equals <group-id>)
//
// Files that don't match any known pattern are ignored.
func ParseRules(entries []providers.FileEntry, configRoot string) []LabelRule {
	root := strings.TrimSuffix(configRoot, "/")
	var rules []LabelRule

	for _, e := range entries {
		rel := relPath(e.Path, root)
		if rel == "" {
			continue
		}

		parts := splitPath(rel)
		if len(parts) == 0 {
			continue
		}

		var selectors map[string]string

		switch {
		// environments/<env>/<file>.yaml
		case len(parts) == 3 && parts[0] == "environments":
			env := parts[1]
			role := stripExt(parts[2])
			selectors = make(map[string]string)

			if env != "default" {
				selectors["deployment.environment"] = env
			}
			if role != "base" && role != "default" {
				selectors["collector.role"] = role
			}
			// If both are empty (default/base.yaml) selectors stays {} → matches all

		// groups/<group-id>.yaml
		case len(parts) == 2 && parts[0] == "groups":
			groupID := stripExt(parts[1])
			selectors = map[string]string{
				"group.id": groupID,
			}

		default:
			continue
		}

		rules = append(rules, LabelRule{
			FilePath:  e.Path,
			Selectors: selectors,
		})
	}

	return rules
}

// MatchAgents returns the subset of agents whose labels satisfy the rule's
// selectors. An agent matches if it has ALL required key=value pairs.
// An empty Selectors map matches every agent.
func MatchAgents[A interface{ GetLabels() map[string]string }](
	rule LabelRule,
	agents []A,
) []A {
	var matched []A
	for _, a := range agents {
		if labelsMatch(rule.Selectors, a.GetLabels()) {
			matched = append(matched, a)
		}
	}
	return matched
}

// labelsMatch returns true when all required k=v pairs are present in labels.
func labelsMatch(required, labels map[string]string) bool {
	for k, v := range required {
		if labels[k] != v {
			return false
		}
	}
	return true
}

// ── Path helpers ──────────────────────────────────────────────────────────────

// relPath returns the portion of filePath after root/, or "" if not under root.
func relPath(filePath, root string) string {
	prefix := root + "/"
	if !strings.HasPrefix(filePath, prefix) {
		return ""
	}
	return strings.TrimPrefix(filePath, prefix)
}

// splitPath splits a slash-separated path into its components,
// discarding empty segments.
func splitPath(p string) []string {
	return strings.Split(path.Clean(p), "/")
}

// stripExt removes the file extension from a filename.
func stripExt(filename string) string {
	ext := path.Ext(filename)
	return strings.TrimSuffix(filename, ext)
}
