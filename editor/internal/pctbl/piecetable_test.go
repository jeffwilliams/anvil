package pctbl

import (
	"fmt"
	"testing"
	"unicode/utf8"
)

func TestPieceTableInserts(t *testing.T) {
	tests := []test{
		{
			name:    "empty",
			initial: "",
			ops: []testOp{{
				opcode:       insert,
				index:        0,
				textToInsert: "",
			}},
			expected: "",
		},
		{
			name:    "append to empty",
			initial: "",
			ops: []testOp{{
				opcode:       insert,
				index:        0,
				textToInsert: "abc",
			}},
			expected: "abc",
		},
		{
			name:    "append to original",
			initial: "abc",
			ops: []testOp{{
				opcode:       insert,
				index:        3,
				textToInsert: "def",
			}},
			expected: "abcdef",
		},
		{
			name:    "insert before original",
			initial: "def",
			ops: []testOp{{
				opcode:       insert,
				index:        0,
				textToInsert: "abc",
			}},
			expected: "abcdef",
		},
		{
			name:    "insert inside original",
			initial: "wonful",
			ops: []testOp{{
				opcode:       insert,
				index:        3,
				textToInsert: "der",
			}},
			expected: "wonderful",
		},
		{
			name:    "double append",
			initial: "won",
			ops: []testOp{
				{opcode: insert, index: 3, textToInsert: "der"},
				{opcode: insert, index: 6, textToInsert: "ful"},
			},
			expected: "wonderful",
		},
		{
			name:    "double prepend",
			initial: "ful",
			ops: []testOp{
				{opcode: insert, index: 0, textToInsert: "der"},
				{opcode: insert, index: 0, textToInsert: "won"},
			},
			expected: "wonderful",
		},
		{
			name:    "double insert in middle",
			initial: "woul",
			ops: []testOp{
				{opcode: insert, index: 2, textToInsert: "nf"}, //wonful
				{opcode: insert, index: 3, textToInsert: "der"},
			},
			expected: "wonderful",
		},
		{
			name:    "boundary insert",
			initial: "abcdef",
			ops: []testOp{
				{opcode: insert, index: 3, textToInsert: "xxx"}, //abcxxxdef
				{opcode: insert, index: 3, textToInsert: "yyy"},
			},
			expected: "abcyyyxxxdef",
		},
		{
			name:    "boundary insert 2",
			initial: "abcdef",
			ops: []testOp{
				{opcode: insert, index: 3, textToInsert: "xxx"}, //abcxxxdef
				{opcode: insert, index: 6, textToInsert: "yyy"},
			},
			expected: "abcxxxyyydef",
		},
		{
			name:    "insert after unicode",
			initial: "☺bcdef",
			ops: []testOp{
				{opcode: insert, index: 3, textToInsert: "xxx"},
			},
			expected: "☺bcxxxdef",
		},
		{
			name:    "set with undo",
			initial: "A day in the life",
			ops: []testOp{
				{
					opcode:         setWithUndo,
					index:          0,
					textToInsert:   "different data",
					shouldBeMarked: false,
				},
			},
			expected: "different data",
		},
	}

	doTests(t, tests)
}

