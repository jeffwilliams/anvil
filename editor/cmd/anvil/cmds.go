package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"runtime/pprof"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"gioui.org/layout"
	"gioui.org/unit"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/jeffwilliams/anvil/editor/internal/escape"
	"github.com/jeffwilliams/anvil/editor/internal/keymap"
	"github.com/jeffwilliams/anvil/editor/internal/sync"
	"github.com/jeffwilliams/flamegraph"
	"golang.org/x/crypto/ssh"
	"golang.org/x/image/colornames"
)

var cmdHistory = NewCommandHistory(100)

var defaultCommandSet commandSet
var defaultDebugCommandSet commandSet

func init() {
	initDebugCommandSet()
	initDefaultCommandSet()
	defaultCommandSet.addCommandlistToHelp()
}

func initDefaultCommandSet() {
	addCommand := func(name string, do func(ctx *CmdContext), shortHelp, longHelp string) {
		defaultCommandSet.AddCommand(name, do, shortHelp, longHelp)
		AddHelp(name, longHelp)
	}

	setHooks := func(name string, h commandHooks) {
		defaultCommandSet.setHooks(name, h)
	}

	var c CmdDefs

	addCommand("Del", c.CmdDel, "Delete Window", "Del closes the current window.")
	addCommand("Del!", c.CmdDelForce, "Delete Window without prompt", "Del! closes the current window. If there are unsaved changes, the user is not prompted to save them.")
	addCommand("Exit", c.CmdExit, "Exit the editor", "Exit exits the editor.")
	addCommand("New", c.CmdNew, "Make a new window or open a path", "Usage: New [path]\n\nNew makes a new window or with an argument opens a path. If a window for that file is already opened, a new window for that file is not created. Otherwise, the window is opened in the column with the most free space. If new is executed with an argument the file or directory with the name of the argument is loaded into the window.")
	addCommand("Acq", c.CmdAcq, "Acquire a path", `Acq 'acquires' its argument. It performs the same function as ALT+Right Click performs on a text object.

Acquire means the following. First, if the word or selection matches a plumbing rule, execute that plumbing rule. If there is no matching plumbing rule, then if the word or selection is a file or directory path and there is already a window open with that path as the filename portion of it's tag, change focus to that window and make it visible. If there is not already a window open for that path, open one. Finally, if a plumbing rule is not executed and the path ends in :N, #C or !X then go to line N, character C or regex X in the file.	
	`)
	addCommand("Newcol", c.CmdNewcol, "Create a column", "Newcol creates a new column.")
	addCommand("Delcol", c.CmdDelcol, "Delete the column", "Delcol deletes the column in which it is executed.")
	addCommand("Cut", c.CmdCut, "Cut selected text", "Cut deletes the last selected text and it to the clipboard.")
	addCommand("Snarf", c.CmdSnarf, "Copy selected text", "Snarf copies the last selected text to the clipboard.")
	addCommand("Id", c.CmdId, "Show window ID", "Id prints the window ID to the +Errors window. Useful when using the API.")
	addCommand("Paste", c.CmdPaste, "Paste text", "Paste writes the text from the clipboard to the window.")
	addCommand("Put", c.CmdPut, "Save the window body", "Put writes the contents of the window body to the path that is the leftmost text in the window tag.")
	addCommand("Get", c.CmdGet, "Load the window body", "Get reads the contents of the path that is the leftmost text in the window tag and replaces the window body contents with it.")
	addCommand("Kill", c.CmdKill, "Kill a running job", "Usage: Kill [jobname...]\n\nKill kills all the jobs that are currently running that have names matching the arguments to the Kill command. If no argument is provided the first job is killed")
	addCommand("Look", c.CmdLook, "Look for a string in the window body", "Usage: Look <term>\n\nLook searches for the next string in the window body that exactly matches the argument to Look.")
	addCommand("Keypass", c.CmdKeyPassword, "Specify the password used to decrypt an ssh private key file or log into a host", "Usage: Keypass <keyfile> <password>\n\nKeypass is used to specify the password used to decrypt an ssh private key file. It takes two arguments: the first is the ssh filename and the second is the password. This is needed when an ssh private key file is encrypted and ssh-agent is not being used.")
	addCommand("Hostpass", c.CmdHostPassword, "Specify the password used to log into an ssh server", "Usage: Hostpass <password> <hostname> [username] [port]\n\nHostpass is used to specify the password used to log into an ssh server. It takes between two and four arguments. The first argument is the password. The second argument is the hostname or IP address of the server. The third argument is the username for the server; if not specified the current user's name is used. The fourth argument is the TCP port number for the server; if not specified 22 is used.")
	addCommand("Zerox", c.CmdZerox, "Clone a window", "Zerox opens a second window which is a copy of the current window")
	addCommand("Title", c.CmdTitle, "Set the editor title", "Usage: Title <title...>\n\nTitle sets the title of the editor to it's joined arguments. The title is usually displayed by the OS window manager in the title bar.")
	addCommand("Syn", c.CmdSyntax, "Enable or disable syntax highlighting, or list supported formats", `Usage: Syn <language>  
Syn off  
Syn list
Syn load <filename>

Syn is used to control syntax highlighting for the current window. With the argument 'off' it disables syntax highlighting, and with the argument 'list' it lists the valid supported languages. With the argument 'load' followed by a filename, it loads the lexer defined in the file.

With any other argument it enables syntax highlighting and highlights the body using the language named by the argument. With no argument it attempts to analyze the text to autodetect the language.
`)
	addCommand("Ansi", c.CmdAnsi, "Enable or disable Ansi colors", "Usage: Ansi [on|off]\n\nAnsi is used to control whether Ansi terminal color escape sequences cause coloring or not. With no argument or the 'on' it enables coloring. With the argument 'off' it disables coloring.")
	addCommand("Dump", c.CmdDump, "Save the editor's state to disk", fmt.Sprintf("Usage: Dump [filename]\n\nDump saves the editor's state to disk: the size of the open windows and the current value of their tags. With an argument the state is written to the file named by the argument. With no argument state is written to the file %s.dump. The state can be loaded using Load", editorName))
	addCommand("Load", c.CmdLoad, "Load the editor's state from disk", fmt.Sprintf("Usage: Load [filename]\n\nLoad loads the editor's state from disk as written by the Dump command. With an argument the state is read from the file named by the argument. With no argument state is read from the file %s.dump", editorName))
	addCommand("Putall", c.CmdPutall, "Save all windows", "Putall executes a Put on all open windows, saving all windows.")
	addCommand("Getall", c.CmdGetall, "Get all windows", "Getall executes a Get on all open windows, reloading all windows.")
	addCommand("Recent", c.CmdRecent, "Display recent files", "Recent writes the list of most recently closed files to the Errors window.")
	addCommand("Mark", c.CmdMark, "Add a bookmark", "Usage: Mark [markname]\n\nMark saves the current cursor position in the window body with the name specified by the argument. If no argument is given it is saved with the name 'def'.")
	addCommand("Goto", c.CmdGoto, "Jump to a bookmark", "Usage: Goto [markname]\n\nGoto sets the current cursor position in the window body to the named bookmark, created by Mark. If no argument is given it jumps to the bookmark 'def'.")
	addCommand("Marks", c.CmdMarks, "Display bookmarks", "Marks displays the currently set bookmarks to the Errors window.")
	addCommand("Marks-", c.CmdClearMarks, "Clear bookmarks", "Marks- clears all the currently set bookmarks.")
	addCommand("SaveStyle", c.CmdSaveStyle, "Save current editor style", fmt.Sprintf("Usage: SaveStyle [filename]\n\nSaveStyle saves the editor style information to a file: the current font and size, colors, etc. With one argument the style is saved to the file named by the argument. With no argument it is saved to %s. When the editor is started the style file %s is loaded", StyleConfigFile(), StyleConfigFile()))
	addCommand("LoadStyle", c.CmdLoadStyle, "Load editor style from file", fmt.Sprintf("Usage: LoadStyle [filename]\n\nLoadStyle loads the editor style information from a file: the current font and size, colors, etc. With one argument the style is loaded from the file named by the argument. With no argument it is loaded from %s. When the editor is started the style file %s is loaded", StyleConfigFile(), StyleConfigFile()))
	addCommand("LoadPlumbing", c.CmdLoadPlumbing, "Load plumbing rules from file", fmt.Sprintf("Usage: LoadPlumbing [filename]\n\nLoadPlumbing loads the plumbing rules from a file. With one argument the plumbing is loaded from the file named by the argument. With no argument it is loaded from %s. When the editor is started the plumbing file %s is loaded", PlumbingConfigFile(), PlumbingConfigFile()))
	addCommand("Help", c.CmdHelp, "Show help", "Usage: Help [topic]\n\nHelp shows a bit of help for the editor. With no argument it lists the main commands and a brief description. With an argument displays information about that topic. The argument may be a command, which displays more detail about the command, or it may be another selected topic.")
	addCommand("◊", c.CmdInsertLozenge, "Insert a ◊ rune, or surround selection with it", "If there are no selections, insert a ◊ rune at the cursor. If there are selections, insert a ◊ before and after each selection.")
	addCommand("Rot", c.CmdRot, "Rotate selections", "Rot rotates the selections when there are multiple selections. The primary selection moves to the next selection, that one to the next and so on, with the last moving to the primary.")
	addCommand("Do", c.CmdDo, "Execute command", "Usage: Do <command...>\n\nDo executes it's arguments as a command; i.e. as if the arguments were selceted and executed alone. This is useful to execute commands from one window in the context of another window.")
	addCommand("About", c.CmdAbout, "About the editor",
		`Print information about the editor, including where some files are expected to be located such as:

  *  The configuration directory
  *  The settings file, and if it was loaded on startup
  *  The style configuration file, and if it was loaded on startup
  *  The SSH key directory, and a list of cached SSH connections
  *  The plumbing configuration file
  *  The API listener port, and a list of active API sessions
`)
	addCommand("Font", c.CmdFont, "Change to next font", "Change to the next font defined in the styles")
	addCommand("Fontsize", c.CmdFontSize, "Change the font size", "Change the font size of the current active font. This command accepts one argument. If the argument is a number, then the font size is set to that value in points. If it is +NUM or -NUM then the current font size is increased or decreased by NUM points respectively")
	addCommand("On", c.CmdOn, "Run command on remote host", "Usage: On <path> <command...>\n\nRun takes two or more arguments. The first is a host and directory (in the format host:directory) and the remaining arguments are the command and arguments to run.")
	setHooks("On", c.hooksForOnCommand())
	addCommand("Cmds", c.CmdCmds, "List the recent external commands", "List the most recent external commands executed")
	addCommand("Cmds*", c.CmdCmdsVerbose, "List the recent external commands verbosely", "List the most recent external commands executed along with the directory they were executed in")
	addCommand("Wins", c.CmdWins, "List the open windows", "List the filenames of the open windows")
	addCommand("Undo", c.CmdUndo, "Undo the last change", "Undo the last change")
	addCommand("Redo", c.CmdRedo, "Redo the last change", "Redo the last change")
	addCommand("PrintCfg", c.CmdPrintCfg, "Print a sample config file", "Usage: PrintCfg <default settings file name>\n\nPrint a sample config file to +Errors. The argument specifies the type of config file to print:\n  ◊PrintCfg settings.toml◊ generates a settings file\n")
	addCommand("Only", c.CmdOnly, "Del other windows in this column", "Only takes effect when executed in a window or its tag. With no argument, close the other windows in this column leaving only this window. With the argument 'above', close windows below this window. With the argument 'below', close windows above this window.")
	addCommand("Clr", c.CmdClr, "Clear (delete) the contents of the window body", "Clear (delete) the contents of the window body")
	addCommand("Shstr", c.CmdShstr, "Set the 'Shell String' for the current window",
		`Usage: Shstr [string...]

When executed with one or more arguments, set the 'Shell String' for the current window: the template string that is used to build the command run on a remote system. It may contain these substitutions within braces:

  Dir: The window directory
  Cmd: The name of the command to be executed
  Args: Arguments to the command

The default Shell String (assuming the current shell is sh) is: sh -c $'cd "{Dir}" && {Cmd} {Args}'

When executed with no arguments, set the Shell String for the current window to the default.
`)

	addCommand("Dbg", c.CmdDbg, "Internal debugging commands", c.dbgCommandLongHelp())
	addCommand("Cols", c.CmdCols, "List columns", "Cols lists all the columns and layers")
	addCommand("Cols*", c.CmdColsVerbose, "List columns verbosely", "Cols* lists all the columns and layers verbosely")
	addCommand("Cols**", c.CmdColsVeryVerbose, "List columns very verbosely", "Cols** lists all the columns and layers verbosely (including the files in each column)")
	addCommand("Tint", c.CmdTint, "Colorize selections", `Usage: Tint list  
Tint <color>  
Tint

Tint is used to color selections of text. When executed with the argument 'list' it shows the pre-defined tint colors. When executed with one argument that is not 'list', it changes the text in all current selections to that color. The argument must be a hex color code in the form #rrggbb or a color name. When executed with no argument and selections present, it removes the coloring for text that overlap the selections. When run with no arguments and no selections it clears all tinting.
`)
	addCommand("Fuzz", c.CmdFuzz, "Perform a fuzzy search", `Usage: Fuzz [terms...]

Fuzz performs a fuzzy search through the lines in the window body. The terms for the search are the arguments to the Fuzz command. The lines which match the search are written to a new window for the current directory with the suffix '+Live'.

The Fuzz command is special in that it can be executed dynamically as you type the search terms. If you add the string '◊Fuzz ' to the tag, then as you type the arguments after the command the search is re-executed and the results updated in the +Live window. You can delimit the end of the search arguments using another ◊`)
	addCommand("Pic", c.CmdPic, "Set background picture", "Usage: Pic <filename> [scaling]\n\nPic sets the background picture for the window body. The first argument should be the name of a .png, .gif or .jpeg image. The second argument, if specified, specifies how to scale the image. If the second argument is the word 'fit', without quotes, the image is scaled to the size of the window width. If the second argument is a number followed by the % character (such as 50%) the image is scaled by that percentage.")
	addCommand("Tab", c.CmdTab, "Set the string inserted when tab is pressed", "Usage: Tab <tabstring>\n\nTab sets the string that Anvil inserts when the tab key is pressed. With no argument, sets the tab key to insert the tab character. With one argument it sets the value to insert to that argument. The argument may be quoted with single-quotes, and may contain the escapes \\t, \\n, \\r, \\', \\\", or \\\\.\n\nFor example, to cause the tab insert four spaces, use: Tab '    '. To insert a tab use: Tab '\\t'.")
	addCommand("Settag", c.CmdSettag, "Set tag", "Usage: Settag [tag...]\n\nSettag sets the tag of the current window when executed from a window body or tag, the tag of the current column when executed from a column tag, or the editor when executed from the editor tag. When executed for a window, only the user-editable area is set. This is meant to be used by programs using the API.\n\nThe argument may be quoted with single-quotes.")
	addCommand("Sort", c.CmdSort, "Sort the windows in the column", "Sort sorts the windows in the column by their file paths")
	addCommand("Rel", c.CmdRel, "Make the window paths in the column relative", "If the column has a column path specified, this command makes all of the paths of the windows in the column relative to that column path.")
	addCommand("Wrap", c.CmdWrap, "Enable or disable line wrapping", "When run with no argument, line wrapping is enabled. When run with the argument 'off', word wrapping is disabled.")
	addCommand("Alias", c.CmdAlias, "Create a command alias", `Usage: Alias [name] [command...]
	
Alias creates command aliases. If no arguments are specified it lists the current aliases. If one argument is specified, then that alias is removed. If two arguments are specified, then the first is the name of the alias, and the rest is the aliased command.

An alias may contain placeholders of the form $1 to $9 which are replaced with the corresponding arguments to the alias when it is executed. The placeholder $* is replaced with all the arguments separated by a space. The placeholder $$ is replaced with $.

If the aliased command consists of multiple commands separated by semi-colon, enclose the aliased command in quotes (' or "), for example:

Alias TintDrawers '!x/§[^\s]*/;Tint yellow'
	`)
	addCommand("Elastic", c.CmdElastic, "Enable or disable elastic tab stops", `Usage: Elastic  
Elastic off

When run with no argument, elastic tab stops are enabled for the window body. When run with the argument 'off', elastic tab stops are disabled.`)
	addCommand("Keymap", c.CmdKeymap, "Manipulate keymaps",
		`Usage: Keymap load <file>  
Keymap show def <name> | defs | stack  
Keymap push <name>  
Keymap pop  
Keymap win push <name>  
Keymap win pop  
Keymap win reset

When run with the 'show' argument, this command shows keymap information. The argument 'defs' shows all defined keymaps. The 'def <name>' shows the definition of the keymap with name <name>. The argument 'stack' shows the current keymap stack, with the top being shown last.

When run with the 'load' argument, load a keymap file which contains one or more keymap definitions. When run with the 'push' argument, push a keymap onto the global keymap stack. When run with the 'pop' argument, pop off the top keymap in the stack. The bottommost keymap is never popped.

If the first argument is win, then the keymap for the local window only is modified. If 'win push' or 'win pop' is executed and the window is using the global keymap stack, then the global keymap stack is copied to the window and a keymap pushed to or popped from that window stack respectively. 'win reset' resets the window's keymap stack to be the global keymap stack again.

`)
	addCommand("Newlyr", c.CmdNewLayer, "Create a new layer", "Create a new layer and switch to it")
	addCommand("Dellyr", c.CmdDelLayer, "Delete the current layer", "Delete the current layer")
	addCommand("Up", c.CmdLayerUp, "Make the layer above this one active", "Make the layer above this one active")
	addCommand("Down", c.CmdLayerDown, "Make the layer below this one active", "Make the layer below this one active")
	addCommand("Setlyr", c.CmdSetLayer, "Set which layer index is active", "Set which layer index is active")
	addCommand("Colup", c.CmdMoveColUp, "Move the current column to the layer above", "Move the current column to the layer above")
	addCommand("Coldown", c.CmdMoveColDown, "Move the current column to the layer below", "Move the current column to the layer below")
	addCommand("Colup^", c.CmdMoveColUpAndChangeActiveLayer, "Move the current column to the layer above and make that layer active", "Move the current column to the layer above and make that layer active")
	addCommand("Coldown^", c.CmdMoveColDownAndChangeActiveLayer, "Move the current column to the layer below and make that layer active", "Move the current column to the layer below and make that layer active")
	addCommand("Lyrname", c.CmdSetLayerName, "Set the name of the layer", "Set the name of the active  layer to the concatenation of the arguments separated by spaces")
	addCommand("Mvlyr", c.CmdMoveLayer, "Move this layer to a different position in the layer stack", "Mvlyr moves the current layer to the index in the layer stack passed as a the first argument")
	addCommand("Float", c.CmdFloat, "For prototyping: add a float", "For prototyping: add a float. Arguments are the text")
	addCommand("Trays", c.CmdTrays, "List trays", "List all the trays that are currently created. This command will list all the global-, session- and file-scope trays.")
	addCommand("Deltray", c.CmdDeltray, "Delete a tray", "Delete the tray with the specified name.")
	addCommand("Unsel", c.CmdUnsel, "Unselect all selections", "Unselect all selections")
	addCommand("Ssh", c.CmdSsh, "Manage SSH connections", `Usage: Ssh clear

The 'Ssh clear' command closes all cached Ssh connections.
`)
	addCommand("Expr", c.CmdExpr, "Execute a range expression", "Execute a range expression")
	addCommand("Right", c.CmdColRight, "Scroll the view of the columns to the right", "Scroll the view of the columns to the right")
	addCommand("Left", c.CmdColLeft, "Scroll the view of the columns to the left", "Scroll the view of the columns to the left")
	addCommand("Fetchcol", c.CmdFetchcol, "Move the specified column into view", "Move the specified column into view")
	addCommand("Widen", c.CmdWiden, "Increase the number of visible columns by one", "Increase the number of visible columns by one. If there are hidden columns then a column adjacent to the currently visible columns is made visible. If there are hidden columns on both the left and right of the visible columns then the column from the right is made visible. If there are no hidden columns a new, empty column is created to the right.")
	addCommand("Narrow", c.CmdNarrow, "Decrease the number of visible columns by one", "Decrease the number of visible columns by one. The rightmost column is hidden.")
	addCommand("Pin", c.CmdPin, "Pin the window to the current layer", "Pin the window in which this command is executed to the current layer. If a window is pinned to a layer, a different layer is active, and the window is modified then the window is not made visible by moving it to the active window. For example, a +Errors window pinned in a different layer which is appended to by some output will not be made visible in the current layer.")
	addCommand("Unpin", c.CmdUnpin, "Unpin the window from the current layer", "Unpin the window in which this command is executed from the current layer. If a window is pinned to a layer, a different layer is active, and the window is modified then the window is not made visible by moving it to the active window. For example, a +Errors window pinned in a different layer which is appended to by some output will not be made visible in the current layer.")

}

