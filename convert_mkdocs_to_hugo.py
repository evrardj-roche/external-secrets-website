#!/usr/bin/env python3
"""Convert mkdocs site to Hugo with front matter.

REQUIREMENTS:
-------------
1. Parse mkdocs.yml navigation structure to extract file hierarchy
2. Generate TOML front matter (+++...+++) with:
   - title: From nav display name
   - linkTitle: Same as title
   - weight: Position in nav (increments by 10)
3. Preserve directory structure from mkdocs.yml (NOT flattened)
4. Create _index.md files for each directory with:
   - title: Section name from mkdocs.yml
   - weight: Section position (first sections have lower weight)
5. Migrate ALL markdown files:
   - Files in mkdocs.yml nav: use their nav position for weight
   - Files NOT in mkdocs.yml nav: assign higher weights (after matched files)
6. Rewrite asset paths:
   - Find assets in --assets-folder recursively
   - Rewrite relative paths (../pictures/...) to absolute (/img/...)
   - Handle URL-encoded filenames (e.g., %20 for spaces):
     * Decode to find actual file
     * Keep encoding in output path
7. Clean markdown content:
   - Strip existing YAML/TOML front matter (including hide_toc metadata)
   - Remove <br> and <br /> tags
   - Remove markdown style attributes (e.g., {: style="width:70%;"})
8. Handle code snippets:
   - Copy snippets folder from source to snippet-destination-folder if it exists
   - Replace include blocks with Hugo readfile shortcode:
     * Format: {{< readfile file=/snippets/filename code="true" lang="yaml" >}}
     * MkDocs snippets: --8<-- "file"
     * Jinja2 includes: {% include "file" %}
   - IMPORTANT: snippet-destination-folder should be <hugo-project-root>/snippets
     (NOT <hugo-project-root>/content/snippets) because Hugo's readfile shortcode
     resolves /snippets/ paths relative to the project root
   - Validate that referenced snippets exist in destination folder
   - Report missing snippet references at end of run
9. Guard against failures:
   - Handle missing assets gracefully (warn but continue)
   - Catch errors in asset rewriting
10. Output summary report:
   - Total files processed (split by matched/unmatched)
   - Assets report: tab-separated list (asset_filename<TAB>markdown_filename)
   - List of files NOT in mkdocs.yml nav with their assigned weights
   - List of missing snippet references

USAGE:
------
python3 convert_mkdocs_to_hugo.py \\
  --config path/to/mkdocs.yml \\
  --source path/to/markdown/files/ \\
  --dest path/to/destination/ \\
  --assets-folder path/to/static/ \\
  --snippet-destination-folder path/to/hugo-project/snippets/

NOTE: The snippet-destination-folder should point to the Hugo project root's
      snippets folder (e.g., /path/to/hugo-project/snippets), NOT the content
      folder (e.g., NOT /path/to/hugo-project/content/snippets). This is
      because Hugo's readfile shortcode resolves /snippets/ paths relative to
      the project root.
"""

import argparse
import os
import re
import sys
import shutil
from pathlib import Path
from collections import defaultdict
from urllib.parse import unquote


