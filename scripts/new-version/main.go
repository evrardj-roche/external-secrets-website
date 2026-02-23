package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"golang.org/x/mod/semver"
)

const (
	ReleaseLandingPageTemplate string = `+++
title = "%s %s Documentation"
linkTitle = "%s"
weight = 1

[[cascade]]
type = "docs"

  [cascade.params]
  project = "%s"
  project_version = "%s"
  sidebar_root_for = "children"
+++

Welcome to the %s %s documentation.
`
)

// Version contains the structure of data/*_versions.toml
type Version struct {
	Version           string   `toml:"version"`
	URL               string   `toml:"url"`
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

var (
	project           = flag.String("project", "", "Project name (eso or reloader)")
	version           = flag.String("version", "", "New version tag (e.g., v0.15)")
	releaseDate       = flag.String("release-date", "", "Release date (YYYY-MM-DD format, defaults to today)")
	testedK8sVersions = flag.String("tested-k8s-versions", "", "Comma separated list of tested k8s version (e.g. v1.35,v1.36) for the release")
	projects          = map[string]ProjectDetails{
		"eso":      {GoModLocation: "https://raw.githubusercontent.com/external-secrets/external-secrets/%s/go.mod", ProjectLongName: "External-Secrets Operator"},
		"reloader": {GoModLocation: "https://raw.githubusercontent.com/external-secrets/reloader/%s/go.mod", ProjectLongName: "Reloader Operator"},
	}
)

func main() {
	flag.Parse()

	if *project == "" || *version == "" {
		log.Fatal("Usage: go run ./scripts/new-version --project eso --version v0.15")
	}

	if *project != "eso" && *project != "reloader" {
		log.Fatalf("project must be 'eso' or 'reloader', got: %s", *project)
	}

	// Determine release date
	if *releaseDate == "" {
		*releaseDate = time.Now().Format("2006-01-02")
	}

	if *testedK8sVersions == "" {
		log.Print("Did not receive the list of the tested k8s versions, will fetch the supported version from release's go.mod")
		url := fmt.Sprintf(projects[*project].GoModLocation, *version)
		if body, err := fetchGoMod(url); err != nil {
			log.Fatalf("failed to fetch from %s: %v", url, err)
		} else {
			clientGo, errParse := parseK8sClientGoVersion(string(body))
			if errParse != nil {
				log.Fatal(errParse)
			}
			*testedK8sVersions = convertClientGoToRealK8sVersion(clientGo)
		}
	}

	// Determine paths
	baseDir := filepath.Join("content", "en", fmt.Sprintf("%s-docs", *project))
	dataFile := filepath.Join("data", fmt.Sprintf("%s_versions.toml", *project))
	projectIndexFile := filepath.Join(baseDir, "_index.md")

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

	// Ensure no doubloons
	for i := range versions.Versions {
		if versions.Versions[i].Version == *version {
			log.Fatal("This release already exists")
		}
	}

	fmt.Printf("Current latest: %s -> %s\n", oldLatest.Tag, oldLatest.Version)
	fmt.Printf("New version: %s\n", *version)

	fmt.Printf("Update versions file for release %s", *version)

	// Update data file: mark old as not latest, add new version
	versions.Versions[oldLatestIdx].Latest = false
	versions.Versions[oldLatestIdx].Version = strings.TrimSuffix(oldLatest.Version, " (latest)")

	newVersion := Version{
		Version:           fmt.Sprintf("%s (latest)", *version),
		URL:               fmt.Sprintf("/%s-docs/%s/", *project, *version),
		Tag:               *version,
		Latest:            true,
		ReleaseDate:       *releaseDate,
		TestedK8sVersions: strings.Split(*testedK8sVersions, ","),
		EndOfLife:         "",
	}
	versions.Versions = append([]Version{newVersion}, versions.Versions...)

	if err := writeVersions(dataFile, versions); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Updated %s\n", dataFile)

	newVersionDir := filepath.Join(baseDir, *version)

	fmt.Printf("Creating release directory %s\n", newVersionDir)
	errMkdir := os.MkdirAll(newVersionDir, 0755)
	if errMkdir != nil {
		log.Fatal(errMkdir)
	}

	fmt.Printf("Copying recursively all the unreleased content to release")
	if errCopy := CopyDir(filepath.Join(baseDir, "unreleased"), filepath.Join(newVersionDir)); errCopy != nil {
		log.Fatalf("Issue occured while copying unreleased folder to release folder %s, %v", newVersionDir, errCopy)
	}

	fmt.Print("Overriding unreleased metadata with new release info\n")

	// Create new version _index.md
	newVersionPath := filepath.Join(newVersionDir, "_index.md")
	content := fmt.Sprintf(ReleaseLandingPageTemplate, projects[*project].ProjectLongName, *version, *version, *project, *version, projects[*project].ProjectLongName, *version)

	if err := os.WriteFile(newVersionPath, []byte(content), 0644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Overwritten %s\n", newVersionPath)

	fmt.Printf("Update project root _index.md with new latest version link")
	if err := updateProjectIndex(projectIndexFile, *project, *version); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Updated %s\n", projectIndexFile)

	fmt.Printf("\nRelease %s prepared successfully!\n", *version)
	fmt.Printf("Next steps:\n")
	fmt.Printf("1. Review the changes\n")
	fmt.Printf("2. Commit and push\n")
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

func updateProjectIndex(filename string, project string, newVersion string) error {
	content, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	text := string(content)

	// Replace the "go to latest" link
	// Match pattern like: [latest version](/eso-docs/v0.14/)
	pattern := fmt.Sprintf(`\[latest version\]\(/%s-docs/v[\d.]+/\)`, project)
	replacement := fmt.Sprintf(`[latest version](/%s-docs/%s/)`, project, newVersion)

	re, err := regexp.Compile(pattern)
	if err != nil {
		return err
	}

	text = re.ReplaceAllString(text, replacement)

	return os.WriteFile(filename, []byte(text), 0644)
}

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
				// Return the version (second field)
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
