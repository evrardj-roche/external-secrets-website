package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"golang.org/x/mod/semver"
)

const (
	ReleaseLandingPageTemplate string = `+++
title = "%s %s Documentation"
linkTitle = "%s"
sidebar_root_for = "self"

[[cascade]]
type = "docs"

  [cascade.params]
  project = "%s"
  project_version = "%s"
+++

Welcome to the %s %s documentation.
`
)

// Version contains the structure of data/*_versions.toml
type Version struct {
	Tag               string   `toml:"tag"`
	Latest            bool     `toml:"latest"`
	ReleaseDate       string   `toml:"release_date"`
	TestedK8sVersions []string `toml:"tested_k8s_versions"`
	EndOfLife         string   `toml:"end_of_life"`
}

// VersionsData contains all the parsed versions of the project
type VersionsData struct {
	Versions []Version `toml:"versions"`
}

// ProjectDetails contains data for processing
type ProjectDetails struct {
	GoModLocation   string
	ProjectLongName string
}

// extractMajorMinor extracts major.minor from a semver tag
// Example: "v0.15.3" -> "v0.15"
func extractMajorMinor(tag string) string {
	if !semver.IsValid(tag) {
		log.Fatalf("Invalid semver tag: %s", tag)
	}
	return semver.MajorMinor(tag)
}

// isDirectoryUsedByOtherRelease checks if a major.minor directory is still used
// by other releases in the versions list
func isDirectoryUsedByOtherRelease(majorMinor string, tagToRemove string, versions []Version) bool {
	for _, v := range versions {
		if v.Tag == tagToRemove {
			continue // Skip the version we're removing
		}
		if extractMajorMinor(v.Tag) == majorMinor {
			return true
		}
	}
	return false
}

var (
	projects = map[string]ProjectDetails{
		"eso":      {GoModLocation: "https://raw.githubusercontent.com/external-secrets/external-secrets/%s/go.mod", ProjectLongName: "External-Secrets Operator"},
		"reloader": {GoModLocation: "https://raw.githubusercontent.com/external-secrets/reloader/%s/go.mod", ProjectLongName: "Reloader Operator"},
	}
)

func main() {
	if len(os.Args) < 2 {
		printReleaseUsage()
		os.Exit(1)
	}

	handleReleaseCommand()
}

func printReleaseUsage() {
	fmt.Println("Usage:")
	fmt.Println("  release add --project <eso|reloader> --tag <version> [--release-date YYYY-MM-DD] [--tested-k8s-versions v1.26,v1.27]")
	fmt.Println("  release delete --project <eso|reloader> --tag <version>")
}

func handleReleaseCommand() {
	action := os.Args[1]

	// Parse flags starting from position 3
	releaseFlags := flag.NewFlagSet("release", flag.ExitOnError)
	project := releaseFlags.String("project", "", "Project name (eso or reloader)")
	tag := releaseFlags.String("tag", "", "Version tag (e.g., v0.15.3)")
	releaseDate := releaseFlags.String("release-date", "", "Release date (YYYY-MM-DD format, defaults to today)")
	testedK8sVersions := releaseFlags.String("tested-k8s-versions", "", "Comma separated list of tested k8s version (e.g. v1.35,v1.36) for the release. Auto-discovered from go.mod if not provided.")

	releaseFlags.Parse(os.Args[2:])

	switch action {
	case "add":
		handleAdd(*project, *tag, *releaseDate, *testedK8sVersions)
	case "delete":
		handleRemove(*project, *tag)
	default:
		fmt.Printf("Unknown release action: %s\n", action)
		printReleaseUsage()
		os.Exit(1)
	}
}

