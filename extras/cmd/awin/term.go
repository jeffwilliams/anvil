package main

import (
	"bytes"
	"fmt"
)

// The types and functions in this file are used to recognize Escape Sequences, of which Control Sequences
// are a subset. Both begin with Escape, but Control Sequences specifically begin with the Control Sequence
// Identifier (CSI): Escape followed by [.
//
// See ECMA-35 and ECMA-48 for reference.
// See ECMA-35 section 13 Structure and use of escape sequences
// and https://www.gnu.org/software/teseq/manual/html_node/Escape-Sequence-Recognition.html
// See https://www.xfree86.org/current/ctlseqs.html
// See https://invisible-island.net/xterm/ctlseqs/ctlseqs.html
// See https://www.gnu.org/software/teseq/manual/html_node/Overview.html#Overview
// See https://learn.microsoft.com/en-us/windows/console/console-virtual-terminal-sequences
//
// Note: we handle OSC and support BEL in addition to ST as terminator. From xterm ctrlseqs.html:
//
//          XTerm accepts either BEL  or ST  for terminating OSC
//          sequences, and when returning information, uses the same
//          terminator used in a query.  While the latter is preferred,
//          the former is supported for legacy applications:
//          o   Although documented in the changes for X.V10R4 (December
//              1986), BEL  as a string terminator dates from X11R4
//              (December 1989).
//          o   Since XFree86-3.1.2Ee (August 1996), xterm has accepted ST
//              (the documented string terminator in ECMA-48).
//
//
// See this quote from https://unix.stackexchange.com/questions/208436/bell-and-escape-character-in-prompt-string:
//
// > Long ago, the developer(s) of xterm added an escape sequence for setting the title. In X11R1 (1987), the program simply read the sequence until it got a nonprinting character. Later, in X11R4 (1989), someone improved this by terminating on a BEL character. The standard had been around longer than that, but the reason for choosing BEL rather than ST is not known. Ultimately that was addressed in the late 1990s, by recognizing either (but keeping BEL as an alternative since many users relied on hardcoded behavior with BEL).
//
//
// cmd.exe in windows also seems to use BEL as ST:
//
//awin: output from process before cleaning as string: 'eserved.[4;1Hc:\temp>]0;C:\WINDOWS\SYSTEM32\cmd.exe[?25h'
//awin: output from process before cleaning as hexdump:
//00000000  65 73 65 72 76 65 64 2e  1b 5b 34 3b 31 48 63 3a  |eserved..[4;1Hc:|
//00000010  5c 74 65 6d 70 3e 1b 5d  30 3b 43 3a 5c 57 49 4e  |\temp>.]0;C:\WIN|
//00000020  44 4f 57 53 5c 53 59 53  54 45 4d 33 32 5c 63 6d  |DOWS\SYSTEM32\cm|
//00000030  64 2e 65 78 65 07 1b 5b  3f 32 35 68              |d.exe..[?25h|
// Note the ESC]0 which I think is terminated by 0x07 (BEL)

const (
	Esc           = 033
	CsiSecondByte = '['
	ApcSecondByte = '_'
	DcsSecondByte = 'P'
	OscSecondByte = ']'
	PmSecondByte  = '^'
	SosSecondByte = 'X'
	StSecondByte  = '\n'
	Bel           = '\a'
)

func isEscapeSequenceFinalByte(b byte) bool {
	// From ECMA-35:
	//
	// Final bytes shall be any of the 79 positions of columns 03 to 07 of the code table excluding position
	// 07/15
	return b >= 0x30 && b <= 0x7F
}

func isControlSequenceFinalByte(b byte) bool {
	// From ECMA-48:
	//
	// "F is the Final Byte; it consists of a bit combination from 04/00 to 07/14; it terminates the control
	// sequence and together with the Intermediate Bytes, if present, identifies the control function. Bit
	// combinations 07/00 to 07/14 are available as Final Bytes of control sequences for private (or
	// experimental) use."
	//
	// The notation XX/YY in decimal represents a byte with high nibble having value XX and low YY.
	return b >= 0x40 && b < 0x7E
}

