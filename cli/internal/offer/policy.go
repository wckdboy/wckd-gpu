package offer

import (
	"fmt"
	"strings"
)

// DefaultRequireRegions is the hard filter when a preset omits require_regions.
// EU is a placement constraint. prefer_regions remains a score factor only.
var DefaultRequireRegions = []string{"EU"}

// RegionPolicy is the hard placement rule applied before scoring.
type RegionPolicy struct {
	Regions []string
	Secure  bool
}

// ResolveRequire builds the hard filter.
// A non-nil override replaces the preset list. An empty override clears the
// filter. A nil preset list (the field was omitted) uses DefaultRequireRegions.
func ResolveRequire(preset []string, override *[]string) (RegionPolicy, error) {
	src := preset
	switch {
	case override != nil:
		src = *override
	case src == nil:
		copied := append([]string(nil), DefaultRequireRegions...)
		src = copied
	}
	return ParseRequire(src)
}

// ParseRequire interprets require_regions tokens.
// "any" and "*" clear the hard filter and must be the only token.
// A token ending in "-secure" (EU-secure) keeps the region prefix and
// requires Secure cloud.
func ParseRequire(tokens []string) (RegionPolicy, error) {
	var regions []string
	secure := false
	optOut := false
	for _, raw := range tokens {
		tok := strings.TrimSpace(raw)
		if tok == "" {
			continue
		}
		low := strings.ToLower(tok)
		if low == "any" || low == "*" {
			optOut = true
			continue
		}
		region, isSecure := splitSecureMode(tok)
		if strings.TrimSpace(region) == "" {
			return RegionPolicy{}, fmt.Errorf("require_regions token %q is empty", raw)
		}
		if isSecure {
			secure = true
		}
		regions = appendUniqueFold(regions, region)
	}
	if optOut {
		if len(regions) > 0 || secure {
			return RegionPolicy{}, fmt.Errorf("require_regions any cannot be combined with other tokens")
		}
		return RegionPolicy{}, nil
	}
	return RegionPolicy{Regions: regions, Secure: secure}, nil
}

// SplitList splits a comma-separated require_regions flag or query value.
// An empty string yields an empty list, which clears the hard filter when
// used as an override.
func SplitList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	if out == nil {
		return []string{}
	}
	return out
}

// Label renders a policy the way an operator typed it.
func (p RegionPolicy) Label() string {
	if len(p.Regions) == 0 && !p.Secure {
		return "any"
	}
	if p.Secure && len(p.Regions) == 1 {
		return p.Regions[0] + "-secure"
	}
	if p.Secure {
		return strings.Join(p.Regions, ",") + " +secure"
	}
	return strings.Join(p.Regions, ",")
}

func splitSecureMode(tok string) (region string, secure bool) {
	const suffix = "-secure"
	if strings.HasSuffix(strings.ToLower(tok), suffix) && len(tok) > len(suffix) {
		return strings.TrimSpace(tok[:len(tok)-len(suffix)]), true
	}
	return tok, false
}

func appendUniqueFold(list []string, item string) []string {
	for _, have := range list {
		if strings.EqualFold(have, item) {
			return list
		}
	}
	return append(list, item)
}
