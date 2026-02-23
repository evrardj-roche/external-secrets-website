#!/usr/bin/env python3
"""Convert mkdocs site to Hugo with front matter."""

import argparse
import os
import re
import sys
from pathlib import Path
from collections import defaultdict


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
        str: Relative path from assets folder root, or None if not found
    """
    assets_path = Path(assets_folder)

    # Search recursively for the file
    for asset_file in assets_path.rglob('*'):
        if asset_file.is_file() and asset_file.name == asset_filename:
            # Return path relative to assets folder root
            return '/' + str(asset_file.relative_to(assets_path))

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
            if pattern.startswith('!'):
                # Markdown image format
                alt_text = match.group(1)
                asset_path = match.group(2)

                # Extract filename from path
                asset_filename = os.path.basename(asset_path)

                # Skip external URLs
                if asset_path.startswith('http://') or asset_path.startswith('https://'):
                    return match.group(0)

                # Find asset in assets folder
                new_path = find_asset_in_folder(asset_filename, assets_folder)

                if new_path:
                    # Track this asset
                    assets_tracking[asset_filename].add(md_filename)
                    return f'![{alt_text}]({new_path})'
                else:
                    print(f"Warning: Asset '{asset_filename}' referenced in '{md_filename}' not found in assets folder", file=sys.stderr)
                    return match.group(0)
            else:
                # HTML img tag format
                asset_path = match.group(1)
                asset_filename = os.path.basename(asset_path)

                # Skip external URLs
                if asset_path.startswith('http://') or asset_path.startswith('https://'):
                    return match.group(0)

                # Find asset in assets folder
                new_path = find_asset_in_folder(asset_filename, assets_folder)

                if new_path:
                    # Track this asset
                    assets_tracking[asset_filename].add(md_filename)
                    return match.group(0).replace(asset_path, new_path)
                else:
                    print(f"Warning: Asset '{asset_filename}' referenced in '{md_filename}' not found in assets folder", file=sys.stderr)
                    return match.group(0)

        modified_content = re.sub(pattern, replace_asset, modified_content)

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


def convert_file(src_path, dst_path, metadata, assets_folder, assets_tracking):
    """Read source, prepend front matter, rewrite assets, write to destination."""
    # Read source file
    with open(src_path, 'r', encoding='utf-8') as f:
        content = f.read()

    # Generate front matter
    front_matter = generate_front_matter(metadata)

    # Rewrite asset paths
    md_filename = os.path.basename(dst_path)
    content = rewrite_asset_paths(content, assets_folder, assets_tracking, md_filename)

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

    print(f"Parsing {args.config}...")
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

    # Create _index.md files for directories
    print("Creating directory index files...")
    for dir_path, metadata in sorted(section_metadata.items()):
        full_dir_path = os.path.join(args.dest, dir_path)
        os.makedirs(full_dir_path, exist_ok=True)
        create_index_file(full_dir_path, metadata)
        print(f"  Created {dir_path}/_index.md (title=\"{metadata['title']}\", weight={metadata['weight']})")

    # Asset tracking
    assets_tracking = defaultdict(set)

    # Convert files
    print("\nConverting markdown files...")
    converted_count = 0
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
            convert_file(src_path, dst_path, metadata, args.assets_folder, assets_tracking)
            converted_count += 1
        except Exception as e:
            print(f"Error converting {filepath}: {e}", file=sys.stderr)
            error_count += 1

    # Print summary report
    print("\n" + "="*70)
    print("CONVERSION SUMMARY")
    print("="*70)
    print(f"Files processed: {converted_count}")
    print(f"Files skipped: {len(unmatched_files)}")
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
        print("UNMATCHED FILES (not in mkdocs.yml nav)")
        print("-"*70)
        for unmatched in sorted(unmatched_files):
            print(f"  {unmatched}")

    print("\nConversion complete!")


if __name__ == '__main__':
    main()