func isControlSequenceParamByte(b byte) bool {
	// From ECMA-48:
	// The format of a control sequence is
	// CSI P ... P I ... I F
	// where
	// a) CSI is represented by bit combinations 01/11 (representing ESC) and 05/11 in a 7-bit code or by bit
	// combination 09/11 in an 8-bit code, see 5.3;
	// b) P ... P are Parameter Bytes, which, if present, consist of bit combinations from 03/00 to 03/15;
	// c) I ... I are Intermediate Bytes, which, if present, consist of bit combinations from 02/00 to 02/15.
	// Together with the Final Byte F, they identify the control function
	return b >= 0x30 && b < 0x3f
}

func isControlSequenceIntermediateByte(b byte) bool {
	return b >= 0x20 && b < 0x2f
}

func isEscapeSequenceIntermediateByte(b byte) bool {
	return isControlSequenceIntermediateByte(b)
}

func isControlStringSecondByte(b byte) bool {
	switch b {
	case ApcSecondByte, DcsSecondByte, OscSecondByte, PmSecondByte, SosSecondByte:
		return true
	}
	return false
}

func isControlStringIntermediateByte(b byte) bool {
	return (b >= 0x8 && b <= 0xd) || (b >= 0x20 && b <= 0x7e)
}

type EscapeSequenceType int

const (
	SeqTypePlain EscapeSequenceType = iota
	SeqTypeControlSequence
	SeqTypeControlString
)

// EscapeSequenceParser parses escape sequences, as defined in ECMA-48, and with some XTerm "extensions".
// Escape sequences have three subcategories: control sequences, which start with CSI; control strings, which
// start with OSC or a few other initiators; or plain escape sequences.
type EscapeSequenceParser struct {
	lastByte    byte
	inEscapeSeq bool
	input       []byte
	// escapeSeq is the current control sequence being built.
	escapeSeq []byte
	// seqType is the type of the current escape sequence
	seqType EscapeSequenceType
	// Output text. re-used to prevent allocations
	text []byte
	// seqStart is the start of the current sequence of text or
	// the control sequence
	seqStart  int
	index     int
	sequences []EscapeSequence
}

func NewEscapeSequenceParser() EscapeSequenceParser {
	return EscapeSequenceParser{
		text:      make([]byte, 0, 1000),
		escapeSeq: make([]byte, 0, 1000),
	}
}

func (parser *EscapeSequenceParser) state() string {
	var buf bytes.Buffer

	fmt.Fprintf(&buf, "{\n")
	fmt.Fprintf(&buf, "  input: '%s'\n", string(parser.input))
	fmt.Fprintf(&buf, "  index: %d\n", parser.index)
	fmt.Fprintf(&buf, "  escapeSeq: '%s'\n", string(parser.escapeSeq))
	fmt.Fprintf(&buf, "  text: '%s'\n", string(parser.text))
	fmt.Fprintf(&buf, "  inEscapeSeq: %v\n", parser.inEscapeSeq)
	fmt.Fprintf(&buf, "  seqStart: %d\n", parser.seqStart)
	fmt.Fprintf(&buf, "}\n")

	return buf.String()
}

// Input passes more data to the parser to parse. It returns the text that is found in
// `data` with any control sequences found in it removed in `text`. It also returns
// any escape sequences that have been found in `text` in `sequences`.
//
// The Index field of the returned sequences indicates the position in `text` where the
// control sequence takes effect: Index i takes effect just before byte i in the text. A
// control sequence may have been started in a previous call to Input and is finished in
// the text of this call.
//
// The return values `text` and `sequences` are overwritten by each call to Input.
//
// The returned escape sequences may also be control sequences.
func (parser *EscapeSequenceParser) Input(data []byte) (text []byte, sequences []EscapeSequence) {
	if len(data) == 0 {
		return data, nil
	}

	parser.index = 0
	parser.input = data
	parser.text = parser.text[:0]
	parser.sequences = parser.sequences[:0]
	parser.seqStart = 0
	parser.lastByte = 0

	//fmt.Printf("escparser: Input: called for '%s'. state:\n %s\n", string(data), parser.state())

	for i, b := range data {
		parser.index = i
		parser.processByte(b)
		parser.lastByte = b
	}

	if !parser.inEscapeSeq {
		parser.appendTextSeq()
	} else {
		parser.appendSeqTo(&parser.escapeSeq, len(parser.input))
	}

	if len(parser.sequences) > 0 {
		sequences = parser.sequences
	}
	text = parser.text
	return
}

