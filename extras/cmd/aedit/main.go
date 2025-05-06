// aedit is meant to be the equivalent of the Acme E command. It is meant to be used as the EDITOR
// environment variable, and will try to open the file in the instance of Anvil that ran the command.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jeffwilliams/anvil/api/go/anvil"

	"github.com/ogier/pflag"
)

var (
	winDir     = ""
	fileToEdit = ""
	httpApi    anvil.Anvil
	wsApi      anvil.Websock
	win        anvil.Window
)

var (
	optDebug = pflag.BoolP("debug", "d", false, "Print debug messages")
)

func main() {
	pflag.Parse()

	determineWinDir()
	debug("winDir: '%s'\n", winDir)
	loadFileToEdit()

	if runningFromAnvil() {
		connectToAnvil()
		calcFilepathAndOpenFileInAnvil()
		waitForWindowDel()
		return
	}

	runAnvilWithFile()
}

func determineWinDir() {
	winDir = os.Getenv("ANVIL_WIN_GLOBAL_DIR")
	localWinDir := os.Getenv("ANVIL_WIN_LOCAL_DIR")
	anvilDir := os.Getenv("ANVIL_DIR")
	debug("local dir: %s global dir: %s\n", localWinDir, winDir)
	isRemotePath := winDir != localWinDir
	if !isRemotePath && winDir != "" && !filepath.IsAbs(winDir) {
		d := filepath.Join(anvilDir, winDir)
		debug("aedit: absolute path of '%s' is '%s'\n", winDir, d)
		winDir = d
	}
}

func loadFileToEdit() {
	if pflag.NArg() < 1 {
		die("expected filename as first argument")
	}
	fileToEdit = pflag.Arg(0)
}

func runningFromAnvil() bool {
	return winDir != ""
}

func connectToAnvil() {
	debug("aedit: connecting to HTTP API\n")
	var err error
	httpApi, err = anvil.NewFromEnv()
	dieIfError(err, "connecting to API failed")

	handlers := anvil.WebsockHandlers{
		Notification: handleFileCloseNotification,
	}

	debug("aedit: connecting to WS API\n")
	wsApi, err = httpApi.Websock(handlers)
	dieIfError(err, "creating websocket failed")

}

func calcFilepathAndOpenFileInAnvil() {
	var winPath string
	if filepath.IsAbs(fileToEdit) {
		debug("aedit: file to edit '%s' is absolute\n", fileToEdit)
		winPath = applyWindowRemoteConnInfoToFilePath(winDir, fileToEdit)
	} else {
		debug("aedit: file to edit '%s' is not absolute\n", fileToEdit)
		winPath = filepath.Join(winDir, fileToEdit)
	}

	debug("aedit: opening file %s in anvil\n", winPath)
	openFileInAnvil(winPath)
	//win = findWindow(winPath)
	win = waitForWindow(winPath)
	debug("aedit: found the window for the path we care about: %#v\n", win)
}

func applyWindowRemoteConnInfoToFilePath(winDir, fileToEdit string) string {
	// If the window directory is remote and we have an absolute path,
	// prepend the connection info to the file to edit. This code below
	// leaves the filepath unchanged if it is not remote.

	if isWindowsPath(fileToEdit) {
		return fileToEdit
	}

	i := strings.IndexRune(winDir, filepath.Separator)
	if i < 0 {
		die(fmt.Sprintf("can't find separator '%c' in ANVIL_WIN_GLOBAL_DIR '%s'", filepath.Separator, winDir))
	}
	stem := winDir[0:i]
	return filepath.Join(stem, fileToEdit)
}

func openFileInAnvil(path string) {
	err := httpApi.Execute("New", []string{path})
	dieIfError(err, "opening file in Anvil failed")
}

func findWindow(path string) (win anvil.Window, ok bool) {
	var wins []anvil.Window
	err := httpApi.GetInto("/wins", &wins)
	dieIfError(err, "reading windows failed")
	for _, w := range wins {
		debug("aedit: check windows path %s\n", w.GlobalPath)
		if w.GlobalPath == path {
			return w, true
		}
	}

	return anvil.Window{}, false
}

func waitForWindow(path string) anvil.Window {
	tries := 10
	for tries > 0 {
		win, ok := findWindow(path)
		if ok {
			return win
		}
		time.Sleep(100 * time.Millisecond)
		tries--
	}
	die(fmt.Sprintf("can't locate window for %s", path))
	return anvil.Window{}
}

func waitForWindowDel() {
	debug("aedit: starting wait for window Del\n")
	wsApi.Run()
}

func handleFileCloseNotification(notif *anvil.Notification, err error) {
	if notif.Op != anvil.NotificationOpFileClosed {
		return
	}
	debug("aedit: got window closed notification: %#v\n", notif)

	if notif.WinId != win.Id {
		debug("aedit: not the window we care about\n")
		return
	}

	// All done!
	os.Exit(0)
}

func runAnvilWithFile() {
	cmd := exec.Command("anvil", fileToEdit)
	err := cmd.Run()
	dieIfError(err, "running anvil failed")
}

func dieIfError(err error, msg string) {
	if err != nil {
		msg := fmt.Sprintf("%s: %s", msg, err)
		die(msg)
	}
}

func die(msg string) {
	fmt.Fprintf(os.Stderr, "aedit: %s\n", msg)
	os.Exit(1)
}

func debug(format string, args ...interface{}) {
	if !*optDebug {
		return
	}
	fmt.Printf(format, args...)
}

func isWindowsPath(path string) bool {
	// Note: Here we allow the form C:/ in addition to the standard
	// C:\. Some tools on Windows (such as the tools installed with Git)
	// use the forward slash instead of backslash even on Windows.
	return len(path) >= 3 &&
		((path[0] >= 'A' && path[0] < 'Z') || (path[0] >= 'a' && path[0] <= 'z')) &&
		path[1] == ':' && (path[2] == '\\' || path[2] == '/')
}
