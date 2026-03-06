package realtime

import (
	"path/filepath"
)

// PathFilter matches file paths against include and exclude glob
// patterns. A path matches if it matches at least one include pattern
// (or there are no include patterns) and does not match any exclude
// pattern. Glob syntax follows filepath.Match.
type PathFilter struct {
	includes []string
	excludes []string
}

// NewPathFilter creates a new PathFilter.
//
// Parameters:
//   - includes: glob patterns that a path must match. If empty, all
//     paths are implicitly included.
//   - excludes: glob patterns that reject a path even if it matches
//     an include pattern. Excludes take priority over includes.
func NewPathFilter(includes, excludes []string) *PathFilter {
	return &PathFilter{
		includes: includes,
		excludes: excludes,
	}
}

// Match reports whether the given path passes the filter.
func (f *PathFilter) Match(path string) bool {
	// Check excludes first — they always take priority.
	for _, pattern := range f.excludes {
		if matched, err := filepath.Match(pattern, path); err == nil && matched {
			return false
		}
		// Also try matching against the base name for convenience.
		base := filepath.Base(path)
		if matched, err := filepath.Match(pattern, base); err == nil && matched {
			return false
		}
	}

	// If no include patterns are specified, everything is included.
	if len(f.includes) == 0 {
		return true
	}

	for _, pattern := range f.includes {
		if matched, err := filepath.Match(pattern, path); err == nil && matched {
			return true
		}
		base := filepath.Base(path)
		if matched, err := filepath.Match(pattern, base); err == nil && matched {
			return true
		}
	}

	return false
}

// Includes returns the include patterns.
func (f *PathFilter) Includes() []string {
	out := make([]string, len(f.includes))
	copy(out, f.includes)
	return out
}

// Excludes returns the exclude patterns.
func (f *PathFilter) Excludes() []string {
	out := make([]string, len(f.excludes))
	copy(out, f.excludes)
	return out
}
