package main

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type helper struct {
	data map[string]string
}

var help = newHelp()

func newHelp() helper {
	h := helper{
		data: map[string]string{},
	}

	h.addTopics()

	return h
}

func (h *helper) addHelp(topic string, text string) {
	h.data[topic] = text
	h.data[strings.ToLower(topic)] = text
}

func (h helper) help(topic string) string {
	t, ok := h.data[topic]
	if !ok {
		return ""
	}
	return t
}

func AddHelp(topic string, text string) {
	help.addHelp(topic, text)
}

func Help(topic string) string {
	return help.help(topic)
}

func (h helper) addTopics() {
	s := fmt.Sprintf("%s is an editor strongly inspired by Rob Pike's Acme editor originally written for Plan 9 (see http://acme.cat-v.org/).",
		strings.Title(editorName))
	h.addHelp("Intro", s)

	h.addEnvironmentHelp()
	h.addRegexHelp()
	h.addRangeStatementHelp()
	h.addKeyActionHelp()
	h.addKeyActionMkdocsHelp()
}

func topLevelHelpString() string {
	var text bytes.Buffer
	fmt.Fprintf(&text,
		`Welcome to the %s editor. In this output, you can click the text delimited by ◊ characters to get help on specific topics. The help text will be appended to the end of this buffer, which you can scroll to by middle-clicking and dragging the scrollbar to the left of the window, or by typing CTRL-END.

You can browse documentation for the editor at: https://anvil-editor.net

Middle-click in ◊Help Intro◊ for a brief introduction of the editor.

=== Help Topics ===

This section lists help topics for various aspects of the editor.

Environment (◊Help Environment◊)
	Information about the environment variables set when external commands are run

Regex (◊Help Regex◊)
	Syntax of regular expressions

Range Statements (◊Help Range Statements◊)
	Syntax of range statements

Commands (◊Help Commands◊)
	A list of built-in editor commands 

Key Actions (◊Help Key Actions◊)
	A list of actions that can be used in key maps

`, strings.Title(editorName))

	return text.String()
}

func (h helper) addEnvironmentHelp() {
	s := `

The following environment variables are set for external commands:

ANVIL_DIR The directory in which Anvil is running

ANVIL_WIN_LOCAL_PATH  The path of the file being edited in the window where the command was executed. This path does not include the hostname (if the command was being executed remotely).

ANVIL_WIN_GLOBAL_PATH The path of the file being edited in the window where the command was executed including the hostname in the ssh format (i.e. HOST:PATH)

ANVIL_WIN_LOCAL_DIR The parent directory of the file being edited in the window where the command was executed, or the file itself if it is a directory. This path does not include the hostname (if the command was being executed remotely).

ANVIL_WIN_GLOBAL_DIR  The parent directory of the file being edited in the window where the command was executed, or the file itself if it is a directory. This includes the hostname in the ssh format (i.e. HOST:PATH)

ANVIL_WIN_ID  The internal numeric ID of the window. This may be used in the API.

ANVIL_API_PORT  The TCP port number on which the Anvil REST API is running. Connections to the API should be performed to the local host; if a remote command is executed an SSH tunnel is created so that commands may connect locally.

ANVIL_API_SESS  Session id used to authenticate the client program against the API.
`
	h.addHelp("Environment", s)
}

