package main

import (
	"fmt"
	"github.com/jeffwilliams/anvil/api/go/anvil"
	"os"

	"github.com/ogier/pflag"
)

var (
	optDebug = pflag.BoolP("debug", "d", false, "Print debug messages")
)

var (
	anvilHttpApi anvil.Anvil
	anvilWsApi   anvil.Websock
	hooks        []Hook
)

func main() {
	pflag.Parse()

	connectToAnvil()
	var err error
	hooks, err = parseConfigFile()
	dieIfError(err, "Parsing config failed")

	anvilWsApi.Run()
}

func connectToAnvil() {
	debug("ado: connecting to HTTP API\n")
	var err error
	anvilHttpApi, err = anvil.NewFromEnv()
	dieIfError(err, "connecting to API failed")

	handlers := anvil.WebsockHandlers{
		Notification: handleNotification,
	}

	debug("ado: connecting to WS API\n")
	anvilWsApi, err = anvilHttpApi.Websock(handlers)
	dieIfError(err, "creating websocket failed")
}

func dieIfError(err error, msg string) {
	if err != nil {
		msg := fmt.Sprintf("%s: %s", msg, err)
		die(msg)
	}
}

func die(msg string) {
	fmt.Fprintf(os.Stderr, "ado: %s\n", msg)
	os.Exit(1)
}

func debug(format string, args ...interface{}) {
	if !*optDebug {
		return
	}
	fmt.Printf(format, args...)
}

func handleNotification(notif *anvil.Notification, err error) {
	if err != nil {
		fmt.Printf("ado: got an error handling notifications: %v\n", err)
		return
	}

	switch notif.Op {
	case anvil.NotificationOpFileOpened:
		handleFileOpenedNotification(notif)
	}

	if notif.Op != anvil.NotificationOpFileOpened {
		return
	}
}

func handleFileOpenedNotification(notif *anvil.Notification) {
	debug("ado: got file opened notification: %#v\n", notif)

	win, err := anvilHttpApi.Window(notif.WinId)
	if err != nil {
		fmt.Printf("ado: error getting window info when opened: %v\n", err)
		return
	}

	for _, hook := range hooks {
		matched := tryHook(win, &hook)
		if matched {
			break
		}
	}
}

func tryHook(win anvil.Window, hook *Hook) (matched bool) {
	submatches := hook.Match.FindStringSubmatchIndex(win.GlobalPath)
	if submatches == nil {
		return
	}
	debug("ado: '%s' matches '%s'\n", win.GlobalPath, hook.Match)

	matched = true

	for _, do := range hook.Do {
		cmd := []byte{}
		cmd = hook.Match.Expand(cmd, []byte(do), []byte(win.GlobalPath), submatches)
		debug("ado: executing '%s'\n", cmd)
		err := anvilHttpApi.ExecuteInWin(win, string(cmd), nil)
		if err != nil {
			fmt.Printf("ado: executing command '%s' in win %d failed: %v \n", cmd, win.Id, err)
		}
	}

	return
}