func handleAdd(project string, tag string, releaseDate string, testedK8sVersions string) {
	// Validate inputs
	if project == "" || tag == "" {
		fmt.Print("Missing project or tag\n")
		printReleaseUsage()
		os.Exit(1)
	}

	if project != "eso" && project != "reloader" {
		log.Fatalf("project must be 'eso' or 'reloader', got: %s", project)
	}

	if !semver.IsValid(tag) {
		log.Fatalf("Invalid semver tag: %s. Use full semver like v0.15.0", tag)
	}

	// Set defaults for release date
	if releaseDate == "" {
		releaseDate = time.Now().Format("2006-01-02")
	}

	// Auto-discover k8s versions if not provided
	if testedK8sVersions == "" {
		log.Print("Did not receive the list of the tested k8s versions, will fetch the supported version from release's go.mod")
		url := fmt.Sprintf(projects[project].GoModLocation, tag)
		if body, err := fetchGoMod(url); err != nil {
			log.Fatalf("failed to fetch from %s: %v", url, err)
		} else {
			clientGo, errParse := parseK8sClientGoVersion(string(body))
			if errParse != nil {
				log.Fatal(errParse)
			}
			testedK8sVersions = convertClientGoToRealK8sVersion(clientGo)
		}
	}

	// Determine paths
	baseDir := filepath.Join("content", "en", fmt.Sprintf("%s-docs", project))
	dataFile := filepath.Join("data", fmt.Sprintf("%s_versions.toml", project))

	// Check if data file exists
	if _, err := os.Stat(dataFile); os.IsNotExist(err) {
		log.Fatalf("Data file not found: %s", dataFile)
	}

	// Read existing versions
	versions, err := readVersions(dataFile)
	if err != nil {
		log.Fatal(err)
	}

	// Find current latest
	var oldLatest *Version
	oldLatestIdx := -1
	for i := range versions.Versions {
		if versions.Versions[i].Latest {
			oldLatest = &versions.Versions[i]
			oldLatestIdx = i
			break
		}
	}

	if oldLatest == nil {
		log.Fatal("No current latest version found in data file")
	}

	// Ensure no duplicates
	for i := range versions.Versions {
		if versions.Versions[i].Tag == tag {
			log.Fatalf("Version %s already exists", tag)
		}
	}

	fmt.Printf("Current latest: %s\n", oldLatest.Tag)
	fmt.Printf("New version: %s\n", tag)

	// Update TOML: mark old as not latest, add new version
	versions.Versions[oldLatestIdx].Latest = false

	newVersion := Version{
		Tag:               tag,
		Latest:            true,
		ReleaseDate:       releaseDate,
		TestedK8sVersions: strings.Split(testedK8sVersions, ","),
		EndOfLife:         "",
	}
	versions.Versions = append([]Version{newVersion}, versions.Versions...)

	// Write TOML
	if err := writeVersions(dataFile, versions); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Updated %s\n", dataFile)

	// Create directory using major.minor
	majorMinor := extractMajorMinor(tag)
	newVersionDir := filepath.Join(baseDir, majorMinor)

	// ALWAYS create/update directory (even if it exists)
	fmt.Printf("Creating/updating release directory %s\n", newVersionDir)
	if err := os.MkdirAll(newVersionDir, 0755); err != nil {
		log.Fatal(err)
	}

	// ALWAYS copy unreleased content (overwrites if directory exists)
	fmt.Printf("Copying unreleased content to %s\n", newVersionDir)
	if err := CopyDir(filepath.Join(baseDir, "unreleased"), newVersionDir); err != nil {
		log.Fatalf("Failed to copy content: %v", err)
	}

	// Create version landing page
	newVersionPath := filepath.Join(newVersionDir, "_index.md")
	content := fmt.Sprintf(ReleaseLandingPageTemplate,
		projects[project].ProjectLongName, majorMinor, majorMinor,
		project, majorMinor, projects[project].ProjectLongName, majorMinor)

	if err := os.WriteFile(newVersionPath, []byte(content), 0644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Overwritten %s\n", newVersionPath)

	fmt.Printf("\nRelease %s added successfully!\n", tag)
	fmt.Printf("Documentation will be available at: /%s-docs/%s/\n", project, majorMinor)
	fmt.Printf("Next steps:\n")
	fmt.Printf("1. Review the changes\n")
	fmt.Printf("2. Commit and push\n")
}

func handleRemove(project string, tag string) {
	// Validate inputs
	if project == "" || tag == "" {
		printReleaseUsage()
		os.Exit(1)
	}

	if project != "eso" && project != "reloader" {
		log.Fatalf("project must be 'eso' or 'reloader', got: %s", project)
	}

	if !semver.IsValid(tag) {
		log.Fatalf("Invalid semver tag: %s", tag)
	}

	// Determine paths
	baseDir := filepath.Join("content", "en", fmt.Sprintf("%s-docs", project))
	dataFile := filepath.Join("data", fmt.Sprintf("%s_versions.toml", project))

	// Read versions
	versions, err := readVersions(dataFile)
	if err != nil {
		log.Fatal(err)
	}

	// Find version to remove
	removeIdx := -1
	var versionToRemove *Version
	for i := range versions.Versions {
		if versions.Versions[i].Tag == tag {
			removeIdx = i
			versionToRemove = &versions.Versions[i]
			break
		}
	}

	if removeIdx == -1 {
		log.Fatalf("Version %s not found", tag)
	}

	// Warn if removing latest
	if versionToRemove.Latest {
		log.Printf("Warning: Removing latest version. Mark another version as latest manually.")
	}

	// Extract major.minor
	majorMinor := extractMajorMinor(tag)
	versionDir := filepath.Join(baseDir, majorMinor)

	// Remove from slice
	versions.Versions = append(versions.Versions[:removeIdx], versions.Versions[removeIdx+1:]...)

	// Check if directory is still used
	if isDirectoryUsedByOtherRelease(majorMinor, tag, versions.Versions) {
		fmt.Printf("Directory %s still used by other releases, keeping it\n", versionDir)
	} else {
		fmt.Printf("Deleting directory %s\n", versionDir)
		if err := os.RemoveAll(versionDir); err != nil {
			log.Fatalf("Failed to delete directory: %v", err)
		}
		fmt.Printf("Deleted %s\n", versionDir)
	}

	// Write updated TOML
	if err := writeVersions(dataFile, versions); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Updated %s (removed %s)\n", dataFile, tag)

	fmt.Printf("\nVersion %s deleted successfully!\n", tag)
	if versionToRemove.Latest {
		fmt.Printf("IMPORTANT: No version is marked as latest now. Please manually mark another version as latest in %s.\n", dataFile)
	}
}

func readVersions(filename string) (*VersionsData, error) {
	var data VersionsData
	if _, err := toml.DecodeFile(filename, &data); err != nil {
		return nil, err
	}
	return &data, nil
}

func writeVersions(filename string, data *VersionsData) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	encoder := toml.NewEncoder(f)
	return encoder.Encode(data)
}