def parse_mkdocs_nav(yaml_path, source_dir):
    """Parse nav section from mkdocs.yml, return file->metadata mapping.

    Returns:
        tuple: (file_metadata dict, section_metadata dict)
            file_metadata: {file_path: {title, section, subsection, weight}}
            section_metadata: {dir_path: {title, weight}}
    """
    with open(yaml_path, 'r') as f:
        content = f.read()

    # Extract nav section
    nav_match = re.search(r'^nav:\s*$', content, re.MULTILINE)
    if not nav_match:
        raise ValueError("No 'nav:' section found in mkdocs.yml")

    nav_start = nav_match.end()
    lines = content[nav_start:].split('\n')

    file_metadata = {}
    section_metadata = {}
    current_section = None
    current_subsection = None
    current_section_dir = None
    current_subsection_dir = None
    section_weight = 0
    subsection_weight = 0
    item_weight = 0

    for line in lines:
        # Stop if we hit a new top-level key
        if line and not line.startswith(' ') and not line.startswith('\t'):
            break

        # Skip empty lines
        if not line.strip() or line.strip().startswith('#'):
            continue

        # Determine indentation level
        indent = len(line) - len(line.lstrip())
        stripped = line.strip()

        # Level 1: Main section (2 spaces)
        if indent == 2 and stripped.startswith('- '):
            section_weight += 10
            item_weight = section_weight
            subsection_weight = 0

            # Extract section name
            match = re.match(r'-\s+(.+?):\s*$', stripped)
            if match:
                current_section = match.group(1)
                current_subsection = None
                current_section_dir = None
                current_subsection_dir = None
            else:
                # Direct file reference at section level
                match = re.match(r'-\s+(.+?):\s+(.+\.md)', stripped)
                if match:
                    title = match.group(1)
                    filepath = match.group(2)
                    current_section = title
                    current_subsection = None

                    # Track section directory
                    dir_path = os.path.dirname(filepath)
                    if dir_path and dir_path not in section_metadata:
                        section_metadata[dir_path] = {
                            'title': current_section,
                            'weight': section_weight
                        }
                        current_section_dir = dir_path

                    full_path = os.path.join(source_dir, filepath)
                    if os.path.exists(full_path):
                        file_metadata[filepath] = {
                            'title': title,
                            'section': current_section,
                            'subsection': None,
                            'weight': item_weight
                        }

        # Level 2: Items within section (6-8 spaces)
        elif indent >= 6 and indent <= 8 and stripped.startswith('- '):
            item_weight += 10

            # Check if it's a subsection or direct file
            match = re.match(r'-\s+(.+?):\s*$', stripped)
            if match:
                # It's a subsection header
                current_subsection = match.group(1)
                subsection_weight = item_weight
                current_subsection_dir = None
            else:
                # It's a file reference
                match = re.match(r'-\s+(.+?):\s+(.+\.md)', stripped)
                if not match:
                    # Try quoted format
                    match = re.match(r'-\s+"(.+\.md)"', stripped)
                    if match:
                        filepath = match.group(1)
                        title = os.path.splitext(os.path.basename(filepath))[0].replace('-', ' ').title()
                    else:
                        continue
                else:
                    title = match.group(1)
                    filepath = match.group(2)

                # Track section directory
                dir_path = os.path.dirname(filepath)
                if dir_path:
                    if dir_path not in section_metadata:
                        section_metadata[dir_path] = {
                            'title': current_subsection if current_subsection else current_section,
                            'weight': section_weight
                        }
                        current_section_dir = dir_path

                full_path = os.path.join(source_dir, filepath)
                if os.path.exists(full_path):
                    file_metadata[filepath] = {
                        'title': title,
                        'section': current_section,
                        'subsection': current_subsection,
                        'weight': item_weight
                    }

        # Level 3: Items within subsection (10-12 spaces)
        elif indent >= 10 and indent <= 12 and stripped.startswith('- '):
            item_weight += 10

            match = re.match(r'-\s+(.+?):\s+(.+\.md)', stripped)
            if not match:
                # Try quoted format
                match = re.match(r'-\s+"(.+\.md)"', stripped)
                if match:
                    filepath = match.group(1)
                    title = os.path.splitext(os.path.basename(filepath))[0].replace('-', ' ').title()
                else:
                    continue
            else:
                title = match.group(1)
                filepath = match.group(2)

            # Track subsection directory
            dir_path = os.path.dirname(filepath)
            if dir_path:
                # Check if this is a nested directory (subsection)
                parent_dir = os.path.dirname(dir_path)
                if parent_dir and parent_dir in section_metadata:
                    # This is a subsection directory
                    if dir_path not in section_metadata:
                        section_metadata[dir_path] = {
                            'title': current_subsection if current_subsection else title,
                            'weight': subsection_weight
                        }

            full_path = os.path.join(source_dir, filepath)
            if os.path.exists(full_path):
                file_metadata[filepath] = {
                    'title': title,
                    'section': current_section,
                    'subsection': current_subsection,
                    'weight': item_weight
                }

    return file_metadata, section_metadata


