package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/jeffwilliams/anvil/api/go/anvil"
)

var (
	anvilHttpApi anvil.Anvil
	anvilWsApi   anvil.Websock
)

func connectToAnvil() {
	debug("connecting to HTTP API\n")
	var err error
	anvilHttpApi, err = anvil.NewFromEnv()
	dieIfError(err, "connecting to API failed")

	handlers := anvil.WebsockHandlers{
		Notification: handleNotification,
	}

	debug("connecting to WS API\n")
	anvilWsApi, err = anvilHttpApi.Websock(handlers)
	dieIfError(err, "creating websocket failed")
}

func notifyOpenAnvilDocs() {
	wins, err := anvilHttpApi.Windows()
	if err != nil {
		printMsg("error getting windows: %v\n", err)
		return
	}

	printMsg("%d open windows\n", len(wins))
	for _, w := range wins {
		notifyAnvilDocOpened(w)
	}
}

func handleNotification(notif *anvil.Notification, err error) {
	if err != nil {
		fmt.Printf("got an error handling notifications: %v\n", err)
		return
	}

	switch notif.Op {
	case anvil.NotificationOpFileOpened:
		handleFileOpenedNotification(notif)
	case anvil.NotificationOpPut:
		handleFilePutNotification(notif)
	case anvil.NotificationOpExec:
		handleExecNotification(notif)
	}

	if notif.Op != anvil.NotificationOpFileOpened {
		return
	}
}

func handleFileOpenedNotification(notif *anvil.Notification) {
	debug("got file opened notification: %#v\n", notif)

	win, err := anvilHttpApi.Window(notif.WinId)
	if err != nil {
		printMsg("error getting window info when opened: %v\n", err)
		return
	}

	notifyAnvilDocOpened(win)

	//addLspToTag(win)
}

func isDocSuitableForLsp(win anvil.Window) bool {
	if win.Path == "" {
		return false
	}

	if strings.HasSuffix(win.Path, "+Errors") {
		return false
	}

	fi, err := os.Stat(win.Path)
	if err == nil && fi.IsDir() {
		return false
	}

	return true
}

func notifyAnvilDocOpened(win anvil.Window) {
	if !isDocSuitableForLsp(win) {
		return
	}

	// Rather than read the contents from Anvil, which might be over the network, use the local file.
	// As well, Anvil notifies the file is opened before it could have been completely loaded
	/*
		data, err := winBody(win)
		if err != nil {
			printMsg("error getting window info when opened: %v\n", err)
			return
		}
	*/
	data, err := ioutil.ReadFile(win.Path)
	if err != nil {
		printMsg("reading local file %s failed\n", win.Path)
		return
	}

	debug("Notifying '%s' opened\n", win.Path)
	lspConn.DocOpened(win.Path, string(data))

}

func handleFilePutNotification(notif *anvil.Notification) {
	debug("got file put notification: %#v\n", notif)

	win, err := anvilHttpApi.Window(notif.WinId)
	if err != nil {
		printMsg("error getting window info when opened: %v\n", err)
		return
	}

	notifyAnvilDocSaved(win)
}

func notifyAnvilDocSaved(win anvil.Window) {
	if !isDocSuitableForLsp(win) {
		return
	}

	debug("Notifying '%s' saved\n", win.Path)
	lspConn.DocSaved(win.Path)

}

func cursorLineAndCol(data []byte, cursor int) (line, col uint) {
	line = 1
	col = 0
	i := 0

	var lastr byte
	for ; i <= cursor; i++ {
		if lastr == '\n' {
			line++
			col = 0
		}

		lastr = data[i]
		col++
	}
	return
}

func winBody(win anvil.Window) (data []byte, err error) {
	body, err := anvilHttpApi.WindowBody(win)
	if err != nil {
		return
	}

	data, err = ioutil.ReadAll(body)
	if err != nil {
		return
	}

	return
}

func handleExecNotification(notif *anvil.Notification) {
	debug("got exec notification: %+v\n", notif)

	win, err := anvilHttpApi.Window(notif.WinId)
	if err != nil {
		fmt.Printf("dq: error getting window info when opened: %v\n", err)
		return
	}

	cmd := notif.Cmd

	switch cmd[0] {
	case "Lhelp":
		printHelp()
	case "Ldef":
		cmdDef(win, cmd)
	case "Ldecl":
		cmdDecl(win, cmd)
	case "Lrefs":
		cmdRefs(win, cmd)
	case "Lhov":
		cmdHover(win, cmd)
	}

}

func printHelp() {
	fmt.Printf("Anvil clangd client\n")
	fmt.Printf("This program interacts with LSP servers\n")
	fmt.Printf("Commands: \n")
	fmt.Printf("  Lhelp\n")
	fmt.Printf("    Print this help\n")
	fmt.Printf("  Ldef\n")
	fmt.Printf("    Find the definition of the symbol under the cursor and acquire it\n")
	fmt.Printf("  Ldecl\n")
	fmt.Printf("    Find the declaration of the symbol under the cursor and acquire it\n")
	fmt.Printf("  Lrefs\n")
	fmt.Printf("    Find references to the symbol\n")
	fmt.Printf("  Lhov\n")
	fmt.Printf("    Print hover information for the symbol under the cursor\n")
}

func addLspToTag(win anvil.Window) {
	tag, err := anvilHttpApi.WindowTag(win)
	if err != nil {
		debug("Lsp: getting window tag for window %d failed: %v\n", win.Id, err)
	}

	if strings.Contains(tag, "L ") || strings.Contains(tag, " L") {
		return
	}

	if strings.HasPrefix(tag, " ") {
		tag = tag + "L "
	} else {
		tag = tag + " L "
	}

	err = anvilHttpApi.SetWindowTag(win, tag)
	if err != nil {
		debug("Lsp: setting window tag for window %d failed: %v\n", win.Id, err)
	}
}

func dieIfError(err error, msg string) {
	if err != nil {
		msg := fmt.Sprintf("%s: %s", msg, err)
		die(msg)
	}
}

func die(msg string) {
	fmt.Fprintf(os.Stderr, "%s\n", msg)
	os.Exit(1)
}

func debug(format string, args ...interface{}) {
	if !optDebug {
		return
	}
	fmt.Printf("Lsp: ")
	fmt.Printf(format, args...)
}

func printMsg(format string, args ...interface{}) {
	fmt.Printf("Lsp: ")
	fmt.Printf(format, args...)
}