// updateProjectIndex is no longer needed as the redirect layout
// now reads the latest version dynamically from the data files.
// Keeping the function commented for reference in case manual updates are needed.
//
// func updateProjectIndex(filename string, project string, newVersion string) error {
// 	content, err := os.ReadFile(filename)
// 	if err != nil {
// 		return err
// 	}
//
// 	text := string(content)
//
// 	// Replace the "go to latest" link
// 	// Match pattern like: [latest version](/eso-docs/v0.14/)
// 	pattern := fmt.Sprintf(`\[latest version\]\(/%s-docs/v[\d.]+/\)`, project)
// 	replacement := fmt.Sprintf(`[latest version](/%s-docs/%s/)`, project, newVersion)
//
// 	re, err := regexp.Compile(pattern)
// 	if err != nil {
// 		return err
// 	}
//
// 	text = re.ReplaceAllString(text, replacement)
//
// 	return os.WriteFile(filename, []byte(text), 0644)
// }

func fetchGoMod(url string) ([]byte, error) {
	// Fetch the go.mod file
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch go.mod: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch go.mod: HTTP %d", resp.StatusCode)
	}

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	return body, nil
}

func parseK8sClientGoVersion(goModContent string) (string, error) {
	lines := strings.Split(goModContent, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Look for lines that start with k8s.io/client-go
		// It should look like this for version 1.35:
		// k8s.io/client-go v0.35.0
		if strings.HasPrefix(line, "k8s.io/client-go") {
			// Split by whitespace to get the module and version
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				// Return the version (second field if no replacement, 4th if replacement)
				if parts[1] == "=>" {
					return parts[3], nil
				}
				return parts[1], nil
			}
		}
	}

	return "", fmt.Errorf("k8s.io/client-go not found in go.mod")
}

// convertClientGoToRealK8sVersion converts client-go version to Kubernetes version
func convertClientGoToRealK8sVersion(clientGoVersion string) string {
	noV := strings.TrimPrefix(clientGoVersion, "v")
	normalizedVersion := "v" + noV

	Major := semver.Major(normalizedVersion) // Major: "v0"
	if Major != "v0" {
		// ClientGo should only start with v0. If I am parsing something else, skip parsing, return input.
		return clientGoVersion
	}

	MajorAndMinorVersion := semver.MajorMinor(normalizedVersion)
	return strings.Replace(MajorAndMinorVersion, Major, "v1", 1)
}
