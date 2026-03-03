package main

// Converts an MkDocs site into Hugo content files with TOML front matter.

/*
Usage (after `go mod tidy`):
  go run ./scripts/converter \
    --mkdocsfile path/to/mkdocs.yml \
    --src path/to/markdown/files/ \
    --dst path/to/destination/ \
    --src-assets-folder path/to/images \
    --src-assets-folder path/to/assets \
    --dst-assets-folder path/to/static/
*/

/*
REQUIREMENTS:
-------------
1. Parse mkdocs.yml navigation structure to extract file hierarchy (nav)
2. Construct a directory structure from mkdocs hierarchy in the destination (dst) folder. Do not flatten hierarchy.
For each of those directories, generate each _index.md files with a TOML front matter (+++...+++) including:
- title: From nav display name (this is taken from Section name from mkdocs.yml)
- linkTitle: Same as title
- weight: Position in nav (increments by 10, first sections have lower weight)
3. Extract static assets:
- Find assets in the multiple --src-assets-folder
- Copy them in dst-assets-folder
4. Extract and cleanup static snippets:
- Find all snippets files in the "snippets" subfolder of the src folder.
- If they contain includes, clean them before pasting them in dst/snippets folder (relative to destination).
	* Replace include blocks with Hugo readfile shortcode:
		* Format: {{< readfile file=snippets/filename code="true" lang="yaml" >}}
		* MkDocs snippets in source: --8<-- "file"
		* Jinja2 includes in source : {% include "file" %}
        * IMPORTANT: Snippets are copied to <dst>/snippets/ folder and referenced with relative paths
	* Validate that the file in the readfile "include" exists in snippets folder. Report missing snippet references at end of run if the file does not exist.

5a. Extract and cleanup markdown source files:
- Find if they are referenced in mkdocs.yml nav: use their nav position for weight
- For all files not referenced in mkdocs.yml nav: assign higher weights (=after matched files)

5b. Each of those files should be cleaned for includes before being pasted in their destination folder (based on hierarchy):
- Strip existing YAML/TOML front matter (including hide_toc metadata)
- Remove <br> and <br /> tags
- Remove markdown style attributes (e.g., {: style="width:70%;"})
- Remove Jinja2 raw/endraw tags (all variations: {% raw %}, {%- raw %}, etc.)
- Rewrite all asset paths:
	* Rewrite relative paths (../pictures/...) to absolute (/img/...)
	* Handle URL-encoded filenames (e.g., %20 for spaces):
		* Decode to find actual file
		* Keep encoding in output path
	* For urls pointing to https://external-secrets.io/, figure out if the full link contains a link to an asset/snippet or other content. For assets, replace with the absolute asset path. If it's a snippet, replace with the /snippets/ path. If uncertain, leave the full link as is, and report it at the end.
- Rewrite/Cleanup any reference to an include/snippet (cleanup like in step 4).

5c. Each of those files should have MkDocs admonitions converted to Hugo/Docsy GFM alerts:
- Transform !!! syntax to blockquote-based alerts (> [!TYPE])
- Type mapping: note→NOTE, warning→WARNING, danger→DANGER, tip→TIP, important→WARNING, info→NOTE
- Preserve titles and multi-line content
- Ignore modifiers like "inline end"
- Maintain blank line between alert header and content

6. Guard against failures:
- Handle missing assets gracefully (warn but continue)
- Catch errors in asset rewriting
7. Write an output summary report:
- Total files processed (split by matched/unmatched)
- Markdown migration reporter: For all migrated files (TAB SEPARATED): source file (assets, source, snippet files), whether it was found in mkdocs.nav (true or false), file destination path , file destination weight (if markdown file, empty otherwise)
- List of missing snippet references
*/