func TestPieceTableDeletes(t *testing.T) {
	tests := []test{
		{
			name:    "empty",
			initial: "",
			ops: []testOp{{
				opcode:         delet,
				index:          0,
				lengthToDelete: 20,
			}},
			expected: "",
		},
		{
			name:    "delete one char front",
			initial: "abc",
			ops: []testOp{{
				opcode:         delet,
				index:          0,
				lengthToDelete: 1,
			}},
			expected: "bc",
		},
		{
			name:    "delete one char back",
			initial: "abc",
			ops: []testOp{{
				opcode:         delet,
				index:          2,
				lengthToDelete: 1,
			}},
			expected: "ab",
		},
		{
			name:    "delete one char mid",
			initial: "abc",
			ops: []testOp{{
				opcode:         delet,
				index:          1,
				lengthToDelete: 1,
			}},
			expected: "ac",
		},
		{
			name:    "delete all",
			initial: "abc",
			ops: []testOp{{
				opcode:         delet,
				index:          0,
				lengthToDelete: 3,
			}},
			expected: "",
		},
		{
			name:    "delete words mid",
			initial: "this is a test",
			ops: []testOp{{
				opcode:         delet,
				index:          5,
				lengthToDelete: 5,
			}},
			expected: "this test",
		},
		{
			name:    "insert then delete",
			initial: "this test",
			ops: []testOp{
				{
					opcode:       insert,
					index:        5,
					textToInsert: "is a ",
				},
				{
					opcode:         delet,
					index:          5,
					lengthToDelete: 5,
				},
			},
			expected: "this test",
		},
		{
			name:    "insert then delete overlap",
			initial: "this test",
			ops: []testOp{
				{
					opcode:       insert,
					index:        5,
					textToInsert: "is a ", // this is a test
				},
				{
					opcode:         delet,
					index:          2,
					lengthToDelete: 6,
				},
			},
			expected: "tha test",
		},
		{
			name:    "insert, delete, insert",
			initial: "this test",
			ops: []testOp{
				{
					opcode:       insert,
					index:        5,
					textToInsert: "is a ", // this is a test
				},
				{
					opcode:         delet,
					index:          2,
					lengthToDelete: 6,
				},
				{
					opcode:       insert,
					index:        3,
					textToInsert: "r be a",
				},
			},
			expected: "thar be a test",
		},
		{
			name:    "insert, delete what was inserted, insert",
			initial: "this test",
			ops: []testOp{
				{
					opcode:       insert,
					index:        5,
					textToInsert: "is a ", // this is a test
				},
				{
					opcode:         delet,
					index:          5,
					lengthToDelete: 5,
				},
				{
					opcode:       insert,
					index:        5,
					textToInsert: "is a ",
				},
			},
			expected: "this is a test",
		},
		{
			name:    "delete after unicode",
			initial: "☺bcdef",
			ops: []testOp{
				{opcode: delet, index: 3, lengthToDelete: 3},
			},
			expected: "☺bc",
		},
		{
			name:    "truncate last insert",
			initial: "abcdef",
			ops: []testOp{
				{opcode: insert, index: 3, textToInsert: "xxx"},
				{opcode: truncate, lengthToDelete: 2},
			},
			expected: "abcxdef",
		},
		{
			name:    "truncate last insert with unicode",
			initial: "abcdef",
			ops: []testOp{
				{opcode: insert, index: 3, textToInsert: "x☺x"},
				{opcode: truncate, lengthToDelete: 2},
			},
			expected: "abcxdef",
		},
		{
			name:    "delete entire piece",
			initial: "abcdef",
			ops: []testOp{
				{opcode: insert, index: 3, textToInsert: "xxx"}, //abcxxxdef
				{opcode: delet, index: 3, lengthToDelete: 3},
			},
			expected:           "abcdef",
			expectedPieceCount: 2, // After the insert we should have 3 pieces, then deleting should leave 2
		},
		{
			name:    "delete start of piece",
			initial: "abcdef",
			ops: []testOp{
				{opcode: insert, index: 3, textToInsert: "xxx"}, //abcxxxdef
				{opcode: delet, index: 3, lengthToDelete: 2},
			},
			expected:           "abcxdef",
			expectedPieceCount: 3, // After the insert we should have 3 pieces, then deleting should still have 3, not 4
		},
		{
			name:    "delete end of piece",
			initial: "abcdef",
			ops: []testOp{
				{opcode: insert, index: 3, textToInsert: "xxx"}, //abcxxxdef
				{opcode: delet, index: 4, lengthToDelete: 2},
			},
			expected:           "abcxdef",
			expectedPieceCount: 3, // After the insert we should have 3 pieces, then deleting should still have 3, not 4
		},
		{
			name:    "delete 1 char insert",
			initial: "abcdef",
			ops: []testOp{
				{opcode: insert, index: 3, textToInsert: "\n"}, //abcxxxdef
				{opcode: delet, index: 4, lengthToDelete: 1},
			},
			expected: "abc\nef",
			debug:    false,
		},
	}
	doTests(t, tests)

}