func initDebugCommandSet() {
	addCommand := func(name string, do func(ctx *CmdContext), shortHelp, longHelp string) {
		defaultDebugCommandSet.AddCommand(name, do, shortHelp, longHelp)
		AddHelp("Dbg "+name, longHelp)
	}

	var c CmdDefs

	addCommand("ProfCpu", c.CmdProfCpu, "Profile CPU usage", "Dbg ProfCpu starts writing profiling information to disk until it is executed a second time at which point it stops profiling.")
	addCommand("ProfHeap", c.CmdProfHeap, "Profile memory usage", "Dbg ProfHeap is a debug command. It starts writing profiling information to disk until it is executed a second time at which point it stops profiling.")
	addCommand("Goroutines", c.CmdGoroutines, "Print all goroutines", "Dbg Goroutines is a debug command. It writes all goroutine stacks to the errors window.")
	addCommand("Logs", c.CmdDbgLogs, "Print or configure internal debug logs", fmt.Sprintf("Usage: Dbg Logs [category...]\nDbg Logs Stream [stdout|window|off]\nDbg Logs Stream Catg [category...]\n\nDbg Logs [catgory] displays internal debug logs to the +Errors window. With no category it writes logs from all categories. With one or more arguments only those categories are printed. The available categories are:\n  %s\nDbg Logs Stream [stdout|window|off] enables streaming all log categories to standard output, the +Errors window, or disables streaming respectively. Dbg Logs Stream Catg [category...] sets the categories to stream.",
		strings.Join(debugLogCategories, "\n  ")))
	addCommand("Pid", c.CmdDbgGetPid, "Print Anvil's PID", "Print the process ID of Anvil")
	addCommand("Psrv", c.CmdDbgPsrv, "Start the Go pprof debug server",
		`This command starts the Go pprof debug http server [1] on localhost port 6060. This server can be used to debug Anvil performance. Once started, some useful URLs to browse are:

  go tool pprof http://localhost:6060/debug/pprof/heap
  go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30
  go tool pprof http://localhost:6060/debug/pprof/block
  go tool pprof http://localhost:6060/debug/pprof/mutex

If the argument 'off' is passed, the debug server is stopped.

[1] https://pkg.go.dev/net/http/pprof
	`)
	addCommand("Paths", c.CmdDbgPaths, "Print window paths", "Print window paths")
	addCommand("Flame", c.CmdDbgFlame, "Profile CPU and show flame graph", "With the argument 'on' this command starts CPU profiling. When run with the command 'off' it stops CPU profiling and serves an SVG flame graph on a newly created HTTP server. With the command 'kill' it kills the running HTTP server.")
	addCommand("Mouse", c.CmdDbgMouse, "Log mouse move events", "With the argument 'on' this command causes Anvil to print mouse move events to the log. When run with the command 'off' it stops logging the events. This is useful to debug widget positioning issues.")
}

type CommandExecutor struct {
	// source is a Window, Col or Editor.
	source interface{}
	commandSet
	debugCommandSet commandSet
}

type command struct {
	name      string
	do        func(ctx *CmdContext)
	shortHelp string
	longHelp  string
	hooks     commandHooks
}

type commandHooks struct {
	clearErrorsWindow func(ctx *CmdContext)
}

type commandSet struct {
	commands map[string]command
}

func (c *commandSet) AddCommand(name string, do func(ctx *CmdContext), shortHelp, longHelp string) {
	if c.commands == nil {
		c.commands = map[string]command{}
	}

	c.commands[name] = command{
		name:      name,
		do:        do,
		shortHelp: shortHelp,
		longHelp:  longHelp,
	}
}

func (c *commandSet) setHooks(name string, h commandHooks) {
	cmd := c.commands[name]
	cmd.hooks = h
	c.commands[name] = cmd
}

func (c *commandSet) Command(name string) (cmd command, ok bool) {
	cmd, ok = c.commands[name]
	return
}

func (c *commandSet) Commands() []command {
	var l []command
	for _, v := range c.commands {
		l = append(l, v)
	}
	return l
}

func (c commandSet) addCommandlistToHelp() {
	var text bytes.Buffer
	var names []string
	for k := range c.commands {
		names = append(names, k)
	}
	sort.Strings(names)

	for _, k := range names {
		v := c.commands[k]
		fmt.Fprintf(&text, "%s  (◊Help %s◊)\n\t%s\n", k, k, v.shortHelp)
	}
	text.WriteRune('\n')

	AddHelp("Commands", text.String())
}

func NewCommandExecutor(source interface{}) *CommandExecutor {
	ex := &CommandExecutor{
		source: source,
	}
	ex.initCommands()
	return ex
}

func (c *CommandExecutor) initCommands() {
	c.commandSet = defaultCommandSet
	c.debugCommandSet = defaultDebugCommandSet
}

func (c *CmdDefs) hooksForOnCommand() commandHooks {
	return commandHooks{clearErrorsWindow: cmdOnClearErrorsWindowHook}
}

func (c *CmdDefs) dbgCommandLongHelp() string {
	var buf bytes.Buffer

	buf.WriteString("Dbg is used to run commands to help debug the internals of Anvil. The available commands are:\n\n")

	for _, c := range defaultDebugCommandSet.Commands() {
		fmt.Fprintf(&buf, "%s  (◊Help %s◊)  \n\t%s  \n", c.name, "Dbg "+c.name, c.shortHelp)
	}
	return buf.String()
}