import (
	_ "embed"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Admonition mapping
var admonitionTypeMapping = map[string]string{
	"note":      "NOTE",
	"warning":   "WARNING",
	"danger":    "DANGER",
	"tip":       "TIP",
	"important": "WARNING",
	"info":      "NOTE",
}

// Compiled regexes
var (
	reYAMLFrontMatter = regexp.MustCompile(`(?s)\A---\s*.*?\n---\s*`)
	reTOMLFrontMatter = regexp.MustCompile(`(?s)\A\+\+\+\s*.*?\n\+\+\+\s*`)
	reBRTags          = regexp.MustCompile(`(?i)<br\s*/?>`)
	reStyleAttrs      = regexp.MustCompile(`\{:\s*style="[^"]*"\s*\}`)
	reJinjaRaw        = regexp.MustCompile(`\{%-?\s*raw\s*-?%\}`)
	reJinjaEndRaw     = regexp.MustCompile(`\{%-?\s*endraw\s*-?%\}`)
	reMarkdownImage   = regexp.MustCompile(`!\[([^\]]*)\]\(([^)]+)\)`)
	reImgSrcDouble    = regexp.MustCompile(`<img\s+[^>]*src="([^"]+)"`)
	reImgSrcSingle    = regexp.MustCompile(`<img\s+[^>]*src='([^']+)'`)
	// Admonition pattern similar to the Python version.
	reAdmonition = regexp.MustCompile(`(?m)^!!! +(\w+)(?: +"([^"]+)")?(?: +'([^']+)')?(?:[^\n]*)?\n((?:(?: {4}.*$|^\s*$)\n?)*)`)
	// Include patterns
	reYamlIncludeBlock = regexp.MustCompile("(?s)```\\s*yaml\\s*\\n\\s*{\\%\\s*include\\s+[\"']([^\"']+)[\"']\\s*\\%}[^\\n]*\\n\\s*```")
	reIncludeInline    = regexp.MustCompile(`\{\%\s*include\s+["']([^"']+)["']\s*\%\}`)
	reMkdocsSnippet    = regexp.MustCompile(`--8<--\s+"([^"]+)"`)
	// External-secrets.io URL pattern
	reExternalSecretsURL = regexp.MustCompile(`https://external-secrets\.io/[^\s\)]+`)
)

// Structures

type FileMeta struct {
	Title       string
	Section     string
	Subsection  string
	Weight      int
	SourcePath  string // Original path from mkdocs.yml (for reading source file)
	DestPath    string // Destination path with section hierarchy (for writing output)
}

type SectionMeta struct {
	Title  string
	Weight int
}

type MissingSnippet struct {
	Snippet      string
	ReferencedIn string
}

type MigrationRecord struct {
	SourceFile  string
	InMkdocsNav bool
	DestPath    string
	Weight      string // string to handle empty for non-markdown files
}

type ExternalURLReport struct {
	URL          string
	MarkdownFile string
	Action       string // "rewritten_asset", "rewritten_snippet", "kept_as_is"
	Reason       string
}

type arrayFlags []string

func (a *arrayFlags) String() string {
	return strings.Join(*a, ", ")
}

func (a *arrayFlags) Set(value string) error {
	*a = append(*a, value)
	return nil
}

func main() {
	// Flags
	var srcAssetFolders arrayFlags
	mkdocsFile := flag.String("mkdocsfile", "", "Path to mkdocs.yml file")
	srcDir := flag.String("src", "", "Path to directory containing markdown files")
	dstDir := flag.String("dst", "", "Destination directory for converted files")
	flag.Var(&srcAssetFolders, "src-assets-folder", "Path to assets folder (can be specified multiple times)")
	dstAssetsFolder := flag.String("dst-assets-folder", "", "Destination folder for assets (e.g., ./static/)")
	flag.Parse()

	// Validate required
	if *mkdocsFile == "" || *srcDir == "" || *dstDir == "" || len(srcAssetFolders) == 0 || *dstAssetsFolder == "" {
		fmt.Fprintln(os.Stderr, "Error: all flags --mkdocsfile, --src, --dst, --src-assets-folder, --dst-assets-folder are required")
		os.Exit(1)
	}

	// Validate paths
	if !exists(*mkdocsFile) {
		fmt.Fprintf(os.Stderr, "Error: Config file not found: %s\n", *mkdocsFile)
		os.Exit(1)
	}
	if !isDir(*srcDir) {
		fmt.Fprintf(os.Stderr, "Error: Source directory not found: %s\n", *srcDir)
		os.Exit(1)
	}
	for _, assetFolder := range srcAssetFolders {
		if !isDir(assetFolder) {
			fmt.Fprintf(os.Stderr, "Error: Assets folder not found: %s\n", assetFolder)
			os.Exit(1)
		}
	}

	// Ensure destination exists
	if err := os.MkdirAll(*dstDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating dest dir: %v\n", err)
		os.Exit(1)
	}

	// Ensure dst assets folder exists
	if err := os.MkdirAll(*dstAssetsFolder, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating dst assets folder: %v\n", err)
		os.Exit(1)
	}

	// Initialize tracking structures
	migrationRecords := []MigrationRecord{}
	externalURLsReported := []ExternalURLReport{}
	copiedAssets := map[string]string{} // decoded asset filename -> destination path

	fmt.Println("Checking for snippets folder...")
	dstSnippetsFolder := filepath.Join(*dstDir, "snippets")
	snippetsFound, snippetMarkdownFiles := copySnippetsFolder(*srcDir, dstSnippetsFolder, &migrationRecords)
	if snippetsFound {
		fmt.Printf("  Copied snippets folder to %s\n", dstSnippetsFolder)
		if len(snippetMarkdownFiles) > 0 {
			fmt.Printf("  Found %d markdown files in snippets to be processed as content files\n", len(snippetMarkdownFiles))
		}
	} else {
		fmt.Println("  No snippets folder found in source directory")
	}

	fmt.Printf("\nParsing %s...\n", *mkdocsFile)
	fileMeta, sectionMeta, err := parseMkdocsNav(*mkdocsFile, *srcDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing mkdocs nav: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Found %d files in navigation\n", len(fileMeta))
	fmt.Printf("Found %d directories to create\n", len(sectionMeta))

	// Scan source dir for all markdown files
	allMd := map[string]bool{}
	err = filepath.WalkDir(*srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			rel, err := filepath.Rel(*srcDir, path)
			if err != nil {
				return err
			}
			// normalize to forward slashes like mkdocs
			rel = filepath.ToSlash(rel)
			allMd[rel] = true
		}
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scanning source markdown files: %v\n", err)
		os.Exit(1)
	}

	// Determine unmatched files
	matchedSet := map[string]bool{}
	for _, meta := range fileMeta {
		matchedSet[meta.SourcePath] = true
	}
	unmatchedList := []string{}
	for f := range allMd {
		if !matchedSet[f] {
			unmatchedList = append(unmatchedList, f)
		}
	}
	sort.Strings(unmatchedList)

	// Find max weight used
	maxWeight := 0
	for _, m := range fileMeta {
		if m.Weight > maxWeight {
			maxWeight = m.Weight
		}
	}

	// Assign weights to unmatched files (start at max+1000 then +10)
	unmatchedWeight := maxWeight + 1000
	for _, u := range unmatchedList {
		unmatchedWeight += 10
		title := deriveTitleFromFilename(u)
		// For unmatched files, SourcePath and DestPath are the same (no reorganization)
		fileMeta[u] = FileMeta{
			Title:      title,
			Section:    "",
			Subsection: "",
			Weight:     unmatchedWeight,
			SourcePath: u,
			DestPath:   u,
		}
	}

	// Track mapping from snippet paths to content paths for link rewriting
	snippetToContentMap := make(map[string]string)

	// Process snippet markdown files as content files
	for _, snippetMd := range snippetMarkdownFiles {
		unmatchedWeight += 10
		// Extract filename without extension for title
		title := deriveTitleFromFilename(snippetMd)
		// Add suffix to distinguish from similarly named content files
		// Create a distinctive filename by adding "-auth" suffix before extension
		baseName := strings.TrimSuffix(filepath.Base(snippetMd), filepath.Ext(snippetMd))
		newFileName := baseName + "-auth.md"

		// Store the file using newFileName as the key (dest path)
		snippetSrcPath := "snippets/" + snippetMd
		fileMeta[newFileName] = FileMeta{
			Title:      title,
			Section:    "",
			Subsection: "",
			Weight:     unmatchedWeight,
			SourcePath: snippetSrcPath,
			DestPath:   newFileName,
		}

		// Track the mapping from snippet path to content filename for link rewriting
		snippetToContentMap[snippetMd] = newFileName
	}

	// Create section _index.md files
	fmt.Println("Creating directory index files...")
	sectionPaths := make([]string, 0, len(sectionMeta))
	for p := range sectionMeta {
		sectionPaths = append(sectionPaths, p)
	}
	sort.Strings(sectionPaths)
	for _, dirPath := range sectionPaths {
		meta := sectionMeta[dirPath]
		fullDir := filepath.Join(*dstDir, filepath.FromSlash(dirPath))
		if err := os.MkdirAll(fullDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating directory %s: %v\n", fullDir, err)
			continue
		}
		createIndexFile(fullDir, meta)
		fmt.Printf("  Created %s/_index.md (title=%q, weight=%d)\n", dirPath, meta.Title, meta.Weight)
	}

	// Missing snippets
	missingSnippets := []MissingSnippet{}

	// Convert files
	fmt.Println("\nConverting markdown files...")
	convertedCount := 0
	convertedMatched := 0
	convertedUnmatched := 0
	errorCount := 0

	// Sort fileMeta keys so output is deterministic
	fileKeys := make([]string, 0, len(fileMeta))
	for k := range fileMeta {
		fileKeys = append(fileKeys, k)
	}
	sort.Strings(fileKeys)

	for _, destPathKey := range fileKeys {
		meta := fileMeta[destPathKey]
		// Use SourcePath for reading (original mkdocs path), DestPath for writing (with hierarchy)
		srcPath := filepath.Join(*srcDir, filepath.FromSlash(meta.SourcePath))
		dstPath := filepath.Join(*dstDir, filepath.FromSlash(meta.DestPath))

		// Check if this is a snippet markdown file that needs special handling
		if strings.HasPrefix(meta.SourcePath, "snippets/") && strings.HasSuffix(strings.ToLower(meta.SourcePath), ".md") {
			snippetRelPath := strings.TrimPrefix(meta.SourcePath, "snippets/")
			if newFileName, ok := snippetToContentMap[snippetRelPath]; ok {
				// Adjust destination path to place it in content root, not snippets folder
				dstPath = filepath.Join(*dstDir, newFileName)
			}
		}

		// Ensure parent dir exists
		if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating parent dir for %s: %v\n", dstPath, err)
			errorCount++
			continue
		}
		// If source file doesn't exist, warn and skip
		if !exists(srcPath) {
			fmt.Fprintf(os.Stderr, "Warning: source file listed in nav not found: %s\n", srcPath)
			errorCount++
			continue
		}
		inMkdocsNav := matchedSet[meta.SourcePath]
		if err := convertFile(srcPath, dstPath, meta, srcAssetFolders, *dstAssetsFolder, &copiedAssets, dstSnippetsFolder, &missingSnippets, &migrationRecords, &externalURLsReported, inMkdocsNav, meta.SourcePath, snippetToContentMap); err != nil {
			fmt.Fprintf(os.Stderr, "Error converting %s: %v\n", meta.SourcePath, err)
			errorCount++
			continue
		}
		convertedCount++
		if inMkdocsNav {
			convertedMatched++
		} else {
			convertedUnmatched++
		}
	}

	// Print summary
	fmt.Println()
	fmt.Println(strings.Repeat("=", 70))
	fmt.Println("CONVERSION SUMMARY")
	fmt.Println(strings.Repeat("=", 70))
	fmt.Printf("Total files processed: %d\n", convertedCount)
	fmt.Printf("  - Files in mkdocs.yml nav: %d\n", convertedMatched)
	fmt.Printf("  - Files NOT in mkdocs.yml nav: %d\n", convertedUnmatched)
	fmt.Printf("Errors: %d\n", errorCount)

	// Sort migration records by source file
	sort.Slice(migrationRecords, func(i, j int) bool {
		return migrationRecords[i].SourceFile < migrationRecords[j].SourceFile
	})

	// Print migration report
	if len(migrationRecords) > 0 {
		fmt.Println()
		fmt.Println(strings.Repeat("-", 70))
		fmt.Println("MIGRATION REPORT (Tab-separated)")
		fmt.Println(strings.Repeat("-", 70))
		fmt.Println("SourceFile\tInMkdocsNav\tDestinationPath\tWeight")
		for _, rec := range migrationRecords {
			fmt.Printf("%s\t%t\t%s\t%s\n", rec.SourceFile, rec.InMkdocsNav, rec.DestPath, rec.Weight)
		}
	}

	if len(missingSnippets) > 0 {
		fmt.Println()
		fmt.Println(strings.Repeat("-", 70))
		fmt.Println("MISSING SNIPPETS")
		fmt.Println(strings.Repeat("-", 70))
		for _, ms := range missingSnippets {
			fmt.Printf("  Snippet '%s' referenced in '%s' not found in %s\n", ms.Snippet, ms.ReferencedIn, dstSnippetsFolder)
		}
	}

	if len(externalURLsReported) > 0 {
		fmt.Println()
		fmt.Println(strings.Repeat("-", 70))
		fmt.Println("EXTERNAL-SECRETS.IO URLs REPORT")
		fmt.Println(strings.Repeat("-", 70))
		fmt.Println("URL\tMarkdownFile\tAction\tReason")
		for _, report := range externalURLsReported {
			fmt.Printf("%s\t%s\t%s\t%s\n", report.URL, report.MarkdownFile, report.Action, report.Reason)
		}
	}

	fmt.Println()
	fmt.Println("Conversion complete!")
}

