package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCopySnippetsFolder_WithMarkdownFiles tests that .md files in snippets are detected and not copied
func TestCopySnippetsFolder_WithMarkdownFiles(t *testing.T) {
	// Create temporary directories
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	srcSnippets := filepath.Join(srcDir, "snippets")
	dstSnippets := filepath.Join(tmpDir, "dst", "snippets")

	// Create source structure
	if err := os.MkdirAll(srcSnippets, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a markdown file and a yaml file in snippets
	mdContent := `# Test Markdown
This is a test markdown file in snippets.`
	yamlContent := `apiVersion: v1
kind: Secret`

	if err := os.WriteFile(filepath.Join(srcSnippets, "test.md"), []byte(mdContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcSnippets, "test.yaml"), []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Run copySnippetsFolder
	var migrationRecords []MigrationRecord
	found, markdownFiles := copySnippetsFolder(srcDir, dstSnippets, &migrationRecords)

	// Assertions
	if !found {
		t.Error("Expected snippets folder to be found")
	}

	if len(markdownFiles) != 1 {
		t.Errorf("Expected 1 markdown file, got %d", len(markdownFiles))
	}

	if len(markdownFiles) > 0 && markdownFiles[0] != "test.md" {
		t.Errorf("Expected markdown file 'test.md', got '%s'", markdownFiles[0])
	}

	// Check that .yaml was copied but .md was not
	if _, err := os.Stat(filepath.Join(dstSnippets, "test.yaml")); os.IsNotExist(err) {
		t.Error("Expected test.yaml to be copied to destination")
	}

	if _, err := os.Stat(filepath.Join(dstSnippets, "test.md")); !os.IsNotExist(err) {
		t.Error("Expected test.md NOT to be copied to destination")
	}
}

// TestReplaceYamlIncludes_WithMarkdownSnippet tests that .md snippets are converted to links
func TestReplaceYamlIncludes_WithMarkdownSnippet(t *testing.T) {
	snippetToContentMap := map[string]string{
		"provider-aws-access.md": "provider-aws-access-auth.md",
	}

	content := `Some text before.
{% include "provider-aws-access.md" %}
Some text after.`

	tmpDir := t.TempDir()
	var missingSnippets []MissingSnippet

	result := replaceYamlIncludes(content, tmpDir, &missingSnippets, "test.md", snippetToContentMap)

	// Check that the include was replaced with a link
	if !contains(result, "[Provider Aws Access Auth](../provider-aws-access-auth/)") {
		t.Errorf("Expected link to be generated, got: %s", result)
	}

	// Check that the original include is not present
	if contains(result, "{% include") {
		t.Error("Expected include directive to be replaced")
	}
}

// TestReplaceYamlIncludes_WithYamlSnippet tests that .yaml snippets are converted to readfile
func TestReplaceYamlIncludes_WithYamlSnippet(t *testing.T) {
	snippetToContentMap := map[string]string{
		"provider-aws-access.md": "provider-aws-access-auth.md",
	}

	content := `Some text before.
{% include "test-snippet.yaml" %}
Some text after.`

	tmpDir := t.TempDir()
	snippetPath := filepath.Join(tmpDir, "test-snippet.yaml")
	if err := os.WriteFile(snippetPath, []byte("test: yaml"), 0644); err != nil {
		t.Fatal(err)
	}

	var missingSnippets []MissingSnippet

	result := replaceYamlIncludes(content, tmpDir, &missingSnippets, "test.md", snippetToContentMap)

	// Check that the include was replaced with readfile shortcode
	if !contains(result, "{{< readfile file=snippets/test-snippet.yaml") {
		t.Errorf("Expected readfile shortcode to be generated, got: %s", result)
	}

	// Check that the original include is not present
	if contains(result, "{% include") {
		t.Error("Expected include directive to be replaced")
	}
}

// TestDeriveTitleFromFilename tests the title derivation function
func TestDeriveTitleFromFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"provider-aws-access.md", "Provider Aws Access"},
		{"test_file.md", "Test File"},
		{"simple.md", "Simple"},
		{"path/to/file-name.md", "File Name"},
	}

	for _, tt := range tests {
		result := deriveTitleFromFilename(tt.input)
		if result != tt.expected {
			t.Errorf("deriveTitleFromFilename(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