func (c CommandExecutor) Do(cmd string, ctx *CmdContext) {
	cmd = strings.TrimLeft(cmd, " \t\n\r")

	ctx.source = c.source
	ctx.executor = &c

	commandList := c.splitCommandOnSemicolons(cmd)

	if len(commandList) == 1 {
		c.doSingleCommand(cmd, ctx)
		return
	}

	w := &CommandListWork{
		commandList: commandList,
		ctx:         ctx,
		executor:    &c,
	}

	go func() {
		editor.WorkChan() <- w
	}()
}

type CommandListWork struct {
	commandList []string
	index       int
	ctx         *CmdContext
	executor    *CommandExecutor
}

func (c *CommandListWork) Service() (done bool) {
	if c.index >= len(c.commandList) {
		return true
	}

	cmd := c.commandList[c.index]
	future := c.executor.doSingleCommand(cmd, c.ctx)
	c.index++

	done = c.index == len(c.commandList)

	if !done {
		go func() {
			future.Wait()
			editor.WorkChan() <- c
		}()
	}

	return
}

func (c *CommandListWork) Job() Job {
	return nil
}

func (c CommandExecutor) doSingleCommand(cmd string, ctx *CmdContext) sync.Future {
	rawCmd := cmd

	ctx = c.copyCtx(ctx)
	cmd, ctx.Args = c.split(cmd, ctx.Args)
	ctx.RawCommand = rawCmd

	if len(cmd) == 0 {
		return sync.CompletedFuture
	}

	log(LogCatgCmd, "CommandExecutor.Do: execute '%s', args %v\n", cmd, ctx.Args)
	var cd CmdDefs

	r, n := utf8.DecodeRuneInString(cmd)
	switch r {
	case '|':
		c.maybeClearErrorsWindow(ctx)
		return cd.CmdExecPipe(cmd[1:], ctx)
	case '>':
		// TODO: Fix up CmdExecGt to use futures
		c.maybeClearErrorsWindow(ctx)
		cd.CmdExecGt(cmd[1:], ctx)
		return sync.CompletedFuture
	case '<':
		c.maybeClearErrorsWindow(ctx)
		return cd.CmdExecLt(cmd[1:], ctx)
	case '!':
		c.maybeClearErrorsWindow(ctx)
		return cd.CmdExprBang(rawCmd[1:], ctx)
	case '§':
		c.maybeClearErrorsWindow(ctx)
		cd.CmdOpenTray(rawCmd[n:], ctx)
		return sync.CompletedFuture
	}

	handled := c.tryAlias(ctx, cmd)
	if handled {
		return sync.CompletedFuture
	}

	doer, ok := c.Command(cmd)
	if ok {
		c.maybeClearErrorsWindowForCommand(&doer, ctx)

		doer.do(ctx)
		return sync.CompletedFuture
	}

	handled = c.tryApiUserDefinedCommand(ctx, cmd)
	if handled {
		return sync.CompletedFuture
	}

	c.maybeClearErrorsWindow(ctx)
	return c.tryOsCmd(ctx, cmd)
}

func (c CommandExecutor) copyCtx(ctx *CmdContext) *CmdContext {
	lctx := new(CmdContext)
	*lctx = *ctx
	return lctx
}

func (c CommandExecutor) split(cmd string, args []string) (newcmd string, newargs []string) {
	a := strings.Fields(cmd)
	if len(a) <= 1 {
		newcmd = cmd
		newargs = args
		return
	}

	newcmd = a[0]
	newargs = a[1:]
	newargs = append(newargs, args...)
	return
}

func (c CommandExecutor) maybeClearErrorsWindow(ctx *CmdContext) {
	c.maybeClearErrorsWindowForCommand(nil, ctx)
}

func (c CommandExecutor) maybeClearErrorsWindowForCommand(cmd *command, ctx *CmdContext) {
	if ctx.Editable == nil {
		return
	}

	if ctx.Flags&CmdFlagClearErrorsWindow == 0 {
		return
	}

	if cmd != nil {
		if cmd.hooks.clearErrorsWindow != nil {
			cmd.hooks.clearErrorsWindow(ctx)
			return
		}

		// Built-in commands usually output to +Errors
		editor.ClearErrors("")
		return
	}

	ctx.Editable.adapter.clearErrors()
}

type CmdContext struct {
	Gtx          layout.Context
	Dir          string
	Completer    *PathCompleter
	Editable     *editable
	Args         []string
	Selections   []*selection
	ShellString  string
	RawCommand   string
	EventContext *EventContext
	Flags        CmdFlags
	// source is a Window, Col or Editor.
	source   interface{}
	executor *CommandExecutor
}

type CmdFlags uint

const (
	CmdFlagClearErrorsWindow = 1 << iota
)

type EventContext struct {
	globalX, globalY float32
	editable         *editable
}

func (c CmdContext) CombinedArgs() string {
	return strings.Join(c.Args, " ")
}

func (c CmdDefs) CmdDel(ctx *CmdContext) {
	switch v := ctx.source.(type) {
	case Window:
	case *Window:
		v.Del()
	case *Float:
		editor.DelFloat(v)
	}
}

func (c CmdDefs) delWindows(wins ...*Window) (someNotDeleted bool) {
	winsNotDeleted := make([]*Window, 0, len(wins))
	for _, w := range wins {
		notDeleted := w.Del()
		if notDeleted {
			winsNotDeleted = append(winsNotDeleted, w)
		}
	}

	someNotDeleted = len(winsNotDeleted) > 0
	return
}

func (c CmdDefs) CmdDelForce(ctx *CmdContext) {
	switch w := ctx.source.(type) {
	case Window:
	case *Window:
		editor.DelWindow(w)
	}
}

func (c CommandExecutor) displayWindowDeletionError(w *Window) {
	editor.AppendError("", fmt.Sprintf("Refusing to delete window for %s because it has unsaved changes. Delete again if you are sure.", w.displayPath.String()))
}

func (c CmdDefs) CmdExit(ctx *CmdContext) {
	wins := editor.Windows()

	someNotDeleted := c.delWindows(wins...)
	if someNotDeleted {
		return
	}
	Exit(0)
}

func (c CmdDefs) CmdNew(ctx *CmdContext) {
	path := ""
	if len(ctx.Args) > 0 {
		path = ctx.Args[0]
	}
	path = strings.TrimSpace(path)

	path, seek, err := parseSeekFromFilename(path)
	if err != nil {
		editor.AppendError("", fmt.Sprintf("Parsing seek failed: %v", err))
	}

	if path == "" {
		loadPath := NewGlobalPath("", GlobalPathUnknown)
		displayPath := loadPath.Clone()
		w := editor.NewWindow(c.column(ctx))
		if w == nil {
			return
		}
		w.markTextAsUnchanged()
		w.SetPathsAndTag(displayPath, loadPath)
		w.SetFocus(ctx.Gtx)
		return

	}

	loadPath, displayPath := completeDisplayAndLoadPaths(ctx.Completer, path)

	var opts LoadFileOpts
	if !seek.empty() {
		opts = LoadFileOpts{GoTo: seek, SelectBehaviour: selectText, GrowBodyBehaviour: dontGrowBodyIfTooSmall}
	}
	w := editor.LoadFileOpts(displayPath, loadPath, opts)
	if w != nil {
		w.SetFocus(ctx.Gtx)
	}

	editor.notifyFileOpened(w)
}

func (c CmdDefs) globalizeAndMakeAbsolute(dir, path string) (fullpath string, err error) {
	var gpath *GlobalPath
	gpath = NewGlobalPath(path, GlobalPathIsFile)

	var d *GlobalPath
	d = NewGlobalPath(dir, GlobalPathIsDir)

	if gpath.IsRemote() {
		fullpath = gpath.String()
		return
	}

	if d.IsRemote() && !gpath.IsRemote() {
		gpath = gpath.GlobalizeRelativeTo(d)
	}

	if !gpath.IsAbsolute() {
		gpath = gpath.MakeAbsoluteRelativeTo(d)
	}

	fullpath = gpath.String()
	return
}

func (c CmdDefs) CmdAcq(ctx *CmdContext) {
	path := ""
	if len(ctx.Args) > 0 {
		path = ctx.CombinedArgs()
	}

	if plumber != nil {
		plumbed, err := plumber.Plumb(path, ctx.executor, ctx)
		if err != nil {
			log(LogCatgCmd, "CommandExecutor: Error plumbing: %v\n", err)
		}

		if plumbed {
			return
		}
	}

	path, seek, err := parseSeekFromFilename(path)
	if err != nil {
		editor.AppendError("", fmt.Sprintf("Parsing seek failed: %v", err))
	}

	loadPath, displayPath := completeDisplayAndLoadPaths(ctx.Completer, path)

	if strings.TrimSpace(path) == "" {
		w := editor.NewWindow(c.column(ctx))
		if w == nil {
			return
		}
		w.markTextAsUnchanged()
		w.SetPathsAndTag(displayPath, loadPath)
		w.SetFocus(ctx.Gtx)
		return
	}

	var opts LoadFileOpts
	if !seek.empty() {
		opts = LoadFileOpts{GoTo: seek, SelectBehaviour: selectText, GrowBodyBehaviour: dontGrowBodyIfTooSmall}
	}
	w := editor.LoadFileOpts(displayPath, loadPath, opts)
	if w != nil {
		w.SetFocus(ctx.Gtx)
	}
}

func (c CmdDefs) column(ctx *CmdContext) *Col {
	var col *Col

	switch v := ctx.source.(type) {
	case Window:
	case *Window:
		col = v.col
	case Col:
		col = &v
	case *Col:
		col = v
	case *Float:
		if v.evoker != nil {
			if win := v.evoker.adapter.window(); win != nil {
				col = win.col
			}
		}
	}

	return col
}

func (c CmdDefs) CmdDelcol(ctx *CmdContext) {
	col, ok := ctx.source.(*Col)
	if !ok {
		return
	}

	editor.markForRemoval(col)

	if activeLayer := editor.activeLayer(); activeLayer != nil {
		if h := activeLayer.AnyHiddenCol(); h != nil {
			editor.MoveColToLayerRelativeToCurrent(h, 0)
		}
	}

	editor.SignalRedrawRequired()
}

func (c CmdDefs) CmdNewcol(ctx *CmdContext) {
	col := editor.NewCol()
	col.Tag.SetTextStringNoUndo(settings.Layout.ColumnTag)
	editor.SignalRedrawRequired()
}

func addCommandToHistory(dir, cmd, arg string) *CommandHistoryEntry {
	return cmdHistory.Started(dir, fmt.Sprintf("%s %s", cmd, arg))
}

func markCommandCompletedInHistory(e *CommandHistoryEntry) {
	cmdHistory.Completed(e)
}

func setExitCodeInHistory(e *CommandHistoryEntry, c int) {
	cmdHistory.SetExitCode(e, c)
}

func (c CommandExecutor) tryApiUserDefinedCommand(ctx *CmdContext, command string) (handled bool) {
	winId := -1
	switch v := ctx.source.(type) {
	case Window:
	case *Window:
		winId = v.Id
	}

	return apiHandleCommand(winId, command, ctx.Args)
}

func printErrs(c chan error) (d chan error) {
	d = make(chan error)
	go func() {
		for e := range d {
			log(LogCatgCmd, "CommandExecutor: command got error %v %T %T\n", e, e, errors.Unwrap(e))
			if ex, ok := e.(*exec.ExitError); ok {
				log(LogCatgCmd, "CommandExecutor: command got error; exit code: %d\n", ex.ExitCode())
			}
			c <- e
		}
		close(c)
	}()
	return
}

func snoopAndSaveFirstError(c chan error, entry *CommandHistoryEntry) (d chan error) {
	d = make(chan error)
	go func() {
		for e := range d {
			log(LogCatgCmd, "Snooped an error and it is a %T\n", e)
			switch t := e.(type) {
			case *exec.ExitError:
				setExitCodeInHistory(entry, t.ExitCode())
			case *ssh.ExitError:
				setExitCodeInHistory(entry, t.ExitStatus())
			}
			c <- e
		}
		close(c)
	}()
	return

}

func mustRunCommandLocally(cmd string) bool {
	return len(cmd) > 0 && cmd[0] == '+'
}

func adjustLocallyRunCommand(cmd string) (newCmd string, dir string) {
	newCmd = cmd[1:]
	dir = ""
	return
}

func (c CommandExecutor) tryOsCmd(ctx *CmdContext, command string) sync.Future {

	dir := ctx.Dir

	if mustRunCommandLocally(command) {
		command, dir = adjustLocallyRunCommand(command)
	}

	sfs, err := GetFs(dir)
	if err != nil {
		editor.AppendError(dir, err.Error())
		return sync.CompletedFuture
	}

	load := NewDataLoad()

	done := make(chan struct{})

	var cd CmdDefs
	ec := execCtx{
		dir:         dir,
		cmd:         command,
		arg:         ctx.CombinedArgs(),
		contents:    load.Contents,
		errs:        load.Errs,
		kill:        load.Kill,
		done:        done,
		shellString: ctx.ShellString,
	}
	cd.setExtraEnv(ctx, &ec)

	hist := addCommandToHistory(dir, ec.cmd, ec.arg)
	ec.errs = snoopAndSaveFirstError(ec.errs, hist)

	err = sfs.execAsync(ec)
	if err != nil {
		log(LogCatgCmd, "CommandExecutor.tryOsCmd: error executing '%s': %v\n", command, err)
		editor.AppendError(dir, err.Error())
		markCommandCompletedInHistory(hist)
		return sync.CompletedFuture
	}

	future := sync.NewFuture()
	go func() {
		<-done
		markCommandCompletedInHistory(hist)
		future.Done()
	}()

	wl := &WindowDataLoad{
		DataLoad: *load,
		Win:      NewWindowHolderForName(editor.ErrorsFileNameOf(dir)),
		Jobname:  command,
		Opts:     LoadFileOpts{GrowBodyBehaviour: growBodyIfTooSmall, Tail: true},
	}

	wl.Start(editor.WorkChan())

	editor.AddJob(wl)

	return future
}

