package expr

// Addressing Expressions
/*
Addresses
An address identifies a substring in a file. In the following, ‘character n’ means the null string after the n-th character in the file, with 1 the first character in the file. ‘Line n’ means the n-th match, starting at the beginning of the file, of the regular expression .*\n?. All files always have a current substring, called dot, that is the default address.

Simple Addresses
#n    The empty string after character n; #0 is the beginning of the file.
n     Line n; 0 is the beginning of the file.
/regexp/
?regexp?
	The substring that matches the regular expression, found by looking toward the end (/) or beginning (?) of the file, and if necessary continuing the search from the other end to the starting point of the search. The matched substring may straddle the starting point. When entering a pattern containing a literal question mark for a backward search, the question mark should be specified as a member of a class.
0     The string before the first full line. This is not necessarily the null string; see + and − below.
$     The null string at the end of the file.
.     Dot.
'     The mark in the file (see the k command below).
"regexp"
	Preceding a simple address (default .), refers to the address evaluated in the unique file whose menu line matches the regular expression.

Compound Addresses
In the following, a1 and a2 are addresses.
a1+a2   The address a2 evaluated starting at the end of a1.
a1−a2   The address a2 evaluated looking in the reverse direction starting at the beginning of a1.
a1,a2   The substring from the beginning of a1 to the end of a2. If a1 is missing, 0 is substituted. If a2 is missing, $ is substituted.
a1;a2   Like a1,a2, but with a2 evaluated at the end of, and dot set to, a1.
The operators + and − are high precedence, while , and ; are low precedence.
In both + and − forms, if a2 is a line or character address with a missing number, the number defaults to 1. If a1 is missing, . is substituted. If both a1 and a2 are present and distinguishable, + may be elided. a2 may be a regular expression; if it is delimited by ?’s, the effect of the + or − is reversed.
It is an error for a compound address to represent a malformed substring. Some useful idioms: a1+− (a1-+) selects the line containing the end (beginning) of a1. 0/regexp/ locates the first match of the expression in the file. (The form 0;// sets dot unnecessarily.) ./regexp/// finds the second following occurrence of the expression, and .,/regexp/ extends dot.

Commands
In the following, text demarcated by slashes represents text delimited by any printable character except alphanumerics. Any number of trailing delimiters may be elided, with multiple elisions then representing null strings, but the first delimiter must always be present. In any delimited text, newline may not appear literally; \n may be typed for newline; and \/ quotes the delimiter, here /. Backslash is otherwise interpreted literally, except in s commands.
Most commands may be prefixed by an address to indicate their range of operation. Those that may not are marked with a * below. If a command takes an address and none is supplied, dot is used. The sole exception is the w command, which defaults to 0,$. In the description, ‘range’ is used to represent whatever address is supplied. Many commands set the value of dot as a side effect. If so, it is always set to the ‘result’ of the change: the empty string for a deletion, the new text for an insertion, etc. (but see the s and e commands).

Text commands
a/text/
or
a
lines of text
.     Insert the text into the file after the range. Set dot.
c
i     Same as a, but c replaces the text, while i inserts before the range.
d     Delete the text in the range. Set dot.
s/regexp/text/
	Substitute text for the first match to the regular expression in the range. Set dot to the modified range. In text the character & stands for the string that matched the expression. Backslash behaves as usual unless followed by a digit: \d stands for the string that matched the subexpression begun by the d-th left parenthesis. If s is followed immediately by a number n, as in s2/x/y/, the n-th match in the range is substituted. If the command is followed by a g, as in s/x/y/g, all matches in the range are substituted.
m a1
t a1   Move (m) or copy (t) the range to after a1. Set dot.

Display commands
p     Print the text in the range. Set dot.
=     Print the line address and character address of the range.
=#    Print just the character address of the range.

Loops and Conditionals
x/regexp/ command
	For each match of the regular expression in the range, run the command with dot set to the match. Set dot to the last match. If the regular expression and its slashes are omitted, /.*\n/ is assumed. Null string matches potentially occur before every character of the range and at the end of the range.
y/regexp/ command
	Like x, but run the command for each substring that lies before, between, or after the matches that would be generated by x. There is no default regular expression. Null substrings potentially occur before every character in the range.
* X/regexp/ command
	For each file whose menu entry matches the regular expression, make that the current file and run the command. If the expression is omitted, the command is run in every file.
* Y/regexp/ command
	Same as X, but for files that do not match the regular expression, and the expression is required.
g/regexp/ command
v/regexp/ command
	If the range contains (g) or does not contain (v) a match for the expression, set dot to the range and run the command.
These may be nested arbitrarily deeply, but only one instance of either X or Y may appear in a single command. An empty command in an x or y defaults to p; an empty command in X or Y defaults to f. g and v do not have defaults.

*/

/*
Language grammar:

expr -> group | term* command*
term -> group | addr | operation
group -> '{' term* '}'
addr -> inneraddr ',' addr | inneraddr ';' addr | inneraddr
inneraddr -> simpleaddr '+' inneraddr | simpleaddr '-' inneraddr | simpleaddr
simpleaddr -> '#' int | int | '/' regexp '/' | '?' regexp '?' | '$' | '\'' m
operation -> "x/" regexp "/" | "y/" regexp "/" | "g/" regexp "/" | "v/" regexp "/" | 'n' int? '/' text ("," text)* "/" text ("," text)* "/" int?
regexp -> any_char_but_/_escaped
command -> p | d | "a/" text "/" | "c/" text "/" | "i/" text "/" | "s/" text "/" text "/"

The n address is for selecting a range delimited by open/close tokens, that may be nested.
The first /block/ is a comma separated list of opening tokens (i.e. { or 'begin') and the second /block/ are the closing
tokens (i.e. } or end). The optional int defines the nesting level to select. The operation will ignore that many
opening tokens before matching. Example: n/{/}/ match opening and closing braces at the right nesting level. For the opening
tokens {,[, and < if the closing token is ommitted it is assumed to be },], or > respectively.

conversion of commands to handler/piecetable:
p -> print
d -> delete
a -> insert at X
c -> delete, insert
i -> insert
s -> delete,insert


*/
