// Package skills provides a skill system for BugBuster Code.
// Skills are reusable sets of instructions that guide the model through
// complex multi-step tasks like debugging, refactoring, code review, etc.
//
// Skills are stored as Markdown files in:
//   - <project>/.bugbuster/skills/*.md  (project-specific, priority)
//   - ~/.bugbuster/skills/*.md          (global, fallback)
//   - Built-in skills                   (embedded in binary)
package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Skill represents a reusable instruction set for the model.
type Skill struct {
	Name        string // Unique identifier (e.g., "debug", "refactor")
	Description string // Short description for /skills listing
	Content     string // Full Markdown content (instructions)
	Source      string // Where the skill comes from: "builtin", "project", "global"
}

// Manager manages available skills.
type Manager struct {
	mu     sync.RWMutex
	skills map[string]*Skill
}

// NewManager creates a new skill manager.
func NewManager() *Manager {
	return &Manager{
		skills: make(map[string]*Skill),
	}
}

// LoadBuiltins registers built-in skills.
func (m *Manager) LoadBuiltins() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, content := range builtinSkills {
		parts := strings.SplitN(content, "\n", 3)
		description := strings.TrimPrefix(parts[0], "# ")
		if description == content {
			description = name
		}
		m.skills[name] = &Skill{
			Name:        name,
			Description: description,
			Content:     content,
			Source:      "builtin",
		}
	}
}

// LoadFromDir loads skills from a directory of .md files.
func (m *Manager) LoadFromDir(dir string, source string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read skills dir %s: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		name := strings.TrimSuffix(entry.Name(), ".md")
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}

		content := string(data)
		description := extractDescription(content)

		m.skills[name] = &Skill{
			Name:        name,
			Description: description,
			Content:     content,
			Source:      source,
		}
	}

	return nil
}

// Get returns a skill by name.
func (m *Manager) Get(name string) (*Skill, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.skills[name]
	if !ok {
		return nil, false
	}
	return s, true
}

// List returns all available skills sorted by name.
func (m *Manager) List() []*Skill {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Skill, 0, len(m.skills))
	for _, s := range m.skills {
		result = append(result, s)
	}
	sort.Slice(result, func(i, j int) bool {
		// Builtins first, then by name
		if result[i].Source != result[j].Source {
			return result[i].Source == "builtin"
		}
		return result[i].Name < result[j].Name
	})
	return result
}

// Activate returns the skill content formatted for injection into the system prompt.
func (m *Manager) Activate(name string) (string, error) {
	s, ok := m.Get(name)
	if !ok {
		available := make([]string, 0)
		for _, s := range m.List() {
			available = append(available, s.Name)
		}
		return "", fmt.Errorf("skill '%s' not found. Available: %s", name, strings.Join(available, ", "))
	}
	return fmt.Sprintf("## Active Skill: %s\n\n%s", s.Name, s.Content), nil
}

// extractDescription gets the first line after # heading from markdown.
func extractDescription(content string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "# ") {
			// Skip the title, look for description on next non-empty line
			continue
		}
		// First non-heading, non-empty line is the description
		return line
	}
	return ""
}

// --- Built-in Skills ---

var builtinSkills = map[string]string{
	"debug": `# Debug

Systematically find and fix bugs in code.
1. Read the error message or symptom carefully
2. Identify the exact file and line where the error occurs
3. Read the surrounding code context (at least 20 lines above and below)
4. Trace the data flow — where does the problematic value come from?
5. Form a hypothesis about the root cause
6. Write a minimal test that reproduces the bug
7. Implement the fix — change only what's necessary
8. Run the test to confirm the fix works
9. Run the full test suite to check for regressions
10. If the fix doesn't work, go back to step 4 with a new hypothesis

Important: Never guess — always read the code first. Never change multiple things at once.
`,

	"refactor": `# Refactor

Safely restructure code without changing behavior.
1. Identify what needs refactoring and why (duplication, complexity, naming)
2. Find ALL usages of the code to be refactored (grep, references, LSP)
3. Build a dependency graph — what depends on this code?
4. Write or verify existing tests cover the current behavior
5. Run tests to establish a green baseline
6. Make ONE small change at a time
7. Run tests after each change
8. If tests fail — revert immediately and reconsider the approach
9. After all changes — run the full test suite
10. Clean up: remove dead code, update comments

Important: Refactoring must never change external behavior. If behavior changes, it's not a refactor — it's a rewrite.
`,

	"review": `# Review

Thoroughly review code changes for quality, security, and correctness.
1. Read the full diff or changed files
2. Check correctness — does the code do what it claims?
3. Check error handling — are all errors handled properly?
4. Check security — SQL injection, XSS, path traversal, secret exposure
5. Check performance — unnecessary allocations, N+1 queries, missing indexes
6. Check style — naming conventions, code organization, consistency
7. Check tests — are new behaviors tested? Are edge cases covered?
8. Check documentation — are public APIs documented?
9. Write a structured review with: issues (must fix), suggestions (should fix), notes (FYI)
10. If critical issues found — do not approve, explain what needs to change

Important: Be constructive. Every criticism should include a suggestion for improvement.
`,

	"architect": `# Architect

Design and plan architecture before writing code.
1. Understand the requirements — what problem are we solving?
2. Identify constraints — performance, compatibility, existing code
3. List the main components and their responsibilities
4. Define interfaces between components
5. Draw the data flow — where does data come from, where does it go?
6. Consider error paths — what happens when things fail?
7. Consider scaling — will this design work at 10x scale?
8. Write the plan as a document with: goals, components, interfaces, data flow, trade-offs
9. Identify risks and mitigation strategies
10. Get feedback before implementing

Important: No code yet. The output of this skill is a plan, not an implementation.
`,

	"test": `# Test

Write comprehensive tests for existing code.
1. Read the code to be tested carefully
2. Identify all public functions/methods
3. For each function, list: normal cases, edge cases, error cases
4. Write tests for normal cases first (happy path)
5. Write tests for edge cases (empty input, nil, zero, max)
6. Write tests for error cases (invalid input, missing dependencies)
7. Run tests and verify they pass
8. Check coverage — are all branches tested?
9. Add regression tests for any bugs found during testing
10. Clean up test code — helpers, table-driven tests, clear names

Important: Tests should be independent and deterministic. No test should depend on another test's side effects.
`,

	"migrate": `# Migrate

Safely migrate code, data, or dependencies to a new version.
1. Understand what's changing — read the migration guide or changelog
2. Identify all affected code — grep for old API usage
3. Create a migration checklist — list every change needed
4. Make a backup or commit current state
5. Migrate ONE component at a time
6. Run tests after each migration step
7. Update configuration files if needed
8. Update documentation
9. Run full test suite
10. Clean up — remove deprecated workarounds, old dependencies

Important: Never migrate and refactor at the same time. One change at a time.
`,
}