func TestPieceTableUndo(t *testing.T) {
	tests := []test{
		{
			name:    "empty",
			initial: "",
			ops: []testOp{{
				opcode: undo,
			}},
			expected: "",
		},
		{
			name:    "undo insert",
			initial: "",
			ops: []testOp{
				{
					opcode:       insert,
					index:        0,
					textToInsert: "abc",
					undoData:     2,
				},
				{
					opcode:   undo,
					undoData: 2,
				},
			},
			expected: "",
		},
		{
			name:    "undo insert nonempty",
			initial: "great lakes swimmers",
			ops: []testOp{
				{
					opcode:       insert,
					index:        6,
					textToInsert: "pulling on a line ",
				},
				{
					opcode: undo,
				},
			},
			expected: "great lakes swimmers",
		},
		{
			name:    "insert, undo, redo",
			initial: "great lakes swimmers",
			ops: []testOp{
				{
					opcode:       insert,
					index:        6,
					textToInsert: "your rocky spine ",
				},
				{
					opcode: undo,
				},
				{
					opcode: redo,
				},
			},
			expected: "great your rocky spine lakes swimmers",
			debug:    true,
		},
		{
			name:    "undo, redo when nothing left",
			initial: "great lakes swimmers",
			ops: []testOp{
				{
					opcode:       insert,
					index:        6,
					textToInsert: "your rocky spine ",
				},
				{opcode: undo},
				{opcode: undo},
				{opcode: undo},
				{opcode: redo},
				{opcode: redo},
				{opcode: redo},
			},
			expected: "great your rocky spine lakes swimmers",
		},
		{
			name:    "insert, insert, undo, redo data test",
			initial: "whitehorse",
			ops: []testOp{
				{
					opcode:       insert,
					index:        0,
					textToInsert: "sometimes amy",
					undoData:     5,
				},
				{
					opcode:       insert,
					index:        0,
					textToInsert: "am i just gonna stand here",
					undoData:     6,
				},
				{
					opcode:   undo,
					undoData: 6,
				},
				{
					opcode:   undo,
					undoData: 5,
				},
				{
					opcode:   redo,
					undoData: 6,
				},
			},
			expected: "sometimes amywhitehorse",
		},
		{
			name:        "marking test",
			initial:     "snowcrash",
			markInitial: true,
			debug:       false,
			ops: []testOp{
				{
					opcode:         insert,
					index:          0,
					textToInsert:   "read ",
					shouldBeMarked: false,
				},
				{
					opcode:         undo,
					shouldBeMarked: true,
				},
				{
					opcode:         redo,
					shouldBeMarked: false,
				},
				{
					opcode:         undo,
					shouldBeMarked: true,
				},
				{
					opcode:         insert,
					index:          0,
					textToInsert:   "sell ",
					shouldBeMarked: false,
				},
				{
					opcode:         mark,
					shouldBeMarked: true,
				},
				{
					opcode:         undo,
					shouldBeMarked: false,
				},
				{
					opcode:         redo,
					shouldBeMarked: true,
				},
			},
			expected: "sell snowcrash",
		},
		{
			name:        "set then undo test",
			initial:     "A day in the life",
			markInitial: true,
			debug:       false,
			ops: []testOp{
				{
					opcode:         setWithUndo,
					index:          0,
					textToInsert:   "different data",
					shouldBeMarked: false,
				},
				{
					opcode:         undo,
					shouldBeMarked: true,
				},
			},
			expected: "A day in the life",
		},
		{
			name:        "merge single undo",
			initial:     "A day in the life",
			markInitial: true,
			debug:       false,
			ops: []testOp{
				{
					opcode:       insert,
					index:        0,
					textToInsert: "The Beatles: ",
				},
				// "The Beatles: A day in the life"
				{
					opcode: disableUndoTracking,
				},
				{opcode: delet,
					index:          12,
					lengthToDelete: 2,
				},
				// "The Beatles: day in the life"
				{
					opcode: enableUndoTracking,
				},
				{
					opcode: undo,
				},
				// "The Beatles: day in the life"
			},
			expected: "The Beatles: A day in the life",
		},
	}

	checkUndoData := func(tc *test) {
		for i, op := range tc.ops {
			if op.opcode == undo && op.undoData != 0 {
				if op.undoData != op.returnedUndoData {
					t.Fatalf("Expected undo data to be %d but it is %d. Object is %p, orig is %p",
						op.undoData, op.returnedUndoData, &op, &tc.ops[i])
				}
			}
			if op.shouldBeMarked != op.returnedMarked {
				t.Fatalf("Expected undo data marked property to be %v but it is %v. Object is %p, orig is %p",
					op.shouldBeMarked, op.returnedMarked, &op, &tc.ops[i])
			}
		}

	}

	doTests(t, tests, checkUndoData)

}

