package main

import (
	"fmt"
	"testing"
)

func TestCompletePath(t *testing.T) {

	tests := []struct {
		name        string
		inputPath   string
		parents     []*GlobalPath
		lfs         testFs
		rfs         testFs
		wd          string
		output      *GlobalPath
		errExpected bool
		noCheck     bool
	}{
		{
			// Completing the empty path will return the working directory
			name:        "empty",
			inputPath:   "",
			parents:     []*GlobalPath{},
			lfs:         newTestFs(),
			rfs:         newTestFs(),
			wd:          "/home/user/dir",
			output:      NewGlobalPath("/home/user/dir", GlobalPathIsDir),
			errExpected: false,
		},
		{
			// Completing a path when we can't determine whether the file is a directory or not
			// will return an error
			name:      "/tmp/a 1",
			inputPath: "a",
			parents: []*GlobalPath{
				NewGlobalPath("/tmp", GlobalPathIsDir),
			},
			lfs:         newTestFs(),
			rfs:         newTestFs(),
			wd:          "/home/user/dir",
			output:      NewGlobalPath("/tmp/a", GlobalPathIsFile),
			errExpected: true,
		},
		{
			// Completing a path when we can determine whether the file is a directory or not
			// should work ok. Test when the path is a directory
			name:      "/tmp/a 2",
			inputPath: "a",
			parents: []*GlobalPath{
				NewGlobalPath("/tmp", GlobalPathIsDir),
			},
			lfs:         newTestFs().Add("/tmp/a", true),
			rfs:         newTestFs(),
			wd:          "/home/user/dir",
			output:      NewGlobalPath("/tmp/a", GlobalPathIsDir),
			errExpected: false,
		},
		{
			// Same as above, but path is a file not a directory
			name:      "/tmp/a 2",
			inputPath: "a",
			parents: []*GlobalPath{
				NewGlobalPath("/tmp", GlobalPathIsDir),
			},
			lfs:         newTestFs().Add("/tmp/a", false),
			rfs:         newTestFs(),
			wd:          "/home/user/dir",
			output:      NewGlobalPath("/tmp/a", GlobalPathIsFile),
			errExpected: false,
		},
		{
			name:      "host:/tmp/a 1",
			inputPath: "a",
			parents: []*GlobalPath{
				NewGlobalPath("host:/tmp", GlobalPathIsDir),
			},
			lfs:         newTestFs(),
			rfs:         newTestFs().Add("host:/tmp/a", false),
			wd:          "/home/user/dir",
			output:      NewGlobalPath("host:/tmp/a", GlobalPathIsFile),
			errExpected: false,
		},
		{
			// Test when the partial path contains a subdirectory
			name:      "/usr/share/app/file.xml",
			inputPath: "app/file.xml",
			parents: []*GlobalPath{
				NewGlobalPath("/usr/share", GlobalPathIsDir),
			},
			lfs:         newTestFs().Add("/usr/share/app/file.xml", false),
			rfs:         newTestFs(),
			wd:          "/home/user/dir",
			output:      NewGlobalPath("/usr/share/app/file.xml", GlobalPathIsFile),
			errExpected: false,
		},
		{
			// Resolve with a column
			name:      "col resolve",
			inputPath: "sub/file.xml",
			parents: []*GlobalPath{
				NewGlobalPath("app", GlobalPathIsDir),
				NewGlobalPath("/usr/share", GlobalPathIsDir),
			},
			lfs:         newTestFs().Add("/usr/share/app/sub/file.xml", false),
			rfs:         newTestFs(),
			wd:          "/home/user/dir",
			output:      NewGlobalPath("/usr/share/app/sub/file.xml", GlobalPathIsFile),
			errExpected: false,
		},
		{
			// Test the user setting a windows path to '.' when there is a column path.
			name:      "dot",
			inputPath: ".",
			parents: []*GlobalPath{
				NewGlobalPath("/usr/share", GlobalPathIsDir),
			},
			lfs:         newTestFs(),
			rfs:         newTestFs(),
			wd:          "/home/user/dir",
			output:      NewGlobalPath("/usr/share", GlobalPathIsDir),
			errExpected: false,
			noCheck:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			completer := NewPathCompleter(tc.parents...)
			completer.lfs = tc.lfs
			completer.rfs = tc.rfs
			completer.wd = tc.wd
			var actual *GlobalPath
			var err error
			if tc.noCheck {
				actual = completer.CompleteNoCheck(tc.inputPath)
			} else {
				actual, err = completer.Complete(tc.inputPath)
			}
			if err != nil {
				if !tc.errExpected {
					t.Fatalf("got error on complete: %v", err)
				}
				if actual.DirState() != GlobalPathUnknown {
					t.Fatalf("Got an error completing a path, but the returned resolved path does not have dirstate unknown")
				}
				return
			}

			if *actual != *tc.output {
				t.Fatalf("expected '%s' but got '%s' (%#v versus %#v)", tc.output, actual, tc.output, actual)
			}
		})
	}
}

