package main

import (
	"fmt"
	"github.com/jeffwilliams/anvil/api/go/anvil"
	"os"
	"path/filepath"
)

func cmdDef(win anvil.Window, cmd []string) {
	locs, err := getLocationsForItemUnderPrimaryCursor(win, lspConn.GetDefinition)
	if err != nil {
		printMsg("error getting def from Lsp server: %v\n", err)
		return
	}
	acquireFirstLocation(win, locs)
}

type getLocationMethod func(path string, line, col uint) ([]SimpleLocation, error)

func getLocationsForItemUnderPrimaryCursor(win anvil.Window, method getLocationMethod) (simple []SimpleLocation, err error) {
	line, col, err := getPrimaryCursorLocation(win)
	if err != nil {
		return
	}

	return method(win.Path, line, col)

	/*
		cursors, err := anvilHttpApi.WindowBodyCursors(win)
		if err != nil {
			err = fmt.Errorf("error reading cursors: %v\n", err)
			return
		}

		data, err := winBody(win)
		if err != nil {
			err = fmt.Errorf("error getting window info when opened: %v\n", err)
			return
		}

		if len(cursors) == 0 {
			return
		}

		return getLocationsForItemUnderCursor(win, data, cursors[0], method)
	*/
}

func getLocationsForItemUnderCursor(win anvil.Window, data []byte, cursor int, method getLocationMethod) (simple []SimpleLocation, err error) {
	line, col := cursorLineAndCol(data, cursor)
	return method(win.Path, line, col)
}

func getPrimaryCursorLocation(win anvil.Window) (line, col uint, err error) {
	cursors, err := anvilHttpApi.WindowBodyCursors(win)
	if err != nil {
		err = fmt.Errorf("error reading cursors: %v\n", err)
		return
	}

	data, err := winBody(win)
	if err != nil {
		err = fmt.Errorf("error getting window info when opened: %v\n", err)
		return
	}

	if len(cursors) == 0 {
		fmt.Errorf("No cursors are present")
		return
	}

	line, col = cursorLineAndCol(data, cursors[0])
	return
}

func acquireFirstLocation(win anvil.Window, locs []SimpleLocation) {
	if len(locs) == 0 {
		return
	}

	l := lspLocationToAnvil(win, locs[0])
	debug("Acquiring '%s'\n", l)

	anvilHttpApi.ExecuteInWin(win, "Acq", []string{l})
}

func cmdDecl(win anvil.Window, cmd []string) {
	locs, err := getLocationsForItemUnderPrimaryCursor(win, lspConn.GetDeclaration)
	if err != nil {
		printMsg("error getting def from Lsp server: %v\n", err)
		return
	}
	acquireFirstLocation(win, locs)

}

func cmdRefs(win anvil.Window, cmd []string) {
	locs, err := getLocationsForItemUnderPrimaryCursor(win, lspConn.GetReferences)
	if err != nil {
		printMsg("error getting def from Lsp server: %v\n", err)
		return
	}

	// TODO: also print a few lines of context (1 above, 1 below) and the name of the surrounding function
	printLocations(win, locs)
}

func cmdHover(win anvil.Window, cmd []string) {
	line, col, err := getPrimaryCursorLocation(win)
	if err != nil {
		return
	}

	info, err := lspConn.Hover(win.Path, line, col)
	if err != nil {
		printMsg("error getting hover info from Lsp server: %v\n", err)
		return
	}

	printMsg(info)
}

func printLocations(win anvil.Window, locs []SimpleLocation) {
	// TODO: make this a new window
	fmt.Printf("References:\n")
	for _, loc := range locs {
		l := lspLocationToAnvilRelative(win, loc)
		fmt.Printf("  %s\n", l)
	}

}

func getDefForCursor(win anvil.Window, data []byte, cursor int) {
	line, col := cursorLineAndCol(data, cursor)
	locs, err := lspConn.GetDefinition(win.Path, line, col)

	if err != nil {
		printMsg("error getting def from Lsp server: %v\n", err)
		return
	}

	if len(locs) == 0 {
		return
	}

	l := lspLocationToAnvil(win, locs[0])
	debug("Acquiring '%s'\n", l)

	anvilHttpApi.ExecuteInWin(win, "Acq", []string{l})
}

func getDeclForCursor(win anvil.Window, data []byte, cursor int) {
	line, col := cursorLineAndCol(data, cursor)
	locs, err := lspConn.GetDeclaration(win.Path, line, col)

	if err != nil {
		printMsg("error getting def from Lsp server: %v\n", err)
		return
	}

	if len(locs) == 0 {
		return
	}

	l := lspLocationToAnvil(win, locs[0])
	debug("Acquiring '%s'\n", l)

	anvilHttpApi.ExecuteInWin(win, "Acq", []string{l})
}

func globalizeLocalPath(win anvil.Window, p string) string {
	isRemotePath := win.GlobalPath != win.Path
	if isRemotePath {
		i := indexRev(win.GlobalPath, ':')
		if i >= 0 {
			hostInfo := win.GlobalPath[:i+1]
			return hostInfo + p
		}
	}
	return p
}

func indexRev(s string, r byte) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == r {
			return i
		}
	}
	return -1
}

func lspLocationToAnvil(win anvil.Window, l SimpleLocation) string {
	if win.GlobalPath != win.Path {
		// Remote path
		l.Path = globalizeLocalPath(win, l.Path)
	}

	return fmt.Sprintf("%s:%d:%d", l.Path, l.Range.Start.Line, l.Range.Start.Character)
}

func lspLocationToAnvilRelative(win anvil.Window, l SimpleLocation) string {
	wd, err := os.Getwd()
	if err != nil {
		return lspLocationToAnvil(win, l)
	}

	/*fi, err := os.Stat(win.Path)
	if err != nil {
		return lspLocationToAnvil(win, l)
	}

	dir := win.Path
	if !fi.IsDir() {
		dir = filepath.Dir(win.Path)
	}*/

	rel, err := filepath.Rel(wd, l.Path)
	if err != nil {
		return lspLocationToAnvil(win, l)
	}

	return fmt.Sprintf("%s:%d:%d", rel, l.Range.Start.Line, l.Range.Start.Character)
}