// Utility helpers

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isDir(path string) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	return fi.IsDir()
}

func deriveTitleFromFilename(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	name = strings.ReplaceAll(name, "-", " ")
	name = strings.ReplaceAll(name, "_", " ")
	return strings.Title(name)
}

// slugify converts a section title to a directory-safe slug
// e.g., "API Types" -> "api-types", "Core Resources" -> "core-resources"
func slugify(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "_", "-")
	// Remove any non-alphanumeric characters except hyphens
	s = regexp.MustCompile(`[^a-z0-9-]+`).ReplaceAllString(s, "")
	// Remove duplicate hyphens
	s = regexp.MustCompile(`-+`).ReplaceAllString(s, "-")
	// Trim hyphens from start/end
	s = strings.Trim(s, "-")
	return s
}

// parseMkdocsNav parses mkdocs.yml, extracts nav and builds file and section metadata.
//
// Behavior: walks nav entries in order, assigns weights incrementally by 10.
// Top-level entries increase the sequence; nested files follow the sequence.
// For simple/messy mkdocs.yml structures this is a best-effort conversion mirroring the Python logic.
func parseMkdocsNav(yamlPath, sourceDir string) (map[string]FileMeta, map[string]SectionMeta, error) {
	data, err := ioutil.ReadFile(yamlPath)
	if err != nil {
		return nil, nil, err
	}

	// Unmarshal whole YAML into a map and find "nav"
	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, nil, fmt.Errorf("YAML unmarshal error: %w", err)
	}

	rawNav, ok := root["nav"]
	if !ok {
		return nil, nil, fmt.Errorf("No 'nav' section found in mkdocs.yml")
	}

	navList, ok := rawNav.([]interface{})
	if !ok {
		// Some mkdocs allow mapping; attempt to decode via Node
		// fallback: try marshal rawNav and unmarshal into []interface{}
		b, err := yaml.Marshal(rawNav)
		if err != nil {
			return nil, nil, fmt.Errorf("Could not interpret nav section")
		}
		if err := yaml.Unmarshal(b, &navList); err != nil {
			return nil, nil, fmt.Errorf("Could not interpret nav as list")
		}
	}

	fileMetadata := map[string]FileMeta{}
	sectionMetadata := map[string]SectionMeta{}

	weight := 0

	// Helper function to process an item which can be:
	// - map[string]interface{} with one key -> either file or subsection
	// - string (filename)
	// sectionPath is the accumulated directory path from nav hierarchy (e.g., "api/core-resources")
	var processList func(items []interface{}, currentSection, currentSubsection, sectionPath string)
	processList = func(items []interface{}, currentSection, currentSubsection, sectionPath string) {
		for _, it := range items {
			switch v := it.(type) {
			case map[string]interface{}:
				// map with single key (typical for mkdocs nav)
				for key, val := range v {
					// If val is string -> file reference: "Title": "path/to/file.md"
					switch vv := val.(type) {
					case string:
						weight += 10
						title := key
						filepathStr := filepath.ToSlash(vv)
						// sometimes val may be quoted filename only; ensures .md
						if strings.HasSuffix(strings.ToLower(filepathStr), ".md") {
							// Check existence in source dir (use original path)
							full := filepath.Join(sourceDir, filepath.FromSlash(filepathStr))
							if exists(full) {
								// Build destination path: if file already has dir structure, use it;
								// otherwise prepend sectionPath
								destPath := filepathStr
								fileDir := path.Dir(filepathStr)
								if fileDir == "." || fileDir == "" {
									// File has no directory in source, so prepend sectionPath
									if sectionPath != "" {
										destPath = path.Join(sectionPath, filepath.Base(filepathStr))
									}
								}

								// Store with destPath as key (this is where it will be written)
								fileMetadata[destPath] = FileMeta{
									Title:      title,
									Section:    currentSection,
									Subsection: currentSubsection,
									Weight:     weight,
									SourcePath: filepathStr, // Original path for reading
									DestPath:   destPath,    // Destination path for writing
								}

								// Register section metadata for destination directory
								dir := path.Dir(destPath)
								if dir != "." && dir != "" {
									if _, ok := sectionMetadata[dir]; !ok {
										sectionMetadata[dir] = SectionMeta{Title: currentSection, Weight: weight}
									}
								}
							}
						}
					case []interface{}:
						// key is a section header (e.g., "Section": [ ... ])
						weight += 10
						sectionTitle := key
						// Build new section path by appending slugified section name
						newSectionPath := sectionPath
						sectionSlug := slugify(sectionTitle)
						if sectionSlug != "" {
							if newSectionPath == "" {
								newSectionPath = sectionSlug
							} else {
								newSectionPath = path.Join(newSectionPath, sectionSlug)
							}
						}
						// Register this section
						if newSectionPath != "" && newSectionPath != sectionPath {
							if _, ok := sectionMetadata[newSectionPath]; !ok {
								sectionMetadata[newSectionPath] = SectionMeta{Title: sectionTitle, Weight: weight}
							}
						}
						// process children with this sectionTitle and new section path
						processList(vv, sectionTitle, "", newSectionPath)
					default:
						// Unhandled types: try to marshal to yaml then interpret
						// if it's a map[string]string or similar
						// Marshal back to YAML and unmarshal into []interface{} maybe
						b, _ := yaml.Marshal(val)
						var childList []interface{}
						if err := yaml.Unmarshal(b, &childList); err == nil {
							weight += 10
							sectionTitle := key
							// Build new section path
							newSectionPath := sectionPath
							sectionSlug := slugify(sectionTitle)
							if sectionSlug != "" {
								if newSectionPath == "" {
									newSectionPath = sectionSlug
								} else {
									newSectionPath = path.Join(newSectionPath, sectionSlug)
								}
							}
							// Register this section
							if newSectionPath != "" && newSectionPath != sectionPath {
								if _, ok := sectionMetadata[newSectionPath]; !ok {
									sectionMetadata[newSectionPath] = SectionMeta{Title: sectionTitle, Weight: weight}
								}
							}
							processList(childList, key, "", newSectionPath)
						}
					}
				}
			case string:
				// plain file path (no title)
				fp := filepath.ToSlash(v)
				if strings.HasSuffix(strings.ToLower(fp), ".md") {
					weight += 10
					title := deriveTitleFromFilename(fp)
					full := filepath.Join(sourceDir, filepath.FromSlash(fp))
					if exists(full) {
						// Build destination path
						destPath := fp
						fileDir := path.Dir(fp)
						if fileDir == "." || fileDir == "" {
							// File has no directory in source, so prepend sectionPath
							if sectionPath != "" {
								destPath = path.Join(sectionPath, filepath.Base(fp))
							}
						}

						fileMetadata[destPath] = FileMeta{
							Title:      title,
							Section:    currentSection,
							Subsection: currentSubsection,
							Weight:     weight,
							SourcePath: fp,
							DestPath:   destPath,
						}
						// Register section metadata for destination directory
						dir := path.Dir(destPath)
						if dir != "." && dir != "" {
							if _, ok := sectionMetadata[dir]; !ok {
								sectionMetadata[dir] = SectionMeta{Title: currentSection, Weight: weight}
							}
						}
					}
				}
			default:
				// ignore others
			}
		}
	}

	processList(navList, "", "", "")

	return fileMetadata, sectionMetadata, nil
}

