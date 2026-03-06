package realtime

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Constructor tests -----------------------------------------------------

func TestNewPathFilter(t *testing.T) {
	f := NewPathFilter(
		[]string{"*.go", "*.ts"},
		[]string{"*_test.go"},
	)
	require.NotNil(t, f)
	assert.Equal(t, []string{"*.go", "*.ts"}, f.Includes())
	assert.Equal(t, []string{"*_test.go"}, f.Excludes())
}

func TestNewPathFilter_Empty(t *testing.T) {
	f := NewPathFilter(nil, nil)
	require.NotNil(t, f)
	assert.Empty(t, f.Includes())
	assert.Empty(t, f.Excludes())
}

// --- Match tests -----------------------------------------------------------

func TestPathFilter_Match_NoPatterns(t *testing.T) {
	f := NewPathFilter(nil, nil)

	// With no patterns, everything matches.
	assert.True(t, f.Match("/any/path/file.txt"))
	assert.True(t, f.Match("relative.go"))
	assert.True(t, f.Match(""))
}

func TestPathFilter_Match_IncludesOnly(t *testing.T) {
	f := NewPathFilter([]string{"*.go", "*.md"}, nil)

	tests := []struct {
		name    string
		path    string
		matched bool
	}{
		{"go file", "main.go", true},
		{"md file", "README.md", true},
		{"ts file", "index.ts", false},
		{"txt file", "notes.txt", false},
		{"go in subdir by basename", "/src/pkg/handler.go", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.matched, f.Match(tt.path))
		})
	}
}

func TestPathFilter_Match_ExcludesOnly(t *testing.T) {
	f := NewPathFilter(nil, []string{"*.log", "*.tmp"})

	tests := []struct {
		name    string
		path    string
		matched bool
	}{
		{"go file passes", "main.go", true},
		{"log file excluded", "app.log", false},
		{"tmp file excluded", "data.tmp", false},
		{"log in subdir excluded", "/var/log/app.log", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.matched, f.Match(tt.path))
		})
	}
}

func TestPathFilter_Match_IncludesAndExcludes(t *testing.T) {
	f := NewPathFilter(
		[]string{"*.go"},
		[]string{"*_test.go"},
	)

	tests := []struct {
		name    string
		path    string
		matched bool
	}{
		{"source file", "handler.go", true},
		{"test file excluded", "handler_test.go", false},
		{"non-go file", "style.css", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.matched, f.Match(tt.path))
		})
	}
}

func TestPathFilter_Match_ExcludeTakesPriority(t *testing.T) {
	// Include everything, but exclude .env files.
	f := NewPathFilter(
		[]string{"*"},
		[]string{".env"},
	)

	assert.True(t, f.Match("config.json"))
	assert.False(t, f.Match(".env"))
}

func TestPathFilter_Match_GlobCharacters(t *testing.T) {
	f := NewPathFilter(
		[]string{"data-[0-9]*"},
		nil,
	)

	assert.True(t, f.Match("data-1.csv"))
	assert.True(t, f.Match("data-99.csv"))
	assert.False(t, f.Match("data-abc.csv"))
}

func TestPathFilter_Match_QuestionMarkGlob(t *testing.T) {
	f := NewPathFilter(
		[]string{"file?.txt"},
		nil,
	)

	assert.True(t, f.Match("file1.txt"))
	assert.True(t, f.Match("fileA.txt"))
	assert.False(t, f.Match("file12.txt"))
	assert.False(t, f.Match("file.txt"))
}

func TestPathFilter_Match_BasenameMatching(t *testing.T) {
	// Patterns without path separators should match the basename
	// of a full path.
	f := NewPathFilter(
		[]string{"*.json"},
		[]string{"secret.json"},
	)

	assert.True(t, f.Match("/etc/config/app.json"))
	assert.False(t, f.Match("/etc/config/secret.json"))
	assert.True(t, f.Match("package.json"))
}

func TestPathFilter_Match_MultipleIncludes(t *testing.T) {
	f := NewPathFilter(
		[]string{"*.go", "*.rs", "*.ts"},
		nil,
	)

	assert.True(t, f.Match("main.go"))
	assert.True(t, f.Match("lib.rs"))
	assert.True(t, f.Match("index.ts"))
	assert.False(t, f.Match("style.css"))
}

func TestPathFilter_Match_MultipleExcludes(t *testing.T) {
	f := NewPathFilter(
		nil,
		[]string{"*.log", "*.tmp", ".DS_Store"},
	)

	assert.True(t, f.Match("main.go"))
	assert.False(t, f.Match("debug.log"))
	assert.False(t, f.Match("swap.tmp"))
	assert.False(t, f.Match(".DS_Store"))
}

// --- Accessor defensive copy tests -----------------------------------------

func TestPathFilter_Includes_DefensiveCopy(t *testing.T) {
	f := NewPathFilter([]string{"*.go"}, nil)
	includes := f.Includes()
	includes[0] = "*.hacked"
	assert.Equal(t, "*.go", f.Includes()[0],
		"modifying returned slice must not affect filter")
}

func TestPathFilter_Excludes_DefensiveCopy(t *testing.T) {
	f := NewPathFilter(nil, []string{"*.log"})
	excludes := f.Excludes()
	excludes[0] = "*.hacked"
	assert.Equal(t, "*.log", f.Excludes()[0],
		"modifying returned slice must not affect filter")
}
