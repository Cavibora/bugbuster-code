package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPermanentSaveWithSameValueDifferentCategory verifies that a permanent fact
// cannot have its category downgraded to non-permanent by saving with the same key+value
// but a different category.
func TestPermanentSaveWithSameValueDifferentCategory(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "memory.md")
	tool := NewMemoryToolWithPath(fp)

	tool.Execute(map[string]string{
		"action": "save", "key": "db_host", "value": "localhost:3306", "category": "permanent",
	})
	tool.Execute(map[string]string{
		"action": "save", "key": "db_host", "value": "localhost:3306", "category": "general",
	})

	result := tool.Execute(map[string]string{"action": "load", "key": "db_host"})
	if !strings.Contains(result.Output, "localhost:3306") {
		t.Fatalf("value should be preserved, got: %s", result.Output)
	}

	data, _ := os.ReadFile(fp)
	content := string(data)
	if !strings.Contains(content, "## permanent") {
		t.Fatalf("category should still be permanent, got file content:\n%s", content)
	}
}

// TestPermanentSaveWithDifferentValueDifferentCategory verifies that a permanent fact
// cannot be overwritten with a different value+category (downgrade attack).
func TestPermanentSaveWithDifferentValueDifferentCategory(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "memory.md")
	tool := NewMemoryToolWithPath(fp)

	tool.Execute(map[string]string{
		"action": "save", "key": "api_endpoint", "value": "https://api.production.com", "category": "permanent",
	})
	tool.Execute(map[string]string{
		"action": "save", "key": "api_endpoint", "value": "https://api.staging.com", "category": "general",
	})

	result := tool.Execute(map[string]string{"action": "load", "key": "api_endpoint"})
	if !strings.Contains(result.Output, "production.com") {
		t.Fatalf("permanent value should be preserved, got: %s", result.Output)
	}
	if strings.Contains(result.Output, "staging.com") {
		t.Fatalf("permanent value should not be overwritten, got: %s", result.Output)
	}
}

// TestPermanentCompressPreservesAllPermanent verifies that compress with very low max_tokens
// never drops permanent facts.
func TestPermanentCompressPreservesAllPermanent(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "memory.md")
	tool := NewMemoryToolWithPath(fp)

	permanentFacts := []struct{ key, value string }{
		{"alpha_config", "value_alpha_for_testing"},
		{"beta_endpoint", "value_beta_for_testing"},
		{"gamma_secret", "value_gamma_for_testing"},
		{"delta_creds", "value_delta_for_testing"},
		{"epsilon_path", "value_epsilon_for_testing"},
	}
	for _, f := range permanentFacts {
		tool.Execute(map[string]string{
			"action": "save", "key": f.key, "value": f.value, "category": "permanent",
		})
	}

	for i := 0; i < 10; i++ {
		tool.Execute(map[string]string{
			"action":   "save",
			"key":      "tmp_" + string(rune('a'+i)) + "_var",
			"value":    "tmp_val_" + string(rune('a'+i)),
			"category": "general",
		})
	}

	tool.Execute(map[string]string{"action": "compress", "max_tokens": "10"})

	for _, f := range permanentFacts {
		result := tool.Execute(map[string]string{"action": "load", "key": f.key})
		if !strings.Contains(result.Output, f.value) {
			t.Errorf("permanent fact %q should survive compress, got: %s", f.key, result.Output)
		}
	}
}

// TestPermanentCompressPreservesCategory verifies that compress() does not downgrade
// permanent facts to a lower category.
func TestPermanentCompressPreservesCategory(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "memory.md")
	tool := NewMemoryToolWithPath(fp)

	tool.Execute(map[string]string{
		"action": "save", "key": "critical_config", "value": "DO_NOT_CHANGE", "category": "permanent",
	})

	tool.Execute(map[string]string{"action": "compress"})

	tool.Execute(map[string]string{"action": "save", "key": "temp_key", "value": "temp", "category": "general"})
	tool.Execute(map[string]string{"action": "delete", "key": "temp_key"})

	data, _ := os.ReadFile(fp)
	content := string(data)
	if !strings.Contains(content, "## permanent") {
		t.Fatalf("permanent category should be preserved after compress, got:\n%s", content)
	}

	loadResult := tool.Execute(map[string]string{"action": "load", "key": "critical_config"})
	if !strings.Contains(loadResult.Output, "DO_NOT_CHANGE") {
		t.Fatalf("permanent value should be intact after compress, got: %s", loadResult.Output)
	}
}