// generateFrontMatter TOML +++ block
func generateFrontMatter(meta FileMeta) string {
	title := escapeTOMLString(meta.Title)
	weight := meta.Weight
	return fmt.Sprintf("+++\ntitle = \"%s\"\nlinkTitle = \"%s\"\nweight = %d\n+++\n\n", title, title, weight)
}

// escapeTOMLString basic escaping for quotes/backslashes
func escapeTOMLString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

// findAndCopyAsset: search for asset in multiple source folders, copy to destination, and track migration.
// Returns the destination path for use in markdown (e.g., /img/diagram.png).
func findAndCopyAsset(assetFilename string, srcAssetFolders []string, dstAssetsFolder string, copiedAssets *map[string]string, migrationRecords *[]MigrationRecord) (string, error) {
	decoded, err := url.PathUnescape(assetFilename)
	if err != nil {
		decoded = assetFilename
	}

	// Check if already copied
	if dstPath, ok := (*copiedAssets)[decoded]; ok {
		return dstPath, nil
	}

	// Search in all source asset folders
	var srcPath string
	var srcFolder string
	var relPath string
	for _, folder := range srcAssetFolders {
		found := false
		err := filepath.WalkDir(folder, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			name := d.Name()
			if name == assetFilename || name == decoded {
				srcPath = path
				srcFolder = folder
				rel, err := filepath.Rel(folder, path)
				if err != nil {
					return err
				}
				relPath = filepath.ToSlash(rel)
				found = true
				return io.EOF // short-circuit walk
			}
			return nil
		})
		if err != nil && err != io.EOF {
			continue
		}
		if found {
			break
		}
	}

	if srcPath == "" {
		return "", nil // asset not found
	}

	// Copy asset to destination
	dstPath := filepath.Join(dstAssetsFolder, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return "", fmt.Errorf("error creating asset dest dir: %w", err)
	}

	srcFile, err := os.Open(srcPath)
	if err != nil {
		return "", fmt.Errorf("error opening source asset: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dstPath)
	if err != nil {
		return "", fmt.Errorf("error creating dest asset: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return "", fmt.Errorf("error copying asset: %w", err)
	}

	// Encode spaces in output path
	outputPath := strings.ReplaceAll(relPath, " ", "%20")
	outputPath = "/" + outputPath

	// Track copied asset
	(*copiedAssets)[decoded] = outputPath

	// Add to migration records (get relative path from src folder)
	srcRelPath, _ := filepath.Rel(srcFolder, srcPath)
	srcFullPath := filepath.Join(filepath.Base(srcFolder), srcRelPath)
	*migrationRecords = append(*migrationRecords, MigrationRecord{
		SourceFile:  srcFullPath,
		InMkdocsNav: false,
		DestPath:    dstPath,
		Weight:      "",
	})

	return outputPath, nil
}

// rewriteExternalSecretsURL attempts to rewrite external-secrets.io URLs to local paths
func rewriteExternalSecretsURL(urlStr string, srcAssetFolders []string, dstAssetsFolder string, copiedAssets *map[string]string, snippetFolder string, migrationRecords *[]MigrationRecord) (newURL string, wasRewritten bool, needsReview bool, reviewReason string) {
	// Check if URL contains asset patterns
	assetPatterns := []string{"/img/", "/static/", "/assets/", "/pictures/"}
	for _, pattern := range assetPatterns {
		if strings.Contains(urlStr, pattern) {
			// Extract filename from URL
			parsedURL, err := url.Parse(urlStr)
			if err != nil {
				return urlStr, false, true, "failed to parse URL"
			}
			assetFilename := filepath.Base(parsedURL.Path)
			// Try to find and copy asset
			newPath, err := findAndCopyAsset(assetFilename, srcAssetFolders, dstAssetsFolder, copiedAssets, migrationRecords)
			if err != nil || newPath == "" {
				return urlStr, false, true, fmt.Sprintf("asset '%s' not found", assetFilename)
			}
			return newPath, true, false, "rewritten to local asset"
		}
	}

	// Check if URL contains snippet pattern
	if strings.Contains(urlStr, "/snippets/") {
		parsedURL, err := url.Parse(urlStr)
		if err != nil {
			return urlStr, false, true, "failed to parse URL"
		}
		// Extract snippet path after /snippets/
		path := parsedURL.Path
		idx := strings.Index(path, "/snippets/")
		if idx >= 0 {
			snippetPath := path[idx+1:] // removes leading "/" to make it relative (snippets/...)
			return snippetPath, true, false, "rewritten to local snippet"
		}
	}

	// Other content - leave as-is but report for review
	return urlStr, false, true, "content link - kept as-is"
}

// rewriteAssetPaths rewrites image paths and html <img src="..."> to new assets paths.
func rewriteAssetPaths(content string, srcAssetFolders []string, dstAssetsFolder string, copiedAssets *map[string]string, migrationRecords *[]MigrationRecord, externalURLsReported *[]ExternalURLReport, snippetFolder string, mdFilename string) string {
	// Markdown images
	content = reMarkdownImage.ReplaceAllStringFunc(content, func(m string) string {
		sub := reMarkdownImage.FindStringSubmatch(m)
		if len(sub) < 3 {
			return m
		}
		alt := sub[1]
		path := strings.TrimSpace(sub[2])

		// If external URL, leave as is (will be handled by external-secrets.io rewriting later)
		if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
			return m
		}
		if path == "" {
			return m
		}
		assetFilename := filepath.Base(path)
		newPath, err := findAndCopyAsset(assetFilename, srcAssetFolders, dstAssetsFolder, copiedAssets, migrationRecords)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error processing asset lookup in %s: %v\n", mdFilename, err)
			return m
		}
		if newPath != "" {
			return fmt.Sprintf("![%s](%s)", alt, newPath)
		}
		decoded, _ := url.PathUnescape(assetFilename)
		fmt.Fprintf(os.Stderr, "Warning: Asset '%s' referenced in '%s' not found in assets folder\n", decoded, mdFilename)
		return m
	})

	// <img src="...">
	content = reImgSrcDouble.ReplaceAllStringFunc(content, func(m string) string {
		sub := reImgSrcDouble.FindStringSubmatch(m)
		if len(sub) < 2 {
			return m
		}
		path := strings.TrimSpace(sub[1])
		if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
			return m
		}
		if path == "" {
			return m
		}
		assetFilename := filepath.Base(path)
		newPath, err := findAndCopyAsset(assetFilename, srcAssetFolders, dstAssetsFolder, copiedAssets, migrationRecords)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error processing asset lookup in %s: %v\n", mdFilename, err)
			return m
		}
		if newPath != "" {
			return strings.Replace(m, path, newPath, 1)
		}
		decoded, _ := url.PathUnescape(assetFilename)
		fmt.Fprintf(os.Stderr, "Warning: Asset '%s' referenced in '%s' not found in assets folder\n", decoded, mdFilename)
		return m
	})

	// <img src='...'>
	content = reImgSrcSingle.ReplaceAllStringFunc(content, func(m string) string {
		sub := reImgSrcSingle.FindStringSubmatch(m)
		if len(sub) < 2 {
			return m
		}
		path := strings.TrimSpace(sub[1])
		if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
			return m
		}
		if path == "" {
			return m
		}
		assetFilename := filepath.Base(path)
		newPath, err := findAndCopyAsset(assetFilename, srcAssetFolders, dstAssetsFolder, copiedAssets, migrationRecords)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error processing asset lookup in %s: %v\n", mdFilename, err)
			return m
		}
		if newPath != "" {
			return strings.Replace(m, path, newPath, 1)
		}
		decoded, _ := url.PathUnescape(assetFilename)
		fmt.Fprintf(os.Stderr, "Warning: Asset '%s' referenced in '%s' not found in assets folder\n", decoded, mdFilename)
		return m
	})

	// Rewrite external-secrets.io URLs
	content = reExternalSecretsURL.ReplaceAllStringFunc(content, func(urlStr string) string {
		newURL, wasRewritten, needsReview, reason := rewriteExternalSecretsURL(urlStr, srcAssetFolders, dstAssetsFolder, copiedAssets, snippetFolder, migrationRecords)

		// Track in report
		action := "kept_as_is"
		if wasRewritten {
			if strings.HasPrefix(newURL, "/snippets/") {
				action = "rewritten_snippet"
			} else {
				action = "rewritten_asset"
			}
		}

		*externalURLsReported = append(*externalURLsReported, ExternalURLReport{
			URL:          urlStr,
			MarkdownFile: mdFilename,
			Action:       action,
			Reason:       reason,
		})

		if needsReview && !wasRewritten {
			fmt.Fprintf(os.Stderr, "Info: external-secrets.io URL in '%s' kept as-is: %s (reason: %s)\n", mdFilename, urlStr, reason)
		}

		return newURL
	})

	return content
}

