package main

import "testing"

func TestGlobalPath(t *testing.T) {

	tests := []struct {
		name   string
		input  string
		global GlobalPath
		output string
	}{
		{
			name:  "file.c",
			input: "file.c",
			global: GlobalPath{
				user: "",
				host: "",
				path: "file.c",
				port: "",
			},
			output: "file.c",
		},
		{
			name:  "/file.c",
			input: "/file.c",
			global: GlobalPath{
				user: "",
				host: "",
				path: "/file.c",
				port: "",
			},
			output: "/file.c",
		},
		{
			name:  "host:file.c",
			input: "host:file.c",
			global: GlobalPath{
				user: "",
				host: "host",
				path: "file.c",
				port: "",
			},
			output: "host:file.c",
		},
		{
			name:  "host:/file.c",
			input: "host:/file.c",
			global: GlobalPath{
				user: "",
				host: "host",
				path: "/file.c",
				port: "",
			},
			output: "host:/file.c",
		},
		{
			name:  "host:22:file.c",
			input: "host:22:file.c",
			global: GlobalPath{
				user: "",
				host: "host",
				path: "file.c",
				port: "22",
			},
			output: "host:22:file.c",
		},
		{
			name:  "user@host:path/file.c",
			input: "user@host:path/file.c",
			global: GlobalPath{
				user: "user",
				host: "host",
				path: "path/file.c",
				port: "",
			},
			output: "user@host:path/file.c",
		},
		{
			name:  "user@host:22:path/file.c",
			input: "user@host:22:path/file.c",
			global: GlobalPath{
				user: "user",
				host: "host",
				path: "path/file.c",
				port: "22",
			},
			output: "user@host:22:path/file.c",
		},
		{
			name:  "host:22:path/file_wth_@.c",
			input: "host:22:path/file_wth_@.c",
			global: GlobalPath{
				user: "",
				host: "host",
				path: "path/file_wth_@.c",
				port: "22",
			},
			output: "host:22:path/file_wth_@.c",
		},
		{
			name:  "host%proxy:path/file_wth_@.c",
			input: "host%proxy:path/file_wth_@.c",
			global: GlobalPath{
				user:      "",
				host:      "host",
				path:      "path/file_wth_@.c",
				proxyHost: "proxy",
			},
			output: "host%proxy:path/file_wth_@.c",
		},
		{
			name:  "host:22%proxy:path/file_wth_@.c",
			input: "host:22%proxy:path/file_wth_@.c",
			global: GlobalPath{
				user:      "",
				host:      "host",
				path:      "path/file_wth_@.c",
				port:      "22",
				proxyHost: "proxy",
			},
			output: "host:22%proxy:path/file_wth_@.c",
		},
		{
			name:  "host:22%user@proxy:path/file_wth_@.c",
			input: "host:22%user@proxy:path/file_wth_@.c",
			global: GlobalPath{
				user:      "",
				host:      "host",
				path:      "path/file_wth_@.c",
				port:      "22",
				proxyHost: "proxy",
				proxyUser: "user",
			},
			output: "host:22%user@proxy:path/file_wth_@.c",
		},
		{
			name:  "host:22%proxy:56:path/file_wth_@.c",
			input: "host:22%proxy:56:path/file_wth_@.c",
			global: GlobalPath{
				user:      "",
				host:      "host",
				path:      "path/file_wth_@.c",
				port:      "22",
				proxyHost: "proxy",
				proxyPort: "56",
			},
			output: "host:22%proxy:56:path/file_wth_@.c",
		},
		{
			name:  "host:22%user@proxy:path/file_wth_@.c",
			input: "host:22%user@proxy:path/file_wth_@.c",
			global: GlobalPath{
				user:      "",
				host:      "host",
				path:      "path/file_wth_@.c",
				port:      "22",
				proxyHost: "proxy",
				proxyUser: "user",
			},
			output: "host:22%user@proxy:path/file_wth_@.c",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			g := NewGlobalPath(tc.input, GlobalPathUnknown)

			if *g != tc.global {
				t.Fatalf("Parsed path does not match. Expected %#v but got %#v", tc.global, g)
			}

			if g.String() != tc.output {
				t.Fatalf("Reassembled path does not match. Expected %#v but got %#v", tc.output, g.String())
			}
		})
	}
}

