package expr

import (
	"testing"

	"github.com/jeffwilliams/anvil/editor/internal/runes"
	"github.com/stretchr/testify/assert"
)

func TestInvertRanges(t *testing.T) {
	tests := []struct {
		name         string
		input        []Range
		inputDataLen int
		expected     []Range
	}{
		{
			name:         "empty",
			input:        []Range{},
			inputDataLen: 30,
			expected:     []Range{&irange{0, 30}},
		},
		{
			name:         "middle",
			input:        []Range{&irange{5, 10}},
			inputDataLen: 30,
			expected:     []Range{&irange{0, 5}, &irange{10, 30}},
		},
		{
			name:         "front",
			input:        []Range{&irange{0, 10}},
			inputDataLen: 30,
			expected:     []Range{&irange{10, 30}},
		},
		{
			name:         "multiple",
			input:        []Range{&irange{2, 4}, &irange{10, 12}},
			inputDataLen: 30,
			expected:     []Range{&irange{0, 2}, &irange{4, 10}, &irange{12, 30}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			actual := invertRanges(tc.input, 0, tc.inputDataLen)

			assert.Equal(t, tc.expected, actual)
		})
	}
}

func TestKeepOnlyTheseRanges(t *testing.T) {
	tests := []struct {
		name        string
		inputRanges []Range
		inputData   []byte
		expected    []byte
	}{
		{
			name:        "empty",
			inputRanges: []Range{},
			inputData:   []byte("abc"),
			expected:    []byte{},
		},
		{
			name:        "middle",
			inputRanges: []Range{&irange{1, 2}},
			inputData:   []byte("abc"),
			expected:    []byte("b"),
		},
		{
			name:        "middle 2",
			inputRanges: []Range{&irange{2, 4}},
			inputData:   []byte("letter"),
			expected:    []byte("tt"),
		},
		{
			name:        "more than one",
			inputRanges: []Range{&irange{0, 4}, &irange{8, 10}, &irange{13, 15}, &irange{16, 18}},
			inputData:   []byte("letter to a friend"),
			expected:    []byte("letto rind"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			dataCopy := make([]byte, len(tc.inputData))
			copy(dataCopy, tc.inputData)
			keepOnlyTheseRanges(&dataCopy, tc.inputRanges)

			assert.Equal(t, tc.expected, dataCopy)
		})
	}
}

func TestAddr(t *testing.T) {
	tests := []struct {
		name       string
		inputData  string
		inputAddr  string
		inputRange Range
		expected   []Range
		dot        int
	}{
		{
			name: "/3/-/line/",
			inputData: `line 1
line 2
line 3`,
			inputAddr:  "/3/-/line/",
			inputRange: &irange{0, 21},
			expected:   []Range{&irange{14, 18}}, // 'line'
		},
		{
			name: "/2/-/line/",
			inputData: `line 1
line 2
line 3`,
			inputAddr:  "/2/-/line/",
			inputRange: &irange{0, 21},
			expected:   []Range{&irange{7, 11}}, // 'line' before 2
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			// Parse address
			var s Scanner
			toks, ok := s.Scan(tc.inputAddr)
			if !ok {
				t.Fatalf("Scan failed")
			}

			var p Parser
			p.matchLimit = 100
			tree, err := p.Parse(toks)

			if err != nil {
				t.Fatalf("Parse failed when it should succeed. Error: %s", err)
			}

			// Get the tree part (we have expr->terms->addr)
			expr := tree.(expr)
			addr := expr.terms[0]

			stage := newAddrStage(addr, tc.dot)
			dataCopy := make([]byte, len(tc.inputData))
			copy(dataCopy, []byte(tc.inputData))
			runeOffsetCache := runes.NewOffsetCache(8000)
			actual, _ := stage.Execute(&dataCopy, &runeOffsetCache, []Range{tc.inputRange})

			msgAndArgs := []interface{}{}
			if len(actual) > 0 {
				msgAndArgs = append(msgAndArgs, "actual data in first range:", applyRangeToString(tc.inputData, actual[0]))
			} else {
				msgAndArgs = append(msgAndArgs, "addr:", addr)
			}

			assert.Equal(t, tc.expected, actual, msgAndArgs)
		})
	}
}