// createIndexFile writes a simple _index.md with TOML front matter
func createIndexFile(dirPath string, meta SectionMeta) error {
	out := fmt.Sprintf("+++\ntitle = \"%s\"\nweight = %d\n+++\n", escapeTOMLString(meta.Title), meta.Weight)
	fpath := filepath.Join(dirPath, "_index.md")
	return ioutil.WriteFile(fpath, []byte(out), 0644)
}

// stripExistingFrontMatter: remove YAML or TOML front matter at beginning
func stripExistingFrontMatter(content string) string {
	content = reYAMLFrontMatter.ReplaceAllString(content, "")
	content = reTOMLFrontMatter.ReplaceAllString(content, "")
	return content
}

// cleanMarkdownContent: remove <br> tags, style attributes, remove jinja raw/endraw
func cleanMarkdownContent(content string) string {
	content = reBRTags.ReplaceAllString(content, "")
	content = reStyleAttrs.ReplaceAllString(content, "")
	content = reJinjaRaw.ReplaceAllString(content, "")
	content = reJinjaEndRaw.ReplaceAllString(content, "")
	return content
}

// convertAdmonitions: converts MkDocs !!! admonitions to blockquote [!TYPE] format
func convertAdmonitions(content string) string {
	return reAdmonition.ReplaceAllStringFunc(content, func(m string) string {
		sub := reAdmonition.FindStringSubmatch(m)
		if len(sub) < 5 {
			return m
		}
		admType := sub[1]
		titleDouble := sub[2]
		titleSingle := sub[3]
		body := sub[4]
		title := ""
		if titleDouble != "" {
			title = titleDouble
		} else if titleSingle != "" {
			title = titleSingle
		}
		alertType := strings.ToUpper(admType)
		if mapped, ok := admonitionTypeMapping[strings.ToLower(admType)]; ok {
			alertType = mapped
		}
		header := ""
		if title != "" {
			header = fmt.Sprintf("> [!%s] %s", alertType, title)
		} else {
			header = fmt.Sprintf("> [!%s]", alertType)
		}

		// process body lines: remove 4-space indent if present
		if body == "" {
			return header + "\n"
		}
		lines := strings.Split(body, "\n")
		processed := []string{}
		for _, ln := range lines {
			if strings.TrimSpace(ln) == "" {
				processed = append(processed, ">")
			} else if strings.HasPrefix(ln, "    ") {
				processed = append(processed, "> "+ln[4:])
			} else {
				processed = append(processed, "> "+strings.TrimLeft(ln, " \t"))
			}
		}
		// remove trailing solitary ">" lines
		for len(processed) > 0 && processed[len(processed)-1] == ">" {
			processed = processed[:len(processed)-1]
		}
		bodyText := strings.Join(processed, "\n")
		if bodyText != "" {
			return header + "\n>\n" + bodyText + "\n"
		}
		return header + "\n"
	})
}

