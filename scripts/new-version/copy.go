package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// CopyDir copies the contents of the directory src into the directory dst.
// If dst does not exist it will be created with the same permission bits as src.
// Behavior:
// - copies files and subdirectories recursively
// - preserves file permission bits and modification times
// - reproduces symlinks as symlinks (does not follow them)
// Usage example:
//
//	err := CopyDir("unreleased", "versionX")
//	if err != nil { log.Fatalf("copy failed: %v", err) }
func CopyDir(src, dst string) error {
	src = filepath.Clean(src)
	dst = filepath.Clean(dst)

	srcInfo, err := os.Lstat(src)
	if err != nil {
		return fmt.Errorf("stat source %q: %w", src, err)
	}
	if !srcInfo.IsDir() {
		return fmt.Errorf("source %q is not a directory", src)
	}

	// create destination root with same permissions as src
	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return fmt.Errorf("create destination %q: %w", dst, err)
	}

	// Walk the source tree
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		// skip the root; it's already created
		if rel == "." {
			return nil
		}

		targetPath := filepath.Join(dst, rel)

		info, err := d.Info()
		if err != nil {
			return err
		}

		// Handle symlinks explicitly (recreate the symlink)
		if info.Mode()&os.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return fmt.Errorf("readlink %q: %w", path, err)
			}
			// remove existing target if present to allow overwrite
			_ = os.Remove(targetPath)
			if err := os.Symlink(linkTarget, targetPath); err != nil {
				return fmt.Errorf("symlink %q -> %q: %w", targetPath, linkTarget, err)
			}
			return nil
		}

		if info.IsDir() {
			// create directory with same mode
			if err := os.MkdirAll(targetPath, info.Mode()); err != nil {
				return fmt.Errorf("mkdir %q: %w", targetPath, err)
			}
			return nil
		}

		// Regular file: copy contents and set mode + modtime
		if err := copyFile(path, targetPath, info.Mode()); err != nil {
			return err
		}
		// preserve modification time
		modTime := info.ModTime()
		if err := os.Chtimes(targetPath, modTime, modTime); err != nil {
			// non-fatal on some platforms, but return error to be strict
			return fmt.Errorf("chtimes %q: %w", targetPath, err)
		}
		return nil
	})
}

func copyFile(srcFile, dstFile string, mode os.FileMode) error {
	// ensure parent dir exists
	if err := os.MkdirAll(filepath.Dir(dstFile), 0o755); err != nil {
		return fmt.Errorf("mkdir parent for %q: %w", dstFile, err)
	}

	in, err := os.Open(srcFile)
	if err != nil {
		return fmt.Errorf("open source file %q: %w", srcFile, err)
	}
	defer in.Close()

	// create destination with the same mode
	out, err := os.OpenFile(dstFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("create destination file %q: %w", dstFile, err)
	}
	defer func() {
		// ensure file is closed and synced
		_ = out.Sync()
		_ = out.Close()
	}()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy %q -> %q: %w", srcFile, dstFile, err)
	}

	// ensure permission bits are set (in case umask changed creation)
	if err := os.Chmod(dstFile, mode); err != nil {
		return fmt.Errorf("chmod %q: %w", dstFile, err)
	}

	// preserve access/mod times using a best-effort approach
	now := time.Now()
	if err := os.Chtimes(dstFile, now, now); err != nil {
		// ignore; not all platforms support Chtimes on all filesystems
	}

	return nil
}