func (c CommandExecutor) tryAlias(ctx *CmdContext, command string) (handled bool) {
	alias, ok := settings.Alias[command]
	if !ok {
		return
	}

	args := ctx.Args
	if ctx.Args != nil {
		ctx.Args = ctx.Args[:0]
	}

	alias = substitute(alias, args)
	c.Do(alias, ctx)

	return true
}

// splitCommandOnSemicolons splits the string `command` which contains subcommands delimited by
// ';' into separate commands. It attempts to preserve OS commands that contain a semicolon
// as one single command. For example, the string "ls -la; echo hi; Load; ls; echo hi" would
// be split into the commands "ls -la; echo hi", "Load", "ls; echo hi"
func (c CommandExecutor) splitCommandOnSemicolons(command string) (commands []string) {
	// Do not try and split the command Expr. Splitting fails for expressions like /start/;/end/
	if strings.HasPrefix(command, "Expr ") {
		return []string{command}
	}

	cmds := c.splitStringOnSemicolons(command)
	isInternal := make([]bool, len(cmds))

	for i, cmd := range cmds {
		cmd := strings.TrimSpace(cmd)
		cmds[i] = cmd
		cmdName := cmd
		s := strings.IndexFunc(cmd, unicode.IsSpace)
		if s > 0 {
			cmdName = cmd[0:s]
		}

		if c.isDefinedCommandOrAlias(cmdName) {
			isInternal[i] = true
		}
	}

	externalRunStart := -1
	for i, cmd := range cmds {
		//fmt.Printf("splitCommandOnSemicolons: i: %d, cmd: %s, externalRunStart: %d\n", i, cmd, externalRunStart)
		if isInternal[i] {
			//fmt.Printf("splitCommandOnSemicolons: %s is internal\n", cmd)
			if externalRunStart != -1 {
				j := strings.Join(cmds[externalRunStart:i], "; ")
				commands = append(commands, j)
			}
			if len(cmd) > 0 {
				commands = append(commands, cmd)
			}
			externalRunStart = -1
		} else {
			if externalRunStart == -1 && len(cmd) > 0 {
				externalRunStart = i
			}
		}

		if i == len(cmds)-1 && externalRunStart != -1 {
			j := strings.Join(cmds[externalRunStart:i+1], "; ")
			commands = append(commands, j)
		}
	}

	if commands == nil {
		commands = []string{}
	}

	return commands
}

// splitStringOnSemicolons is a helper function used by splitCommandOnSemicolons
// that splits a string on ;, but avoids splitting at positions where ; is preceeded by \,
// or is enclosed by quotes or // .
func (c CommandExecutor) splitStringOnSemicolons(command string) (commands []string) {
	const (
		normal = iota
		inQuote
	)

	countOfPreceedingConsecutiveBackslashes := func(i int) int {
		c := 0
		for i--; i >= 0; i-- {
			if command[i] == '\\' {
				c++
				continue
			}
			break
		}
		return c
	}

	isEven := func(n int) bool {
		return n%2 == 0
	}

	seemsLikeSlashIsPartOfRangeStatement := func(index, lastSplit int) bool {
		for _, r := range command[lastSplit+1:] {
			if unicode.IsSpace(r) {
				continue
			}
			return r == '!'
		}
		return false
	}

	state := normal
	lastSplit := -1
	var quoteChar rune

	for i, r := range command {
		switch state {
		case normal:
			switch r {
			case '"':
				state = inQuote
				quoteChar = '"'
			case '\'':
				state = inQuote
				quoteChar = '\''
			case '/':
				if seemsLikeSlashIsPartOfRangeStatement(i, lastSplit) {
					state = inQuote
					quoteChar = '/'
				}
			case ';':
				commands = append(commands, command[lastSplit+1:i])
				lastSplit = i
			}
		case inQuote:
			if r == quoteChar {
				if isEven(countOfPreceedingConsecutiveBackslashes(i)) {
					state = normal
				}
			}
		}
	}

	commands = append(commands, command[lastSplit+1:len(command)])

	if commands == nil {
		commands = []string{}
	}

	return commands
}

func (c CommandExecutor) isDefinedCommandOrAlias(s string) bool {
	if len(s) == 0 {
		return false
	}

	switch s[0] {
	case '|', '>', '<', '!':
		return true
	}

	_, ok := settings.Alias[s]
	if ok {
		return true
	}

	_, ok = c.Command(s)
	if ok {
		return true
	}

	return apiCommandIsDefined(s)
}

// substitute replaces escapes in the form $1 to $9 with
// the value from `replacements` at that index -1. For example,
// $1 is replaced with replacements[0]. Additionally $* is replaced
// with all replacement entries separated by a space. $$ is replaced with $.
func substitute(template string, replacements []string) string {
	var buf bytes.Buffer
	const (
		normal = iota
		inEscape
	)
	state := normal
	for _, c := range template {
		switch state {
		case normal:
			if c == '$' {
				state = inEscape
				continue
			}
			buf.WriteRune(c)
		case inEscape:
			switch {
			case c == '$':
				buf.WriteRune(c)
			case c == '*':
				buf.WriteString(strings.Join(replacements, " "))
				continue
			case unicode.IsDigit(c):
				v, err := strconv.Atoi(string(c))
				if err != nil {
					continue
				}
				if v < 1 || v > len(replacements) {
					continue
				}
				buf.WriteString(replacements[v-1])
			}
			state = normal
		}
	}

	return buf.String()
}

func (c CmdDefs) setExtraEnv(ctx *CmdContext, ex *execCtx) {

	localizeDir := func(dir string) string {
		glb := NewGlobalPath(dir, GlobalPathIsDir)
		return glb.Path()
	}

	winId := ""
	winGlobalPath := ""
	winLocalPath := ""
	winGlobalDir := ""
	winLocalDir := ""
	winPathBase := ""

	setPropsFromWindow := func(w *Window) {
		winId = strconv.Itoa(w.Id)
		winGlobalPath = w.loadPath.String()
		winLocalPath = w.loadPath.Path()
		winGlobalDir = ctx.Dir
		winLocalDir = localizeDir(ctx.Dir)
		winPathBase = w.loadPath.Base()
	}

	switch v := ctx.source.(type) {
	case *Window:
		setPropsFromWindow(v)
	case *Float:
		var win *Window
		if v.evoker != nil {
			win = v.evoker.adapter.window()
		}
		if win != nil {
			setPropsFromWindow(win)
		}
	}

	ex.extraEnv = []string{
		fmt.Sprintf("ANVIL_WIN_ID=%s", winId),
		fmt.Sprintf("ANVIL_WIN_GLOBAL_PATH=%s", winGlobalPath),
		fmt.Sprintf("ANVIL_WIN_LOCAL_PATH=%s", winLocalPath),
		fmt.Sprintf("ANVIL_WIN_GLOBAL_DIR=%s", winGlobalDir),
		fmt.Sprintf("ANVIL_WIN_LOCAL_DIR=%s", winLocalDir),
		fmt.Sprintf("ANVIL_CFG_DIR=%s", ConfDir),
		fmt.Sprintf("f=%s", winLocalPath),
		fmt.Sprintf("b=%s", winPathBase),
		fmt.Sprintf("d=%s", winLocalDir),
	}

	if d, err := os.Getwd(); err == nil {
		ex.extraEnv = append(ex.extraEnv, fmt.Sprintf("ANVIL_DIR=%s", d))
	}

	if activeLayer := editor.activeLayer(); activeLayer != nil {
		ex.extraEnv = append(ex.extraEnv, fmt.Sprintf("ANVIL_LAYER_NAME=%s", activeLayer.Name))
	}

	for k, v := range settings.Env {
		ex.extraEnv = append(ex.extraEnv, fmt.Sprintf("%s=%s", k, v))
	}
}

type CmdDefs struct{}

func (c CmdDefs) CmdCut(ctx *CmdContext) {
	//editor.cutLastSelection(ctx.Gtx)
	editor.cutAllSelectionsFromLastSelectedEditable(ctx.Gtx)
}

func (c CmdDefs) CmdSnarf(ctx *CmdContext) {
	//editor.copyLastSelection(ctx.Gtx)
	editor.copyAllSelectionsFromLastSelectedEditable(ctx.Gtx)
}

func (c CmdDefs) CmdId(ctx *CmdContext) {
	switch v := ctx.source.(type) {
	case Window:
	case *Window:
		editor.AppendError("", fmt.Sprintf("%d", v.Id))
	}

}

func (c CmdDefs) CmdPaste(ctx *CmdContext) {
	editor.pasteToFocusedEditable(ctx.Gtx)
}

func (c CmdDefs) CmdPut(ctx *CmdContext) {
	switch v := ctx.source.(type) {
	case Window:
	case *Window:
		v.Put()
	case *Float:
		if v.evoker != nil {
			win := v.evoker.adapter.window()
			win.Put()
		}
	}
}

func (c CmdDefs) CmdGet(ctx *CmdContext) {
	var win *Window
	switch v := ctx.source.(type) {
	case *Window:
		win = v
	case *Float:
		if v.evoker != nil {
			win = v.evoker.adapter.window()
		}
	}

	if win == nil {
		return
	}

	win.Get()
	win.SetFocus(ctx.Gtx)
}

func (c CmdDefs) CmdKill(ctx *CmdContext) {
	if len(ctx.Args) == 0 {
		editor.KillJob("")
		return
	}

	for _, s := range ctx.Args {
		editor.KillJob(s)
	}
}

func (c CmdDefs) CmdLook(ctx *CmdContext) {
	needle := ctx.CombinedArgs()
	ctx.Editable.SearchAndUpdateEditable(ctx.Gtx, needle, ctx.Editable.firstCursorIndex(), Forward)
	ctx.Editable.SetFocus(ctx.Gtx)
}

func (c CmdDefs) CmdKeyPassword(ctx *CmdContext) {
	if len(ctx.Args) < 2 {
		editor.AppendError("", "Not enough arguments to Keypass")
		return
	}
	file := ctx.Args[0]
	pass := ctx.Args[1]
	sshClientCache.SetKeyfilePassword(file, pass)
	editor.AppendError("", fmt.Sprintf("Added keyfile password for %s", file))
}

func (c CmdDefs) CmdHostPassword(ctx *CmdContext) {
	if len(ctx.Args) < 2 {
		editor.AppendError("", "Not enough arguments to Hostpass")
		return
	}

	pass := ctx.Args[0]
	host := ctx.Args[1]
	user := ""
	port := ""
	if len(ctx.Args) > 2 {
		user = ctx.Args[2]
	}
	if len(ctx.Args) > 3 {
		port = ctx.Args[3]
	}
	hop := sshClientCache.SetSshHopPassword(user, host, port, pass)
	editor.AppendError("", fmt.Sprintf("Added host password for %s", hop))
}

func (c CmdDefs) CmdZerox(ctx *CmdContext) {
	if editor.focusedWindow == nil {
		return
	}

	_, err := editor.focusedWindow.Zerox()
	if err != nil {
		editor.AppendError("", fmt.Sprintf("Zerox failed: %v", err.Error()))
		return
	}

}

func (c CmdDefs) CmdTitle(ctx *CmdContext) {
	if len(ctx.Args) < 1 {
		application.SetTitle(editorName)
	}

	application.SetTitle(ctx.CombinedArgs())
}

func (c CmdDefs) textToPipe(ctx *CmdContext) (text []string, selections []*selection) {
	if ctx.Editable.SelectionsPresent() {
		for _, sel := range ctx.Editable.selectionsInDisplayOrder() {
			text = append(text, ctx.Editable.textOfSelection(sel))
			selections = append(selections, sel)
		}
		return
	}

	text = []string{ctx.Editable.String()}
	return
}

func (c CmdDefs) CmdExecPipe(command string, ctx *CmdContext) sync.Future {
	log(LogCatgCmd, "CommandExecutor.CmdExecPipe: running command %s\n", command)

	text, sels := c.textToPipe(ctx)
	dir := ctx.Dir

	if mustRunCommandLocally(command) {
		command, dir = adjustLocallyRunCommand(command)
	}

	sfs, err := GetFs(dir)
	if err != nil {
		editor.AppendError(dir, err.Error())
		return sync.CompletedFuture
	}

	var cf sync.CompoundFuture
	for i, t := range text {
		sel := (*selection)(nil)
		if sels != nil && i < len(sels) {
			sel = sels[i]
		}
		future := c.execPipeForOneSelection(command, ctx, dir, t, sel, sfs)
		cf = append(cf, future)
	}
	return cf
}

