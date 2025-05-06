# Anvil

The Anvil text editor. Visit [anvil-editor.net](http://anvil-editor.net) for the latest documentation and downloads.

# Building

The editor proper can be found under the `editor/` directory. Auxiliary programs are found under `extras/`. 

To build the editor for your local UNIX-like system, install Go and from the root of the repository run `./build.sh`.

Alternately you can create a Go workspace using `./build.sh -w` or running `go work init ./editor ./extras ./api/go/anvil`, then use `go build` to build the directory `editor/cmd/anvil` or any of the directories under`extras/cmd/`.

To build for different architectures, run `./build.sh` and use the -a and -o options to specify the architecture and OS, respectively. For example, the following command builds for windows 64-bit:

    ./build.sh -a amd64 -o windows

Using `./build -h` provides help about the available options.

# Basic Usage

## Screen Layout

Anvil primarily consists of a collection of windows tiled into columns. Each window has a Tag, a Scrollbar, a Body and a Layout Box. In addition there is a Tag for each column and for the editor itself.

In each window, the layout box is a small pink rectangle to at the top left of the window that is used for resizing the window and moving it to other columns. The scrollbar is on the left and is used to scroll, and the colored area represents the portion of text currently visible in the view compared to the entire buffer. The Body is the contents of the file being edited, while the Tag is an editable area at the top of the window that is useful for storing commands. Commands executed from the Tag generally apply to the Body.

## Mouse Use on a Layout Box:

For windows, the following actions apply:

| Button | Event             | Behaviour                         |
| ------ | ----------------- | --------------------------------- |
| Left   | Single click      | Make the window slightly larger   |
| Left   | Drag vertically   | Change the height of the window¹  |
| Left   | Drag horizontally | Move the window to another column |
| Right  | Single click      | Minimize other windows            |
| Middle | Single click      | Hide other windows                |

For columns, these actions apply:

| Button | Event                  | Behaviour |
| ------ | ---------------------- | --------- |
| Left   | Shift + click and hold | Make window tags display basenames rather than full paths |

¹ Only if there are other windows in the column.

## Mouse Use on a Scrollbar:

| Button | Event           | Behaviour                                     |
| ------ | --------------- | --------------------------------------------- |
| Left   | Single click    | Scroll upwards¹                               |
| Right  | Single click    | Scroll downwards                              |
| Middle | Drag vertically | Scroll to that relative position in the file  |

¹ The amount scrolled is relative to the vertical position of the click within the scrollbar. Near the bottom scrolls an entire page, while near the top scrolls a line.


## Mouse Use Within a Window Body or Window Tag:

### Unchorded

| Button | Event        | Behaviour |
| ------ | ------------ | -------------------- |
| Left   | Single click | Set cursor position and remove other cursors and selections |
| Left   | Alt + Single click | Create an additional cursor |
| Left   | Double click | Select the string of alphanumeric characters under the mouse |
| Left   | Triple click | Select the space-separated word under the mouse |
| Left   | Quad click   |   Select the current line |
| Left   | Double click on the first character of a line | Select current line |
| Left   | Double click on a delimiting character¹ | select text enclosed by the brackets, excluding the brackets² |
| Left   | Triple click on a delimiting character¹ | select text enclosed by the brackets, including the brackets² |
| Left   | Drag | Select text and remove other selections |
| Left   | Alt + Drag   | Create an additional selection |
| Right  | Single click | Search for word or selection under cursor and make a new selection. If word or selection is surrounded by / then it is treated as a regular expression. |
| Right  | Alt + Single click | Acquire the word or selection under the cursor³ |
| Right  | Alt + Ctrl + Single click | Like Alt + Single click, but if a new window would be opened for a path, instead load the path in the current window |
| Middle | Single click | Execute the word or selection under the mouse. This can be an Anvil command (New, Del, Paste, etc) or a shell command |
| Middle | Select and release | Execute the selected command |
| Scroll | Ctrl + Scroll | Increase or reduce font size |

¹ A delimiting character refers to an opening or closing bracket, or an opening or closing quote character. Explicitly, one of `{`, `[`, `(`, `<` or its matching closer, or one of `"`, ```, ``` or `◊`.

² For quotes, if it is ambiguous where the matching quote character would be because there is a candidate match both to the left and the right, then no action is taken.

### Chorded

| Primary Button | Secondary Button / Key | Behaviour |
| -----| -------- | -------------------- |
| Left | Middle | Cut selected text to clipboard |
| Left | Right | Paste text from clipboard |
| Middle | Left | Execute command with argument¹ |
| Middle | Ctrl | Execute command with argument¹ |
| Left | F1-F12 | Create a mark at primary cursor position who's name is the function key. |

¹ These events execute a command, but with the text of the selected text from any window body as the first argument to the command.

A common idiom for copying text is to cut and paste in place with the mouse. Use left button + middle button, then release the middle button, then press the right button.
 
# Keyboard 

## Shortcuts

| Key Combination | Behaviour |
| --------------- | --------- |
| HOME            | Go to start of line |
| END             |    Go to end of line |
| PG DN           |  Scroll down a page |
| PG UP           |  Scroll up a page |
| SHIFT-Enter     | Begin new line, like enter, but do not autoindent |
| CTRL-Enter      | Execute the entire line as a command |
| F1-F12          | Go to mark created using Left Button + function key |
| ESC             | If there are selections present, create a cursor at the beginning of each line the selection intersects. Otherwise, if there are multiple cursors present, reduce the cursors to one. Finally, if there is only one cursor present, select the recently typed text |
| CTRL-A          | Select all text |
| CTRL-C          | Copy |
| CTRL-D          | Delimit selections with cursors | replace each selection with a cursor at the beginning and end |
| CTRL-E          | Scroll up a line |
| CTRL-F          | Complete filename |
| CTRL-G          | Get |
| CTRL-K          | Delete from the current cursor position to the end of the line |
| CTRL-L          | Surround each selection with Lozenge (◊) characters |
| CTRL-N          | Complete word or substitute next completion |
| CTRL-P          | Complete word or substitute previous completion |
| CTRL-R          | Redo |
| CTRL-S          | Put |
| CTRL-T          | Execute the selected text |
| CTRL-U          | Delete each line containing a cursor |
| CTRL-V          | Paste |
| CTRL-Z          | Undo |
| CTRL-X          | Cut |
| CTRL-Y          | Scroll down a line |
| CTRL-Right      | Move one space-separated word right |
| CTRL-Left       | Move one space-separated word left |
| CTRL-Home       | Go to start of file |
| CTRL-End        | Go to end of file |
| Up/Down Arrow   | Move all cursors up or down one line respectively |
| Left Arrow      | If cursors are present, move each cursor left one character. If selections are present, change the selections to cursors at the beginning of each selection.  |
| Right Arrow     | If cursors are present, move each cursor right one character. If selections are present, change the selections to cursors at the end of each selection.  |

## Special Behaviours

* When there are selections, pressing the left arrow creates cursors at the beginning of each selection. Pressing right arrow creates cursors at the end of each selection.

* If there are an even number of cursors present and you type a type of bracket (one of '(', '<', '{', or '\[') then each second cursor will instead type the matching closing bracket. If you then undo, it will convert the second brackets back to the originally typed bracket.

## Addressing Expressions

Executing text of the form `!...` executes an expression that selects and manipulates text in the Body of a window. The expression consists of a series of basic operations that are executed in series. Some operations select text, and some perform a command on the selected text. Those that select text perform their selection relative to the previous selections in the expression. The first selection in the expression operates relative to each of the current selections in the window body, and if there are no previous selection the entire text of the window body is used.

The simple expressions are:

| Expression | Behaviour | 
| ---------- | --------- |
| N          | If N is a number, select that line within the ranges |
| #N         | If N is a number, select and go to that character    |
| /RE/       | Select the first regular expression RE in the ranges |
| 0          | The beginning of the file                            |
| $          | The end of the file                                  |
| .          | The position of the primary cursor                   |

Expressions may be combined using four operators:

| Operator    | Behaviour | 
| ----------- | --------- |
| EXPR1+EXPR2 | Execute EXPR2 starting from the end of EXPR1 |
| EXPR1,EXPR2 | Select from the beginning of EXPR1 to the end of EXPR2 |
| EXPR1-EXPR2 | Execute EXPR2 looking in the reverse direction starting at the beginning of EXPR1 |
| EXPR1;EXPR2 | Select from the beginning of EXPR1 to the end of EXPR2, but evaluating EXPR2 at the end of EXPR1 |


Expressions may be executed concurrently rather than in series by surrounding them in braces, i.e.:

    { EXPR1 EXPR2 }

This has the effect of executing EXPR1 and EXPR2 on the same range, rather than executing EXPR2 on the ranges produced by EXPR1 like normal.

There are five looping and filtering expressions:

| Expression | Behaviour |
| ---------- | --------- |
| x/RE/      | For each matching regular expression RE in the ranges create a new range |
| y/RE/      | For each section of text between the matching regular expression RE create a new range |
| z/RE/      | For each match of RE, create a new range from the start of the match to the start of the next match of the RE |
| g/RE/      | For each range, only keep those that contain RE |
| v/RE/      | For each range, only keep those that do not contain RE |

The commands that operate on the previous selections defined by the selections are:

| Expression | Behaviour |
| ---------- | --------- |
| d          | Delete the text |
| p          | Print the text |
| c/TEXT/    | Change the text of all selections to be TEXT |
| i/TEXT/    | Insert TEXT at the beginning of each selection |
| a/TEXT/    | Append TEXT at the end of each selection |
| s/RE/REPL/ | Replace the text matching RE with the text REPL |
| =          | Print the filenames and line numbers of the ranges |
| C          | Copy the text in the ranges to the clipboard. The text from the ranges are concatenated |

### Examples

For example, executing this expression will create multiple selections, one for each line that ends with an opening brace:

    !x/^.*{$/

We can add an additional 'g' expression to the end to further refine the selection to only those that also contain the word `func`. Note: addressing expressions operate on the set of current selections, so if you want to operate on the full text of the file, remove extra selections by left clicking once. The new expresssion is:

    !x/^.*{$/ g/func/

We can insert some text before those lines those lines (i.e. begin a // comment):

    !x/^.*{$/ g/func/ i/\/\//

As another example, to select from the first occurrence of `begin` in the file to the first occurrence of `end` inclusive, execute this expression:

    !/begin/,/end/

## Commands

The following are the basic Anvil builtin commands. To get more help on them type `Help COMMAND` somewhere, highlight it, and middle-click. For example Help New.

A number of commands take an argument. For example executing `New` makes a new empty window, but executing `New file.c` makes a new window with the contents of file.c. When executing a command with an argument you must highlight the command and it's argument when middle-clicking. Alternately you can delimit the command with the '◊' character in which case middle clicking any place within the '◊'-delimited text executes the command. For example ◊ls -l◊.

| Command | Behaviour |
| ------- | --------- |
| About |	About the editor |
| Acq | Acq 'acquires' it's argument, as if you performed ALT+Right-Click on a text object.
| Ansi |	Enable or disable Ansi colors |
| Clr | Clear (delete) the contents of the window body |
| Cmds |	List the recent external commands |
| Cmds* |	List the most recent external commands executed along with the directory they were executed in |
| Cols | Cols lists all the columns, including whether they are visible or not
| Cols* | Cols* lists all the columns verbosely (including the files in each column) |
| Cut |	Cut selected text |
| Dbg |	Commands for debugging the editor |
| Del |	Delete Window |
| Del! |	Delete Window. If there are unsaved changes, the user is not prompted to save them |
| Delcol |	Delete the column |
| Do |	Execute command |
| Dump |	Save the editor's state to disk |
| Exit |	Exit the editor |
| Font |	Change to next font |
| Fuzz |  Perform a fuzzy search for the arguments in the lines of the body and print matches in a +Live window.  |
| Get |	Load the window body |
| Goto |	Jump to a bookmark |
| Help |	Show help |
| Hidecol | Hidecol hides the current column |
| Id |	Show window ID |
| Kill |	Kill a running job |
| Load |	Load the editor's state from disk |
| LoadStyle | Load style (colors, fonts, &c.) from a file |
| Look |	Look for a string in the window body |
| Mark |	Add a bookmark |
| Marks |	Display bookmarks |
| Marks- |	Clear bookmarks |
| New |	Make a new window |
| Newcol |	Create a column |
| On | Run a command in the specified directory on a remote server |
| Only | Del windows other than the current one |
| Keypass |	Specify the password used to decrypt an ssh private key file |
| Paste |	Paste text |
| Pic | Set background picture for the window body |
| PrintCfg | Print a sample config file to +Errors |
| Put |	Save the window body |
| Putall |	Save all windows |
| Recent |	Display recent files |
| Redo |	Redo the last change |
| Rot |	Rotate selections |
| SaveStyle |	Save current editor style |
| Showcol | Showcol makes the column with the name that matches the first argument visible |
| Shstr | Set the 'shell string' for the current window |
| Snarf |	Copy selected text |
| Syn |	Enable or disable syntax highlighting, or list supported formats |
| Tint | Color selections of text |
| Title |	Set the editor title |
| Undo |	Undo the last change |
| Wins | List the filenames of the open windows |
| Zerox |	Clone a window |
| ◊ |	Insert a ◊ rune, or surround selection with it |

Executing a command of the form SHCMD executes the command SHCMD in the shell and appends the output of the command to the +Errors window of the current directory.

In addition executing a command of the form: `|SHCMD`, `>SHCMD` or `<SHCMD` executes the command SHCMD from the shell and changes the body:

`|SHCMD`: Run the command SHCMD with the primary selection as its stdin and then replace the selection with the output of the command. If there is no primary selection, the entiry window body is used. For example selecting lines and then executing `|sort` will sort those lines.

`>SHCMD`: Run the command SHCMD with the primary selection as its stdin. The output of the command is appended to the +Errors window of the current directory.

`<SHCMD`: Run the command SHCMD and append it's output at the current cursor position. If there is a selection, the selection is replaced with its output.

If any of the SHCMD above begin with '+' then the '+' is stripped and the remainder of the command is run locally, even if it is run in a window that is editing a remote file. This is particulatly useful for plumbing rules.

## Editing on Remote Hosts 

If a file path with the form `<host specifier>:<path>` is opened in a new window, it is treated as a remote file and Anvil attempts to open it over ssh. When such a window is open, executing commands in the tag or body of the window executes the command on the remote system in the directory of the path. 

The `<host specifier>` part of the path may take the following forms:

  * `<host>`          Connect to the host over ssh with the username of the current user
  * `<host>:<port>`   Connect to the host on the specified port
  * `<user>@<host>`   Connect to the host as the specified user
  * `<user>@<host>:<port>` Connect with specific user on specified port
  * `<host>%<proxy>   Connect to the host `proxy` and create a tunnel to `host`, then connect to `host`. The above modifiers to specify a user or port may modify either `host` or `proxy`.

For example, this path specifies editing the file `/tmp/file.txt` on the host `asura`:

    asura:/tmp/file.txt
    
Alternately, if ssh is running on a different port (2222) and requires a different username (rob):

    rob@asura:2222:/tmp/file.txt

Anvil expects to use key-based authentication to connect to the remote host. Anvil will attempt to authenticate using key files found under the directory sshkeys under the anvil configuration directory. The keys may be passwordless, or require a password in which case the `Pass` command should be used to specify a password. If Anvil is running on Linux, it can also load keys from the ssh-agent if it is running.

Anvil requires that remote hosts must be running a Linux-like operating system; specifically it requires the `sh` shell and the commands `cat` and `ls` to be available.