// testFs is used to represent a filesystem in PathCompleter. You can add entries
// for directories that say whether a file is a directory or not. If it is queried for
// a path that is not defined, an error is returned: this allows you to simulate
// the fs encountering an error
type testFs struct {
	dirs map[string]bool
}

func newTestFs() testFs {
	return testFs{make(map[string]bool)}
}

func (t testFs) Add(path string, isDir bool) testFs {
	t.dirs[path] = isDir
	return t
}

func (t testFs) isDir(p string) (bool, error) {
	b, ok := t.dirs[p]

	if !ok {
		return false, fmt.Errorf("error getting directory state for '%s'", p)
	}

	return b, nil
}

func TestPathCompleterDir(t *testing.T) {

	tests := []struct {
		name    string
		parents []*GlobalPath
		wd      string
		output  *GlobalPath
	}{
		{
			name:    "empty",
			parents: []*GlobalPath{},
			wd:      "/home/user/dir",
			output:  NewGlobalPath("/home/user/dir", GlobalPathIsDir),
		},
		{
			name: "window is dir",
			parents: []*GlobalPath{
				NewGlobalPath("/path/to/project", GlobalPathIsDir),
			},
			wd:     "/home/user/dir",
			output: NewGlobalPath("/path/to/project", GlobalPathIsDir),
		},
		{
			name: "window is file",
			parents: []*GlobalPath{
				NewGlobalPath("/path/to/project/file.txt", GlobalPathIsFile),
			},
			wd:     "/home/user/dir",
			output: NewGlobalPath("/path/to/project", GlobalPathIsDir),
		},
		{
			name: "window is errors",
			parents: []*GlobalPath{
				NewGlobalPath("/path/to/project+Errors", GlobalPathIsFile),
			},
			wd:     "/home/user/dir",
			output: NewGlobalPath("/path/to/project", GlobalPathIsDir),
		},
		{
			name: "window is relative with subdir",
			parents: []*GlobalPath{
				NewGlobalPath("subdir/file.txt", GlobalPathIsFile),
				NewGlobalPath("/path/to/project", GlobalPathIsDir),
			},
			wd:     "/home/user/dir",
			output: NewGlobalPath("/path/to/project/subdir", GlobalPathIsDir),
		},
		{
			name: "window is relative no subdir",
			parents: []*GlobalPath{
				NewGlobalPath("file.txt", GlobalPathIsFile),
				NewGlobalPath("/path/to/project", GlobalPathIsDir),
			},
			wd:     "/home/user/dir",
			output: NewGlobalPath("/path/to/project", GlobalPathIsDir),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			completer := NewPathCompleter(tc.parents...)
			completer.lfs = newTestFs()
			completer.rfs = newTestFs()
			completer.wd = tc.wd
			actual := completer.Dir()
			if *actual != *tc.output {
				t.Fatalf("expected '%s' but got '%s' (%#v versus %#v)", tc.output, actual, tc.output, actual)
			}
		})
	}
}
