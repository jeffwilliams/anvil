package main

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
)

const (
	LogCatgApp        = "Application"
	LogCatgUI         = "UI"
	LogCatgEd         = "Editable"
	LogCatgSyntax     = "Syntax"
	LogCatgAPI        = "API"
	LogCatgFS         = "Filesystem"
	LogCatgCompletion = "Completion"
	LogCatgPlumb      = "Plumbing"
	LogCatgWin        = "Window"
	LogCatgCmd        = "Commands"
	LogCatgCol        = "Column"
	LogCatgConf       = "Config"
	LogCatgEditor     = "Editor"
	LogCatgPack       = "Packing"
	LogCatgSsh        = "SSH"
	LogCatgExpr       = "Expressions"
	LogCatgFuzzy      = "FuzzySearch"
)

var debugLogCategories = []string{
	LogCatgApp,
	LogCatgUI,
	LogCatgEd,
	LogCatgSyntax,
	LogCatgAPI,
	LogCatgFS,
	LogCatgCompletion,
	LogCatgPlumb,
	LogCatgWin,
	LogCatgCmd,
	LogCatgCol,
	LogCatgConf,
	LogCatgEditor,
	LogCatgPack,
	LogCatgSsh,
	LogCatgExpr,
	LogCatgFuzzy,
}

var killPprofDebugServer = make(chan struct{})

func startPprofDebugServer() {
	go func() {
		server := &http.Server{Addr: "localhost:6060"}

		go func() {
			<-killPprofDebugServer
			server.Close()
		}()

		err := server.ListenAndServe()

		if err != nil && err.Error() != "http: Server closed" {
			w := basicWork{func() {
				editor.AppendError("", fmt.Sprintf("Error starting pprof debug server: %v %T", err, err))
			}}
			editor.WorkChan() <- w
			return
		}

	}()
}

func stopPprofDebugServer() {
	select {
	case killPprofDebugServer <- struct{}{}:
	default:
	}
}