func TestOperation(t *testing.T) {
	tests := []struct {
		name       string
		typ        simpleAddrType
		inputData  string
		inputOp    string
		inputRange Range
		expected   []Range
	}{
		{
			name:       "x: empty",
			inputData:  "",
			inputOp:    "x/abc/",
			inputRange: &irange{},
			expected:   []Range{},
		},
		{
			name:       "x: no match",
			inputData:  "here we are",
			inputOp:    "x/abc/",
			inputRange: &irange{0, 11},
			expected:   []Range{},
		},
		{
			name:       "x: one match",
			inputData:  "here we are",
			inputOp:    "x/we/",
			inputRange: &irange{0, 11},
			expected:   []Range{&irange{5, 7}},
		},
		{
			name:       "x: one match re",
			inputData:  "here we are",
			inputOp:    "x/w./",
			inputRange: &irange{0, 11},
			expected:   []Range{&irange{5, 7}},
		},
		{
			name:       "x: two matches",
			inputData:  "here we are",
			inputOp:    "x/re/",
			inputRange: &irange{0, 11},
			expected:   []Range{&irange{2, 4}, &irange{9, 11}},
		},
		{
			name:       "x: one match unicode",
			inputData:  "←ere←we are",
			inputOp:    "x/w./",
			inputRange: &irange{0, 11},
			expected:   []Range{&irange{5, 7}},
		},
		{
			name:       "x: out of range",
			inputData:  "here we are",
			inputOp:    "x/are/",
			inputRange: &irange{0, 5},
			expected:   []Range{},
		},
		{
			name:       "x: out of range 2",
			inputData:  "here we are",
			inputOp:    "x/here/",
			inputRange: &irange{5, 11},
			expected:   []Range{},
		},
		{
			name:       "y: one match",
			inputData:  "here we are",
			inputOp:    "y/we/",
			inputRange: &irange{0, 11},
			expected:   []Range{&irange{0, 5}, &irange{7, 11}},
		},
		{
			name:       "y: two matches",
			inputData:  "here we are",
			inputOp:    "y/re/",
			inputRange: &irange{0, 11},
			expected:   []Range{&irange{0, 2}, &irange{4, 9}, &irange{11, 11}},
		},
		{
			name:       "g: matches",
			inputData:  "here we are",
			inputOp:    "g/we/",
			inputRange: &irange{0, 11},
			expected:   []Range{&irange{0, 11}},
		},
		{
			name:       "g: doesn't match",
			inputData:  "here we are",
			inputOp:    "g/heart/",
			inputRange: &irange{0, 11},
			expected:   []Range{},
		},
		{
			name:       "v: matches",
			inputData:  "here we are",
			inputOp:    "v/we/",
			inputRange: &irange{0, 11},
			expected:   []Range{},
		},
		{
			name:       "v: doesn't match",
			inputData:  "here we are",
			inputOp:    "v/heart/",
			inputRange: &irange{0, 11},
			expected:   []Range{&irange{0, 11}},
		},
		{
			name:       "n: more braces",
			inputData:  "some text {braced {internally} } a",
			inputOp:    "n/{/}/",
			inputRange: &irange{0, 34},
			expected:   []Range{&irange{10, 32}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			// Parse address
			var s Scanner
			toks, ok := s.Scan(tc.inputOp)
			if !ok {
				t.Fatalf("Scan failed")
			}

			var p Parser
			p.matchLimit = 100
			tree, err := p.Parse(toks)

			if err != nil {
				t.Fatalf("Parse failed when it should succeed. Error: %s", err)
			}

			// Get the tree part (we have expr->terms->addr)
			expr := tree.(expr)
			oper := expr.terms[0].(operation)

			stage, err := newOperationStage(oper)
			assert.NoError(t, err)

			dataCopy := make([]byte, len(tc.inputData))
			copy(dataCopy, []byte(tc.inputData))
			runeOffsetCache := runes.NewOffsetCache(8000)
			actual, _ := stage.Execute(&dataCopy, &runeOffsetCache, []Range{tc.inputRange})

			msgAndArgs := []interface{}{}
			if len(actual) > 0 {
				msgAndArgs = append(msgAndArgs, "actual data in first range:", applyRangeToString(tc.inputData, actual[0]))
			}

			assert.Equal(t, tc.expected, actual, msgAndArgs)
		})
	}
}