func (h helper) addRegexHelp() {
	s := `

The syntax of regular expressions used in Anvil is the same as the regular expressions of the Go regexp package. A brief summary is printed here.

Single characters:

.              any character, possibly including newline (flag s=true)
[xyz]          character class
[^xyz]         negated character class
\d             Perl character class
\D             negated Perl character class
[[:alpha:]]    ASCII character class
[[:^alpha:]]   negated ASCII character class
\pN            Unicode character class (one-letter name)
\p{Greek}      Unicode character class
\PN            negated Unicode character class (one-letter name)
\P{Greek}      negated Unicode character class

Composites:

xy             x followed by y
x|y            x or y (prefer x)

Repetitions:

x*             zero or more x, prefer more
x+             one or more x, prefer more
x?             zero or one x, prefer one
x{n,m}         n or n+1 or ... or m x, prefer more
x{n,}          n or more x, prefer more
x{n}           exactly n x
x*?            zero or more x, prefer fewer
x+?            one or more x, prefer fewer
x??            zero or one x, prefer zero
x{n,m}?        n or n+1 or ... or m x, prefer fewer
x{n,}?         n or more x, prefer fewer
x{n}?          exactly n x

Grouping:

(re)           numbered capturing group (submatch)
(?P<name>re)   named & numbered capturing group (submatch)
(?:re)         non-capturing group
(?flags)       set flags within current group; non-capturing
(?flags:re)    set flags during re; non-capturing

Flag syntax is xyz (set) or -xyz (clear) or xy-z (set xy, clear z). The flags are:

i              case-insensitive (default false)
m              multi-line mode: ^ and $ match begin/end line in addition to begin/end text (default false)
s              let . match \n (default false)
U              ungreedy: swap meaning of x* and x*?, x+ and x+?, etc (default false)

Empty strings:

^              at beginning of text or line (flag m=true)
$              at end of text (like \z not \Z) or line (flag m=true)
\A             at beginning of text
\b             at ASCII word boundary (\w on one side and \W, \A, or \z on the other)
\B             not at ASCII word boundary
\z             at end of text

Escape sequences:

\a             bell (== \007)
\f             form feed (== \014)
\t             horizontal tab (== \011)
\n             newline (== \012)
\r             carriage return (== \015)
\v             vertical tab character (== \013)
\*             literal *, for any punctuation character *
\123           octal character code (up to three digits)
\x7F           hex character code (exactly two digits)
\x{10FFFF}     hex character code
\Q...\E        literal text ... even if ... has punctuation

Character class elements:

x              single character
A-Z            character range (inclusive)
\d             Perl character class
[:foo:]        ASCII character class foo
\p{Foo}        Unicode character class Foo
\pF            Unicode character class F (one-letter name)

Named character classes as character class elements:

[\d]           digits (== \d)
[^\d]          not digits (== \D)
[\D]           not digits (== \D)
[^\D]          not not digits (== \d)
[[:name:]]     named ASCII class inside character class (== [:name:])
[^[:name:]]    named ASCII class inside negated character class (== [:^name:])
[\p{Name}]     named Unicode property inside character class (== \p{Name})
[^\p{Name}]    named Unicode property inside negated character class (== \P{Name})

Perl character classes (all ASCII-only):

\d             digits (== [0-9])
\D             not digits (== [^0-9])
\s             whitespace (== [\t\n\f\r ])
\S             not whitespace (== [^\t\n\f\r ])
\w             word characters (== [0-9A-Za-z_])
\W             not word characters (== [^0-9A-Za-z_])

ASCII character classes:

[[:alnum:]]    alphanumeric (== [0-9A-Za-z])
[[:alpha:]]    alphabetic (== [A-Za-z])
[[:ascii:]]    ASCII (== [\x00-\x7F])
[[:blank:]]    blank (== [\t ])
[[:cntrl:]]    control (== [\x00-\x1F\x7F])
[[:digit:]]    digits (== [0-9])
[[:graph:]]    graphical (== [!-~] == [A-Za-z0-9!"#$%&'()*+,\-./:;<=>?@[\\\]^_` + "`" + `{|}~])
[[:lower:]]    lower case (== [a-z])
[[:print:]]    printable (== [ -~] == [ [:graph:]])
[[:punct:]]    punctuation (== [!-/:-@[-` + "`" + `{-~])
[[:space:]]    whitespace (== [\t\n\v\f\r ])
[[:upper:]]    upper case (== [A-Z])
[[:word:]]     word characters (== [0-9A-Za-z_])
[[:xdigit:]]   hex digit (== [0-9A-Fa-f])

Unicode character classes are those in unicode.Categories and unicode.Scripts.

`
	h.addHelp("Regex", s)

}