func (c CmdDefs) execPipeForOneSelection(command string, ctx *CmdContext, dir string, text string, sel *selection, sfs simpleFs) sync.Future {
	load := NewDataLoad()
	done := make(chan struct{})

	ec := execCtx{
		dir:      dir,
		cmd:      command,
		arg:      ctx.CombinedArgs(),
		stdin:    []byte(text),
		contents: load.Contents,
		errs:     load.Errs,
		kill:     load.Kill,
		done:     done,
	}
	c.setExtraEnv(ctx, &ec)

	err := sfs.execAsync(ec)
	if err != nil {
		log(LogCatgCmd, "CommandExecutor.CmdExecPipe: error executing '%s': %v\n", command, err)
		editor.AppendError(dir, err.Error())
		return sync.CompletedFuture
	}

	var makeWork func(job Job, ed *editable, data []byte, first bool) Work
	if sel != nil {
		makeWork = func(job Job, ed *editable, data []byte, first bool) Work {
			return &edAppendToSelection{job: job, ed: ed, data: data, first: first, sel: sel}
		}
	} else {
		makeWork = func(job Job, ed *editable, data []byte, first bool) Work {
			return &edAppend{job: job, ed: ed, data: data, first: first}
		}
	}

	future := sync.NewFuture()
	go func() {
		<-done
		future.Done()
	}()

	wl := &EditableModify{
		DataLoad: *load,
		Jobname:  command,
		Editable: ctx.Editable,
		MakeWork: makeWork,
	}

	wl.Start(editor.WorkChan())

	editor.AddJob(wl)

	return future
}

func (c CmdDefs) CmdExecGt(command string, ctx *CmdContext) {
	// This code is a little complex. We want to support running the >command on multiple selections,
	// but also want the output in the +Errors window generated for each selection to appear in the order
	// that the selections appear in the input text. If we just ran the commands asynchronously the output
	// could intermix or appear in the wrong order. To solve this we make a linked list of jobs, one per
	// selection and execute then in order. When the first one completes, the editor checks if there is another
	// in the list and executes that. The list nodes are GtExecutor structs, and each may or may not have
	// it's next property set.
	//
	// Since the work and jobs for loading the data into the editable are separate entities from the GtExecutor,
	// and it is the GtExecutor that knows what job to run next, we need a way for the work's job to
	// refer to the current GtExecutor so that we can get the next when the job completes. We do this
	// by overriding the Job that the WindowDataLoad usually returns to be a GtExecutorJob that is a
	// facade for the WindowDataLoad for the purposes of Killing and Naming the job, but that implements
	// a StartNexter so that we can start the next job when the current one ends.

	log(LogCatgCmd, "CommandExecutor.CmdExecGt: running command %s\n", command)

	text, _ := c.textToPipe(ctx)
	dir := ctx.Dir

	if mustRunCommandLocally(command) {
		command, dir = adjustLocallyRunCommand(command)
	}

	sfs, err := GetFs(dir)
	if err != nil {
		editor.AppendError(dir, err.Error())
		return
	}

	var first, last *GtExecutor

	for _, t := range text {

		executor := c.gtExecutorForOneSelection(command, ctx, dir, t, sfs)

		if executor == nil {
			continue
		}

		if last != nil {
			last.next = executor
			last = executor
			continue
		}

		if first == nil {
			first = executor
			last = executor
			continue
		}
	}

	/*log(LogCatgCmd,"CommandExecutor.CmdExecGt: job list:\n")
	for n := first; n != nil; n = n.next {
		log(LogCatgCmd,"  %p\n", n)
	}*/

	if first != nil {
		first.Start()
	}
}

func (c CmdDefs) gtExecutorForOneSelection(command string, ctx *CmdContext, dir string, text string, sfs simpleFs) *GtExecutor {
	load := NewDataLoad()

	ec := execCtx{
		dir:      dir,
		cmd:      command,
		arg:      ctx.CombinedArgs(),
		stdin:    []byte(text),
		contents: load.Contents,
		errs:     load.Errs,
		kill:     load.Kill,
	}
	c.setExtraEnv(ctx, &ec)

	ge := &GtExecutor{
		load:    load,
		execCtx: ec,
		sfs:     sfs,
	}

	return ge
}

type GtExecutor struct {
	load    *DataLoad
	execCtx execCtx
	sfs     simpleFs
	next    *GtExecutor
}

func (g GtExecutor) StartNext() {
	if g.next != nil {
		g.next.Start()
	}
}

func (g *GtExecutor) Start() {
	log(LogCatgCmd, "GtExecutor.Start: called for %p\n", &g)
	err := g.sfs.execAsync(g.execCtx)
	if err != nil {
		log(LogCatgCmd, "CommandExecutor.CmdExecPipe: error executing '%s': %v\n", g.execCtx.cmd, err)
		editor.AppendError(g.execCtx.dir, err.Error())
		g.StartNext()
		return
	}

	wl := &WindowDataLoad{
		DataLoad: *g.load,
		Win:      NewWindowHolderForName(editor.ErrorsFileNameOf(g.execCtx.dir)),
		Jobname:  g.execCtx.cmd,
		Opts:     LoadFileOpts{GrowBodyBehaviour: growBodyIfTooSmall, Tail: true},
	}

	wl.Start(editor.WorkChan())
	j := &GtExecutorJob{
		executor:    g,
		winDataLoad: wl,
	}
	wl.Job = j
	editor.AddJob(j)
}

type GtExecutorJob struct {
	executor    *GtExecutor
	winDataLoad *WindowDataLoad
}

func (j GtExecutorJob) Kill() {
	j.winDataLoad.Kill()
}

func (j GtExecutorJob) Name() string {
	return j.winDataLoad.Name()
}

func (j GtExecutorJob) StartNext() {
	j.executor.StartNext()
}

func (c CmdDefs) CmdExecLt(command string, ctx *CmdContext) sync.Future {
	log(LogCatgCmd, "CommandExecutor.CmdExecLt: running command %s\n", command)

	dir := ctx.Dir

	if mustRunCommandLocally(command) {
		command, dir = adjustLocallyRunCommand(command)
	}

	sfs, err := GetFs(dir)
	if err != nil {
		editor.AppendError(dir, err.Error())
		return sync.CompletedFuture
	}

	load := NewDataLoad()
	done := make(chan struct{})

	ec := execCtx{
		dir:      dir,
		cmd:      command,
		arg:      ctx.CombinedArgs(),
		contents: load.Contents,
		errs:     load.Errs,
		kill:     load.Kill,
		done:     done,
	}
	c.setExtraEnv(ctx, &ec)

	err = sfs.execAsync(ec)
	if err != nil {
		log(LogCatgCmd, "CommandExecutor.CmdExecLt: error executing '%s': %v\n", command, err)
		editor.AppendError(dir, err.Error())
		return sync.CompletedFuture
	}

	future := sync.NewFuture()
	go func() {
		<-done
		future.Done()
	}()

	wl := &EditableModify{
		DataLoad: *load,
		Jobname:  command,
		Editable: ctx.Editable,
		MakeWork: func(job Job, ed *editable, data []byte, first bool) Work {
			return &edInsertText{job: job, ed: ed, data: data}
		},
	}

	wl.Start(editor.WorkChan())

	editor.AddJob(wl)

	return future
}

type EditableModify struct {
	DataLoad
	Jobname  string
	Editable *editable
	MakeWork func(job Job, ed *editable, data []byte, first bool) Work
}

func (f *EditableModify) Start(c chan Work) {
	go f.pump(c)
}

func (f *EditableModify) pump(c chan Work) {
	/*
		For ssh execution or loading we might not know if there is an error until
		we call wait at the end of the session at which point we might have already closes
		the contents.
	*/
	contentsClosed := false
	errsClosed := false
	workIsDone := func() bool {
		return (contentsClosed && errsClosed)
	}

	firstAppend := true

	log(LogCatgCmd, "EditableSelectionReplace.pump: started\n")
FOR:
	for {
		select {
		case x, ok := <-f.Contents:
			if !ok {
				log(LogCatgCmd, "EditableSelectionReplace.pump: contents closed\n")
				contentsClosed = true
				f.Contents = nil
				if workIsDone() {
					break FOR
				}
				break
			}

			work := f.MakeWork(f, f.Editable, x, firstAppend)
			c <- work
			firstAppend = false
		case x, ok := <-f.Errs:
			if !ok {
				log(LogCatgCmd, "EditableSelectionReplace.pump: errs closed\n")
				errsClosed = true
				f.Errs = nil
				if workIsDone() {
					break FOR
				}
				break
			}
			log(LogCatgCmd, "EditableSelectionReplace.pump: Got an error: %v (%T)\n", x, x)
			if e, ok := x.(*fs.PathError); ok {
				log(LogCatgCmd, "  (%T)\n", e)
			}

			c <- &winLoadErr{job: f, err: x}
			//break FOR
		}
	}

	c <- &jobDone{job: f}
}

func (l *EditableModify) Kill() {
	select {
	case l.DataLoad.Kill <- struct{}{}:
	default:
	}
}

func (l *EditableModify) Name() string {
	return l.Jobname
}

type edAppendToSelection struct {
	job   Job
	ed    *editable
	data  []byte
	first bool
	sel   *selection
}

func (l edAppendToSelection) Service() (done bool) {
	l.ed.invalidateLayedoutText()
	if l.first {
		l.ed.ReplaceSelectionWith(l.sel, string(l.data))
	} else {
		l.ed.appendToSelection(l.sel, string(l.data))
	}

	editor.SignalRedrawRequired()
	return false
}

func (l edAppendToSelection) Job() Job {
	return l.job
}

type edAppend struct {
	job   Job
	ed    *editable
	data  []byte
	first bool
}

func (l edAppend) Service() (done bool) {
	if l.first {
		l.ed.SetText(l.data)
	} else {
		l.ed.Append(l.data)
	}

	return false
}

func (l edAppend) Job() Job {
	return l.job
}

type jobDone struct {
	job Job
}

func (l jobDone) Service() (done bool) {
	return true
}

func (l jobDone) Job() Job {
	return l.job
}

type edInsertText struct {
	job  Job
	ed   *editable
	data []byte
}

func (l edInsertText) Service() (done bool) {
	l.ed.InsertText(string(l.data))
	return false
}

func (l edInsertText) Job() Job {
	return l.job
}

func (c CmdDefs) CmdSyntax(ctx *CmdContext) {
	if len(ctx.Args) > 0 && ctx.Args[0] == "list" {
		names := lexers.Names(true)
		msg := fmt.Sprintf("syntax highlighting languages:\n%s\n", strings.Join(names, "\n"))
		editor.AppendError("", msg)
		return
	}

	if len(ctx.Args) > 0 && ctx.Args[0] == "load" {
		if len(ctx.Args) < 2 {
			editor.AppendError("", "Syn load expectes a filename to load.")
			return
		}

		fname := ctx.Args[1]
		path, _ := c.globalizeAndMakeAbsolute(ctx.Dir, fname)
		err := LoadLexerFromFile(path)
		if err != nil {
			msg := fmt.Sprintf("loading syntax lexer from file '%s' failed: %v\n", fname, err)
			editor.AppendError("", msg)
		}
		return
	}

	var win *Window
	switch v := ctx.source.(type) {
	case *Window:
		win = v
	case *Float:
		if v.evoker != nil {
			win = v.evoker.adapter.window()
		}
	}

	if win == nil {
		return
	}

	if len(ctx.Args) < 1 {
		win.Body.SetSyntaxAnalyse(true)
		return
	}

	if ctx.Args[0] == "off" {
		win.Body.DisableSyntax()
		win.Body.HighlightSyntax()
		return
	}

	win.Body.SetSyntaxLanguage(ctx.Args[0])
	win.Body.HighlightSyntax()

}

func (c CmdDefs) CmdAnsi(ctx *CmdContext) {
	on := true
	if len(ctx.Args) > 0 {
		switch ctx.Args[0] {
		case "off":
			on = false
		case "on":
			on = true
		default:
			return
		}
	}

	if ctx.Editable != nil {
		ctx.Editable.ColorizeAnsiEscapes(on)
	}
}

func (c CmdDefs) determineDumpFilename(ctx *CmdContext) string {
	filename := fmt.Sprintf("%s.dump", editorName)

	if len(ctx.Args) >= 1 {
		filename = ctx.CombinedArgs()
		filename, _ = c.globalizeAndMakeAbsolute(ctx.Dir, filename)
	}

	return filename
}

func (c CmdDefs) CmdDump(ctx *CmdContext) {
	state := application.State()
	filename := c.determineDumpFilename(ctx)

	err := WriteState(filename, state)
	if err != nil {
		editor.AppendError("", fmt.Sprintf("Dump failed: %v", err))
		return
	}

	fileScopedTrays.Save()
	globalScopedTrays.Save()
}

func (c CmdDefs) CmdLoad(ctx *CmdContext) {
	filename := c.determineDumpFilename(ctx)
	var state ApplicationState

	err := ReadState(filename, &state)
	if err != nil {
		editor.AppendError("", fmt.Sprintf("Load failed: %v", err))
		return
	}

	application.SetState(&state)
}

