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

// TestRewriteMarkdownLinks tests the link rewriting functionality
func TestRewriteMarkdownLinks(t *testing.T) {
	// Build a sample file metadata map
	fileMeta := map[string]FileMeta{
		"guides/guides-templating.md": {
			SourcePath: "guides-templating.md",
			DestPath:   "guides/guides-templating.md",
		},
		"provider/google/provider-google-secrets-manager.md": {
			SourcePath: "provider-google-secrets-manager.md",
			DestPath:   "provider/google/provider-google-secrets-manager.md",
		},
		"api-overview.md": {
			SourcePath: "api-overview.md",
			DestPath:   "api-overview.md",
		},
		"guides/guides-common-k8s-secret-types.md": {
			SourcePath: "guides-common-k8s-secret-types.md",
			DestPath:   "guides/guides-common-k8s-secret-types.md",
		},
	}

	filenameToDestMap := buildFilenameToDestMap(fileMeta)

	tests := []struct {
		name            string
		content         string
		currentDestPath string
		expectedLink    string
		description     string
	}{
		{
			name: "Same directory link",
			content: `Check the [templating guide](guides-templating.md) for details.`,
			currentDestPath: "guides/guides-common-k8s-secret-types.md",
			expectedLink: "[templating guide](guides-templating.md)",
			description: "Link to file in same directory should be relative",
		},
		{
			name: "Cross-directory link",
			content: `See [Google Secrets Manager](provider-google-secrets-manager.md) guide.`,
			currentDestPath: "guides/guides-common-k8s-secret-types.md",
			expectedLink: "[Google Secrets Manager](../provider/google/provider-google-secrets-manager.md)",
			description: "Link from guides/ to provider/google/ should go up and down",
		},
		{
			name: "Link to root from subdirectory",
			content: `Read the [API overview](api-overview.md) first.`,
			currentDestPath: "guides/guides-templating.md",
			expectedLink: "[API overview](../api-overview.md)",
			description: "Link from guides/ to root should go up one level",
		},
		{
			name: "Link with fragment",
			content: `See [section](guides-templating.md#advanced) for details.`,
			currentDestPath: "guides/guides-common-k8s-secret-types.md",
			expectedLink: "[section](guides-templating.md#advanced)",
			description: "Links with fragments should preserve the fragment",
		},
		{
			name: "External link unchanged",
			content: `Visit [External](https://example.com/page.md) site.`,
			currentDestPath: "guides/guides-templating.md",
			expectedLink: "[External](https://example.com/page.md)",
			description: "External links should not be modified",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := rewriteMarkdownLinks(tt.content, tt.currentDestPath, filenameToDestMap)
			if !contains(result, tt.expectedLink) {
				t.Errorf("%s\nExpected link: %s\nGot result: %s", tt.description, tt.expectedLink, result)
			}
		})
	}
}

// TestBuildFilenameToDestMap tests the filename mapping function
func TestBuildFilenameToDestMap(t *testing.T) {
	fileMeta := map[string]FileMeta{
		"guides/guides-templating.md": {
			SourcePath: "guides-templating.md",
			DestPath:   "guides/guides-templating.md",
		},
		"provider/google/provider-google-secrets-manager.md": {
			SourcePath: "provider-google-secrets-manager.md",
			DestPath:   "provider/google/provider-google-secrets-manager.md",
		},
	}

	filenameMap := buildFilenameToDestMap(fileMeta)

	// Check that source filenames map to destination paths
	if dest, ok := filenameMap["guides-templating.md"]; !ok || dest != "guides/guides-templating.md" {
		t.Errorf("Expected guides-templating.md to map to guides/guides-templating.md, got %s", dest)
	}

	if dest, ok := filenameMap["provider-google-secrets-manager.md"]; !ok || dest != "provider/google/provider-google-secrets-manager.md" {
		t.Errorf("Expected provider-google-secrets-manager.md to map to provider/google/provider-google-secrets-manager.md, got %s", dest)
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