def generate_front_matter(metadata):
    """Generate TOML front matter string."""
    title = metadata['title']
    weight = metadata['weight']

    front_matter = f"""+++
title = "{title}"
linkTitle = "{title}"
weight = {weight}
+++

"""

    return front_matter


def find_asset_in_folder(asset_filename, assets_folder):
    """Search for an asset file recursively in the assets folder.

    Returns:
        str: Relative path from assets folder root (URL-encoded), or None if not found
    """
    assets_path = Path(assets_folder)

    # URL-decode the filename (handles %20 and other encoded chars)
    decoded_filename = unquote(asset_filename)

    # Search recursively for the file (try both encoded and decoded versions)
    for asset_file in assets_path.rglob('*'):
        if asset_file.is_file():
            # Check both the original and decoded filename
            if asset_file.name == asset_filename or asset_file.name == decoded_filename:
                # Return path relative to assets folder root
                # Keep spaces as %20 in the URL
                relative_path = str(asset_file.relative_to(assets_path))
                # URL encode spaces and other special characters
                encoded_path = relative_path.replace(' ', '%20')
                return '/' + encoded_path

    return None


def rewrite_asset_paths(content, assets_folder, assets_tracking, md_filename):
    """Rewrite asset paths in markdown content.

    Args:
        content: Markdown content
        assets_folder: Path to assets folder
        assets_tracking: Dict to track asset->md file mappings
        md_filename: Name of the markdown file being processed

    Returns:
        str: Content with rewritten asset paths
    """
    # Pattern to match markdown images and HTML img tags
    patterns = [
        r'!\[([^\]]*)\]\(([^)]+)\)',  # ![alt](path)
        r'<img\s+src="([^"]+)"',       # <img src="path">
        r'<img\s+src=\'([^\']+)\'',    # <img src='path'>
    ]

    modified_content = content

    for pattern in patterns:
        def replace_asset(match):
            try:
                if pattern.startswith('!'):
                    # Markdown image format
                    alt_text = match.group(1)
                    asset_path = match.group(2)

                    # Extract filename from path and decode URL encoding
                    asset_filename = os.path.basename(asset_path)

                    # Skip external URLs
                    if asset_path.startswith('http://') or asset_path.startswith('https://'):
                        return match.group(0)

                    # Skip if asset_path is empty or just whitespace
                    if not asset_filename or not asset_filename.strip():
                        return match.group(0)

                    # Find asset in assets folder
                    new_path = find_asset_in_folder(asset_filename, assets_folder)

                    if new_path:
                        # Track this asset (using decoded name for readability)
                        decoded_filename = unquote(asset_filename)
                        assets_tracking[decoded_filename].add(md_filename)
                        return f'![{alt_text}]({new_path})'
                    else:
                        # Keep original path but warn
                        decoded_filename = unquote(asset_filename)
                        print(f"Warning: Asset '{decoded_filename}' referenced in '{md_filename}' not found in assets folder", file=sys.stderr)
                        return match.group(0)
                else:
                    # HTML img tag format
                    asset_path = match.group(1)
                    asset_filename = os.path.basename(asset_path)

                    # Skip external URLs
                    if asset_path.startswith('http://') or asset_path.startswith('https://'):
                        return match.group(0)

                    # Skip if asset_path is empty or just whitespace
                    if not asset_filename or not asset_filename.strip():
                        return match.group(0)

                    # Find asset in assets folder
                    new_path = find_asset_in_folder(asset_filename, assets_folder)

                    if new_path:
                        # Track this asset (using decoded name for readability)
                        decoded_filename = unquote(asset_filename)
                        assets_tracking[decoded_filename].add(md_filename)
                        return match.group(0).replace(asset_path, new_path)
                    else:
                        # Keep original path but warn
                        decoded_filename = unquote(asset_filename)
                        print(f"Warning: Asset '{decoded_filename}' referenced in '{md_filename}' not found in assets folder", file=sys.stderr)
                        return match.group(0)
            except Exception as e:
                # Guard against any regex or processing errors
                print(f"Error processing asset in '{md_filename}': {e}", file=sys.stderr)
                return match.group(0)

        try:
            modified_content = re.sub(pattern, replace_asset, modified_content)
        except Exception as e:
            print(f"Error rewriting assets in '{md_filename}': {e}", file=sys.stderr)

    return modified_content


