package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/jeffwilliams/anvil/api/go/anvil"
	"github.com/ogier/pflag"
)

var (
	noBody   io.Reader
	httpApi  anvil.Anvil
	cmds     = []string{}
	watchWin anvil.Window
)

var (
	optDebug = pflag.BoolP("debug", "d", false, "Print debug messages")
)

func main() {
	pflag.Parse()

	var err error
	httpApi, err = anvil.NewFromEnv()
	dieIfError(err, "connecting to API failed")

	handlers := anvil.WebsockHandlers{
		Notification: handlePutNotification,
	}

	wsApi, err := httpApi.Websock(handlers)
	dieIfError(err, "creating websocket failed")

	loadFirstCommand()
	watchWin = findOrCreateWindow(&httpApi, watchPath())

	runCmdsAndUpdateWindow()

	wsApi.Run()
}

func debug(format string, args ...interface{}) {
	if !*optDebug {
		return
	}
	fmt.Printf(format, args...)
}

func dieIfError(err error, msg string) {
	if err != nil {
		msg := fmt.Sprintf("%s: %s", msg, err)
		die(msg)
	}
}

func die(msg string) {
	fmt.Fprintf(os.Stderr, "awatch: %s\n", msg)
	os.Exit(1)
}

func loadFirstCommand() {
	if len(pflag.Args()) < 1 {
		die("no arguments were passed. The arguments must be a command to run")
	}

	cmds = append(cmds, strings.Join(pflag.Args(), " "))
}

func run(cmd string) (output []byte, err error) {
	c := newCmd(cmd)
	return c.CombinedOutput()
}

func findOrCreateWindow(api *anvil.Anvil, watchPath string) anvil.Window {
	var wins []anvil.Window
	err := api.GetInto("/wins", &wins)
	dieIfError(err, "reading windows failed")
	for _, w := range wins {
		if w.Path == watchPath {
			return w
		}
	}

	win := createNewWindow(api)
	setWindowTag(api, win.Id, watchPath)
	return win
}

func createNewWindow(api *anvil.Anvil) anvil.Window {
	rsp, err := api.Post("/wins", noBody)
	dieIfError(err, "creating new window failed")

	raw, err := ioutil.ReadAll(rsp.Body)
	dieIfError(err, "reading response from creating window failed")

	var win anvil.Window
	err = json.Unmarshal(raw, &win)
	dieIfError(err, "decoding JSON response after creating window failed")
	return win
}

func watchPath() string {
	anvilGlobalPath := os.Getenv("ANVIL_WIN_GLOBAL_DIR")
	return filepath.Join(anvilGlobalPath, "+watch")
}

func setWindowTag(anvil *anvil.Anvil, winId int, watchPath string) {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "%s Del! Snarf | Look ", watchPath)
	anvil.Put(fmt.Sprintf("/wins/%d/tag", winId), &buf)
}

func handlePutNotification(notif *anvil.Notification, err error) {
	if err != nil {
		// Parsing notification failed.
		fmt.Fprintf(os.Stderr, "awatch: parsing notification failed: %v\n", err)
		return
	}

	if notif.Op != anvil.NotificationOpPut {
		return
	}

	debug("awatch: got put notification for window %d\n", notif.WinId)

	var info anvil.Window
	err = httpApi.GetInto(fmt.Sprintf("/wins/%d/info", notif.WinId), &info)
	if err != nil {
		// Parsing notification failed.
		fmt.Fprintf(os.Stderr, "awatch: getting info for window %d failed: %v\n", notif.WinId, err)
		return
	}

	e := os.Getenv("ANVIL_WIN_LOCAL_DIR")
	localDir, err := filepath.Abs(e)
	if err != nil {
		fmt.Fprintf(os.Stderr, "awatch: getting absolute path of %s failed: %v\n", e, os.Getenv("ANVIL_WIN_LOCAL_DIR"))
		return
	}

	winPath, err := filepath.Abs(info.Path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "awatch: getting absolute path of %s failed: %v\n", info.Path, err)
		return
	}

	if !strings.HasPrefix(winPath, localDir) {
		debug("awatch: %s doesn't match our dir %s\n", winPath, localDir)
		return
	}

	updateCmdsFromWindow()
	runCmdsAndUpdateWindow()
}

func runCmdsAndUpdateWindow() {
	output := runCmds()
	httpApi.Put(fmt.Sprintf("/wins/%d/body", watchWin.Id), output)
}

// updateCmdsFromWindow reads all the lines from the window body that begin with '% ' and
// treat them as commands to run.
func updateCmdsFromWindow() {
	body, err := httpApi.WindowBody(watchWin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "awatch: reading watch window failed: %v\n", err)
		return
	}

	cmds = cmds[:0]
	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "% ") {
			cmds = append(cmds, line[2:])
		}
	}
}

func runCmds() (output *bytes.Buffer) {
	buf := new(bytes.Buffer)

	for _, c := range cmds {
		fmt.Fprintf(buf, "%% %s\n", c)
		debug("awatch: running command: %s\n", c)
		output, err := run(c)
		if err != nil {
			fmt.Fprintf(buf, "(execution error: %v)\n", err)
		}
		buf.Write(output)
	}

	return buf
}
