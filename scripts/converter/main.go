package main

// convert_mkdocs_to_hugo.go
//
// convert an MkDocs site into Hugo content files with TOML front matter.
//
// Usage (after `go mod tidy`):
//   go run ./convert_mkdocs_to_hugo.go \
//     --config path/to/mkdocs.yml \
//     --source path/to/markdown/files/ \
//     --dest path/to/destination/ \
//     --assets-folder path/to/static/ \
//     --snippet-destination-folder path/to/hugo-project/snippets/

import (
	_ "embed"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"net/url"
	"os"
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
)

// Structures

type FileMeta struct {
	Title      string
	Section    string
	Subsection string
	Weight     int
}

type SectionMeta struct {
	Title  string
	Weight int
}

type MissingSnippet struct {
	Snippet      string
	ReferencedIn string
}

func main() {
	// Flags
	configPath := flag.String("config", "", "Path to mkdocs.yml file")
	sourceDir := flag.String("source", "", "Path to directory containing markdown files")
	destDir := flag.String("dest", "", "Destination directory for converted files")
	assetsFolder := flag.String("assets-folder", "", "Path to assets folder (e.g., ./static/)")
	snippetDest := flag.String("snippet-destination-folder", "", "Destination folder for code snippets (hugo root)/snippets")
	flag.Parse()

	// Validate required
	if *configPath == "" || *sourceDir == "" || *destDir == "" || *assetsFolder == "" || *snippetDest == "" {
		fmt.Fprintln(os.Stderr, "Error: all flags --config, --source, --dest, --assets-folder, --snippet-destination-folder are required")
		os.Exit(1)
	}

	// Validate paths
	if !exists(*configPath) {
		fmt.Fprintf(os.Stderr, "Error: Config file not found: %s\n", *configPath)
		os.Exit(1)
	}
	if !isDir(*sourceDir) {
		fmt.Fprintf(os.Stderr, "Error: Source directory not found: %s\n", *sourceDir)
		os.Exit(1)
	}
	if !isDir(*assetsFolder) {
		fmt.Fprintf(os.Stderr, "Error: Assets folder not found: %s\n", *assetsFolder)
		os.Exit(1)
	}

	// Ensure destination exists
	if err := os.MkdirAll(*destDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating dest dir: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Checking for snippets folder...")
	snippetsFound := copySnippetsFolder(*sourceDir, *snippetDest)
	if snippetsFound {
		fmt.Printf("  Copied snippets folder to %s\n", *snippetDest)
	} else {
		fmt.Println("  No snippets folder found in source directory")
	}

	fmt.Printf("\nParsing %s...\n", *configPath)
	fileMeta, sectionMeta, err := parseMkdocsNav(*configPath, *sourceDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing mkdocs nav: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Found %d files in navigation\n", len(fileMeta))
	fmt.Printf("Found %d directories to create\n", len(sectionMeta))

	// Scan source dir for all markdown files
	allMd := map[string]bool{}
	err = filepath.WalkDir(*sourceDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			rel, err := filepath.Rel(*sourceDir, path)
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
	for k := range fileMeta {
		matchedSet[k] = true
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
		fileMeta[u] = FileMeta{
			Title:      title,
			Section:    "",
			Subsection: "",
			Weight:     unmatchedWeight,
		}
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
		fullDir := filepath.Join(*destDir, filepath.FromSlash(dirPath))
		if err := os.MkdirAll(fullDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating directory %s: %v\n", fullDir, err)
			continue
		}
		createIndexFile(fullDir, meta)
		fmt.Printf("  Created %s/_index.md (title=%q, weight=%d)\n", dirPath, meta.Title, meta.Weight)
	}

	// Asset tracking
	assetsTracking := map[string]map[string]bool{} // decoded asset -> set of md filenames

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

	for _, filepathRel := range fileKeys {
		meta := fileMeta[filepathRel]
		srcPath := filepath.Join(*sourceDir, filepath.FromSlash(filepathRel))
		dstPath := filepath.Join(*destDir, filepath.FromSlash(filepathRel))
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
		if err := convertFile(srcPath, dstPath, meta, *assetsFolder, assetsTracking, *snippetDest, &missingSnippets); err != nil {
			fmt.Fprintf(os.Stderr, "Error converting %s: %v\n", filepathRel, err)
			errorCount++
			continue
		}
		convertedCount++
		if matchedSet[filepathRel] {
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

	if len(assetsTracking) > 0 {
		fmt.Println()
		fmt.Println(strings.Repeat("-", 70))
		fmt.Println("ASSETS REPORT")
		fmt.Println(strings.Repeat("-", 70))
		assetNames := make([]string, 0, len(assetsTracking))
		for a := range assetsTracking {
			assetNames = append(assetNames, a)
		}
		sort.Strings(assetNames)
		for _, a := range assetNames {
			mds := make([]string, 0, len(assetsTracking[a]))
			for m := range assetsTracking[a] {
				mds = append(mds, m)
			}
			sort.Strings(mds)
			for _, m := range mds {
				fmt.Printf("%s\t%s\n", a, m)
			}
		}
	}

	if len(unmatchedList) > 0 {
		fmt.Println()
		fmt.Println(strings.Repeat("-", 70))
		fmt.Println("FILES NOT IN mkdocs.yml nav (migrated with higher weights)")
		fmt.Println(strings.Repeat("-", 70))
		for _, u := range unmatchedList {
			fmt.Printf("  %s (weight: %d)\n", u, fileMeta[u].Weight)
		}
	}

	if len(missingSnippets) > 0 {
		fmt.Println()
		fmt.Println(strings.Repeat("-", 70))
		fmt.Println("MISSING SNIPPETS")
		fmt.Println(strings.Repeat("-", 70))
		for _, ms := range missingSnippets {
			fmt.Printf("  Snippet '%s' referenced in '%s' not found in %s\n", ms.Snippet, ms.ReferencedIn, *snippetDest)
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
	var processList func(items []interface{}, currentSection, currentSubsection string)
	processList = func(items []interface{}, currentSection, currentSubsection string) {
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
							// Check existence in source dir
							full := filepath.Join(sourceDir, filepath.FromSlash(filepathStr))
							if exists(full) {
								// record
								fileMetadata[filepathStr] = FileMeta{
									Title:      title,
									Section:    currentSection,
									Subsection: currentSubsection,
									Weight:     weight,
								}
								// register section metadata for its dir
								dir := filepath.Dir(filepathStr)
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
						// process children with this sectionTitle
						processList(vv, sectionTitle, "")
					default:
						// Unhandled types: try to marshal to yaml then interpret
						// if it's a map[string]string or similar
						// Marshal back to YAML and unmarshal into []interface{} maybe
						b, _ := yaml.Marshal(val)
						var childList []interface{}
						if err := yaml.Unmarshal(b, &childList); err == nil {
							weight += 10
							processList(childList, key, "")
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
						fileMetadata[fp] = FileMeta{
							Title:      title,
							Section:    currentSection,
							Subsection: currentSubsection,
							Weight:     weight,
						}
						dir := filepath.Dir(fp)
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

	processList(navList, "", "")

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

// findAssetInFolder: search recursively; matches either encoded or decoded filename.
// returns path relative to assets folder, URL-encoded (space -> %20), prefixed with '/'
func findAssetInFolder(assetFilename, assetsFolder string) (string, error) {
	decoded, err := url.PathUnescape(assetFilename)
	if err != nil {
		decoded = assetFilename
	}
	var found string
	err = filepath.WalkDir(assetsFolder, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if name == assetFilename || name == decoded {
			rel, err := filepath.Rel(assetsFolder, path)
			if err != nil {
				return err
			}
			// Use forward slashes
			rel = filepath.ToSlash(rel)
			// Encode spaces as %20 (keep other characters untouched)
			rel = strings.ReplaceAll(rel, " ", "%20")
			found = "/" + rel
			return io.EOF // short-circuit walk
		}
		return nil
	})
	if err != nil && err != io.EOF {
		// some error during walk
		return "", err
	}
	if found == "" {
		return "", nil
	}
	return found, nil
}

// rewriteAssetPaths rewrites image paths and html <img src="..."> to new assets paths.
// assetsTracking maps decoded asset filename -> set of md filenames
func rewriteAssetPaths(content, assetsFolder string, assetsTracking map[string]map[string]bool, mdFilename string) string {
	// Markdown images
	content = reMarkdownImage.ReplaceAllStringFunc(content, func(m string) string {
		sub := reMarkdownImage.FindStringSubmatch(m)
		if len(sub) < 3 {
			return m
		}
		alt := sub[1]
		path := strings.TrimSpace(sub[2])

		// If external URL, leave as is
		if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
			return m
		}
		if path == "" {
			return m
		}
		assetFilename := filepath.Base(path)
		newPath, err := findAssetInFolder(assetFilename, assetsFolder)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error processing asset lookup in %s: %v\n", mdFilename, err)
			return m
		}
		if newPath != "" {
			decoded, _ := url.PathUnescape(assetFilename)
			if _, ok := assetsTracking[decoded]; !ok {
				assetsTracking[decoded] = map[string]bool{}
			}
			assetsTracking[decoded][mdFilename] = true
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
		newPath, err := findAssetInFolder(assetFilename, assetsFolder)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error processing asset lookup in %s: %v\n", mdFilename, err)
			return m
		}
		if newPath != "" {
			decoded, _ := url.PathUnescape(assetFilename)
			if _, ok := assetsTracking[decoded]; !ok {
				assetsTracking[decoded] = map[string]bool{}
			}
			assetsTracking[decoded][mdFilename] = true
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
		newPath, err := findAssetInFolder(assetFilename, assetsFolder)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error processing asset lookup in %s: %v\n", mdFilename, err)
			return m
		}
		if newPath != "" {
			decoded, _ := url.PathUnescape(assetFilename)
			if _, ok := assetsTracking[decoded]; !ok {
				assetsTracking[decoded] = map[string]bool{}
			}
			assetsTracking[decoded][mdFilename] = true
			return strings.Replace(m, path, newPath, 1)
		}
		decoded, _ := url.PathUnescape(assetFilename)
		fmt.Fprintf(os.Stderr, "Warning: Asset '%s' referenced in '%s' not found in assets folder\n", decoded, mdFilename)
		return m
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

// copySnippetsFolder: copies source/snippets to snippetDestFolder, cleaning jinja tags in text files
func copySnippetsFolder(sourceDir, snippetDestFolder string) bool {
	srcSnippets := filepath.Join(sourceDir, "snippets")
	if !isDir(srcSnippets) {
		return false
	}
	if err := os.MkdirAll(snippetDestFolder, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating snippet dest folder: %v\n", err)
		return false
	}
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
			return err
		}
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error copying snippets: %v\n", err)
		return false
	}
	return true
}

// replaceYamlIncludes: replaces include blocks with Hugo readfile shortcodes,
// records missing snippet references in missingSnippets
func replaceYamlIncludes(content string, snippetDestFolder string, missingSnippets *[]MissingSnippet, mdFilename string) string {
	replaceInclude := func(snippetPath string) string {
		fullSnippetPath := filepath.Join(snippetDestFolder, filepath.FromSlash(snippetPath))
		if !exists(fullSnippetPath) {
			*missingSnippets = append(*missingSnippets, MissingSnippet{Snippet: snippetPath, ReferencedIn: mdFilename})
		}
		// preserve subdirectories
		return fmt.Sprintf("{{< readfile file=/snippets/%s code=\"true\" lang=\"yaml\" >}}", snippetPath)
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
func convertFile(srcPath, dstPath string, meta FileMeta, assetsFolder string, assetsTracking map[string]map[string]bool, snippetDestFolder string, missingSnippets *[]MissingSnippet) error {
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
	content = rewriteAssetPaths(content, assetsFolder, assetsTracking, mdFilename)
	// Replace includes
	content = replaceYamlIncludes(content, snippetDestFolder, missingSnippets, mdFilename)
	// Combine and write
	out := front + content
	return ioutil.WriteFile(dstPath, []byte(out), 0644)
}
