package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ogier/pflag"
)

var optDepth = pflag.IntP("depth", "d", 9999, "Maximum depth of headings to display")

func main() {

	pflag.Usage = usage
	pflag.Parse()

	file := ""
	if pflag.NArg() > 0 {
		file = pflag.Arg(0)
	}
	if file == "" {
		file = os.Getenv("ANVIL_WIN_LOCAL_PATH")
		file = filepath.Base(file)
	}
	fmt.Printf("File to process: %s\n", file)

	f, err := os.Open(file)
	if err != nil {
		fmt.Printf("Opening %s failed: %v\n", file, err)
		return
	}

	base := filepath.Base(file)

	process(base, f)
}

func process(path string, in io.Reader) {
	s := bufio.NewScanner(in)
	lineno := 1
	for s.Scan() {
		line := s.Text()
		d := headingDepth(line)
		//if len(line) > 0 && line[0] == '#' {
		if d > 0 && d <= *optDepth {
			fmt.Printf("%s  \t\t%s:%d\n", line, path, lineno)
		}
		lineno++
	}
}

func headingDepth(s string) int {
	for i, c := range s {
		if c != '#' {
			return i
		}
	}
	return 0
}

func usage() {
	fmt.Fprintf(os.Stdout, "Usage: %s [filename]\n", os.Args[0])
	pflag.PrintDefaults()
}
