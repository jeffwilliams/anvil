package main

import (
	"os"
	"strings"
)

type PathCompleter struct {
	// lfs, rfs and wd fields are for testing.
	lfs, rfs isDirer
	wd       string
	parents  []*GlobalPath
	typ      pathCompleterType
}
type pathCompleterType int

const (
	pathCompleterForUnknownParent = iota
	pathCompleterForWindow
	pathCompleterForColumn
)

type isDirer interface {
	isDir(path string) (bool, error)
}

func NewPathCompleter(parents ...*GlobalPath) *PathCompleter {
	return &PathCompleter{
		parents: parents,
	}
}

func NewPathCompleterWithType(t pathCompleterType, parents ...*GlobalPath) *PathCompleter {
	return &PathCompleter{
		parents: parents,
		typ:     t,
	}
}

func NewPathCompleterForWindow(w *Window) *PathCompleter {
	winPath := w.LoadPath()
	if w.col == nil {
		return NewPathCompleterWithType(pathCompleterForWindow, winPath)
	}

	colPath, ok := w.col.Path()
	if !ok {
		return NewPathCompleterWithType(pathCompleterForWindow, winPath)
	}

	return NewPathCompleterWithType(pathCompleterForWindow, winPath, colPath)
}

func NewPathCompleterForColumn(c *Col) *PathCompleter {
	colPath, ok := c.Path()
	if !ok {
		return NewPathCompleterWithType(pathCompleterForColumn)
	}

	return NewPathCompleterWithType(pathCompleterForColumn, colPath)
}

func (p PathCompleter) NumParents() int {
	return len(p.parents)
}

func (p PathCompleter) CompleteNoCheck(path string) (resolved *GlobalPath) {
	return p.CompleteNoCheckN(path, -1)
}

func (p PathCompleter) CompleteNoCheckN(path string, n int) (resolved *GlobalPath) {
	inpath := NewGlobalPath(path, GlobalPathUnknown)
	if path == "." || path == ".." {
		inpath.SetDirState(GlobalPathIsDir)
	}

	if inpath.IsRemote() || n == 0 {
		resolved = inpath
		return
	}

	var dir *GlobalPath
	if n < 0 || n > len(p.parents) {
		dir = p.Dir()
	} else {
		dir = p.dirWithParents(p.parents[:n])
	}

	if dir.IsRemote() && !inpath.IsRemote() && !isWindowsPath(inpath.Path()) {
		inpath = inpath.GlobalizeRelativeTo(dir)
	}

	if !inpath.IsAbsolute() {
		inpath = inpath.MakeAbsoluteRelativeTo(dir)
	}

	if path == "" {
		inpath.SetDirState(GlobalPathIsDir)
	}

	resolved = inpath
	return
}

func (p PathCompleter) getwd() *GlobalPath {
	if p.wd == "" {
		p.wd = Getwd()
	}
	return NewGlobalPath(p.wd, GlobalPathIsDir)
}

// Dir returns the directory in which file load or execute operations
// should be performed. For operations in a Window, it is the directory
// of the window. For operations
func (pc PathCompleter) Dir() (dir *GlobalPath) {
	return pc.dirWithParents(pc.parents)
}

func (pc PathCompleter) dirWithParents(parents []*GlobalPath) (dir *GlobalPath) {
	switch len(parents) {
	case 0:
		// No window parent. This path must be relative to the editor. Use the current directory.
		dir = pc.getwd()
	case 1:
		dir = pc.dir(parents[0])
	case 2:
		cmpl := NewPathCompleter(parents[1])
		var parent *GlobalPath
		parent = cmpl.CompleteNoCheck(parents[0].String())
		if parents[0].Path() != "" {
			// If the window has no path, then use the dirstate of the column (the result of CompleteNoCheck).
			// However if the window has a path, use the dirstate of that window
			parent.SetDirState(parents[0].DirState())
		}
		dir = pc.dir(parent)
	}
	return

}

func (pc PathCompleter) dir(path *GlobalPath) *GlobalPath {
	p := path.Path()

	if p == "" {
		return pc.getwd()
	}

	if strings.HasSuffix(p, "+Errors") {
		p = p[:len(p)-7]

		result := path.Clone()
		result.SetPath(p)
		result.SetDirState(GlobalPathIsDir)
		return result
	}

	if strings.HasSuffix(p, "+Live") {
		p = p[:len(p)-5]

		result := path.Clone()
		result.SetPath(p)
		result.SetDirState(GlobalPathIsDir)
		return result
	}

	return path.Dir()
}

var cwd string

func Getwd() string {
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			cwd = "."
		}
	}
	return cwd
}

func (p PathCompleter) DirState(path *GlobalPath) (state GlobalPathDirState, err error) {
	if path.DirState() != GlobalPathUnknown {
		return path.DirState(), nil
	}

	lfs := p.getLfs()
	rfs := p.getRfs()

	var isDir bool
	if path.IsRemote() {
		isDir, err = rfs.isDir(path.String())
	} else {
		isDir, err = lfs.isDir(path.String())
	}

	if err != nil {
		return
	}

	if isDir {
		state = GlobalPathIsDir
	} else {
		state = GlobalPathIsFile
	}

	return
}

// These are for testing, not caching.
func (p PathCompleter) getLfs() isDirer {
	if p.lfs != nil {
		return p.lfs
	}
	var lfs localFs
	return lfs
}

func (p PathCompleter) getRfs() isDirer {
	if p.rfs != nil {
		return p.rfs
	}
	rfs := NewSshFs(sshOptsFromSettings())
	return rfs
}

// Complete resolves a path so that it is complete enough to be used to reference a file. An unresolved path
// is a suffix of a path that is relative either to the window that the path is written in; to the editor, meaning
// the editor's current directory. In turn, the window's path can be relative to the column it is in as well.
//
// The input path `path` is resolved relative to the parents in `parents`. The window or editor should be first in
// parents, and if the window is the first element, the column should be second.
//
// This function will return an error if and only if it cannot determine if the path is a file or not
// (it may not exist). When an error is returned, the resolved path is still usable; it just has a dirstate of
// Unknown.
func (p PathCompleter) Complete(path string) (resolved *GlobalPath, err error) {
	resolved = p.CompleteNoCheck(path)
	st, err := p.DirState(resolved)
	if err != nil {
		return
	}
	resolved.SetDirState(st)
	return
}

// CompleteN is like Complete, but only completes using 'n' parents instead of all.
func (p PathCompleter) CompleteN(path string, n int) (resolved *GlobalPath, err error) {
	resolved = p.CompleteNoCheck(path)
	st, err := p.DirState(resolved)
	if err != nil {
		return
	}
	resolved.SetDirState(st)
	return
}

// completeDisplayAndLoadPaths completes the relative path `partialPath` relative to the parent window and column.
// The loadPath is fully completed and so will be absolute (as long as one of the ancestors in the PathCompleter is
// absolute).
//
// The displayPath may be absolute or relative. If there is no column path set, displayPath will be absolute.
// If there is a column path, then displayPath will be relative to the column path (unless partialPath is absolute
// in which case displayPath will also be absolute).
func completeDisplayAndLoadPaths(completer *PathCompleter, partialPath string) (displayPath, loadPath *GlobalPath) {
	loadPath = completer.CompleteNoCheck(partialPath)
	if completer.typ == pathCompleterForColumn {
		displayPath = completer.CompleteNoCheckN(partialPath, 0)
		return
	}

	displayPath = completer.CompleteNoCheck(partialPath)
	return
}