func TestPieceTableUndoDisable(t *testing.T) {
	tests := []test{
		{
			name:    "skip undo",
			initial: "test sentence",
			debug:   false,
			ops: []testOp{
				{
					opcode:       insert,
					index:        5,
					textToInsert: "this ",
				},
				// "test this sentence"
				{
					opcode: disableUndoTracking,
				},
				{
					opcode:       insert,
					index:        5,
					textToInsert: "into ",
				},
				// "test into this sentence"
				{
					opcode: enableUndoTracking,
				},
				{
					opcode:       insert,
					index:        5,
					textToInsert: "thing ",
				},
				// "test thing into this sentence"
				{
					opcode: undo,
				},
				// "test into this sentence"
			},
			expected: "test into this sentence",
		},
		{
			name:    "skip multiple inserts",
			initial: "test sentence",
			debug:   false,
			ops: []testOp{
				{
					opcode:       insert,
					index:        5,
					textToInsert: "this ",
				},
				// "test this sentence"
				{
					opcode: disableUndoTracking,
				},
				{
					opcode:       insert,
					index:        5,
					textToInsert: "well ",
				},
				// "test well this sentence"
				{
					opcode:       insert,
					index:        5,
					textToInsert: "really ",
				},
				// "test really well this sentence"
				{
					opcode: enableUndoTracking,
				},
				{
					opcode:       insert,
					index:        5,
					textToInsert: "it ",
				},
				// "test it really well this sentence"
				{
					opcode: undo,
				},
				// "test really well this sentence"
				{
					opcode: undo,
				},
				// "test this sentence"
				// Here the undos for the inserts of "well " and "really " are merged into one undo, so they are undone atomically.
			},
			expected: "test this sentence",
		},
		{
			name:    "skip multiple deletes and inserts",
			initial: "test sentence",
			debug:   false,
			ops: []testOp{
				{
					opcode:       insert,
					index:        5,
					textToInsert: "this ",
				},
				// "test this sentence"
				{
					opcode: disableUndoTracking,
				},
				{
					opcode:         delet,
					index:          5,
					lengthToDelete: 5,
				},
				// "test sentence"
				{
					opcode:         delet,
					index:          0,
					lengthToDelete: 4,
				},
				// "sentence"
				{
					opcode:       insert,
					index:        0,
					textToInsert: "A ",
				},
				// "A sentence"
				{
					opcode: enableUndoTracking,
				},
				{
					opcode:       insert,
					index:        1,
					textToInsert: "good ",
				},
				// "A good sentence"
				{
					opcode: undo,
				},
				// "A sentence"
				{
					opcode: undo,
				},
				// "test this sentence"
				// Here the undos for the deletes of "this " and "test " and the insert of "good " are merged into one undo, so they are undone atomically.
			},
			expected: "test this sentence",
		},
		{
			name:    "skip undo and redo",
			initial: "test sentence",
			debug:   false,
			ops: []testOp{
				{
					opcode:       insert,
					index:        5,
					textToInsert: "this ",
				},
				// "test this sentence"
				{
					opcode: disableUndoTracking,
				},
				{
					opcode:       insert,
					index:        5,
					textToInsert: "into ",
				},
				// "test into this sentence"
				{
					opcode: enableUndoTracking,
				},
				{
					opcode:       insert,
					index:        5,
					textToInsert: "thing ",
				},
				// "test thing into this sentence"
				{
					opcode: undo,
				},
				// "test this sentence" (we skip the "into" insert)
				{
					opcode: redo,
				},
				// "test thing into this sentence"
			},
			expected: "test thing into this sentence",
		},
		{
			name:    "disable then delete, insert",
			initial: "test sentence",
			debug:   false,
			ops: []testOp{
				{
					opcode:       insert,
					index:        5,
					textToInsert: "this ",
				},
				// "test this sentence"
				{
					opcode: disableUndoTracking,
				},
				{
					opcode:         delet,
					index:          5,
					lengthToDelete: 4,
				},
				// "test sentence"
				{
					opcode:       insert,
					index:        5,
					textToInsert: "my ",
				},
				// "test my sentence"
				{
					opcode: enableUndoTracking,
				},
				{
					opcode: undo,
				},
				// "test this sentence" (we skip the "into" insert)
			},
			expected: "test this sentence",
		},
	}

	doTests(t, tests)

}

