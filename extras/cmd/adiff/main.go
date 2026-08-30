package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/jeffwilliams/anvil/api/go/anvil"
)

var (
	winsToDiff []Window
	httpApi    anvil.Anvil
)

type Window struct {
	anvil.Window
	order        int
	bodyFilename string
}

func main() {
	var err error
	httpApi, err = anvil.NewFromEnv()
	dieIfError(err, "connecting to API failed")

	if len(os.Args) > 1 && os.Args[1] == "clr" {
		clearMarksFromWindowTags()
		return
	}

	computeAndDisplayDiff()
}

func computeAndDisplayDiff() {
	loadWinsToDiff()
	fetchBodies()

	b := diff()
	writeToDiffWindow(b)

	removeTempfiles()
}

func loadWinsToDiff() {
	wins, err := httpApi.Windows()
	dieIfError(err, "getting windows failed")

	for _, win := range wins {
		tag, err := httpApi.WindowTag(win)
		if err != nil {
			fmt.Printf("adiff: warning: couldn't get tag for window %d: %v\n", win.Id, err)
		}

		if i := strings.Index(tag, "&&"); i >= 0 {
			if len(tag) > i+2 {
				n, err := strconv.Atoi(tag[i+2 : i+3])
				if err != nil {
					fmt.Printf("adiff: warning: found '&&' in window %d, but it's not followed by a digit\n", win.Id)
					continue
				}

				winsToDiff = append(winsToDiff, Window{win, n, ""})
			}
		}
	}

	if len(winsToDiff) == 0 {
		die(fmt.Sprintf("adiff: no windows are marked to diff. Mark them with &&1 and &&2"))
	}

	if len(winsToDiff) != 2 {
		die(fmt.Sprintf("adiff: found %d marked windows, but I can only diff exactly 2", len(winsToDiff)))
	}

	sort.Slice(winsToDiff, func(i, j int) bool {
		return winsToDiff[i].order < winsToDiff[j].order
	})
}

func fetchBodies() {
	for i, win := range winsToDiff {
		rdr, err := httpApi.WindowBody(win.Window)
		dieIfError(err, "getting window body failed")

		f, err := os.CreateTemp(os.TempDir(), "AnvilDiff")
		dieIfError(err, "making tempfile failed")

		_, err = io.Copy(f, rdr)
		dieIfError(err, "filling tempfile failed")
		f.Close()

		winsToDiff[i].bodyFilename = f.Name()
	}
}

func diff() []byte {
	args := []string{"-u"}
	for _, win := range winsToDiff {
		args = append(args, win.bodyFilename)
	}

	cmd := exec.Command("diff", args...)
	b, _ := cmd.CombinedOutput()
	return b
}

func writeToDiffWindow(diff []byte) {
	win, err := findOrCreateWindow("+Diff")
	dieIfError(err, "creating +Diff window failed")

	err = httpApi.SetWindowBodyString(win, string(diff))
	dieIfError(err, "Putting body failed")

	httpApi.SetWindowTag(win, fmt.Sprintf("%s Del! | Look", "+Diff"))
	httpApi.ExecuteInWin(win, "Syn", []string{"diff"})
}

func removeTempfiles() {
	for _, win := range winsToDiff {
		if win.bodyFilename == "" {
			continue
		}

		err := os.Remove(win.bodyFilename)
		if err != nil {
			fmt.Printf("adiff: warning: deleting temporary file %s failed: %v", win.bodyFilename, err)
		}
	}
}

func findOrCreateWindow(path string) (win anvil.Window, err error) {
	wins, err := httpApi.Windows()
	if err != nil {
		return
	}

	for _, w := range wins {
		if w.Path == path {
			win = w
			return
		}
	}

	win, err = httpApi.NewWindow()
	if err != nil {
		return
	}

	return
}

func clearMarksFromWindowTags() {
	wins, err := httpApi.Windows()
	dieIfError(err, "getting windows failed")

	for _, win := range wins {
		tag, err := httpApi.WindowTag(win)
		if err != nil {
			fmt.Printf("adiff: warning: couldn't get tag for window %d: %v\n", win.Id, err)
		}

		if i := strings.Index(tag, "&&"); i >= 0 {
			spaceIndex := strings.Index(tag[i:], " ")
			var newTag string
			if spaceIndex >= 0 {
				spaceIndex += i
				// If there is a preceeding space, then delete the trailing space.
				if i > 0 && tag[i-1] == ' ' || tag[i-1] == '\t' {
					spaceIndex++
				}
				newTag = tag[0:i] + tag[spaceIndex:]
			} else {
				// Must be at end of line
				newTag = tag[0:i]
			}
			httpApi.SetWindowTag(win, newTag)
		}
	}
}

func dieIfError(err error, msg string) {
	if err != nil {
		msg := fmt.Sprintf("%s: %s", msg, err)
		die(msg)
	}
}

func die(msg string) {
	fmt.Fprintf(os.Stderr, "adiff: %s\n", msg)
	removeTempfiles()
	os.Exit(1)
}
