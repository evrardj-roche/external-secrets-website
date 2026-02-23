#!/usr/bin/env python3
"""Tests for convert_mkdocs_to_hugo.py conversion functions."""

import os
import tempfile
import shutil
import unittest
from pathlib import Path
from collections import defaultdict

# Import functions from the converter
from convert_mkdocs_to_hugo import (
    strip_existing_front_matter,
    clean_markdown_content,
    generate_front_matter,
    replace_yaml_includes,
    find_asset_in_folder,
    copy_snippets_folder,
)


class TestStripFrontMatter(unittest.TestCase):
    """Test stripping of YAML and TOML front matter."""

    def test_strip_yaml_front_matter(self):
        content = """---
title: Test
hide_toc: true
---

# Hello World
"""
        expected = "# Hello World\n"
        result = strip_existing_front_matter(content)
        self.assertEqual(result, expected)

    def test_strip_toml_front_matter(self):
        content = """+++
title = "Test"
weight = 10
+++

# Hello World
"""
        expected = "# Hello World\n"
        result = strip_existing_front_matter(content)
        self.assertEqual(result, expected)

    def test_no_front_matter(self):
        content = "# Hello World\n\nSome content"
        result = strip_existing_front_matter(content)
        self.assertEqual(result, content)


class TestCleanMarkdownContent(unittest.TestCase):
    """Test cleaning of markdown content."""

    def test_remove_br_tags(self):
        content = "Line 1<br>Line 2<br />Line 3<BR>Line 4"
        expected = "Line 1Line 2Line 3Line 4"
        result = clean_markdown_content(content)
        self.assertEqual(result, expected)

    def test_remove_style_attributes(self):
        content = '![image](path.png){: style="width:70%;"}'
        expected = '![image](path.png)'
        result = clean_markdown_content(content)
        self.assertEqual(result, expected)

    def test_multiple_style_attributes(self):
        content = '''
![img1](a.png){: style="width:50%;"}
Some text
![img2](b.png){: style="height:100px;"}
'''
        expected = '''
![img1](a.png)
Some text
![img2](b.png)
'''
        result = clean_markdown_content(content)
        self.assertEqual(result, expected)


class TestGenerateFrontMatter(unittest.TestCase):
    """Test TOML front matter generation."""

    def test_basic_front_matter(self):
        metadata = {
            'title': 'Test Page',
            'weight': 10
        }
        result = generate_front_matter(metadata)
        self.assertIn('+++', result)
        self.assertIn('title = "Test Page"', result)
        self.assertIn('linkTitle = "Test Page"', result)
        self.assertIn('weight = 10', result)

    def test_front_matter_with_special_chars(self):
        metadata = {
            'title': 'Test "Quoted" Page',
            'weight': 20
        }
        result = generate_front_matter(metadata)
        self.assertIn('title = "Test "Quoted" Page"', result)


