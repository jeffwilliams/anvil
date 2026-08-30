package words

import (
	"bytes"
	"unicode"
	"unicode/utf8"

	"github.com/armon/go-radix"
	"github.com/jeffwilliams/anvil/editor/internal/slice"
)

type Completion struct {
	word    string
	sources []string
}

func (c Completion) Word() string {
	return c.word
}

func (c Completion) Sources() []string {
	return c.sources
}

type Completer struct {
	tree *radix.Tree
}

func NewCompleter() *Completer {
	return &Completer{
		tree: radix.New(),
	}
}
func (c *Completer) Len() int {
	return c.tree.Len()
}

// Build deletes all completion information from the specified source from the tree,
// then replaces it with the words in `text`.
func (c *Completer) Build(source string, text []byte) {
	c.DeleteAllFromSource(source)

	words := wordsIn(text)
	for _, w := range words {
		c.insert(w, source)
	}
}

func (c *Completer) insert(word string, source string) {
	node, ok := c.tree.Get(word)
	var compl *Completion
	if !ok {
		compl := &Completion{
			word:    word,
			sources: []string{source},
		}
		c.tree.Insert(word, compl)
		return
	}
	compl = node.(*Completion)
	for _, e := range compl.sources {
		if e == source {
			return
		}
	}
	compl.sources = append(compl.sources, source)
}

func (c *Completer) DeleteAllFromSource(source string) {
	var toDelFromTree []string

	fn := func(s string, v interface{}) bool {
		compl := v.(*Completion)

		l := slice.RemoveFirstMatchFromSlice(compl.sources, func(i int) bool {
			return compl.sources[i] == source
		})

		compl.sources = l.([]string)
		if len(compl.sources) == 0 {
			toDelFromTree = append(toDelFromTree, compl.word)
		}

		// walk all values
		return false
	}

	c.tree.Walk(fn)

	for _, v := range toDelFromTree {
		c.tree.Delete(v)
	}
}

func (c *Completer) Completions(word string) (comps []Completion, commonPrefix string) {
	fn := func(s string, v interface{}) bool {
		if s == word {
			return false
		}
		comps = append(comps, *v.(*Completion))
		// walk all values
		return false
	}

	c.tree.WalkPrefix(word, fn)
	commonPrefix = c.commonPrefix(comps)

	return
}

func (c *Completer) commonPrefix(comps []Completion) string {
	strs := make([]string, len(comps))
	for i, s := range comps {
		strs[i] = s.word
	}

	return CommonPrefix(strs...)
}

func CommonPrefix(s ...string) string {
	if len(s) == 0 {
		return ""
	}

	if len(s) == 1 {
		return s[0]
	}

	pfx := s[0]

	for _, c := range s[1:] {
		pfx = commonPrefix(pfx, c)
		if pfx == "" {
			break
		}
	}

	return pfx
}

func commonPrefix(s1, s2 string) string {
	r1 := []rune(s1)
	r2 := []rune(s2)

	n := len(r1)
	if len(r2) < n {
		n = len(r2)
	}

	i := 0
	for ; i < n; i++ {
		if r1[i] != r2[i] {
			break
		}
	}

	return string(r1[:i])
}

func wordsIn(text []byte) (words []string) {
	t := text

	isWordChar := func(r rune) bool {
		return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
	}

	var word bytes.Buffer

	pack := func() {
		if word.Len() > 0 {
			words = append(words, word.String())
		}
	}

	for len(t) > 0 {

		r, size := utf8.DecodeRune(t)

		if isWordChar(r) {
			word.WriteRune(r)
		} else {
			pack()
			word.Reset()
		}

		t = t[size:]
	}

	pack()

	return
}