func (parser *EscapeSequenceParser) processByte(b byte) {
	/*
		r := rune(b)
		if unicode.IsPrint(r) {
			fmt.Printf("escparser: got '%c'", b)
		} else {
			fmt.Printf("escparser: got 0x%x", b)
		}
		fmt.Printf(" state: \n%s\n", parser.state())
	*/

	if parser.inEscapeSeq {
		parser.processByteInEscapeSeq(b)
	} else {
		parser.processByteOutsideEscapeSeq(b)
	}
}

func (parser *EscapeSequenceParser) processByteInEscapeSeq(b byte) {
	if parser.index == parser.seqStart {
		// The second byte of the sequence determines its type
		if b == '[' {
			parser.seqType = SeqTypeControlSequence
			return
		} else if isControlStringSecondByte(b) {
			parser.seqType = SeqTypeControlString
			return
		}
	}

	if parser.isEscSeqFinalByte(b) {
		parser.appendSeqTo(&parser.escapeSeq, parser.index)
		parser.buildEscapeSeq(b)
		parser.escapeSeq = parser.escapeSeq[:0]
		parser.seqStart = parser.index + 1
		parser.inEscapeSeq = false
		parser.seqType = SeqTypePlain
	}
}

func (parser *EscapeSequenceParser) isEscSeqFinalByte(b byte) bool {
	switch parser.seqType {
	case SeqTypeControlSequence:
		return isControlSequenceFinalByte(b)
	case SeqTypeControlString:
		return parser.isControlStringFinalByte(b)
	default:
		return isEscapeSequenceFinalByte(b)
	}
}

func (parser *EscapeSequenceParser) isControlStringFinalByte(b byte) bool {
	// A control string should end in ST, but because of an xterm bug it can
	// also be ended with BEL. From https://invisible-island.net/xterm/ctlseqs/ctlseqs.html:
	//
	//          XTerm accepts either BEL  or ST  for terminating OSC
	//          sequences, and when returning information, uses the same
	//          terminator used in a query.  While the latter is preferred,
	//          the former is supported for legacy applications:
	//          o   Although documented in the changes for X.V10R4 (December
	//              1986), BEL  as a string terminator dates from X11R4
	//              (December 1989).
	//          o   Since XFree86-3.1.2Ee (August 1996), xterm has accepted ST
	//              (the documented string terminator in ECMA-48).
	//
	// From https://unix.stackexchange.com/questions/208436/bell-and-escape-character-in-prompt-string:
	//
	// > Long ago, the developer(s) of xterm added an escape sequence for setting the title. In X11R1 (1987), the
	// > program simply read the sequence until it got a nonprinting character. Later, in X11R4 (1989), someone
	// > improved this by terminating on a BEL character. The standard had been around longer than that, but the
	// > reason for choosing BEL rather than ST is not known. Ultimately that was addressed in the late 1990s, by
	// recognizing either (but keeping BEL as an alternative since many users relied on hardcoded behavior with BEL).

	return (parser.lastByte == Esc && b == StSecondByte) || b == Bel
}

func (parser *EscapeSequenceParser) processByteOutsideEscapeSeq(b byte) {
	if b == Esc {
		parser.appendSeqTo(&parser.text, parser.index)
		parser.inEscapeSeq = true
		parser.seqStart = parser.index + 1
	}
}

func (parser *EscapeSequenceParser) appendTextSeq() {
	if parser.lastByte == Esc {
		parser.text = append(parser.text, Esc)
	}
	parser.appendSeqTo(&parser.text, parser.index+1)
}

func (parser *EscapeSequenceParser) appendSeqTo(dst *[]byte, end int) {
	*dst = append(*dst, parser.input[parser.seqStart:end]...)
}