class TestReplaceYamlIncludes(unittest.TestCase):
    """Test replacement of include blocks with Hugo readfile shortcodes."""

    def setUp(self):
        # Create temporary snippet folder for testing
        self.temp_dir = tempfile.mkdtemp()
        self.snippet_folder = os.path.join(self.temp_dir, 'snippets')
        os.makedirs(self.snippet_folder)

        # Create some test snippet files
        with open(os.path.join(self.snippet_folder, 'test.yaml'), 'w') as f:
            f.write('test: content')
        with open(os.path.join(self.snippet_folder, 'example.yaml'), 'w') as f:
            f.write('example: data')

    def tearDown(self):
        shutil.rmtree(self.temp_dir)

    def test_jinja_include_with_yaml_block_single_quotes(self):
        content = '''```yaml
{% include 'test.yaml' %}
```'''
        missing_snippets = []
        result = replace_yaml_includes(content, self.snippet_folder, missing_snippets, 'test.md')

        expected = '{{% readfile file="test.yaml" %}}'
        self.assertEqual(result, expected)
        self.assertEqual(len(missing_snippets), 0)

    def test_jinja_include_with_yaml_block_double_quotes(self):
        content = '''```yaml
{% include "example.yaml" %}
```'''
        missing_snippets = []
        result = replace_yaml_includes(content, self.snippet_folder, missing_snippets, 'test.md')

        expected = '{{% readfile file="example.yaml" %}}'
        self.assertEqual(result, expected)
        self.assertEqual(len(missing_snippets), 0)

    def test_jinja_include_without_yaml_block(self):
        content = '{% include "test.yaml" %}'
        missing_snippets = []
        result = replace_yaml_includes(content, self.snippet_folder, missing_snippets, 'test.md')

        expected = '{{% readfile file="test.yaml" %}}'
        self.assertEqual(result, expected)
        self.assertEqual(len(missing_snippets), 0)

    def test_mkdocs_snippets_syntax(self):
        content = '--8<-- "test.yaml"'
        missing_snippets = []
        result = replace_yaml_includes(content, self.snippet_folder, missing_snippets, 'test.md')

        expected = '{{% readfile file="test.yaml" %}}'
        self.assertEqual(result, expected)
        self.assertEqual(len(missing_snippets), 0)

    def test_multiple_includes_in_content(self):
        content = '''
```yaml
{% include 'test.yaml' %}
```

Some text here

{% include "example.yaml" %}

More text

--8<-- "test.yaml"
'''
        missing_snippets = []
        result = replace_yaml_includes(content, self.snippet_folder, missing_snippets, 'test.md')

        # All includes should be replaced
        self.assertNotIn('{% include', result)
        self.assertNotIn('--8<--', result)
        self.assertNotIn('```yaml', result)
        self.assertEqual(result.count('{{% readfile'), 3)
        self.assertEqual(len(missing_snippets), 0)

    def test_missing_snippet_is_tracked(self):
        content = '{% include "missing.yaml" %}'
        missing_snippets = []
        result = replace_yaml_includes(content, self.snippet_folder, missing_snippets, 'test.md')

        # Should still replace but track as missing
        expected = '{{% readfile file="missing.yaml" %}}'
        self.assertEqual(result, expected)
        self.assertEqual(len(missing_snippets), 1)
        self.assertEqual(missing_snippets[0]['snippet'], 'missing.yaml')
        self.assertEqual(missing_snippets[0]['referenced_in'], 'test.md')

    def test_nested_path_extracts_basename(self):
        # Even if include has a path, we should extract just the filename
        content = '{% include "path/to/test.yaml" %}'
        missing_snippets = []
        result = replace_yaml_includes(content, self.snippet_folder, missing_snippets, 'test.md')

        # Should use just the basename
        expected = '{{% readfile file="test.yaml" %}}'
        self.assertEqual(result, expected)
        self.assertEqual(len(missing_snippets), 0)


class TestFindAssetInFolder(unittest.TestCase):
    """Test finding assets in the assets folder."""

    def setUp(self):
        # Create temporary assets folder structure
        self.temp_dir = tempfile.mkdtemp()
        self.assets_folder = os.path.join(self.temp_dir, 'assets')
        os.makedirs(os.path.join(self.assets_folder, 'images'))
        os.makedirs(os.path.join(self.assets_folder, 'pictures'))

        # Create test files
        Path(os.path.join(self.assets_folder, 'logo.png')).touch()
        Path(os.path.join(self.assets_folder, 'images', 'hero.jpg')).touch()
        Path(os.path.join(self.assets_folder, 'pictures', 'test image.png')).touch()

    def tearDown(self):
        shutil.rmtree(self.temp_dir)

    def test_find_asset_in_root(self):
        result = find_asset_in_folder('logo.png', self.assets_folder)
        self.assertEqual(result, '/logo.png')

    def test_find_asset_in_subdirectory(self):
        result = find_asset_in_folder('hero.jpg', self.assets_folder)
        self.assertEqual(result, '/images/hero.jpg')

    def test_find_asset_with_spaces(self):
        # Filename with space
        result = find_asset_in_folder('test image.png', self.assets_folder)
        self.assertEqual(result, '/pictures/test%20image.png')

    def test_find_asset_with_url_encoded_input(self):
        # Input is URL-encoded
        result = find_asset_in_folder('test%20image.png', self.assets_folder)
        self.assertEqual(result, '/pictures/test%20image.png')

    def test_asset_not_found(self):
        result = find_asset_in_folder('nonexistent.png', self.assets_folder)
        self.assertIsNone(result)