func TestCommand(t *testing.T) {
	tests := []struct {
		name      string
		typ       simpleAddrType
		inputData string
		inputExpr string
		expected  []handleCall
	}{
		{
			name:      "delete: simple",
			inputData: "This is a test.",
			inputExpr: "#6,#8d",
			expected: []handleCall{
				{handleDelete, 5, 8, ""},
			},
		},
		{
			name:      "delete: complex",
			inputData: "This is a test.",
			inputExpr: "x/is/d",
			expected: []handleCall{
				{handleDelete, 2, 4, ""},
				// The next delete will affect the range _after_ the first delete has already been applied
				// so it must be shifted to affect the original text meant to be deleted.
				// I.e. it affects the new input "Th is a test.",
				{handleDelete, 3, 5, ""},
			},
		},
		{
			name:      "replace: complex",
			inputData: "This is a test.",
			inputExpr: "x/is/c/and/",
			expected: []handleCall{
				{handleDelete, 2, 4, ""},
				{handleInsert, 2, 0, "and"},

				{handleDelete, 6, 8, ""},
				{handleInsert, 6, 0, "and"},
			},
		},
		{
			name:      "x as noop: beginning of lines",
			inputData: "line1\nline2",
			inputExpr: "x/(?m)^/",
			expected: []handleCall{
				{handleNoop, 0, 0, ""},
				{handleNoop, 6, 6, ""},
			},
		},
		{
			name:      "s: basic with backref",
			inputData: "hello world\nhello world\n",
			inputExpr: `s/hell(o)/\1/`,
			expected: []handleCall{
				{handleDelete, 0, 5, ""},
				{handleInsert, 0, 0, "o"},

				{handleDelete, 8, 13, ""},
				{handleInsert, 8, 0, "o"},
			},
		},
		{
			name: "s: multiple matches",
			inputData: `1   CAT
2   DOG
`,
			inputExpr: `s/([^\s]+)( +)([^\s]+)/\3\2\1/`,
			expected: []handleCall{
				{handleDelete, 0, 7, ""},
				{handleInsert, 0, 0, "CAT   1"},

				{handleDelete, 8, 15, ""},
				{handleInsert, 8, 0, "DOG   2"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			// Parse address
			var s Scanner
			toks, ok := s.Scan(tc.inputExpr)
			if !ok {
				t.Fatalf("Scan failed")
			}

			var p Parser
			p.matchLimit = 100
			tree, err := p.Parse(toks)

			if err != nil {
				t.Fatalf("Parse failed when it should succeed. Error: %s", err)
			}

			var handler testHandler

			// Get the tree part (we have expr->terms->addr)
			dataCopy := make([]byte, len(tc.inputData))
			copy(dataCopy, []byte(tc.inputData))

			vm, err := NewInterpreter(dataCopy, tree, &handler, 0)
			if err != nil {
				t.Fatalf("Creating interpreter failed: %s", err)
			}

			vm.Execute([]Range{&irange{0, len(tc.inputData)}})

			/*msgAndArgs := []interface{}{}
			if len(actual) > 0 {
				msgAndArgs = append(msgAndArgs, "actual data in first range:", applyRangeToString(tc.inputData, actual[0]))
			}*/

			assert.Equal(t, tc.expected, handler.calls)
		})
	}
}

func applyRangeToString(data string, r Range) string {
	if r.Start() == 0 && r.End() == 0 {
		return ""
	}

	walker := runes.NewWalker([]byte(data))
	return string(walker.TextBetweenRuneIndices(r.Start(), r.End()))
}

type handleCallType int

const (
	handleDelete handleCallType = iota
	handleInsert
	handleDisplay
	handleDisplayContents
	handleNoop
	handleCopy
)

type handleCall struct {
	typ        handleCallType
	start, end int
	value      string
}

type testHandler struct {
	calls []handleCall
}

func (t *testHandler) Delete(r Range) {
	t.calls = append(t.calls, handleCall{handleDelete, r.Start(), r.End(), ""})
}
func (t *testHandler) Insert(index int, value []byte) {
	t.calls = append(t.calls, handleCall{handleInsert, index, 0, string(value)})
}
func (t *testHandler) Display(r Range) {
	t.calls = append(t.calls, handleCall{handleDisplay, r.Start(), r.End(), ""})
}
func (t *testHandler) DisplayContents(r Range, prefix string, displayPosition bool) {
	t.calls = append(t.calls, handleCall{handleDisplayContents, r.Start(), r.End(), ""})
}
func (t *testHandler) Noop(r Range) {
	t.calls = append(t.calls, handleCall{handleNoop, r.Start(), r.End(), ""})
}
func (t *testHandler) Copy(r Range) {
	t.calls = append(t.calls, handleCall{handleCopy, r.Start(), r.End(), ""})
}
func (t *testHandler) Done() {
}

func TestRangeUnion(t *testing.T) {
	tests := []struct {
		name     string
		input    []Range
		expected []Range
	}{
		{
			name:     "empty",
			input:    []Range{},
			expected: []Range{},
		},
		{
			name:     "single",
			input:    []Range{&irange{1, 5}},
			expected: []Range{&irange{1, 5}},
		},
		{
			name:     "same",
			input:    []Range{&irange{1, 5}, &irange{1, 5}},
			expected: []Range{&irange{1, 5}},
		},
		{
			name:     "within",
			input:    []Range{&irange{1, 5}, &irange{2, 4}},
			expected: []Range{&irange{1, 5}},
		},
		{
			name:     "within 2",
			input:    []Range{&irange{2, 4}, &irange{1, 5}},
			expected: []Range{&irange{1, 5}},
		},
		{
			name:     "within 3",
			input:    []Range{&irange{1, 3}, &irange{1, 5}},
			expected: []Range{&irange{1, 5}},
		},
		{
			name:     "within 4",
			input:    []Range{&irange{1, 5}, &irange{3, 5}},
			expected: []Range{&irange{1, 5}},
		},
		{
			name:     "overlap",
			input:    []Range{&irange{1, 5}, &irange{3, 7}},
			expected: []Range{&irange{1, 7}},
		},
		{
			name:     "overlap reverse",
			input:    []Range{&irange{3, 7}, &irange{1, 5}},
			expected: []Range{&irange{1, 7}},
		},
		{
			name:     "multi",
			input:    []Range{&irange{1, 5}, &irange{2, 3}, &irange{2, 6}, &irange{10, 15}, &irange{9, 12}, &irange{20, 21}},
			expected: []Range{&irange{1, 6}, &irange{9, 15}, &irange{20, 21}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			output := unionOfRanges(tc.input)

			assert.Equal(t, tc.expected, output)
		})
	}
}

func TestGroups(t *testing.T) {
	tests := []struct {
		name       string
		inputData  string
		inputOp    string
		inputRange Range
		expected   []Range
	}{
		{
			name:       "group",
			inputData:  "this is a test",
			inputOp:    "{x/is/x/test/}",
			inputRange: &irange{0, 14},
			expected:   []Range{&irange{2, 4}, &irange{5, 7}, &irange{10, 14}},
		},
		{
			name:       "group 2",
			inputData:  "this is a test",
			inputOp:    "{#2,#4 x/test/}",
			inputRange: &irange{0, 14},
			expected:   []Range{&irange{1, 4}, &irange{10, 14}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			// Parse address
			var s Scanner
			toks, ok := s.Scan(tc.inputOp)
			if !ok {
				t.Fatalf("Scan failed")
			}

			var p Parser
			p.matchLimit = 100
			tree, err := p.Parse(toks)

			if err != nil {
				t.Fatalf("Parse failed when it should succeed. Error: %s", err)
			}

			// Get the tree part (we have expr->terms->group)
			expr := tree.(expr)
			oper := expr.terms[0].(group)

			stage, err := newGroupStage(oper, 0)
			assert.NoError(t, err)

			dataCopy := make([]byte, len(tc.inputData))
			copy(dataCopy, []byte(tc.inputData))
			runeOffsetCache := runes.NewOffsetCache(8000)
			actual, _ := stage.Execute(&dataCopy, &runeOffsetCache, []Range{tc.inputRange})

			msgAndArgs := []interface{}{}
			if len(actual) > 0 {
				msgAndArgs = append(msgAndArgs, "actual data in first range:", applyRangeToString(tc.inputData, actual[0]))
			}

			assert.Equal(t, tc.expected, actual, msgAndArgs)
		})
	}
}
