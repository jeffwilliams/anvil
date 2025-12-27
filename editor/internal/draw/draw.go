package draw

import (
	"bytes"
	"fmt"
	"strconv"
	"unicode"
)

type OpCode int

const (
	Begin = iota
	Move
	Line
	Close
	Color
)

type Op struct {
	Code OpCode
	Args [4]interface{}
}

func (op *Op) ResolveConsts(consts map[Const]float32) {
	for i := range op.Args {
		if c, ok := op.Args[i].(Const); ok {
			f, ok := consts[c]
			if !ok {
				f = float32(0)
			}
			op.Args[i] = f
		}
	}
}

type Const int

const (
	ConstLineHeight Const = iota
	ConstNegLineHeight
)

// "color(255,255,255,255);begin;mv(-3,0);ln(7,0);ln(0,3);line(0,lh); close"
func Parse(instructions string) ([]Op, error) {
	var s scanner
	return s.scan(instructions)
}

type scannerState int

const (
	stateFuncName scannerState = iota
	stateInParen
	stateAfterParen
)

type scanner struct {
	input  string
	pos    int
	state  scannerState
	tok    bytes.Buffer
	argNum int
	curOp  Op
	ops    []Op
}

func (s *scanner) scan(input string) (ops []Op, err error) {
	s.input = input
	s.pos = 0
	for !s.atEnd() {
		c := s.input[s.pos]

		if unicode.IsSpace(rune(c)) {
			s.pos++
			continue
		}

		switch s.state {
		case stateFuncName:
			err = s.handleStateFuncName(c)
		case stateInParen:
			err = s.handleStateInParen(c)
		case stateAfterParen:
			err = s.handleStateAfterParen(c)
		}

		if err != nil {
			return
		}
		s.pos++
	}

	switch s.state {
	case stateFuncName:
		if s.tok.Len() > 0 {
			s.setOpcode()
			s.outputOp()
		}
	case stateAfterParen:
		s.outputOp()
	}

	ops = s.ops
	return
}

func (s *scanner) handleStateFuncName(c byte) error {
	var err error
	switch c {
	case '(':
		if s.tok.Len() > 0 {
			err = s.setOpcode()
		}
		s.changeToStateInParen()
	case ';':
		if s.tok.Len() > 0 {
			err = s.setOpcode()
			s.outputOp()
		}
		s.clearTok()
	default:
		s.tok.WriteByte(c)
	}
	return err
}

func (s *scanner) setOpcode() error {
	var err error
	s.curOp.Code, err = s.opcodeOfString(s.tok.String())
	return err
}

func (s *scanner) changeToStateInParen() {
	s.argNum = 0
	s.state = stateInParen
	s.clearTok()
}

func (s *scanner) clearTok() {
	s.tok.Reset()
}

func (s *scanner) clearOp() {
	clear(s.curOp.Args[:])
}

func (s *scanner) outputOp() {
	s.ops = append(s.ops, s.curOp)
	s.clearOp()
}

func (s *scanner) handleStateInParen(c byte) error {
	switch c {
	case ')':
		s.state = stateAfterParen
		fallthrough
	case ',':
		err := s.fillOpArg(s.tok.String())
		if err != nil {
			return err
		}
		s.argNum++
		s.clearTok()
	default:
		s.tok.WriteByte(c)
	}
	return nil
}

func (s *scanner) fillOpArg(val string) error {
	if s.argNum >= len(s.curOp.Args) {
		return nil
	}

	switch val {
	case "lh":
		s.curOp.Args[s.argNum] = ConstLineHeight
		return nil
	case "-lh": // Negative line height
		s.curOp.Args[s.argNum] = ConstNegLineHeight
		return nil
	}

	f, err := strconv.ParseFloat(val, 32)
	if err != nil {
		return s.Errorf("invalid constant or floating point number '%s': %v", s.tok.String(), err)
	}
	s.curOp.Args[s.argNum] = float32(f)

	return nil
}

func (s *scanner) handleStateAfterParen(c byte) error {
	switch c {
	case ';':
		s.outputOp()
		s.clearTok()
		s.state = stateFuncName
	}
	return nil

}

func (s *scanner) opcodeOfString(str string) (opcode OpCode, err error) {
	switch str {
	case "begin":
		opcode = Begin
	case "move":
		opcode = Move
	case "line":
		opcode = Line
	case "close":
		opcode = Close
	case "color":
		opcode = Color
	default:
		err = s.Errorf("invalid operation '%s'", str)
	}

	return
}

func (s *scanner) Errorf(m string, args ...interface{}) error {
	m = fmt.Sprintf("parse error at offset %d: %s", s.pos+1, m)
	return fmt.Errorf(m, args...)
}

func (s *scanner) atEnd() bool {
	return s.pos >= len(s.input)
}

func opcodeString(opcode OpCode) string {
	switch opcode {
	case Begin:
		return "begin"
	case Move:
		return "move"
	case Line:
		return "line"
	case Close:
		return "close"
	case Color:
		return "color"
	default:
		return "?"
	}
}

func OpsString(ops []Op) string {
	return OpsStringResolve(ops, nil)
}

func OpsStringResolve(ops []Op, consts map[Const]float32) string {
	var buf bytes.Buffer

	for i, op := range ops {
		if consts != nil {
			op.ResolveConsts(consts)
		}
		fmt.Fprintf(&buf, "%d) %s(", i+1, opcodeString(op.Code))
		for i, a := range op.Args {
			if a == nil {
				break
			}
			if i > 0 {
				fmt.Fprintf(&buf, ",")
			}
			fmt.Fprintf(&buf, "%.1f", a)
		}

		fmt.Fprintf(&buf, ")\n")
	}
	return buf.String()

}