func (parser *EscapeSequenceParser) buildEscapeSeq(finalByte byte) {
	// From ECMA-35
	//
	// > An escape sequence shall consist of two or more bytes. In an 8-bit code a byte shall be an 8-bit combination. In a
	// > 7-bit code a byte shall be a 7-bit combination.
	// > The first byte of an escape sequence shall be the bit combination representing the ESCAPE character and the last
	// > shall be known as the Final Byte. An escape sequence may also contain one or more bytes known as Intermediate
	// > bytes.
	// > The function represented by an escape sequence shall be determined by its Intermediate byte(s), if any, and by its
	// > Final Byte.
	// > Intermediate bytes shall be any of the 16 positions of column 02 of the code table; they are denoted by the
	// > symbol I.
	// > Final bytes shall be any of the 79 positions of columns 03 to 07 of the code table excluding position 07/15; they are
	// > denoted by the symbol F

	// From ECMA-48:
	// > The format of a control sequence is
	// > CSI P ... P I ... I F
	// > where
	// > a) CSI is represented by bit combinations 01/11 (representing ESC) and 05/11 in a 7-bit code or by bit
	// > combination 09/11 in an 8-bit code, see 5.3;
	// > b) P ... P are Parameter Bytes, which, if present, consist of bit combinations from 03/00 to 03/15;
	// > c) I ... I are Intermediate Bytes, which, if present, consist of bit combinations from 02/00 to 02/15.
	// > Together with the Final Byte F, they identify the control function

	// Here we are a little forgiving. After CSI we ignore bytes that are not in the parameter byte range until
	// we detect one in that range, then accept the consequtive sequence of bytes in that range as parameter bytes
	// until we find one outside that range. Then we do the same for intermediate bytes. Put another way, we
	// actually accept this format, where X is an invalid byte:
	//
	// CSI X ... X P ... P X ... X I ... I X ... X F

	c := EscapeSequence{
		Index:     len(parser.text),
		FinalByte: finalByte,
		Type:      parser.seqType,
	}

	//if len(parser.escapeSeq) > 0 && parser.escapeSeq[0] == '[' {
	if parser.seqType == SeqTypeControlSequence || parser.seqType == SeqTypeControlString {
		parser.escapeSeq = parser.escapeSeq[1:]
	}

	copyMatchingBytesToEscapeSeq := func(startIndex int, predicate func(byte) bool, dst *[]byte) (newIndex int) {
		i := startIndex
		start := -1

		copyRest := func() {
			if start >= 0 {
				*dst = make([]byte, i-start)
				copy(*dst, parser.escapeSeq[start:i])
			}
		}

		for ; i < len(parser.escapeSeq); i++ {
			b := parser.escapeSeq[i]
			if predicate(b) {
				if start < 0 {
					start = i
				}
			} else {
				if start >= 0 {
					copyRest()
					break
				}
			}
		}
		copyRest()
		return i
	}

	switch parser.seqType {
	case SeqTypeControlSequence:
		i := 0
		i = copyMatchingBytesToEscapeSeq(i, isControlSequenceParamByte, &c.ParameterBytes)
		copyMatchingBytesToEscapeSeq(i, isControlSequenceIntermediateByte, &c.IntermediateBytes)
	case SeqTypeControlString:
		copyMatchingBytesToEscapeSeq(0, isControlStringIntermediateByte, &c.IntermediateBytes)
	default:
		copyMatchingBytesToEscapeSeq(0, isEscapeSequenceIntermediateByte, &c.IntermediateBytes)
	}

	parser.sequences = append(parser.sequences, c)
}

type EscapeSequence struct {
	// Index is the index in the corresponding slice of text where the control sequence was found
	Index int
	Type  EscapeSequenceType
	// FinalByte is the final byte of the escape sequence, which usually determines what its function
	// is. For control strings, which can end in the two byte "string terminator", this will either be
	// set to '\n' (if the ending was the string terminator) or '\a' (if the ending was the BEL character)
	FinalByte byte
	// ParameterBytes are the parameter bytes of a control sequence. This is only set if the Type is
	// SeqTypeControlSequence. These are the bytes that follow the CSI and are before the intermediate
	// bytes---if they are set---and if not, the final byte.
	ParameterBytes []byte
	// IntermediateBytes are the intermediate bytes of an escape sequence. These are the bytes after
	// the opening of the escape sequence, and before the final byte. For control sequences this
	// does not include the parameter bytes.
	IntermediateBytes []byte
}