func (c CmdDefs) CmdProfCpu(ctx *CmdContext) {
	c.CmdProf(ctx, ProfileCPU)
}

func (c CmdDefs) CmdProfHeap(ctx *CmdContext) {
	c.CmdProf(ctx, ProfileHeap)
}

func (c CmdDefs) CmdProf(ctx *CmdContext, what ProfileCategory) {
	if isProfiling() {
		stopProfiling()
	} else {
		startProfiling(what)
	}
}

func (c CmdDefs) CmdGoroutines(ctx *CmdContext) {
	buf := make([]byte, 100000)
	sz := runtime.Stack(buf, true)
	buf = buf[0:sz]
	editor.AppendError("", string(buf))
}

func (c CmdDefs) CmdPutall(ctx *CmdContext) {
	editor.Putall()
}

func (c CmdDefs) CmdGetall(ctx *CmdContext) {
	editor.Getall()
}

func (c CmdDefs) CmdRecent(ctx *CmdContext) {
	s := strings.Join(editor.RecentFiles(), "\n")
	editor.AppendError("", s)
}

func (c CmdDefs) CmdExprBang(cmd string, ctx *CmdContext) sync.Future {
	return c.executeRangeExpression(cmd, ctx)
}

func (c CmdDefs) CmdExpr(ctx *CmdContext) {
	raw := ctx.RawCommand
	i := strings.Index(raw, " ")
	if i < 0 {
		return
	}

	for ; raw[i] == ' ' && i < len(raw); i++ {
	}

	expr := raw[i:]

	c.executeRangeExpression(expr, ctx)
}

func (c CmdDefs) executeRangeExpression(expr string, ctx *CmdContext) sync.Future {
	handler := ctx.Editable.makeExprHandler()

	win, _ := ctx.source.(*Window)
	executor := NewEditableExprExecutor(ctx.Editable, win, ctx.Dir, handler)
	return executor.Do(expr)
}

func (c CmdDefs) CmdOpenTray(cmd string, ctx *CmdContext) {

	trayName, scope := c.trayScope(cmd, ctx)

	var trays *Trays
	switch scope {
	case trayScopeSession:
		trays = &sessionScopedTrays
		log(LogCatgTrays, "CmdTray: using session trays\n")
	case trayScopeFile:
		var err error
		trays, err = fileScopedTraysForTray(ctx.Editable)
		if err != nil {
			log(LogCatgTrays, "CmdTray: getting file-scoped trays failed: %v\n", err)
			return
		}

		log(LogCatgTrays, "CmdTray: using file-scoped trays\n")
	case trayScopeGlobal:
		trays = &globalScopedTrays
		log(LogCatgTrays, "CmdTray: using global-scoped trays\n")
	}

	f, created := trays.Get(trayName)
	if ctx.EventContext != nil {
		f.SetPos(int(ctx.EventContext.globalX), int(ctx.EventContext.globalY))
	}
	editor.AddFloat(f)
	f.SetFocus(ctx.Gtx)
	f.SetEvoker(ctx.Editable)
	f.content.executeOn = ctx.Editable.executeOn

	if created && (scope == trayScopeFile || scope == trayScopeGlobal) {
		// Load file
		loadTrayContents(trayName, scope, f)
	}

	srcFloat, ok := ctx.source.(*Float)
	if ok && srcFloat != f {
		editor.SignalRedrawRequired()
	}
}

func (c CmdDefs) trayScope(cmd string, ctx *CmdContext) (trayName string, scope trayScope) {
	trayName, scope = parseTrayCmd(cmd)
	if scope == trayScopeUnknown {
		cr := creator(ctx)
		if cr != nil {
			if _, ok := cr.(*Body); ok {
				scope = trayScopeFile
			}
		}
	}

	if scope == trayScopeUnknown {
		scope = trayScopeSession
	}

	return
}

func creator(ctx *CmdContext) interface{} {
	if ctx.EventContext == nil {
		return nil
	}

	e := ctx.EventContext.editable
	if e == nil {
		return nil
	}

	return e.adapter.getCreator()
}

func (c CmdDefs) CmdMark(ctx *CmdContext) {

	win, ok := ctx.source.(*Window)
	if !ok {
		return
	}

	if ctx.Editable == nil {
		return
	}

	markName := "def"
	if len(ctx.Args) > 0 {
		markName = ctx.Args[0]
	}

	editor.Marks.Set(markName, &win.displayPath, &win.loadPath, ctx.Editable.firstCursorIndex())
}

func (c CmdDefs) CmdGoto(ctx *CmdContext) {
	markName := "def"
	if len(ctx.Args) > 0 {
		markName = ctx.Args[0]
	}

	displayPath, loadPath, seek, ok := editor.Marks.Seek(markName)
	if ok {
		editor.LoadFileOpts(displayPath, loadPath, LoadFileOpts{GoTo: seek, SelectBehaviour: dontSelectText})
	}
}

func (c CmdDefs) CmdMarks(ctx *CmdContext) {
	s := editor.Marks.String()
	s = fmt.Sprintf("Marks:\n%s", s)
	editor.AppendError("", s)
}

func (c CmdDefs) CmdClearMarks(ctx *CmdContext) {
	editor.Marks.Clear()
}

func (c CmdDefs) CmdSaveStyle(ctx *CmdContext) {
	file := StyleConfigFile()
	if len(ctx.Args) > 0 {
		file = ctx.CombinedArgs()
		file, _ = c.globalizeAndMakeAbsolute(ctx.Dir, file)
	}

	log(LogCatgCmd, "Saved style to file %s\n", file)
	err := SaveCurrentStyleToFile(file)
	if err != nil {
		editor.AppendError("", fmt.Sprintf("Saving style to file '%s' failed: %v", file, err))
		return
	}
}

func (c CmdDefs) CmdLoadStyle(ctx *CmdContext) {
	file := StyleConfigFile()
	if len(ctx.Args) > 0 {
		file = ctx.CombinedArgs()
		file, _ = c.globalizeAndMakeAbsolute(ctx.Dir, file)
	}

	log(LogCatgCmd, "Loading style from file %s\n", file)
	err := LoadCurrentStyleFromFile(file, &DefaultWindowStyle)
	if err != nil {
		editor.AppendError("", fmt.Sprintf("Loading style from file '%s' failed: %v", file, err))
		return
	}

}

func (c CmdDefs) CmdLoadPlumbing(ctx *CmdContext) {
	file := PlumbingConfigFile()
	if len(ctx.Args) > 0 {
		file = ctx.CombinedArgs()
		file, _ = c.globalizeAndMakeAbsolute(ctx.Dir, file)
	}

	log(LogCatgCmd, "Loading plumbing rules from file %s\n", file)
	err := HirePlumberUsingFile(file)
	if err != nil {
		editor.AppendError("", fmt.Sprintf("Loading plumbing rules from file '%s' failed: %v.", file, err))
		return
	}
}

func (c CmdDefs) CmdInsertLozenge(ctx *CmdContext) {
	if ctx.Editable != nil && editor.focusedEditable != nil {
		e := editor.focusedEditable
		e.InsertLozenge()
	}
}

func (c CmdDefs) CmdHelp(ctx *CmdContext) {

	if len(ctx.Args) > 0 {
		t := Help(ctx.CombinedArgs())
		if t == "" {
			t = "No help for that."
		}
		editor.AppendError("", t)
		editor.AppendError("", "\n")
		return
	}

	editor.AppendError("", topLevelHelpString())
}

func (c CmdDefs) CmdRot(ctx *CmdContext) {
	ctx.Editable.RotateSelections()
}

func (c CmdDefs) CmdDo(ctx *CmdContext) {
	if len(ctx.Args) == 0 {
		return
	}

	cmd := ctx.Args[0]
	args := ctx.Args[1:]
	ctx.Args = args

	ctx.executor.Do(cmd, ctx)
}

func (c CmdDefs) CmdAbout(ctx *CmdContext) {
	wasLoaded := "was loaded on startup"
	wasntLoaded := "was not loaded on startup"

	loadedStr := func(loaded bool) string {
		if loaded {
			return wasLoaded
		} else {
			return wasntLoaded
		}
	}

	var text bytes.Buffer
	fmt.Fprintf(&text, "%s was written by Jeff Williams\n\n", strings.Title(editorName))
	fmt.Fprintf(&text, "Version: %s %s\n", buildVersion, buildTime)
	fmt.Fprintf(&text, "Config directory: %s\n", ConfDir)
	fmt.Fprintf(&text, "Settings file: %s (%s)\n", SettingsConfigFile(), loadedStr(settingsLoadedFromFile))
	fmt.Fprintf(&text, "Style config file: %s (%s)\n", StyleConfigFile(), loadedStr(styleLoadedFromFile))
	fmt.Fprintf(&text, "SSH key directory: %s\n", SshKeyDir())
	fmt.Fprintf(&text, "Plumbing config file: %s (%s)\n", PlumbingConfigFile(), loadedStr(plumbingLoadedFromFile))
	fmt.Fprintf(&text, "Keymaps config file: %s (%s)\n", KeymapConfigFile(), loadedStr(keymapsLoadedFromFile))
	fmt.Fprintf(&text, "API listener port: %d\n", LocalAPIPort())

	sshKeys := sshClientCache.Keys()
	sshEntries := sshClientCache.Entries()
	if len(sshKeys) > 0 {
		fmt.Fprintf(&text, "Cached SSH connections:\n")
		for i, k := range sshKeys {
			fmt.Fprintf(&text, "  %s\n", k)
			if i < len(sshEntries) && len(sshEntries) > 0 {
				fmt.Fprintf(&text, "    API listener port: %d\n", sshEntries[i].client.ListenerPort())
			}
		}
	} else {
		fmt.Fprintf(&text, "No cached SSH connections\n")
	}

	sshPassEndpts := sshClientCache.HopPasswordEndpoints()
	if len(sshPassEndpts) > 0 {
		fmt.Fprintf(&text, "SSH hosts having passwords set:\n")
		for _, k := range sshPassEndpts {
			fmt.Fprintf(&text, "  %s\n", k)
		}
	} else {
		fmt.Fprintf(&text, "No SSH host passwords defined\n")
	}

	keypass := sshClientCache.KeyfilesWithPasswords()
	if len(keypass) > 0 {
		fmt.Fprintf(&text, "Keyfiles having passwords set:\n")
		for _, k := range keypass {
			fmt.Fprintf(&text, "  %s\n", k)
		}
	} else {
		fmt.Fprintf(&text, "No SSH keyfile passwords defined\n")
	}

	apiSessions := getApiSessions()
	if len(apiSessions) > 0 {
		fmt.Fprintf(&text, "API sessions:\n")
		for _, e := range apiSessions {
			s := strings.Join(e.userDefinedCommands, ", ")
			if len(s) > 0 {
				s = fmt.Sprintf(" user-defined commands: [%s]", s)
			}
			fmt.Fprintf(&text, "  %s %s%s\n", e.Cmd(), e.Id(), s)
		}
	} else {
		fmt.Fprintf(&text, "No API sessions\n")
	}

	editor.AppendError("", text.String())
}

func (c CmdDefs) CmdFont(ctx *CmdContext) {
	var win *Window
	switch v := ctx.source.(type) {
	case *Window:
		win = v
	case *Float:
		if v.evoker != nil {
			win = v.evoker.adapter.window()
		}
	}

	if win == nil {
		return
	}

	win.Body.NextFont()
}

func (c CmdDefs) CmdFontSize(ctx *CmdContext) {
	if len(ctx.Args) == 0 {
		return
	}

	var ad adapter
	switch v := ctx.source.(type) {
	case *Window:
		ad = v.Body.adapter
	case *Col:
		ad = v.Tag.adapter
	case *Editor:
		ad = v.Tag.adapter
	default:
		return
	}

	v := ctx.Args[0]
	v = strings.TrimSpace(v)

	rel := false
	if strings.HasPrefix(v, "-") || strings.HasPrefix(v, "+") {
		rel = true
	}
	size, err := strconv.Atoi(v)
	if err != nil {
		editor.AppendError("", "FontSize expects a numeric argument")
		return
	}

	style := ad.style()
	for i := range style.Fonts {
		if rel {
			style.Fonts[i].FontSize += unit.Sp(size)
		} else {
			style.Fonts[i].FontSize = unit.Sp(size)
		}

		if style.Fonts[i].FontSize < 1 {
			style.Fonts[i].FontSize = 1
		}
	}
	ad.setStyle(style)
	editor.SignalRedrawRequired()

}

func (c CmdDefs) CmdOn(ctx *CmdContext) {
	if len(ctx.Args) < 2 {
		editor.AppendError("", "The On command needs at least two arguments: the directory and the command")
		return
	}

	dir := ctx.Args[0]
	cmd := ctx.Args[1]
	ctx.Args = ctx.Args[2:]
	ctx.Dir = dir

	ctx.executor.tryOsCmd(ctx, cmd)
}

func cmdOnClearErrorsWindowHook(ctx *CmdContext) {
	dir := ctx.Args[0]
	editor.ClearErrors(dir)
}

func (c CmdDefs) CmdCmds(ctx *CmdContext) {
	editor.AppendError("", cmdHistory.String(NotVerbose))
}

