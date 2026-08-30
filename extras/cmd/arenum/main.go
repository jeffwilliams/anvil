package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/janne/go-lisp/lisp"
	"github.com/jeffwilliams/anvil/api/go/anvil"
	"github.com/spf13/pflag"
)

var (
	httpApi anvil.Anvil
	win     anvil.Window
)

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: %s [options] <code>\n\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "Renumber the numbers the selections according to the specified lisp code.\n")
	fmt.Fprintf(os.Stderr, "The code has access to pre-defined variables:\n\n")
	fmt.Fprintf(os.Stderr, "  i: the index of the selection, starting from 0\n")
	fmt.Fprintf(os.Stderr, "  n: the number in the selection\n")
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "For example, to replace the selections with numbers starting from 1 do:\n\n")
	fmt.Fprintf(os.Stderr, "  %s '(+ i 1)' \n\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "to add one to each selection:\n\n")
	fmt.Fprintf(os.Stderr, "  %s '(+ n 1)' \n\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "Options:\n")
	pflag.PrintDefaults()
}

func main() {
	pflag.Usage = usage
	pflag.Parse()

	if pflag.NArg() < 1 {
		die("Pass the lisp code to execute as the program arguments")
	}
	code := strings.Join(os.Args[1:], " ")

	var err error
	httpApi, err = anvil.NewFromEnv()
	dieIfError(err, "connecting to API failed")
	getWindow()

	sels, err := httpApi.WindowBodySelections(win)
	dieIfError(err, "getting selections failed")
	if len(sels) == 0 {
		cursors, err := httpApi.WindowBodyCursors(win)
		dieIfError(err, "getting cursors failed")

		// Simulate cursors using zero-length selections so we can reuse the runLispOnSelections code.
		sels = make([]anvil.Selection, len(cursors))
		for i, c := range cursors {
			sels[i].Start = c
			sels[i].End = c
			sels[i].Len = 0
		}
	}

	body, err := httpApi.WindowBody(win)
	dieIfError(err, "getting window body failed")

	content, err := ioutil.ReadAll(body)
	dieIfError(err, "reading window body failed")

	contentStr := string(content)
	runes := []rune(contentStr)

	runLispOnSelections(code, sels, runes)

}

func runLispOnSelections(code string, sels []anvil.Selection, content []rune) {
	sort.Slice(sels, func(i, j int) bool {
		return sels[i].Start < sels[j].Start
	})

	replacements := make([]string, len(sels))
	replacementValid := make([]bool, len(sels))

	for i, sel := range sels {
		v := string(content[sel.Start:sel.End])
		n, err := strconv.Atoi(v)
		if err != nil {
			//			fmt.Fprintf(os.Stderr, "Error: selection %d is not a number. Using 0\n", i)
			//			continue
			n = 0
		}

		wrappedCode := fmt.Sprintf("(begin (define i %d) (define n %d) %s)", i, n, code)
		result, err := lisp.EvalString(wrappedCode)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: evaluating '%s' for selection %d failed: %v\n", wrappedCode, i, err)
			continue
		}

		replacementValid[i] = true
		replacements[i] = result.String()
	}

	var buf bytes.Buffer
	last := 0
	for i, sel := range sels {
		if replacementValid[i] {
			buf.WriteString(string(content[last:sel.Start]))
			buf.WriteString(replacements[i])
		} else {
			buf.WriteString(string(content[last:sel.End]))
		}
		last = sel.End
	}
	buf.WriteString(string(content[last:]))

	err := httpApi.SetWindowBody(win, &buf)
	dieIfError(err, "writing window body failed")
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
	fmt.Fprintf(os.Stderr, "arenum: %s\n", msg)
	os.Exit(1)
}