class TestCopySnippetsFolder(unittest.TestCase):
    """Test copying snippets folder."""

    def setUp(self):
        self.temp_dir = tempfile.mkdtemp()
        self.source_dir = os.path.join(self.temp_dir, 'source')
        self.dest_dir = os.path.join(self.temp_dir, 'dest')
        os.makedirs(self.source_dir)
        os.makedirs(self.dest_dir)

    def tearDown(self):
        shutil.rmtree(self.temp_dir)

    def test_copy_snippets_folder_exists(self):
        # Create snippets folder with content
        snippets_dir = os.path.join(self.source_dir, 'snippets')
        os.makedirs(snippets_dir)

        test_file = os.path.join(snippets_dir, 'test.yaml')
        with open(test_file, 'w') as f:
            f.write('test: content')

        # Copy snippets
        snippet_dest = os.path.join(self.dest_dir, 'snippets')
        result = copy_snippets_folder(self.source_dir, snippet_dest)

        self.assertTrue(result)
        self.assertTrue(os.path.exists(os.path.join(snippet_dest, 'test.yaml')))

        # Verify content
        with open(os.path.join(snippet_dest, 'test.yaml'), 'r') as f:
            content = f.read()
        self.assertEqual(content, 'test: content')

    def test_copy_snippets_folder_with_subdirectories(self):
        # Create snippets folder with subdirectories
        snippets_dir = os.path.join(self.source_dir, 'snippets')
        sub_dir = os.path.join(snippets_dir, 'provider')
        os.makedirs(sub_dir)

        with open(os.path.join(sub_dir, 'config.yaml'), 'w') as f:
            f.write('config: data')

        # Copy snippets
        snippet_dest = os.path.join(self.dest_dir, 'snippets')
        result = copy_snippets_folder(self.source_dir, snippet_dest)

        self.assertTrue(result)
        self.assertTrue(os.path.exists(os.path.join(snippet_dest, 'provider', 'config.yaml')))

    def test_copy_snippets_folder_not_exists(self):
        # No snippets folder in source
        snippet_dest = os.path.join(self.dest_dir, 'snippets')
        result = copy_snippets_folder(self.source_dir, snippet_dest)

        self.assertFalse(result)


class TestIntegrationScenarios(unittest.TestCase):
    """Integration tests for complete conversion scenarios."""

    def test_complete_content_transformation(self):
        """Test a complete transformation with all features."""
        content = """---
title: Old Title
hide_toc: true
---

# Provider Configuration

Here's the configuration:<br>

```yaml
{% include 'config.yaml' %}
```

And another one:

{% include "secret.yaml" %}

Style test{: style="width:50%;"}

--8<-- "snippet.yaml"
"""

        # Create temp snippet folder
        temp_dir = tempfile.mkdtemp()
        snippet_folder = os.path.join(temp_dir, 'snippets')
        os.makedirs(snippet_folder)

        # Create snippet files
        for filename in ['config.yaml', 'secret.yaml', 'snippet.yaml']:
            with open(os.path.join(snippet_folder, filename), 'w') as f:
                f.write(f'{filename}: data')

        try:
            missing_snippets = []

            # Strip front matter
            content = strip_existing_front_matter(content)

            # Clean markdown
            content = clean_markdown_content(content)

            # Replace includes
            content = replace_yaml_includes(content, snippet_folder, missing_snippets, 'test.md')

            # Verify transformations
            self.assertNotIn('---', content)  # Front matter removed
            self.assertNotIn('hide_toc', content)  # Metadata removed
            self.assertNotIn('<br>', content)  # BR tags removed
            self.assertNotIn('{: style=', content)  # Style attributes removed
            self.assertNotIn('{% include', content)  # Jinja includes replaced
            self.assertNotIn('--8<--', content)  # MkDocs snippets replaced
            self.assertNotIn('```yaml', content)  # YAML blocks around includes removed

            # Should have 3 readfile shortcodes
            self.assertEqual(content.count('{{% readfile'), 3)
            self.assertIn('{{% readfile file="config.yaml" %}}', content)
            self.assertIn('{{% readfile file="secret.yaml" %}}', content)
            self.assertIn('{{% readfile file="snippet.yaml" %}}', content)

            # No missing snippets
            self.assertEqual(len(missing_snippets), 0)

        finally:
            shutil.rmtree(temp_dir)


if __name__ == '__main__':
    # Run tests with verbose output
    unittest.main(verbosity=2)