def create_index_file(dir_path, metadata):
    """Create _index.md file for a directory with title and weight."""
    index_path = os.path.join(dir_path, '_index.md')

    title = metadata['title']
    weight = metadata['weight']

    content = f"""+++
title = "{title}"
weight = {weight}
+++
"""

    with open(index_path, 'w', encoding='utf-8') as f:
        f.write(content)


def strip_existing_front_matter(content):
    """Remove existing YAML or TOML front matter from content."""
    # Strip YAML front matter (--- ... ---)
    yaml_pattern = r'^---\s*\n.*?\n---\s*\n'
    content = re.sub(yaml_pattern, '', content, flags=re.DOTALL)

    # Strip TOML front matter (+++ ... +++)
    toml_pattern = r'^\+\+\+\s*\n.*?\n\+\+\+\s*\n'
    content = re.sub(toml_pattern, '', content, flags=re.DOTALL)

    return content


def clean_markdown_content(content):
    """Remove unwanted HTML and markdown attributes."""
    # Remove <br> and <br /> tags
    content = re.sub(r'<br\s*/?>', '', content, flags=re.IGNORECASE)

    # Remove markdown style attributes like {: style="width:70%;"}
    content = re.sub(r'\{:\s*style="[^"]*"\s*\}', '', content)

    return content


def copy_snippets_folder(source_dir, snippet_dest_folder):
    """Copy snippets folder from source to destination.

    Args:
        source_dir: Path to source directory (where mkdocs files are)
        snippet_dest_folder: Destination folder for snippets

    Returns:
        bool: True if snippets folder was found and copied, False otherwise
    """
    snippets_path = os.path.join(source_dir, 'snippets')

    if not os.path.exists(snippets_path) or not os.path.isdir(snippets_path):
        return False

    # Create destination directory if it doesn't exist
    os.makedirs(snippet_dest_folder, exist_ok=True)

    # Copy all files from snippets folder to destination
    for item in os.listdir(snippets_path):
        src_item = os.path.join(snippets_path, item)
        dst_item = os.path.join(snippet_dest_folder, item)

        if os.path.isfile(src_item):
            shutil.copy2(src_item, dst_item)
        elif os.path.isdir(src_item):
            if os.path.exists(dst_item):
                shutil.rmtree(dst_item)
            shutil.copytree(src_item, dst_item)

    return True