func (h helper) addRangeStatementHelp() {
	s := `Range Statements

Executing text of the form '!...' executes an expression that selects and manipulates text in the Body of a window. The expression consists of a series of basic operations that are executed in series. Some operations select text, and some perform a command on the selected text. Those that select text perform their selection relative to the previous selections in the expression. The first selection in the expression operates relative to each of the current selections in the window body, and if there are no previous selection the entire text of the window body is used.

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

Examples

For example, executing this expression will create multiple selections, one for each line that ends with an opening brace:

    !x/^.*{$/

We can add an additional 'g' expression to the end to further refine the selection to only those that also contain the word 'func'. Note: addressing expressions operate on the set of current selections, so if you want to operate on the full text of the file, remove extra selections by left clicking once. The new expresssion is:

    !x/^.*{$/ g/func/

We can insert some text before those lines those lines (i.e. begin a // comment):

    !x/^.*{$/ g/func/ i/\/\//

As another example, to select from the first occurrence of 'begin' in the file to the first occurrence of 'end' inclusive, execute this expression:

    !/begin/,/end/
`
	h.addHelp("Range Statements", s)

}

func (h helper) addKeyActionHelp() {
	var text bytes.Buffer
	fmt.Fprintf(&text, `Key Actions

The following actions can be used in key mappings.

`)
	
	sortedKeyActions := make([]keyAction, len(keyActions))
	copy(sortedKeyActions, keyActions)
	sort.Slice(sortedKeyActions, func(i, j int) bool {
		return keyActions[i].name < keyActions[j].name
	})

	for _, a := range sortedKeyActions {
		fmt.Fprintf(&text, "%s", a.name)
		for i, p := range a.paramLabels {
			if i == 0 {
				fmt.Fprintf(&text, "  [")
			} else {
				fmt.Fprintf(&text, "  ")
			}

			fmt.Fprintf(&text, "%s", p)

			if i == len(a.paramLabels)-1 {
				fmt.Fprintf(&text, "]")
			}
		}
		fmt.Fprintf(&text, "\n\t%s\n", a.desc)
	}

	h.addHelp("Key Actions", text.String())
}

// addKeyActionMkdocsHelp is used to help me update the website docs.
func (h helper) addKeyActionMkdocsHelp() {
	var text bytes.Buffer

	fmt.Fprintf(&text, `| Name | Arguments | Description |
| ----- | --- |--------- |
`)

	sortedKeyActions := make([]keyAction, len(keyActions))
	copy(sortedKeyActions, keyActions)
	sort.Slice(sortedKeyActions, func(i, j int) bool {
		return keyActions[i].name < keyActions[j].name
	})

	for _, a := range sortedKeyActions {
		fmt.Fprintf(&text, "| %s | ", a.name)
		for i, p := range a.paramLabels {
			if i > 0 {
				fmt.Fprintf(&text, "  ")
			}

			fmt.Fprintf(&text, "%s", p)
		}
		fmt.Fprintf(&text, " | %s |\n", a.desc)

	}

	h.addHelp("Key Actions mkdocs", text.String())
}

// addCommandMkdocsHelp is used to help me update the website docs.
func (h helper) addCommandMkdocsHelp() {
	var text bytes.Buffer
	
	cmds := NewCommandExecutor(nil)
	sorted :=	 make([]command, 0, len(cmds.commands))
	for _, cmd := range cmds.commands {
		sorted = append(sorted, cmd)
	}
	sort.Slice(sorted, func(i,j int) bool {
		return sorted[i].name < sorted[j].name
	})

	r, err := regexp.Compile(`[^ ]*/(plumbing|style.js)`)
	if err != nil {
		r = nil
	}
	replacePaths := func(s string) string {	
		if r == nil {
			return s
		}
		return string(r.ReplaceAll([]byte(s), []byte("[&lt;config-dir&gt;](config.md)/$1")))
	}
	
	for _, cmd := range sorted {
		fmt.Fprintf(&text, "## %s\n\n", cmd.name)
		fmt.Fprintf(&text, "%s\n\n", replacePaths(cmd.longHelp))
	}


	h.addHelp("Commands mkdocs", text.String())
}