func (c CmdDefs) CmdCmdsVerbose(ctx *CmdContext) {
	editor.AppendError("", cmdHistory.String(Verbose))
}

func (c CmdDefs) CmdUndo(ctx *CmdContext) {
	ctx.Editable.Undo(ctx.Gtx)
}

func (c CmdDefs) CmdRedo(ctx *CmdContext) {
	ctx.Editable.Redo(ctx.Gtx)
}

func (c CmdDefs) CmdPrintCfg(ctx *CmdContext) {
	if len(ctx.Args) < 1 {
		editor.AppendError("", "The PrintCfg command needs an argument.")
		return
	}

	fname := ctx.Args[0]

	switch fname {
	case "settings.toml":
		editor.AppendError("", GenerateSampleSettings())
	}
}

func (c CmdDefs) CmdWins(ctx *CmdContext) {
	var paths []string
	for _, win := range editor.Windows() {
		path, _, _, err := win.Tag.Parts()
		if err != nil {
			editor.AppendError("", fmt.Sprintf("(error getting path of window: %v)", err))
			continue
		}
		paths = append(paths, path)
	}

	sort.Slice(paths, func(i, j int) bool {
		return paths[i] < paths[j]
	})

	for _, path := range paths {
		editor.AppendError("", path)
	}
}

func (c CmdDefs) CmdOnly(ctx *CmdContext) {

	const (
		equal = iota
		above
		below
	)

	var win *Window
	switch v := ctx.source.(type) {
	case *Window:
		win = v
	case *Float:
		if v.evoker != nil {
			win = v.evoker.adapter.window()
		}
	}

	if win == nil || win.col == nil {
		return
	}

	keep := equal
	if len(ctx.Args) > 0 {
		switch ctx.Args[0] {
		case "above":
			keep = above
		case "below":
			keep = below
		}
	}

	found := false
	wins := make([]*Window, 0, len(win.col.Windows))
	for _, w := range win.col.Windows {
		if w == win {
			found = true
			continue
		}

		if (keep == above && found) || (keep == below && !found) || keep == equal {
			wins = append(wins, w)
		}
	}

	c.delWindows(wins...)
}

func (c CmdDefs) CmdClr(ctx *CmdContext) {
	ctx.Editable.SetText([]byte{})
	ctx.Editable.ClearManualHighlights()
}

func (c CmdDefs) CmdShstr(ctx *CmdContext) {
	win, ok := ctx.source.(*Window)
	if !ok {
		editor.AppendError("", "Shstr only works in window tags or bodies")
		return
	}

	b, err := isRemoteFilenameOrDir(ctx.Dir)
	if err == nil && !b {
		editor.AppendError("", "Shstr only works for remote files")
		return
	}

	if len(ctx.Args) == 0 {
		win.Body.adapter.setShellString("")
		win.Tag.adapter.setShellString("")
		return
	}

	win.Body.adapter.setShellString(ctx.CombinedArgs())
	win.Tag.adapter.setShellString(ctx.CombinedArgs())
}

func (c CmdDefs) CmdDbg(ctx *CmdContext) {
	if len(ctx.Args) == 0 {
		editor.AppendError("", "Dbg expects at least one argument")
		return
	}

	doer, ok := defaultDebugCommandSet.Command(ctx.Args[0])
	if !ok {
		editor.AppendError("", fmt.Sprintf("There is no such debug command as %s", ctx.Args[0]))
		return
	}

	ctx.Args = ctx.Args[1:]
	doer.do(ctx)
}

func (c CmdDefs) CmdDbgLogs(ctx *CmdContext) {
	if len(ctx.Args) >= 2 && ctx.Args[0] == "Stream" {
		switch ctx.Args[1] {
		case "stdout":
			*optDebugStdout = true
		case "window":
			*optDebugToWindow = true
		case "off":
			*optDebugStdout = false
			*optDebugToWindow = false
			categoriesToStream = make(map[string]struct{})
		case "Catg":
			categories := ctx.Args[2:]
			for _, c := range categories {
				categoriesToStream[c] = struct{}{}
			}
		}
		return
	}

	msg := debugLog.String(ctx.Args...)
	editor.AppendError("", msg)
}

func (c CmdDefs) CmdDbgGetPid(ctx *CmdContext) {
	os.Getpid()
	msg := fmt.Sprintf("pid: %d", os.Getpid())
	editor.AppendError("", msg)
}

func (c CmdDefs) CmdDbgPsrv(ctx *CmdContext) {
	if len(ctx.Args) > 0 && ctx.Args[0] == "off" {
		stopPprofDebugServer()
		return
	}
	startPprofDebugServer()
}

func (c CmdDefs) CmdDbgPaths(ctx *CmdContext) {
	switch v := ctx.source.(type) {
	case Window:
	case *Window:
		msg := fmt.Sprintf("window display path: '%s' load path: '%s' file type: '%s'", v.DisplayPath(), v.LoadPath(), v.fileType)
		editor.AppendError("", msg)
	}
}

var (
	flameGraphBuf *bytes.Buffer
	flameServer   *http.Server
)

func (c CmdDefs) CmdDbgFlame(ctx *CmdContext) {
	if len(ctx.Args) == 0 {
		editor.AppendError("", "Dbg Flame expects an argument of 'on' or 'off'")
		return
	}

	switch ctx.Args[0] {
	case "on":
		flameGraphBuf = new(bytes.Buffer)
		pprof.StartCPUProfile(flameGraphBuf)
		editor.AppendError("", "started sampling")
	case "off":
		if flameGraphBuf == nil {
			return
		}
		pprof.StopCPUProfile()
		stacks, err := flamegraph.BuildSamplesFromPProfCPUProfile(flameGraphBuf)
		if err != nil {
			msg := fmt.Sprintf("building samples from CPU profile failed: %v", err)
			editor.AppendError("", msg)
			return
		}

		flameGraphBuf.Reset()
		flamegraph.Render(stacks, flameGraphBuf)
		var url string
		url, flameServer, err = serveFlamegraph(flameGraphBuf.Bytes())
		if err != nil {
			editor.AppendError("", fmt.Sprintf("serving flame graph failed: %v", err))
			flameServer = nil
			return
		}
		editor.AppendError("", fmt.Sprintf("Flame graph URL: %s", url))
	case "kill":
		if flameServer != nil {
			flameServer.Close()
		}
	default:
		editor.AppendError("", "Dbg Flame expects an argument of 'on' or 'off'")
		return

	}

}

func (c CmdDefs) CmdDbgMouse(ctx *CmdContext) {
	if len(ctx.Args) == 0 {
		editor.AppendError("", "Dbg Mouse expects an argument of 'on' or 'off'")
		return
	}

	switch ctx.Args[0] {
	case "on":
		editor.doPrintMouseMoveEvents = true
	case "off":
		editor.doPrintMouseMoveEvents = false
	}
}

func (c CmdDefs) CmdCols(ctx *CmdContext) {
	editor.AppendError("", editor.ListCols(false, false))
}

func (c CmdDefs) CmdColsVerbose(ctx *CmdContext) {
	editor.AppendError("", editor.ListCols(false, true))
}

func (c CmdDefs) CmdColsVeryVerbose(ctx *CmdContext) {
	editor.AppendError("", editor.ListCols(true, true))
}

func (c CmdDefs) CmdTint(ctx *CmdContext) {
	if len(ctx.Args) == 0 {
		if ctx.Editable.SelectionsPresent() {
			ctx.Editable.ClearSelectedManualHighlights()
		} else {
			ctx.Editable.ClearManualHighlights()
		}
		return
	}

	if ctx.Args[0] == "list" {
		c.appendColorNamesInColor(ctx)
		return
	}

	fgColor, bgColor, err := ParseTint(ctx.Args[0])

	if err != nil {
		editor.AppendError("", err.Error())
		return
	}

	ctx.Editable.AddManualHighlightForEachSelection(fgColor, bgColor)
}

func (c CmdDefs) appendColorNamesInColor(ctx *CmdContext) {
	fname := editor.ErrorsFileNameOf("")
	win := editor.FindOrCreateWindow(fname)

	for _, n := range colornames.Names {
		str := "▆▆▆"
		start := win.Body.text.Len()
		end := start + utf8.RuneCountInString(str)
		color, ok := ColorFromName(n)
		line := fmt.Sprintf("%s %s\n", str, n)
		editor.AppendError("", line)
		if ok {
			win.Body.AddManualHighlight(start, end, color, Color{0, 0, 0, 0})
		}
	}
	return

}

func (c CmdDefs) CmdFuzz(ctx *CmdContext) {
	win, ok := ctx.source.(*Window)
	if !ok {
		return
	}

	win.fuzzySearch.search(ctx.Args)
}

func (c CmdDefs) CmdPic(ctx *CmdContext) {
	// Pic file.jpg
	// Pic file.jpg <scale %> # scale x%
	// Pic file.jpg fit # scale to fit window size

	win, ok := ctx.source.(*Window)
	if !ok {
		return
	}

	path := ""
	if len(ctx.Args) > 0 {
		path = ctx.Args[0]
	}

	if path == "" {
		win.Body.bgimage.img = nil
		return
	}

	if strings.TrimSpace(path) != "" {
		path, _ = c.globalizeAndMakeAbsolute(ctx.Dir, path)
	}

	err := win.Body.bgimage.Load(path)
	if err != nil {
		editor.AppendError("", err.Error())
		return
	}

	if len(ctx.Args) > 1 {
		arg := ctx.Args[1]
		if arg == "fit" {
			win.Body.bgimage.scalingType = scaleToFitWindow
			return
		}

		if strings.HasSuffix(arg, "%") {
			pct, err := strconv.Atoi(arg[:len(arg)-1])
			if err != nil || pct <= 0 {
				editor.AppendError("", fmt.Sprintf("The second argument to Pic must be a non-negative percentage like 50%%, or the word fit. Parsing percentage failed: %v", err.Error()))
				return
			}
			win.Body.bgimage.scalingType = scaleToPercent
			win.Body.bgimage.fraction = float32(pct) / 100.0
		}
	}

}

func (c CmdDefs) CmdTab(ctx *CmdContext) {
	if len(ctx.Args) == 0 {
		editor.setInsertWhenTabPressed("\t")
		return
	}

	arg := ctx.RawCommand
	if strings.HasPrefix(arg, "Tab ") {
		arg = arg[4:]
		arg = strings.TrimLeft(arg, " \t\n\r")
	}

	s, err := escape.ExpandEscapesAndUnquote(arg)
	toSet := s
	if err != nil {
		log(LogCatgCmd, "CommandExecutor.CmdTab: error parsing quoted string '%s': %v\n", arg, err)
		// Must not be a quoted string. Just apply it directly.
		//editor.setInsertWhenTabPressed(arg)
		toSet = arg
		return
	}
	//editor.setInsertWhenTabPressed(s)

	win, ok := ctx.source.(*Window)
	if !ok {
		editor.setInsertWhenTabPressed(toSet)
		return
	}

	win.setInsertWhenTabPressed(toSet)
}

func (c CmdDefs) CmdSettag(ctx *CmdContext) {
	//userArea := ctx.CombinedArgs()

	userArea := ctx.RawCommand
	if strings.HasPrefix(userArea, "Settag ") {
		userArea = userArea[7:]
		userArea = strings.TrimLeft(userArea, " \t\n\r")
	}

	s, err := escape.ExpandEscapesAndUnquote(userArea)
	if err == nil {
		userArea = s
	}

	win, ok := ctx.source.(*Window)
	if ok {
		path, edArea, _, err := win.Tag.Parts()
		if err != nil {
			editor.AppendError("", fmt.Sprintf("parsing current tag failed: %v", err))
			return
		}
		win.Tag.Set(path, edArea, userArea)
		win.SetTagFromDisplayPath()
		return
	}

	switch t := ctx.source.(type) {
	case *Col:
		t.Tag.SetTextString(userArea)
	case *Editor:
		t.Tag.SetTextString(userArea)
	}
}

func (c CmdDefs) CmdSort(ctx *CmdContext) {
	var col *Col
	switch v := ctx.source.(type) {
	case *Window:
		col = v.col
	case *Col:
		col = v
	case *Float:
		if v.evoker != nil {
			if win := v.evoker.adapter.window(); win != nil {
				col = win.col
			}
		}
	}

	if col == nil {
		return
	}

	col.Sort()
}

func (c CmdDefs) CmdRel(ctx *CmdContext) {
	var col *Col
	switch v := ctx.source.(type) {
	case *Window:
		col = v.col
	case *Col:
		col = v
	}

	if col == nil {
		return
	}

	_, ok := col.Path()
	if !ok {
		return
	}

	// TODO: Not working yet for matching directory.
	for _, w := range col.Windows {
		// This makes the display path relative because SetPathsAndTag does
		// that internally.
		w.SetPathsAndTag(w.DisplayPath(), w.LoadPath())
	}
}

func (c CmdDefs) CmdWrap(ctx *CmdContext) {
	wrap := true
	if len(ctx.Args) > 0 && ctx.Args[0] == "off" {
		wrap = false
	}

	switch v := ctx.source.(type) {
	case Window:
	case *Window:
		v.Body.invalidateLayedoutText()
		v.Body.wrap = wrap
		editor.SignalRedrawRequired()
	case *Float:
		v.content.invalidateLayedoutText()
		v.content.wrap = wrap
		editor.SignalRedrawRequired()
	}
}