// TestPermanentCriticalCategoryAlias verifies that "critical" category is also protected.
func TestPermanentCriticalCategoryAlias(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "memory.md")
	tool := NewMemoryToolWithPath(fp)

	tool.Execute(map[string]string{
		"action": "save", "key": "secret_key", "value": "super_secret_value_123", "category": "critical",
	})

	result := tool.Execute(map[string]string{"action": "delete", "key": "secret_key"})
	if strings.Contains(result.Output, "Deleted") {
		t.Fatal("critical fact should not be deletable")
	}

	tool.Execute(map[string]string{
		"action": "save", "key": "secret_key", "value": "overwritten_value", "category": "general",
	})

	loadResult := tool.Execute(map[string]string{"action": "load", "key": "secret_key"})
	if !strings.Contains(loadResult.Output, "super_secret_value_123") {
		t.Fatalf("critical fact should preserve original value, got: %s", loadResult.Output)
	}
}

// TestPermanentCompressDedupPrefersPermanent verifies that when compress encounters
// duplicate values, it keeps the permanent version over the non-permanent one.
func TestPermanentCompressDedupPrefersPermanent(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "memory.md")
	tool := NewMemoryToolWithPath(fp)

	// Save permanent fact first
	tool.Execute(map[string]string{
		"action": "save", "key": "perm_host", "value": "localhost:5432", "category": "permanent",
	})

	// Try to save same value under different key — save() will refuse to overwrite permanent
	tool.Execute(map[string]string{
		"action": "save", "key": "temp_host", "value": "localhost:5432", "category": "general",
	})

	// Compress
	tool.Execute(map[string]string{"action": "compress", "max_tokens": "1000"})

	// Permanent fact should survive
	permResult := tool.Execute(map[string]string{"action": "load", "key": "perm_host"})
	if !strings.Contains(permResult.Output, "localhost:5432") {
		t.Fatalf("permanent fact should survive compress dedup, got: %s", permResult.Output)
	}
}

// TestPermanentDeleteAllCannotRemovePermanent verifies that compress with very aggressive settings
// never removes permanent facts.
func TestPermanentDeleteAllCannotRemovePermanent(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "memory.md")
	tool := NewMemoryToolWithPath(fp)

	tool.Execute(map[string]string{
		"action": "save", "key": "alpha_secret", "value": "value_alpha", "category": "permanent",
	})
	tool.Execute(map[string]string{
		"action": "save", "key": "beta_token", "value": "value_beta", "category": "permanent",
	})

	tool.Execute(map[string]string{"action": "compress", "max_tokens": "1"})

	r1 := tool.Execute(map[string]string{"action": "load", "key": "alpha_secret"})
	if !strings.Contains(r1.Output, "value_alpha") {
		t.Errorf("alpha_secret should survive aggressive compress, got: %s", r1.Output)
	}
	r2 := tool.Execute(map[string]string{"action": "load", "key": "beta_token"})
	if !strings.Contains(r2.Output, "value_beta") {
		t.Errorf("beta_token should survive aggressive compress, got: %s", r2.Output)
	}
}

// TestPermanentSaveUpgradeNonPermanentToPermanent verifies that saving with category=permanent
// on an existing non-permanent fact correctly upgrades it.
func TestPermanentSaveUpgradeNonPermanentToPermanent(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "memory.md")
	tool := NewMemoryToolWithPath(fp)

	tool.Execute(map[string]string{
		"action": "save", "key": "config_path", "value": "/etc/myapp/config.yaml", "category": "general",
	})
	tool.Execute(map[string]string{
		"action": "save", "key": "config_path", "value": "/etc/myapp/config.yaml", "category": "permanent",
	})

	delResult := tool.Execute(map[string]string{"action": "delete", "key": "config_path"})
	if strings.Contains(delResult.Output, "Deleted") {
		t.Fatal("upgraded fact should be permanent and not deletable")
	}

	loadResult := tool.Execute(map[string]string{"action": "load", "key": "config_path"})
	if !strings.Contains(loadResult.Output, "/etc/myapp/config.yaml") {
		t.Fatalf("value should be preserved after upgrade, got: %s", loadResult.Output)
	}
}

