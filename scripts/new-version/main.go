package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

type Version struct {
	Version string `toml:"version"`
	URL     string `toml:"url"`
	Tag     string `toml:"tag"`
	Latest  bool   `toml:"latest"`
}

type VersionsData struct {
	Versions []Version `toml:"versions"`
}

var (
	project = flag.String("project", "", "Project name (eso or reloader)")
	version = flag.String("version", "", "New version tag (e.g., v0.15)")
)

func main() {
	flag.Parse()

	if *project == "" || *version == "" {
		log.Fatal("Usage: go run ./scripts/new-version --project eso --version v0.15")
	}

	if *project != "eso" && *project != "reloader" {
		log.Fatalf("project must be 'eso' or 'reloader', got: %s", *project)
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

	fmt.Printf("Current latest: %s -> %s\n", oldLatest.Tag, oldLatest.Version)
	fmt.Printf("New version: %s\n", *version)

	// Update data file: mark old as not latest, add new version
	versions.Versions[oldLatestIdx].Latest = false
	versions.Versions[oldLatestIdx].Version = strings.TrimSuffix(oldLatest.Version, " (latest)")

	newVersion := Version{
		Version: fmt.Sprintf("%s (latest)", *version),
		URL:     fmt.Sprintf("/%s-docs/%s/", *project, *version),
		Tag:     *version,
		Latest:  true,
	}
	versions.Versions = append([]Version{newVersion}, versions.Versions...)

	if err := writeVersions(dataFile, versions); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Updated %s\n", dataFile)

	// Create new version directory
	newVersionDir := filepath.Join(baseDir, *version)
	os.MkdirAll(newVersionDir, 0755)

	// Create new version _index.md
	newVersionPath := filepath.Join(newVersionDir, "_index.md")
	projectName := *project
	if *project == "eso" {
		projectName = "ESO"
	} else if *project == "reloader" {
		projectName = strings.ToUpper(projectName[:1]) + projectName[1:]
	}

	content := fmt.Sprintf(`+++
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
`, projectName, *version, *version, *project, *version, projectName, *version)

	if err := ioutil.WriteFile(newVersionPath, []byte(content), 0644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Created %s\n", newVersionPath)

	// Update project root _index.md with new latest version link
	if err := updateProjectIndex(projectIndexFile, *project, *version); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Updated %s\n", projectIndexFile)

	fmt.Printf("\nRelease %s prepared successfully!\n", *version)
	fmt.Printf("Next steps:\n")
	fmt.Printf("1. Review the changes\n")
	fmt.Printf("2. Add content to %s\n", newVersionDir)
	fmt.Printf("3. Commit and push\n")
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
	content, err := ioutil.ReadFile(filename)
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

	return ioutil.WriteFile(filename, []byte(text), 0644)
}