func (c CmdDefs) CmdAlias(ctx *CmdContext) {
	m := ctx.RawCommand
	if !strings.HasPrefix(m, "Alias") {
		editor.AppendError("", "Alias executed by non-Alias")
		return
	}
	m = strings.TrimLeft(m[5:], " \t")

	if len(m) == 0 {
		var buf bytes.Buffer
		fmt.Fprintf(&buf, "Aliases:\n")
		for k, v := range settings.Alias {
			fmt.Fprintf(&buf, "  %s: %s\n", k, v)
		}

		editor.AppendError("", buf.String())
		return
	}

	parts := strings.SplitN(m, " ", 2)
	if len(parts) == 0 {
		editor.AppendError("", "Alias command expects the name as the first argument")
		return
	}

	name := parts[0]

	if len(parts) == 1 {
		delete(settings.Alias, name)
		editor.AppendError("", fmt.Sprintf("Deleted alias '%s'", name))
		return
	}

	cmd := escape.MustUnquote(parts[1])

	settings.Alias[name] = cmd
	editor.AppendError("", fmt.Sprintf("Created alias '%s'", name))
}

func (c CmdDefs) CmdElastic(ctx *CmdContext) {
	elastic := true
	if len(ctx.Args) > 0 && ctx.Args[0] == "off" {
		elastic = false
	}

	var win *Window
	switch v := ctx.source.(type) {
	case *Window:
		win = v
	case *Float:
		if v.evoker != nil {
			win = v.evoker.adapter.window()
		}
	}

	if win == nil {
		return
	}

	win.Body.invalidateLayedoutText()
	win.Body.SetElastic(elastic)
	editor.SignalRedrawRequired()
}

func (c CmdDefs) CmdKeymap(ctx *CmdContext) {
	if len(ctx.Args) == 0 {
		return
	}

	cmd := ctx.Args[0]

	switch cmd {
	case "show":
		c.showKeymap(ctx)
	case "load":
		c.loadKeymap(ctx)
	case "push":
		c.pushKeymap(ctx)
	case "pop":
		c.popKeymap()
	case "win":
		if len(ctx.Args) < 2 {
			return
		}

		win, ok := ctx.source.(*Window)
		if !ok {
			editor.AppendError("", "Keymap win subcommands must be executed in a window")
			return
		}
		cmd2 := ctx.Args[1]
		switch cmd2 {
		case "push":
			c.pushWinKeymap(ctx, win)
		case "pop":
			c.popWinKeymap(win)
		case "reset":
			c.resetWinKeymap(win)
		default:
			editor.AppendError("", fmt.Sprintf("No such Keymap win subcommand '%s'", cmd2))
			return
		}
	default:
		editor.AppendError("", fmt.Sprintf("No such Keymap subcommand '%s'", cmd))
		return

	}
}

func (c CmdDefs) showKeymap(ctx *CmdContext) {
	if len(ctx.Args) < 2 {
		editor.AppendError("", fmt.Sprintf("Show what?"))
		return
	}

	mode := ctx.Args[1]
	switch mode {
	case "def":
		if len(ctx.Args) < 3 {
			return
		}
		name := ctx.Args[2]
		def, ok := keymapDefs[name]
		if !ok {
			editor.AppendError("", fmt.Sprintf("No keymap '%s' defined", def))
			return
		}
		editor.AppendError("", def.String())
	case "defs":
		for _, def := range keymapDefs {
			editor.AppendError("", fmt.Sprintf("%s\n", def.String()))
		}
	case "stack":
		e := ctx.Editable
		win, ok := ctx.source.(*Window)
		if ok {
			e = &win.Body.blockEditable.editable
		}
		if e != nil {
			editor.AppendError("", fmt.Sprintf("%s\n", e.keys.String()))
		}
	default:
		editor.AppendError("", fmt.Sprintf("I don't know how to show that about keymaps"))
		return
	}
}

func (c CmdDefs) loadKeymap(ctx *CmdContext) {
	if len(ctx.Args) < 2 {
		return
	}

	file := ctx.Args[1]
	file, _ = c.globalizeAndMakeAbsolute(ctx.Dir, file)

	log(LogCatgCmd, "Loading keymaps from file %s\n", file)

	defs, err := keymap.LoadDefinitionsFromFile(file)
	if err != nil {
		editor.AppendError("", fmt.Sprintf("keymap load failed: %v", err))
		return
	}

	log(LogCatgCmd, "loaded %d keymap definitions\n", len(defs))
	for _, def := range defs {
		log(LogCatgCmd, "%s\n", def.String())
	}

	for _, def := range defs {
		//		if def.Name == "base" {
		//			log(LogCatgCmd, "Ignoring keymap 'base'")
		//			continue
		//		}
		buildAndInstallKeymap(def, keyActions)
	}

}

func (c CmdDefs) pushKeymap(ctx *CmdContext) {
	if len(ctx.Args) < 2 {
		return
	}

	name := ctx.Args[1]
	km, ok := keymaps[name]
	if !ok {
		editor.AppendError("", fmt.Sprintf("No such keymap '%s'", name))
		return
	}
	globalKeymapStack.Push(km)

}

func (c CmdDefs) popKeymap() {
	globalKeymapStack.Pop()
}

func (c CmdDefs) pushWinKeymap(ctx *CmdContext, win *Window) {
	if len(ctx.Args) < 3 {
		return
	}

	name := ctx.Args[2]
	km, ok := keymaps[name]
	if !ok {
		editor.AppendError("", fmt.Sprintf("No such keymap '%s'", km))
		return
	}

	if win.Body.keys == &globalKeymapStack {
		win.Body.keys = globalKeymapStack.Clone()
	}

	win.Body.keys.Push(km)

}

func (c CmdDefs) popWinKeymap(win *Window) {
	if win.Body.keys == &globalKeymapStack {
		// Don't mess up the global keymap
		return
	}

	win.Body.keys.Pop()
}

func (c CmdDefs) resetWinKeymap(win *Window) {
	win.Body.keys = &globalKeymapStack
}

func (c CmdDefs) CmdNewLayer(ctx *CmdContext) {
	editor.AddLayer()
	editor.ActivateLayer(len(editor.Layers) - 1)
	editor.SignalRedrawRequired()
}

func (c CmdDefs) CmdDelLayer(ctx *CmdContext) {
	editor.DelActiveLayer()
	editor.SignalRedrawRequired()
}

func (c CmdDefs) CmdLayerUp(ctx *CmdContext) {
	editor.ActivateLayerRelativeToCurrent(+1)
	editor.SignalRedrawRequired()
}

func (c CmdDefs) CmdLayerDown(ctx *CmdContext) {
	editor.ActivateLayerRelativeToCurrent(-1)
	editor.SignalRedrawRequired()
}

func (c CmdDefs) CmdSetLayer(ctx *CmdContext) {
	index := 0
	if len(ctx.Args) < 1 {
		index = 0
	}

	index, err := strconv.Atoi(ctx.Args[0])
	if err != nil {
		editor.AppendError("", "Lyr command expects a layer index as the first argument")
		return
	}

	editor.ActivateLayer(index)
	editor.SignalRedrawRequired()
}

func (c CmdDefs) CmdMoveColUp(ctx *CmdContext) {
	c.moveColToLayerRelativeToCurrent(ctx, +1)
	editor.SignalRedrawRequired()
}

func (c CmdDefs) CmdMoveColDown(ctx *CmdContext) {
	c.moveColToLayerRelativeToCurrent(ctx, -1)
	editor.SignalRedrawRequired()
}

func (c CmdDefs) CmdMoveColUpAndChangeActiveLayer(ctx *CmdContext) {
	c.moveColToLayerRelativeToCurrent(ctx, +1)
	editor.ActivateLayerRelativeToCurrent(+1)
	editor.SignalRedrawRequired()
}

func (c CmdDefs) CmdMoveColDownAndChangeActiveLayer(ctx *CmdContext) {
	c.moveColToLayerRelativeToCurrent(ctx, -1)
	editor.ActivateLayerRelativeToCurrent(-1)
	editor.SignalRedrawRequired()
}

func (c CmdDefs) moveColToLayerRelativeToCurrent(ctx *CmdContext, delta int) {
	var col *Col
	switch v := ctx.source.(type) {
	case *Window:
		col = v.col
	case *Col:
		col = v
	}

	editor.MoveColToLayerRelativeToCurrent(col, delta)

}

func (c CmdDefs) CmdSetLayerName(ctx *CmdContext) {
	name := ctx.CombinedArgs()
	editor.SetActiveLayerName(name)
}

func (c CmdDefs) CmdMoveLayer(ctx *CmdContext) {
	index, err := strconv.Atoi(ctx.Args[0])
	if err != nil {
		editor.AppendError("", "the first argument to Mvlyr must be an integer")
		return
	}

	editor.MoveActiveLayerTo(index)
}

func (c CmdDefs) CmdFloat(ctx *CmdContext) {
	arg := ctx.RawCommand
	if strings.HasPrefix(arg, "Float ") {
		arg = arg[6:]
		arg = strings.TrimLeft(arg, " \t\n\r")
	}

	s, err := escape.ExpandEscapesAndUnquote(arg)
	if err != nil {
		editor.AppendError("", "error parsing string")
		return
	}

	f := NewFloat(editor.layout.style.trayStyle(), editor.work)
	f.opts.ShowAllLines = true
	//f.Body.SetTextString(s)
	f.content.SetTextString(s)
	f.SetPos(int(ctx.EventContext.globalX), int(ctx.EventContext.globalY))

	editor.AddFloat(f)
}

func (c CmdDefs) CmdTrays(ctx *CmdContext) {

	// List all global and file trays, even the unloaded ones
	editor.AppendError("", "Session scope:")
	for _, n := range sessionScopedTrays.Names() {
		editor.AppendError("", fmt.Sprintf("  %s", n))
	}

	editor.AppendError("", "Global scope:")
	for _, n := range globalScopedTrays.Names() {
		editor.AppendError("", fmt.Sprintf("  %s", n))
	}

	editor.AppendError("", "File scope:")
	for path, trays := range fileScopedTrays {
		editor.AppendError("", fmt.Sprintf("%s", path))
		for _, n := range trays.Names() {
			editor.AppendError("", fmt.Sprintf("  %s", n))
		}
	}

}

func (c CmdDefs) CmdDeltray(ctx *CmdContext) {
	trayName := ctx.Args[0]
	trayName, scope := c.trayScope(trayName, ctx)

	var trays *Trays
	switch scope {
	case trayScopeSession:
		trays = &sessionScopedTrays
		log(LogCatgTrays, "CmdDeltray: using session trays\n")
	case trayScopeFile:
		var err error
		trays, err = fileScopedTraysForTray(ctx.Editable)
		if err != nil {
			log(LogCatgTrays, "CmdDeltray: getting file-scoped trays failed: %v\n", err)
			return
		}

		log(LogCatgTrays, "CmdDeltray: using file-scoped trays\n")
	case trayScopeGlobal:
		trays = &globalScopedTrays
		log(LogCatgTrays, "CmdDeltray: using global-scoped trays\n")
	}

	deleted := trays.Del(trayName)

	if deleted && (scope == trayScopeFile || scope == trayScopeGlobal) {
		// TODO: Delete file
		// Really we can let the user manage that manually.
	}
}

func (c CmdDefs) CmdUnsel(ctx *CmdContext) {
	if ctx.Editable == nil {
		return
	}

	ctx.Editable.selections = ctx.Editable.selections[:0]
}

func (c CmdDefs) CmdSsh(ctx *CmdContext) {
	sshClientCache.Clear()
}

func (c CmdDefs) CmdColRight(ctx *CmdContext) {
	editor.ScrollColsInActiveLayer(+1)
}

func (c CmdDefs) CmdColLeft(ctx *CmdContext) {
	editor.ScrollColsInActiveLayer(-1)
}

func (c CmdDefs) CmdFetchcol(ctx *CmdContext) {

	name := ctx.CombinedArgs()
	col := editor.FindColByName(name)
	if col == nil {
		editor.AppendError("", "No such col")
		return
	}

	editor.MoveColToLayerRelativeToCurrent(col, 0)
	editor.SignalRedrawRequired()
}

func (c CmdDefs) CmdWiden(ctx *CmdContext) {
	activeLayer := editor.activeLayer()
	if activeLayer == nil {
		return
	}

	activeLayer.WidenView()
}

func (c CmdDefs) CmdNarrow(ctx *CmdContext) {
	activeLayer := editor.activeLayer()
	if activeLayer == nil {
		return
	}

	activeLayer.NarrowView()
}

func (c CmdDefs) CmdPin(ctx *CmdContext) {
	switch w := ctx.source.(type) {
	case Window:
	case *Window:
		w.SetPinnedToCurrentLayer(true)
	}
	editor.SignalRedrawRequired()
}

func (c CmdDefs) CmdUnpin(ctx *CmdContext) {
	switch w := ctx.source.(type) {
	case Window:
	case *Window:
		w.SetPinnedToCurrentLayer(false)
	}
	editor.SignalRedrawRequired()
}
