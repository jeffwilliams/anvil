// adiffplumb is meant to be invoked from an Anvil plumbing rule of this form:
//
// match @@ -\d+,\d+ \+\d+,\d+ @@
// do adiffplumb
//
// When a Git diff hunk line (the line starting with @@) is plumbed, this program will acquire the source file referenced by that line in Anvil.
//
// Limitations:
//
//   - The diff contents must be contained in a window who's path is within the git repository that the diff was taken from.
package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jeffwilliams/anvil/api/go/anvil"
)

var (
	httpApi anvil.Anvil
	win     anvil.Window
)

func main() {
	var err error
	httpApi, err = anvil.NewFromEnv()
	dieIfError(err, "connecting to API failed")
	getWindow()

	sels, err := httpApi.WindowBodySelections(win)
	dieIfError(err, "getting selections failed")
	if len(sels) != 1 {
		die("adiffplum expects exactly one hunk range line in the diff of the form `@@ ...` to be selected")
	}

	body, err := httpApi.WindowBody(win)
	dieIfError(err, "getting window body failed")

	content, err := ioutil.ReadAll(body)
	dieIfError(err, "reading window body failed")

	contentStr := string(content)
	lines := strings.Split(contentStr, "\n")
	runes := []rune(contentStr)
	hunkRangeLineContent := string(runes[sels[0].Start:sels[0].End])
	hunkRangeLine := lineContainingPosition(lines, sels[0].Start)
	lineStart, _ := getHunkRangeLineOffsets(hunkRangeLineContent)

	path := getPathOfFileBeingDiffed(lines, hunkRangeLine)
	if path == "" {
		die("can't find `diff --git` line")
	}

	cmd := fmt.Sprintf("Acq %s:%d", path, lineStart)
	//fmt.Printf("command: Acq %s:%d", path, lineStart)
	httpApi.ExecuteInWin(win, cmd, nil)
}

func getHunkRangeLineOffsets(line string) (lineStart, lineEnd int) {
	r, err := regexp.Compile(`@@ -\d+,\d+ \+(\d+),(\d+) @@`)
	dieIfError(err, "compiling regexp failed")

	match := r.FindStringSubmatch(line)
	if match == nil {
		die("selected line doesn't seem to be a hunk range line of the form `@@ ...`")
	}

	s, err := strconv.Atoi(match[1])
	dieIfError(err, "selected line doesn't seem to be a hunk range line of the form `@@ ...`: first number is not a number")

	l, err := strconv.Atoi(match[2])
	dieIfError(err, "selected line doesn't seem to be a hunk range line of the form `@@ ...`: second number is not a number")

	lineStart = s
	lineEnd = s + l
	return
}

func lineContainingPosition(lines []string, pos int) (line int) {
	// This is slow.

	for pos >= 0 {
		if pos < len(lines[line]) {
			return
		}
		line++
		pos -= utf8.RuneCountInString(lines[line])
	}
	return
}

func getPathOfFileBeingDiffed(lines []string, hunkRangeLine int) (path string) {
	for i := hunkRangeLine; i >= 0; i-- {
		if strings.HasPrefix(lines[i], "diff --git") {
			fields := strings.Fields(lines[i])
			file1 := fields[2]
			if strings.HasPrefix(file1, "a/") {
				file1 = file1[2:]
			}

			gitParentDir, err := ancestorDirectoryContaining(".", ".git")
			dieIfError(err, "Can't find ancestor directory of %s that contains .git")
			return filepath.Join(gitParentDir, file1)
		}
	}

	return ""
}

func ancestorDirectoryContaining(startPath, file string) (string, error) {
	path, err := filepath.Abs(startPath)
	if err != nil {
		return "", err
	}

	// If a file is provided, start from its directory
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		path = filepath.Dir(path)
	}

	for {
		d := filepath.Join(path, file)
		if stat, err := os.Stat(d); err == nil && stat.IsDir() {
			return path, nil
		}

		parent := filepath.Dir(path)
		if parent == path {
			// Reached filesystem root
			break
		}
		path = parent
	}

	return "", os.ErrNotExist
}

func getWindow() {
	winId := os.Getenv("ANVIL_WIN_ID")
	id, err := strconv.Atoi(winId)
	dieIfError(err, fmt.Sprintf("converting window id '%s' to int failed", winId))
	win, err = httpApi.Window(id)
	dieIfError(err, fmt.Sprintf("getting window %d failed", id))
}

func dieIfError(err error, msg string) {
	if err != nil {
		msg := fmt.Sprintf("%s: %s", msg, err)
		die(msg)
	}
}

func die(msg string) {
	fmt.Fprintf(os.Stderr, "adiffplumb: %s\n", msg)
	os.Exit(1)
}