// copySnippetsFolder: copies source/snippets to snippetDestFolder, cleaning jinja tags in text files.
// Returns a list of .md files found in snippets that should be processed as content files.
func copySnippetsFolder(sourceDir, snippetDestFolder string, migrationRecords *[]MigrationRecord) (bool, []string) {
	srcSnippets := filepath.Join(sourceDir, "snippets")
	if !isDir(srcSnippets) {
		return false, nil
	}
	if err := os.MkdirAll(snippetDestFolder, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating snippet dest folder: %v\n", err)
		return false, nil
	}

	markdownFiles := []string{}

	err := filepath.WalkDir(srcSnippets, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcSnippets, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(snippetDestFolder, rel)
		if d.IsDir() {
			return os.MkdirAll(dest, 0755)
		}

		// Skip .md files - they should be processed as content files
		if strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			markdownFiles = append(markdownFiles, filepath.ToSlash(rel))
			return nil
		}

		// file
		// try reading as UTF-8 text; if error copy as binary
		b, err := ioutil.ReadFile(path)
		if err != nil {
			return err
		}
		content := string(b)
		content = cleanMarkdownContent(content)
		if err := ioutil.WriteFile(dest, []byte(content), 0644); err != nil {
			// fallback to binary copy
			in, err := os.Open(path)
			if err != nil {
				return err
			}
			defer in.Close()
			out, err := os.Create(dest)
			if err != nil {
				return err
			}
			defer out.Close()
			_, err = io.Copy(out, in)
			if err != nil {
				return err
			}
		}

		// Track in migration records
		srcRelPath := filepath.Join("snippets", rel)
		dstRelPath := filepath.Join(filepath.Base(snippetDestFolder), rel)
		*migrationRecords = append(*migrationRecords, MigrationRecord{
			SourceFile:  srcRelPath,
			InMkdocsNav: false,
			DestPath:    dstRelPath,
			Weight:      "",
		})
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error copying snippets: %v\n", err)
		return false, markdownFiles
	}
	return true, markdownFiles
}