func TestGlobalPathMakeAbsoluteRelativeTo(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		relativeTo string
		result     GlobalPath
	}{
		{
			name:       "/tmp/file.c",
			path:       "file.c",
			relativeTo: "/tmp",
			result: GlobalPath{
				user:     "",
				host:     "",
				path:     "/tmp/file.c",
				port:     "",
				dirState: GlobalPathIsFile,
			},
		},
		{
			name:       "/tmp/file.c 2",
			path:       "file.c",
			relativeTo: "/tmp/",
			result: GlobalPath{
				user:     "",
				host:     "",
				path:     "/tmp/file.c",
				port:     "",
				dirState: GlobalPathIsFile,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			path := NewGlobalPath(tc.path, GlobalPathIsFile)
			relativeTo := NewGlobalPath(tc.relativeTo, GlobalPathIsDir)

			path = path.MakeAbsoluteRelativeTo(relativeTo)

			if *path != tc.result {
				t.Fatalf("Result does not match expected result. Expected %#v but got %#v", tc.result, path)
			}

		})
	}
}

// Test the method GlobalPath.MakeRelativeUsingPrefix. This is used when we drag a window
// from one column to another.
func TestGlobalPathMakeRelativeUsingPrefix(t *testing.T) {
	tests := []struct {
		name   string
		path   *GlobalPath
		prefix *GlobalPath
		result *GlobalPath
	}{
		{
			name:   "file.c",
			path:   NewGlobalPath("file.c", GlobalPathIsFile),
			prefix: NewGlobalPath("/tmp/path.go", GlobalPathIsFile),
			result: nil,
		},
		{
			name:   "prefix-doesnt-match",
			path:   NewGlobalPath("/usr/bin/cmd", GlobalPathIsFile),
			prefix: NewGlobalPath("/tmp", GlobalPathIsDir),
			result: nil,
		},
		{
			name:   "/tmp/file.c",
			path:   NewGlobalPath("/tmp/file.c", GlobalPathIsFile),
			prefix: NewGlobalPath("/tmp", GlobalPathIsDir),
			result: &GlobalPath{
				user:     "",
				host:     "",
				path:     "file.c",
				port:     "",
				dirState: GlobalPathIsFile,
			},
		},
		{
			name:   "/tmp/file.c-with-slash",
			path:   NewGlobalPath("/tmp/file.c", GlobalPathIsFile),
			prefix: NewGlobalPath("/tmp/", GlobalPathIsDir),
			result: &GlobalPath{
				user:     "",
				host:     "",
				path:     "file.c",
				port:     "",
				dirState: GlobalPathIsFile,
			},
		},
		{
			name:   "remote 1",
			path:   NewGlobalPath("host:/tmp/file.c", GlobalPathIsFile),
			prefix: NewGlobalPath("host:/tmp/", GlobalPathIsDir),
			result: &GlobalPath{
				user:     "",
				host:     "",
				path:     "file.c",
				port:     "",
				dirState: GlobalPathIsFile,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			path := tc.path.MakeRelativeUsingPrefix(tc.prefix)

			if path == nil && tc.result == nil {
				return
			}

			if tc.result == nil && path != nil {
				t.Fatalf("Result does not match expected result. Expected nil result but got %#v", path)
			}

			if path == nil && tc.result != nil {
				t.Fatalf("Result does not match expected result. Expected %#v but got nil", tc.result)
			}

			if *path != *tc.result {
				t.Fatalf("Result does not match expected result. Expected %#v but got %#v", tc.result, path)
			}

		})
	}
}
