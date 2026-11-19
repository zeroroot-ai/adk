// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package agentskill

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// ReadFolder reads the regular files of the folder at dir. The name of the
// folder is the last element of the path. It returns a problem for each entry
// that it cannot use, such as a link. It never reads a file that is over a
// limit, so a large file does not use memory. The error is not nil only when
// the folder cannot be read at all.
func ReadFolder(dir string) (name string, files []File, problems []string, err error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", nil, nil, fmt.Errorf("resolve %q: %w", dir, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", nil, nil, fmt.Errorf("read the folder: %w", err)
	}
	if !info.IsDir() {
		return "", nil, nil, fmt.Errorf("%q is not a folder", dir)
	}
	name = filepath.Base(abs)

	var total int64
	count := 0
	walkErr := filepath.WalkDir(abs, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(abs, p)
		if err != nil {
			return fmt.Errorf("make %q relative to the folder: %w", p, err)
		}
		rel = filepath.ToSlash(rel)
		if !d.Type().IsRegular() {
			problems = append(problems, fmt.Sprintf("the entry %q is not a regular file", rel))
			return nil
		}
		count++
		if count > MaxFiles {
			// The count problem comes from Check. Stop the read here.
			return filepath.SkipAll
		}
		fi, err := d.Info()
		if err != nil {
			return fmt.Errorf("stat %q: %w", rel, err)
		}
		if fi.Size() > MaxFileBytes || total+fi.Size() > MaxFolderBytes {
			// Report the limit here. Do not read the content. The marker
			// keeps the file in the list so the name check still sees it.
			files = append(files, File{Path: rel, Content: make([]byte, 0)})
			problems = append(problems, fmt.Sprintf("the file %q has %d bytes and is over a size limit", rel, fi.Size()))
			return nil
		}
		data, err := os.ReadFile(p) //nolint:gosec // G304: the path comes from the walk of the folder the user named.
		if err != nil {
			return fmt.Errorf("read %q: %w", rel, err)
		}
		total += int64(len(data))
		files = append(files, File{Path: rel, Content: data})
		return nil
	})
	if walkErr != nil {
		return "", nil, nil, fmt.Errorf("read the folder: %w", walkErr)
	}
	if count > MaxFiles {
		problems = append(problems, fmt.Sprintf("the folder has more than %d files", MaxFiles))
	}
	return name, files, problems, nil
}