// replaceYamlIncludes: replaces include blocks with Hugo readfile shortcodes or links for .md files,
// records missing snippet references in missingSnippets
func replaceYamlIncludes(content string, snippetDestFolder string, missingSnippets *[]MissingSnippet, mdFilename string, snippetToContentMap map[string]string) string {
	replaceInclude := func(snippetPath string) string {
		// Check if this is a markdown file that should be linked instead of included
		if strings.HasSuffix(strings.ToLower(snippetPath), ".md") {
			if contentFileName, ok := snippetToContentMap[snippetPath]; ok {
				// Generate a link to the content file
				// Extract title from the filename
				title := deriveTitleFromFilename(contentFileName)
				// Generate relative link (assuming same directory level)
				linkTarget := strings.TrimSuffix(contentFileName, ".md")
				return fmt.Sprintf("For more information, see [%s](../%s/).", title, linkTarget)
			}
		}

		fullSnippetPath := filepath.Join(snippetDestFolder, filepath.FromSlash(snippetPath))
		if !exists(fullSnippetPath) {
			*missingSnippets = append(*missingSnippets, MissingSnippet{Snippet: snippetPath, ReferencedIn: mdFilename})
		}
		// preserve subdirectories, use relative path
		return fmt.Sprintf("{{< readfile file=snippets/%s code=\"true\" lang=\"yaml\" >}}", snippetPath)
	}

	// Pattern 1: yaml fenced block include
	content = reYamlIncludeBlock.ReplaceAllStringFunc(content, func(m string) string {
		sub := reYamlIncludeBlock.FindStringSubmatch(m)
		if len(sub) < 2 {
			return m
		}
		sn := sub[1]
		return replaceInclude(sn)
	})

	// Pattern 2: jinja include inline
	content = reIncludeInline.ReplaceAllStringFunc(content, func(m string) string {
		sub := reIncludeInline.FindStringSubmatch(m)
		if len(sub) < 2 {
			return m
		}
		sn := sub[1]
		return replaceInclude(sn)
	})

	// Pattern 3: mkdocs snippet --8<--
	content = reMkdocsSnippet.ReplaceAllStringFunc(content, func(m string) string {
		sub := reMkdocsSnippet.FindStringSubmatch(m)
		if len(sub) < 2 {
			return m
		}
		sn := sub[1]
		return replaceInclude(sn)
	})

	return content
}

