package main

import (
	"testing"
)

func TestPathConstruction(t *testing.T) {
	tests := []struct {
		name               string
		configuredBuildDir string
		sourceFile         string
		// The value that will be written to the field `directory` in compile_commands.json
		expectedBuildDir string
		// The value that will be written to the field `file` in compile_commands.json
		expectedFilePath string
	}{
		{
			name:               "no build dir",
			configuredBuildDir: "",
			sourceFile:         "/workspace/src/file.c",
			expectedBuildDir:   "/workspace/src/",
			expectedFilePath:   "file.c",
		},
		{
			name:               "build dir build/",
			configuredBuildDir: "/workspace/build/",
			sourceFile:         "/workspace/src/file.c",
			expectedBuildDir:   "/workspace/build/",
			expectedFilePath:   "../src/file.c",
		},
		{
			name:               "build dir build/",
			configuredBuildDir: "/workspace/src/",
			sourceFile:         "/workspace/src/file.c",
			expectedBuildDir:   "/workspace/src/",
			expectedFilePath:   "file.c",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			g := DbGenerator{buildDir: test.configuredBuildDir}
			d := g.buildDirFor(test.sourceFile)

			if d != test.expectedBuildDir {
				t.Fatalf("Expected build dir '%s' but got '%s'\n", test.expectedBuildDir, d)
			}

			f, err := g.filePathFromBuildDir(d, test.sourceFile)
			if err != nil {
				t.Fatalf("Error getting file path: %v\n", err)
			}
			if f != test.expectedFilePath {
				t.Fatalf("Expected file path '%s' but got '%s'\n", test.expectedFilePath, f)
			}
		})
	}

	g := DbGenerator{buildDir: "/workspace/build"}
	if "/workspace/build" != g.buildDirFor("/workspace/src/file.c") {
		t.Fatalf("")
	}
}