type test struct {
	name               string
	initial            string
	markInitial        bool
	ops                []testOp
	expected           string
	expectedPieceCount int
	debug              bool
}

var debugTests = false

func doTests(t *testing.T, tests []test, extraChecks ...func(*test)) {
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pt := NewPieceTable([]byte(tc.initial))
			if tc.markInitial {
				pt.Mark()
			}
			if tc.debug {
				fmt.Printf("------------\n")
				fmt.Printf("At start of test %s piece table is %s\n", tc.name, pt.DebugString())
			}
			for i, op := range tc.ops {
				tc.ops[i].apply(pt)
				if tc.debug {
					fmt.Printf("------------\n")
					fmt.Printf("After applying op %d (%s) piece table is %s\n", i, op.Name(), pt.DebugString())
				}
			}

			text := pt.String()
			if text != tc.expected {
				t.Fatalf("Expected ‘%s’ but got ‘%s’ (‘%+q’ vs ‘%+q’)", tc.expected, text, tc.expected, text)
			}

			l := utf8.RuneCountInString(tc.expected)
			if l != pt.Len() {
				t.Fatalf("Expected piece table length to be %d but got %d", l, pt.Len())
			}

			if tc.expectedPieceCount != 0 && pt.pieces.Len() != tc.expectedPieceCount {
				s := pt.DebugString()
				t.Fatalf("Expected %d pieces after modifications but got %d. Piece table: %s",
					tc.expectedPieceCount, pt.pieces.Len(), s)

			}

			for _, fn := range extraChecks {
				fn(&tc)
			}

		})
	}

}

type opcode int

const (
	insert opcode = iota
	delet
	undo
	redo
	truncate
	mark
	disableUndoTracking
	enableUndoTracking
	setWithUndo
)

type testOp struct {
	opcode           opcode
	index            int
	textToInsert     string
	lengthToDelete   int
	undoData         int
	returnedUndoData int
	shouldBeMarked   bool
	returnedMarked   bool
}

func (o *testOp) apply(pt *PieceTable) {
	switch o.opcode {
	case insert:
		if o.undoData != 0 {
			pt.InsertWithUndoData(o.index, o.textToInsert, o.undoData)
		} else {
			pt.Insert(o.index, o.textToInsert)
		}
	case delet:
		pt.Delete(o.index, o.lengthToDelete)
	case undo:
		u := pt.Undo()
		if len(u) > 0 {
			o.returnedUndoData, _ = u[0].(int)
		}
	case redo:
		r := pt.Redo()
		if len(r) > 0 {
			o.returnedUndoData, _ = r[0].(int)
		}
	case truncate:
		pt.TruncateLastInsert(o.lengthToDelete)
	case mark:
		pt.Mark()
	case disableUndoTracking:
		pt.StartTransaction()
	case enableUndoTracking:
		pt.EndTransaction()
	case setWithUndo:
		pt.SetWithUndo([]byte(o.textToInsert))
	}
	o.returnedMarked = pt.marked
}

func (o testOp) Name() string {
	switch o.opcode {
	case insert:
		return "insert"
	case delet:
		return "delet"
	case undo:
		return "undo"
	case redo:
		return "redo"
	case truncate:
		return "truncate"
	case mark:
		return "mark"
	case disableUndoTracking:
		return "disableUndoTracking"
	case enableUndoTracking:
		return "enableUndoTracking"
	case setWithUndo:
		return "setWithUndo"
	}
	return "unknown"
}