def replace_yaml_includes(content, snippet_dest_folder, missing_snippets, md_filename):
    """Replace include blocks with Hugo readfile shortcodes.

    Handles both:
    - MkDocs snippets syntax: --8<-- "path/to/file"
    - Jinja2 include syntax: {% include "path/to/file" %} or {% include 'path/to/file' %}
    - Removes surrounding ```yaml ``` code blocks if present
    - Preserves subdirectory structure (e.g., 'gitops/file.yaml' -> /snippets/gitops/file.yaml)

    Args:
        content: Markdown content
        snippet_dest_folder: Path to snippet destination folder
        missing_snippets: List to track missing snippet references
        md_filename: Name of the markdown file being processed

    Returns:
        str: Content with replaced include blocks
    """
    def replace_include(match):
        snippet_path = match.group(1)
        # Preserve the full relative path (including subdirectories)
        # e.g., 'gitops/kustomization.yaml' stays as 'gitops/kustomization.yaml'

        # Check if snippet exists in destination folder (using full path)
        full_snippet_path = os.path.join(snippet_dest_folder, snippet_path)

        if not os.path.exists(full_snippet_path):
            missing_snippets.append({
                'snippet': snippet_path,
                'referenced_in': md_filename
            })

        # Replace with Hugo readfile shortcode
        # Path /snippets/ is relative to Hugo project root
        # Preserve subdirectory structure in the path
        return f'{{{{< readfile file=/snippets/{snippet_path} code="true" lang="yaml" >}}}}'

    modified_content = content

    # Pattern 1: {% include with surrounding ```yaml ``` block (single or double quotes)
    # Matches: ```yaml\n{% include 'file' %}\n``` or with trailing chars like :
    pattern_yaml_block = r'```yaml\s*\n\s*{%\s*include\s+["\']([^"\']+)["\']\s*%}[^\n]*\n\s*```'
    modified_content = re.sub(pattern_yaml_block, replace_include, modified_content)

    # Pattern 2: {% include without yaml block (single or double quotes)
    pattern_include = r'{%\s*include\s+["\']([^"\']+)["\']\s*%}'
    modified_content = re.sub(pattern_include, replace_include, modified_content)

    # Pattern 3: MkDocs snippets syntax: --8<-- "path/to/file"
    pattern_mkdocs = r'--8<--\s+"([^"]+)"'
    modified_content = re.sub(pattern_mkdocs, replace_include, modified_content)

    return modified_content


def convert_file(src_path, dst_path, metadata, assets_folder, assets_tracking, snippet_dest_folder, missing_snippets):
    """Read source, prepend front matter, rewrite assets, write to destination."""
    # Read source file
    with open(src_path, 'r', encoding='utf-8') as f:
        content = f.read()

    # Strip any existing front matter (including hide_toc metadata)
    content = strip_existing_front_matter(content)

    # Clean markdown content (remove <br>, style attributes, etc.)
    content = clean_markdown_content(content)

    # Generate front matter
    front_matter = generate_front_matter(metadata)

    # Rewrite asset paths
    md_filename = os.path.basename(dst_path)
    content = rewrite_asset_paths(content, assets_folder, assets_tracking, md_filename)

    # Replace YAML include blocks with readfile shortcode
    content = replace_yaml_includes(content, snippet_dest_folder, missing_snippets, md_filename)

    # Combine and write
    output = front_matter + content

    with open(dst_path, 'w', encoding='utf-8') as f:
        f.write(output)