// convertFile: read source, create front matter, rewrite assets, replace includes, write destination
func convertFile(srcPath, dstPath string, meta FileMeta, srcAssetFolders []string, dstAssetsFolder string, copiedAssets *map[string]string, snippetDestFolder string, missingSnippets *[]MissingSnippet, migrationRecords *[]MigrationRecord, externalURLsReported *[]ExternalURLReport, inMkdocsNav bool, srcRelPath string, snippetToContentMap map[string]string) error {
	b, err := ioutil.ReadFile(srcPath)
	if err != nil {
		return err
	}
	content := string(b)

	// Strip front matter
	content = stripExistingFrontMatter(content)
	// Clean
	content = cleanMarkdownContent(content)
	// Convert admonitions
	content = convertAdmonitions(content)
	// Generate front matter
	front := generateFrontMatter(meta)
	// Rewrite asset paths
	mdFilename := filepath.Base(dstPath)
	content = rewriteAssetPaths(content, srcAssetFolders, dstAssetsFolder, copiedAssets, migrationRecords, externalURLsReported, snippetDestFolder, mdFilename)
	// Replace includes
	content = replaceYamlIncludes(content, snippetDestFolder, missingSnippets, mdFilename, snippetToContentMap)
	// Combine and write
	out := front + content
	err = ioutil.WriteFile(dstPath, []byte(out), 0644)
	if err != nil {
		return err
	}

	// Track in migration records
	*migrationRecords = append(*migrationRecords, MigrationRecord{
		SourceFile:  srcRelPath,
		InMkdocsNav: inMkdocsNav,
		DestPath:    dstPath,
		Weight:      fmt.Sprintf("%d", meta.Weight),
	})

	return nil
}