// TestPermanentCannotBeOverwrittenEvenWithPermanent verifies that a permanent fact
// cannot be overwritten even when saving with category=permanent (different value).
func TestPermanentCannotBeOverwrittenEvenWithPermanent(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "memory.md")
	tool := NewMemoryToolWithPath(fp)

	tool.Execute(map[string]string{
		"action": "save", "key": "api_key", "value": "original_key_123", "category": "permanent",
	})
	tool.Execute(map[string]string{
		"action": "save", "key": "api_key", "value": "malicious_key_456", "category": "permanent",
	})

	loadResult := tool.Execute(map[string]string{"action": "load", "key": "api_key"})
	if !strings.Contains(loadResult.Output, "original_key_123") {
		t.Fatalf("permanent fact should not be overwritten even with permanent category, got: %s", loadResult.Output)
	}
	if strings.Contains(loadResult.Output, "malicious_key_456") {
		t.Fatalf("permanent fact should not be overwritten, got: %s", loadResult.Output)
	}
}

// TestPermanentMassDeleteProtection verifies that compress doesn't accidentally
// remove permanent facts when doing mass cleanup.
func TestPermanentMassDeleteProtection(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "memory.md")
	tool := NewMemoryToolWithPath(fp)

	permKeys := []string{"alpha_cfg", "beta_ep", "gamma_sec", "delta_cr", "epsilon_pt"}
	permVals := []string{"val_A", "val_B", "val_C", "val_D", "val_E"}
	for i := 0; i < 5; i++ {
		tool.Execute(map[string]string{
			"action":   "save",
			"key":      permKeys[i],
			"value":    permVals[i],
			"category": "permanent",
		})
	}
	for i := 0; i < 20; i++ {
		tool.Execute(map[string]string{
			"action":   "save",
			"key":      "tmp_" + string(rune('a'+i)) + "_var_" + string(rune('a'+i)),
			"value":    "tmp_" + string(rune('a'+i)),
			"category": "general",
		})
	}

	tool.Execute(map[string]string{"action": "compress", "max_tokens": "5"})

	for i := 0; i < 5; i++ {
		result := tool.Execute(map[string]string{"action": "load", "key": permKeys[i]})
		if !strings.Contains(result.Output, permVals[i]) {
			t.Errorf("permanent fact %q should survive mass compress, got: %s", permKeys[i], result.Output)
		}
	}
}

// TestPermanentCategoryNotDowngradedOnSameValueSave is the core test for the
// vulnerability where model saves with same key+value but different category
// to strip the permanent protection.
func TestPermanentCategoryNotDowngradedOnSameValueSave(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "memory.md")
	tool := NewMemoryToolWithPath(fp)

	tool.Execute(map[string]string{
		"action": "save", "key": "db_url", "value": "postgres://localhost:5432", "category": "permanent",
	})

	// Attack: save with same key, same value, but category="general"
	tool.Execute(map[string]string{
		"action": "save", "key": "db_url", "value": "postgres://localhost:5432", "category": "general",
	})

	// Now try to delete — should still be protected
	delResult := tool.Execute(map[string]string{"action": "delete", "key": "db_url"})
	if strings.Contains(delResult.Output, "Deleted") {
		t.Fatal("permanent fact should still be protected after same-value category downgrade attempt")
	}

	data, _ := os.ReadFile(fp)
	content := string(data)
	if !strings.Contains(content, "## permanent") {
		t.Fatalf("file should still have permanent category after downgrade attempt:\n%s", content)
	}
}

// TestPermanentFileTamperingSimulation verifies that even if the memory file is
// manually edited to remove the permanent category, the system handles it gracefully.
func TestPermanentFileTamperingSimulation(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "memory.md")
	tool := NewMemoryToolWithPath(fp)

	tool.Execute(map[string]string{
		"action": "save", "key": "protected_val", "value": "must_not_change", "category": "permanent",
	})

	// Simulate file tampering: rewrite file with category changed to "general"
	tampered := "# BugBuster Agent Memory\n\n## general\n- **protected_val**: must_not_change\n"
	os.WriteFile(fp, []byte(tampered), 0644)

	tool2 := NewMemoryToolWithPath(fp)

	result := tool2.Execute(map[string]string{"action": "load", "key": "protected_val"})
	if !strings.Contains(result.Output, "must_not_change") {
		t.Fatalf("fact should still be readable after tampering, got: %s", result.Output)
	}
}