def main():
    parser = argparse.ArgumentParser(description='Convert mkdocs site to Hugo with front matter')
    parser.add_argument('--config', required=True, help='Path to mkdocs.yml file')
    parser.add_argument('--source', required=True, help='Path to directory containing markdown files')
    parser.add_argument('--dest', required=True, help='Destination directory for converted files')
    parser.add_argument('--assets-folder', required=True, help='Path to assets folder (e.g., ./static/)')
    parser.add_argument('--snippet-destination-folder', required=True, help='Destination folder for code snippets')

    args = parser.parse_args()

    # Validate paths
    if not os.path.exists(args.config):
        print(f"Error: Config file not found: {args.config}", file=sys.stderr)
        sys.exit(1)

    if not os.path.isdir(args.source):
        print(f"Error: Source directory not found: {args.source}", file=sys.stderr)
        sys.exit(1)

    if not os.path.isdir(args.assets_folder):
        print(f"Error: Assets folder not found: {args.assets_folder}", file=sys.stderr)
        sys.exit(1)

    # Create destination directory if it doesn't exist
    os.makedirs(args.dest, exist_ok=True)

    # Copy snippets folder if it exists
    print("Checking for snippets folder...")
    snippets_found = copy_snippets_folder(args.source, args.snippet_destination_folder)
    if snippets_found:
        print(f"  Copied snippets folder to {args.snippet_destination_folder}")
    else:
        print("  No snippets folder found in source directory")

    print(f"\nParsing {args.config}...")
    file_metadata, section_metadata = parse_mkdocs_nav(args.config, args.source)

    print(f"Found {len(file_metadata)} files in navigation")
    print(f"Found {len(section_metadata)} directories to create")

    # Scan source directory for all markdown files
    source_path = Path(args.source)
    all_md_files = set()
    for md_file in source_path.rglob('*.md'):
        rel_path = md_file.relative_to(source_path)
        all_md_files.add(str(rel_path))

    # Find unmatched files
    matched_files = set(file_metadata.keys())
    unmatched_files = all_md_files - matched_files

    # Find maximum weight used in matched files
    max_weight = max([meta['weight'] for meta in file_metadata.values()]) if file_metadata else 0

    # Add unmatched files to file_metadata with higher weights
    unmatched_weight = max_weight + 1000  # Start high to ensure they come after matched files
    for unmatched_file in sorted(unmatched_files):
        unmatched_weight += 10
        # Derive title from filename
        filename = os.path.basename(unmatched_file)
        title = os.path.splitext(filename)[0].replace('-', ' ').replace('_', ' ').title()

        file_metadata[unmatched_file] = {
            'title': title,
            'section': None,
            'subsection': None,
            'weight': unmatched_weight
        }

    # Create _index.md files for directories
    print("Creating directory index files...")
    for dir_path, metadata in sorted(section_metadata.items()):
        full_dir_path = os.path.join(args.dest, dir_path)
        os.makedirs(full_dir_path, exist_ok=True)
        create_index_file(full_dir_path, metadata)
        print(f"  Created {dir_path}/_index.md (title=\"{metadata['title']}\", weight={metadata['weight']})")

    # Asset tracking
    assets_tracking = defaultdict(set)

    # Missing snippets tracking
    missing_snippets = []

    # Convert files
    print("\nConverting markdown files...")
    converted_count = 0
    converted_matched = 0
    converted_unmatched = 0
    error_count = 0

    for filepath, metadata in file_metadata.items():
        src_path = os.path.join(args.source, filepath)

        # Preserve directory structure from source
        dst_path = os.path.join(args.dest, filepath)

        # Create parent directory if it doesn't exist
        dst_dir = os.path.dirname(dst_path)
        if dst_dir:
            os.makedirs(dst_dir, exist_ok=True)

        try:
            convert_file(src_path, dst_path, metadata, args.assets_folder, assets_tracking, args.snippet_destination_folder, missing_snippets)
            converted_count += 1

            # Track whether this was matched or unmatched
            if filepath in unmatched_files:
                converted_unmatched += 1
            else:
                converted_matched += 1
        except Exception as e:
            print(f"Error converting {filepath}: {e}", file=sys.stderr)
            error_count += 1

    # Print summary report
    print("\n" + "="*70)
    print("CONVERSION SUMMARY")
    print("="*70)
    print(f"Total files processed: {converted_count}")
    print(f"  - Files in mkdocs.yml nav: {converted_matched}")
    print(f"  - Files NOT in mkdocs.yml nav: {converted_unmatched}")
    print(f"Errors: {error_count}")

    if assets_tracking:
        print("\n" + "-"*70)
        print("ASSETS REPORT")
        print("-"*70)
        for asset_filename in sorted(assets_tracking.keys()):
            for md_filename in sorted(assets_tracking[asset_filename]):
                print(f"{asset_filename}\t{md_filename}")

    if unmatched_files:
        print("\n" + "-"*70)
        print("FILES NOT IN mkdocs.yml nav (migrated with higher weights)")
        print("-"*70)
        for unmatched in sorted(unmatched_files):
            # Get the weight assigned to this file
            weight = file_metadata[unmatched]['weight']
            print(f"  {unmatched} (weight: {weight})")

    if missing_snippets:
        print("\n" + "-"*70)
        print("MISSING SNIPPETS")
        print("-"*70)
        for missing in missing_snippets:
            print(f"  Snippet '{missing['snippet']}' referenced in '{missing['referenced_in']}' not found in {args.snippet_destination_folder}")

    print("\nConversion complete!")


if __name__ == '__main__':
    main()
