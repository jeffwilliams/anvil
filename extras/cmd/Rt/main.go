package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jeffwilliams/anvil/api/go/anvil"

	"github.com/ogier/pflag"
)

// Ctags format: http://ctags.sourceforge.net/FORMAT

func main() {
	pflag.Parse()

	if pflag.NArg() == 0 {
		fmt.Printf("Rt: Pass the symbol to find as the first argument\n")
		return
	}

	tag := pflag.Arg(0)

	var ldr AnvilLoader
	anvil, err := anvil.NewFromEnv()
	if err != nil {
		fmt.Printf("Rt: loading anvil API failed: %v. Continuing anyway.", err)
	} else {
		ldr.anvil = &anvil
	}

	c := make(chan string)
	err = findAllTagsFiles(c)
	if err != nil {
		fmt.Printf("Rt: Can't load tags files: %v\n", err)
		return
	}

	count := 0
	for path := range c {
		count++
		found := searchInTagsFile(path, tag, printAnvilPathForTag, ldr.acquireInAnvil)
		if found {
			break
		}
	}
	if count == 0 {
		fmt.Printf("Rt: No tags file found\n")
	}
}

func findTagsFile() (path string, err error) {
	dir, err := os.Getwd()
	if err != nil {
		return
	}

	dir, err = filepath.Abs(dir)
	if err != nil {
		return
	}

	for dir != "/" {
		f := filepath.Join(dir, "tags")
		if _, err = os.Stat(f); err == nil {
			path = f
			return
		}

		dir = filepath.Dir(dir)
	}

	err = os.ErrNotExist
	return
}

func findAllTagsFiles(c chan string) (err error) {
	dir, err := os.Getwd()
	if err != nil {
		return
	}

	dir, err = filepath.Abs(dir)
	if err != nil {
		return
	}

	go func() {
		for dir != "/" {
			f := filepath.Join(dir, "tags")
			if _, err = os.Stat(f); err == nil {
				c <- f
			}

			dir = filepath.Dir(dir)
		}

		close(c)
	}()

	return
}

type ActionWhenFound func(pathBuilder *PathBuilder, tag *Tag)

func searchInTagsFile(tagsPath, tag string, actions ...ActionWhenFound) (found bool) {
	pathBuilder := PathBuilder{tagsPath}

	fmt.Printf("Rt: tags file found at %s. Searching for '%s'\n", tagsPath, tag)

	f, err := os.Open(tagsPath)
	if err != nil {
		fmt.Printf("Rt: Opening tags file failed: %v\n", err)
		return
	}

	s := bufio.NewScanner(f)
	pfx := tag + "\t"
	var ptag Tag
	for s.Scan() {
		l := s.Text()
		if strings.HasPrefix(l, pfx) {
			parseTag(l, &ptag)
			for _, action := range actions {
				action(&pathBuilder, &ptag)
			}
			//printAnvilPathForTag(&pathBuilder, &ptag)
			found = true
			// Keep looping to see if it is found in a second file
		}
	}
	return
}

type Tag struct {
	Tagname    string
	Tagfile    string
	Tagaddress string
}

func (t Tag) AnvilAddress() string {
	if t.Tagaddress[0] == '/' {
		return "!" + ctagsRegexToGoRegex(t.Tagaddress[1:len(t.Tagaddress)-1])
	} else {
		return ":" + t.Tagaddress
	}
}

func printAnvilPathForTag(pathBuilder *PathBuilder, tag *Tag) {
	f := pathBuilder.AnvilPath(tag.Tagfile)
	fmt.Printf("%s%s\n", f, tag.AnvilAddress())
}

func parseTag(line string, tag *Tag) {
	parts := strings.Split(line, "\t")
	tag.Tagname = parts[0]
	if len(parts) > 1 {
		tag.Tagfile = parts[1]
	}
	if len(parts) > 2 {
		parts = strings.Split(parts[2], ";")
		tag.Tagaddress = parts[0]
	}
}

type PathBuilder struct {
	// Where the tags file is found on the local FS
	tagsFilePath string
}

func (b PathBuilder) AnvilPath(relativeTagFile string) string {
	path := filepath.Join(filepath.Dir(b.tagsFilePath), relativeTagFile)
	path = makeLocaAbsolutelFileGlobal(path)
	return path
}

func makeLocaAbsolutelFileGlobal(path string) string {
	gl := os.Getenv("ANVIL_WIN_GLOBAL_PATH")
	pfx := ""
	i := strings.LastIndex(gl, ":")
	if i >= 0 {
		pfx = gl[:i+1]
	}

	return pfx + path
}

func ctagsRegexToGoRegex(s string) string {
	s = strings.ReplaceAll(s, "(", `\(`)
	s = strings.ReplaceAll(s, ")", `\)`)
	s = strings.ReplaceAll(s, "*", `\*`)
	return s
}

type AnvilLoader struct {
	anvil *anvil.Anvil
}

func (l *AnvilLoader) acquireInAnvil(pathBuilder *PathBuilder, tag *Tag) {
	if l.anvil == nil {
		return
	}

	f := pathBuilder.AnvilPath(tag.Tagfile)

	b := []byte(fmt.Sprintf(`{"cmd": "Acq", "args": ["%s%s"], "winid": -1}`,
		f,
		insertEscapesForJson(tag.AnvilAddress())))
	cmd := bytes.NewReader(b)
	fmt.Printf("Rt: sending command: %s\n", string(b))

	_, err := l.anvil.Post("/execute", cmd)
	if err != nil {
		fmt.Printf("Rt: executing Anvil Acq failed: %v\n", err)
		return
	}
}

func insertEscapesForJson(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, `"`, `\"`)
}
