package utils

import (
	"io/fs"
	"os"
	"path"
	"strings"
)

type statDirEntry struct {
	info fs.FileInfo
}

type WalkDirFunc func(path string, d fs.DirEntry, err error) error

func WalkDirWithDepth(parent string, root string, fn WalkDirFunc, workDepth int) error {
	fsys := os.DirFS(parent)

	depth := len(strings.Split(root, "/"))
	if depth > workDepth {
		return nil
	}
	info, err := fs.Stat(fsys, root)
	if err != nil {
		if depth == workDepth {
			err = fn(parent, nil, err)
		}
	} else {
		err = walkDirWithDepth(fsys, root, parent, &statDirEntry{info}, fn, depth, workDepth)
	}
	if err == fs.SkipDir {
		return nil
	}
	return err
}

func walkDirWithDepth(fsys fs.FS, name string, parent string, d fs.DirEntry, walkDirFn WalkDirFunc, depth int, workDepth int) error {
	if depth > workDepth {
		return nil
	}

	fullPath := path.Join(parent, name)

	if depth == workDepth {
		if err := walkDirFn(fullPath, d, nil); err != nil || !d.IsDir() {
			if err == fs.SkipDir && d.IsDir() {
				// Successfully skipped directory.
				err = nil
			}
			return err
		}
	}

	dirs, err := fs.ReadDir(fsys, name)
	if err != nil && depth == workDepth {
		// Second call, to report ReadDir error.
		err = walkDirFn(fullPath, d, err)
		if err != nil {
			return err
		}
	}

	for _, d1 := range dirs {
		name1 := path.Join(name, d1.Name())
		if err := walkDirWithDepth(fsys, name1, parent, d1, walkDirFn, depth+1, workDepth); err != nil {
			if err == fs.SkipDir {
				break
			}
			return err
		}
	}
	return nil
}

func (d *statDirEntry) Name() string               { return d.info.Name() }
func (d *statDirEntry) IsDir() bool                { return d.info.IsDir() }
func (d *statDirEntry) Type() fs.FileMode          { return d.info.Mode().Type() }
func (d *statDirEntry) Info() (fs.FileInfo, error) { return d.info, nil }
