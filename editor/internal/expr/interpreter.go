package expr

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jeffwilliams/anvil/editor/internal/regex"
	"github.com/jeffwilliams/anvil/editor/internal/runes"
)

var RuneOffsetCacheInterval int = 8000

type Interpreter struct {
	tree            interface{}
	pipeline        []stage
	data            []byte
	runeOffsetCache runes.OffsetCache
	handler         Handler
	dot             int
}

func NewInterpreter(data []byte, parseTree interface{}, handler Handler, dot int) (Interpreter, error) {
	in := Interpreter{data: data, runeOffsetCache: runes.NewOffsetCache(RuneOffsetCacheInterval), tree: parseTree, handler: handler, dot: dot}
	err := in.buildPipeline()
	return in, err
}

func (in *Interpreter) buildPipeline() error {
	expr, ok := in.tree.(expr)
	if !ok {
		return fmt.Errorf("tree root is not an expr")
	}

	var err error
	in.pipeline, err = buildStagesFromTerms(expr.terms, in.dot)
	if err != nil {
		return err
	}

	for _, cmd := range expr.commands {
		stage := newCommandStage(cmd, in.handler)
		in.pipeline = append(in.pipeline, stage)
	}

	if len(expr.commands) == 0 {
		in.pipeline = append(in.pipeline, newNoopCommandStage(in.handler))
	}

	return nil
}

func buildStagesFromTerms(terms []interface{}, dot int) (stages []stage, err error) {
	for _, term := range terms {
		var stage stage
		stage, err = buildStageFromTerm(term, dot)
		if err != nil {
			return
		}
		stages = append(stages, stage)
	}
	return
}

func buildStageFromTerm(term interface{}, dot int) (stage stage, err error) {
	switch t := term.(type) {
	case simpleAddr:
		stage = newAddrStage(t, dot)
	case complexAddr:
		stage = newAddrStage(t, dot)
	case group:
		stage, err = newGroupStage(t, dot)
	case operation:
		stage, err = newOperationStage(t)
	}
	return
}

func (in *Interpreter) Execute(ranges []Range) error {
	dbg("Interpreter.Execute: called with ranges %s", rangesToString(ranges))

	clonedRanges := make([]Range, len(ranges))
	for i, x := range ranges {
		clonedRanges[i] = &irange{x.Start(), x.End()}
	}

	return in.execute(clonedRanges)
}

func (in *Interpreter) execute(ranges []Range) error {
	// Sort the ranges
	sort.Slice(ranges, func(i, j int) bool {
		return ranges[i].Start() < ranges[j].Start()
	})

	dbg("Interpreter.Execute: sorted ranges: %s", rangesToString(ranges))

	var err error
	for i, stage := range in.pipeline {
		var err2 error
		ranges, err2 = stage.Execute(&in.data, &in.runeOffsetCache, ranges)
		dbg("Interpreter.Execute: after executing stage %d (%T) ranges are: %s", i, stage, rangesToString(ranges))
		if err2 != nil {
			err = err2
		}
	}

	in.handler.Done()
	return err
}

type Range interface {
	Start() int
	End() int
}

type stage interface {
	// A stage takes a set of ranges as input, and returns a new modified
	// set of ranges.
	Execute(data *[]byte, runeOffsetCache *runes.OffsetCache, ranges []Range) ([]Range, error)
}

type addrStage struct {
	addrTree        interface{}
	data            []byte
	dot             int
	runeOffsetCache *runes.OffsetCache
}

func newAddrStage(addrTree interface{}, dot int) addrStage {
	return addrStage{addrTree: addrTree, dot: dot}
}

func (s addrStage) Execute(data *[]byte, runeOffsetCache *runes.OffsetCache, ranges []Range) ([]Range, error) {
	s.data = *data
	s.runeOffsetCache = runeOffsetCache
	result := []Range{}

	for _, r := range ranges {
		o := s.execute(r)
		if isEmptyRange(o) {
			continue
		}
		result = append(result, o)
	}
	return result, nil
}

func isEmptyRange(r Range) bool {
	return r.Start() >= r.End()
}

func (s *addrStage) execute(rang Range) Range {
	return s.executeAddr(s.addrTree, rang)
}

func (s *addrStage) executeAddr(addr interface{}, r Range) Range {
	if len(s.data) == 0 {
		return &irange{}
	}

	switch t := addr.(type) {
	case simpleAddr:
		return s.executeSimpleAddr(t, r)
	case complexAddr:
		return s.executeComplexAddr(t, r)
	}
	return &irange{}
}

func (s *addrStage) executeSimpleAddr(addr simpleAddr, r Range) (result Range) {
	dbg("executing simple addr (%s) on range %s", addr.typ, rangeToString(r))
	defer func() {
		dbg("  simple addr result: %s", rangeToString(result))
	}()
	switch addr.typ {
	case lineAddrType:
		return s.executeLineAddr(addr, r)
	case charAddrType:
		return s.executeCharAddr(addr, r)
	case endAddrType:
		return &irange{r.End() - 1, r.End()}
	case forwardRegexAddrType:
		return s.executeRegexpAddr(addr, r, forwardDir)
	case backwardRegexAddrType:
		return s.executeRegexpAddr(addr, r, backwardDir)
	case dotAddrType:
		return s.executeDotAddr(addr, r)
	}
	return &irange{}
}

func (s *addrStage) executeLineAddr(addr simpleAddr, r Range) Range {
	val := addr.val - 1
	if val < 0 {
		// Select the length 0 range just before the passed range
		return &irange{r.Start(), r.Start()}
	}
	walker := runes.NewWalker(s.data)
	walker.SetRunePosCache(r.Start(), s.runeOffsetCache)
	eof := walker.ForwardLines(val)
	if eof {
		// Out of range
		return &irange{}
	}
	start := walker.RunePos()
	// We want to include the \n as part of the line
	walker.ForwardToEndOfLine()
	walker.Forward(1)
	end := walker.RunePos()

	end = min(end, r.End())
	return &irange{start, end}

}

func (s *addrStage) executeCharAddr(addr simpleAddr, r Range) Range {
	val := addr.val - 1
	if val < 0 {
		// Select the length 0 range just before the passed range
		return &irange{r.Start(), r.Start()}
	}

	if r.Start()+val > r.End() {
		// Out of range
		return &irange{}
	}

	walker := runes.NewWalker(s.data)
	walker.SetRunePosCache(r.Start(), s.runeOffsetCache)
	walker.Forward(val)
	start := walker.RunePos()
	return &irange{start, start + 1}

}

func (s *addrStage) executeRegexpAddr(addr simpleAddr, r Range, dir readDirection) Range {
	reText := addr.regex
	if addr.rev {
		var err error
		reText, err = regex.ReverseRegex(addr.regex)
		if err != nil {
			return &irange{}
		}
	}

	re, err := CompileRegexpWithMultiline(reText)
	if err != nil {
		return &irange{}
	}

	walker := runes.NewWalker(s.data)
	data := walker.TextBetweenRuneIndicesCache(r.Start(), r.End(), s.runeOffsetCache)

	var match []int
	if dir == forwardDir {
		match = re.FindIndex(data)
	} else {
		all := re.FindAllIndex(data, -1)
		if all == nil {
			return &irange{}
		}
		match = all[len(all)-1]
	}

	if match == nil {
		return &irange{}
	}

	convertRegexMatchIndicesToRuneRanges(s.data, s.runeOffsetCache, r.Start(), [][]int{match})
	return &irange{match[0], match[1]}
}

func (s *addrStage) executeDotAddr(addr simpleAddr, r Range) Range {
	dbg("executing dot addr with dot=%d on range %s", s.dot, rangeToString(r))
	if s.dot < r.Start() || s.dot > r.End() {
		return r
	}

	return &irange{s.dot, s.dot + 1}
}

func (s *addrStage) executeComplexAddr(addr complexAddr, r Range) (result Range) {
	dbg("executing complex addr on range %s", rangeToString(r))

	defer func() {
		dbg("  complex addr result: %s", rangeToString(result))
	}()

	reverse := func(buf []byte) {
		for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
			buf[i], buf[j] = buf[j], buf[i]
		}
	}

	switch addr.op {
	case ',':
		left := s.executeAddr(addr.l, r)
		right := s.executeAddr(addr.r, r)
		return &irange{left.Start(), right.End()}
	case '+':
		left := s.executeAddr(addr.l, r)
		r2 := &irange{start: left.End(), end: r.End()}
		return s.executeAddr(addr.r, r2)
	case '-':
		left := s.executeAddr(addr.l, r)

		// Reverse the data we will execute on, then execute the right address
		li := r.Start()
		ri := left.Start()

		if li >= len(s.data) || ri > len(s.data) || ri < li {
			return &irange{}
		}

		rev := s.data[li:ri]
		reverse(rev)

		// If we are going to run a regex, reverse the regex
		addr.reverse(true)

		right := s.executeAddr(addr.r, irange{li, ri})
		// Since we ran in reverse the match is actually from the end of the text
		right = &irange{ri - (right.End() - li), ri - (right.Start() - li)}

		// Undo our reversal
		reverse(rev)
		return &irange{right.Start(), right.End()}
	case ';':
		left := s.executeAddr(addr.l, r)
		r2 := &irange{start: left.End(), end: r.End()}
		right := s.executeAddr(addr.r, r2)
		return &irange{left.Start(), right.End()}
	}
	return &irange{}
}

type operationStage struct {
	op              operation
	re              *regexp.Regexp
	data            []byte
	runeOffsetCache *runes.OffsetCache
}

func newOperationStage(op operation) (stage operationStage, err error) {
	var re *regexp.Regexp
	re, err = CompileRegexpWithMultiline(op.regex)
	if err != nil {
		return
	}

	stage = operationStage{
		op: op,
		re: re,
	}

	return
}

func (s operationStage) Execute(data *[]byte, runeOffsetCache *runes.OffsetCache, ranges []Range) ([]Range, error) {
	s.data = *data
	s.runeOffsetCache = runeOffsetCache
	result := []Range{}
	for _, r := range ranges {
		rs := s.execute(r)
		result = append(result, rs...)
	}
	return result, nil
}

func appendNonEmpty(list, toAppend []Range) []Range {
	for _, r := range toAppend {
		if isEmptyRange(r) {
			continue
		}
		list = append(list, r)
	}
	return list
}

func (s *operationStage) execute(r Range) []Range {
	//return s.executeAddr(s.addrTree, rang)
	switch s.op.op {
	case 'x':
		return s.executeX(r)
	case 'y':
		return s.executeY(r)
	case 'z':
		return s.executeZ(r)
	case 'g':
		return s.executeG(r)
	case 'v':
		return s.executeV(r)
	case 'n':
		return s.executeN(r)
	case 'l':
		return s.executeL(r)
	}
	return []Range{}
}

func (s *operationStage) executeX(rg Range) []Range {
	return s.executeXOrYOrZ(rg, 'x')
}

func (s *operationStage) executeL(rg Range) []Range {
	return s.executeXOrYOrZ(rg, 'x')
}

func (s *operationStage) executeY(rg Range) []Range {
	return s.executeXOrYOrZ(rg, 'y')
}

func (s *operationStage) executeZ(rg Range) []Range {
	return s.executeXOrYOrZ(rg, 'z')
}

func (s *operationStage) executeXOrYOrZ(rg Range, op rune) []Range {
	walker := runes.NewWalker(s.data)
	data := walker.TextBetweenRuneIndicesCache(rg.Start(), rg.End(), s.runeOffsetCache)

	indices := s.re.FindAllIndex(data, -1)

	convertRegexMatchIndicesToRuneRanges(s.data, s.runeOffsetCache, rg.Start(), indices)

	ranges := make([]Range, len(indices))
	for i := range indices {
		ranges[i] = &irange{indices[i][0], indices[i][1]}
	}

	if op == 'y' {
		ranges = invertRanges(ranges, rg.Start(), rg.End())
	} else if op == 'z' {
		ranges = expandRangesToStartOfNextRange(ranges, rg.Start(), rg.End())
	}

	return ranges
}

// regexMatchIndicesToRuneRanges converts a series of start,end match indexes returned by a regex match
// (which are in units of bytes) to ranges which are in units of runes.
// data is the input data the ranges are in, and start is the index of the start of the portion of data the
// regex ranges apply to. The regex matches ranges are passed in the parameter match.
//
// The indexes in the match are updated in place, so they are the rune values on return.
func convertRegexMatchIndicesToRuneRanges(data []byte, runeOffsetCache *runes.OffsetCache, start int, match [][]int) {
	walker := runes.NewWalker(data)
	walker.SetRunePosCache(start, runeOffsetCache)

	startByte := walker.BytePos()
	startRune := walker.RunePos()
	subdata := data[startByte:]

	// Each pair in the match slice must have values that are higher or equal to the
	// previous pair, and each pair's second element must be higher or equal to the first element.
	// Thus, we can cache the result of our last RuneCount and add to it for a faster result.
	lastRuneCountIndex := -1
	lastRuneCountResult := -1

	for i := range match {
		for j := range match[i] {
			mo := match[i][j]
			// Preserve -1 since they mean no match
			if mo < 0 {
				continue
			}

			var c int
			if lastRuneCountIndex < 0 || mo < lastRuneCountIndex {
				if mo < lastRuneCountIndex {
					dbg("convertRegexMatchIndicesToRuneRanges: the next index in the match was less than the previous (%d vs %d)", mo, lastRuneCountIndex)
				}
				c = utf8.RuneCount(subdata[:mo])
			} else {
				c = lastRuneCountResult + utf8.RuneCount(subdata[lastRuneCountIndex:mo])
			}
			lastRuneCountIndex = mo
			lastRuneCountResult = c

			match[i][j] = c + startRune
		}
	}
}

func (s *operationStage) executeG(rg Range) []Range {
	return s.executeGOrV(rg, 'g')
}

func (s *operationStage) executeV(rg Range) []Range {
	return s.executeGOrV(rg, 'v')
}

func (s *operationStage) executeGOrV(rg Range, op rune) []Range {
	walker := runes.NewWalker(s.data)
	data := walker.TextBetweenRuneIndicesCache(rg.Start(), rg.End(), s.runeOffsetCache)

	keepRange := s.re.Match(data)
	if op == 'v' {
		keepRange = !keepRange
	}

	if keepRange {
		return []Range{rg}
	}

	return []Range{}
}

func (s *operationStage) executeN(rg Range) []Range {
	// TODO: Don't allow special regex characters in the terms. Escape them.

	openers := strings.Join(strings.Split(s.op.args[0], ","), "|")
	closers := strings.Join(strings.Split(s.op.args[1], ","), "|")

	both := fmt.Sprintf("(%s)|(%s)", openers, closers)

	isOpenerMatch := func(indices []int) bool {
		// The `both` regexp when run with FindSubmatchIndex will return the start and end
		// of the while match as the first two values, then the start and end of the opener as the
		// next two if the opener matched. If it did not, these will be -1
		return indices[2] >= 0
	}

	openerRe, err := regexp.Compile(openers)
	if err != nil {
		return []Range{}
	}

	bothRe, err := regexp.Compile(both)
	if err != nil {
		return []Range{}
	}

	walker := runes.NewWalker(s.data)
	data := walker.TextBetweenRuneIndicesCache(rg.Start(), rg.End(), s.runeOffsetCache)

	// Find first match
	indices := openerRe.FindIndex(data)
	if indices == nil {
		return []Range{}
	}

	moveForwardTo := indices[1]
	convertRegexMatchIndicesToRuneRanges(data, s.runeOffsetCache, rg.Start(), [][]int{indices})
	data = data[moveForwardTo:]
	start := indices[0]
	end := indices[1]

	unclosedOpenerCount := 1

	// Continue until the nesting is closed
	for unclosedOpenerCount > 0 {
		//fmt.Printf("operationStage.executeN: looking in '%s'\n", data)
		indices = bothRe.FindSubmatchIndex(data)

		if indices == nil {
			return []Range{}
		}

		moveForwardTo := indices[1]
		convertRegexMatchIndicesToRuneRanges(data, s.runeOffsetCache, rg.Start(), [][]int{indices})
		//fmt.Printf("operationStage.executeN:match at %d to %d, indices: %#v\n", indices[0], indices[1], indices)
		data = data[moveForwardTo:]
		end += indices[1]

		if isOpenerMatch(indices) {
			//fmt.Printf("operationStage.executeN: was opener\n")
			unclosedOpenerCount++
		} else {
			//fmt.Printf("operationStage.executeN: was closer\n")
			unclosedOpenerCount--
		}
	}

	return []Range{&irange{start, end}}

}

type commandStage struct {
	cmd             command
	handler         Handler
	runeOffsetCache *runes.OffsetCache
}

func newCommandStage(cmd command, handler Handler) (stage commandStage) {
	stage = commandStage{
		cmd:     cmd,
		handler: handler,
	}

	return
}

func (s commandStage) Execute(data *[]byte, runeOffsetCache *runes.OffsetCache, ranges []Range) ([]Range, error) {
	var err error
	s.runeOffsetCache = runeOffsetCache
	switch s.cmd.op {
	case 'd':
		err = s.executeDelete(ranges)
	case 'p':
		s.executePrint(ranges, false)
	case 'P':
		s.executePrint(ranges, true)
	case '=':
		s.executeDisplay(ranges)
	case 'i':
		err = s.executeInsert(ranges)
	case 'a':
		s.executeAppend(ranges)
	case 'c':
		s.executeReplace(ranges)
	case 's':
		s.executeSubst(*data, ranges)
	case 'C':
		s.executeCopy(ranges)
	}

	//s.handler.Perform(s.cmd.op, ranges)
	return ranges, err
}

func (s commandStage) executeDelete(ranges []Range) error {
	offset := 0
	for i, r := range ranges {
		mod := irange{start: r.Start() + offset, end: r.End() + offset}
		dbg("executing delete on range %s", rangeToString(mod))
		s.handler.Delete(mod)

		l := r.End() - r.Start()
		ir, ok := r.(*irange)
		if !ok {
			return fmt.Errorf("internal/expr/commandStage.executeDelete: the Range is not an *irange but is a %T", ir)
		}
		ir.end = ir.start

		s.shiftLaterRanges(ranges, i+1, -l)
	}
	return nil
}

func (s commandStage) executeCopy(ranges []Range) {
	for _, r := range ranges {
		s.handler.Copy(r)
	}
}

func (s commandStage) executePrint(ranges []Range, displayPosition bool) {
	prefix := ""
	for _, r := range ranges {
		s.handler.DisplayContents(r, prefix, displayPosition)
		prefix = s.cmd.args[0]
	}
}

func (s commandStage) executeDisplay(ranges []Range) {
	for _, r := range ranges {
		s.handler.Display(r)
	}
}

func (s commandStage) executeInsert(ranges []Range) error {
	var err error
	for i, r := range ranges {
		b := []byte(s.cmd.args[0])
		s.handler.Insert(r.Start(), b)
		l := utf8.RuneCount(b)
		err2 := s.shiftLaterRanges(ranges, i, l)
		if err2 != nil {
			err = err2
		}
	}
	return err
}

func (s commandStage) shiftLaterRanges(ranges []Range, firstIndex, amt int) error {
	for j := firstIndex; j < len(ranges); j++ {
		ir, ok := ranges[j].(*irange)
		if !ok {
			return fmt.Errorf("internal/expr/commandStage.executeInsert: the Range %d is not an *irange but is a %T\n", j, ranges[j])
		}
		ir.start += amt
		ir.end += amt
	}
	return nil
}

func rangesToString(ranges []Range) string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "[")
	for _, r := range ranges {
		buf.WriteString(rangeToString(r))
		//fmt.Fprintf(&buf, "[%d,%d) ", r.Start(), r.End())
	}
	fmt.Fprintf(&buf, "]")
	return buf.String()
}

func rangeToString(r Range) string {
	return fmt.Sprintf("[%d,%d) ", r.Start(), r.End())
}

func (s commandStage) executeAppend(ranges []Range) {
	for i, r := range ranges {
		b := []byte(s.cmd.args[0])
		s.handler.Insert(r.End(), b)
		l := utf8.RuneCount(b)
		s.shiftLaterRanges(ranges, i+1, l)
	}
}

func (s commandStage) executeReplace(ranges []Range) {
	offset := 0
	for _, r := range ranges {
		offset = s.replace(r.Start(), r.End(), offset, []byte(s.cmd.args[0]))
	}
}

// replace replaces the text between start+offset and end+offset with the value `value`. It updates
// offset to account for the additional bytes added by the replacement so that offset points to the same
// character in the modified text as it did in the original text, and returns that value in newOffset.
func (s commandStage) replace(start, end, offset int, value []byte) (newOffset int) {
	mod := irange{start: start + offset, end: end + offset}
	s.handler.Delete(mod)
	deleteLen := end - start
	s.handler.Insert(start+offset, value)
	insertLen := utf8.RuneCount(value)
	newOffset = offset + (insertLen - deleteLen)
	return
}

func (s commandStage) executeSubst(data []byte, ranges []Range) {
	offset := 0
	for _, r := range ranges {
		offset = s.subst(data, r, offset)
	}
}

func (s commandStage) subst(data []byte, r Range, offset int) (newOffset int) {
	newOffset = offset

	re, err := CompileRegexpWithMultiline(s.cmd.args[0])
	if err != nil {
		return
	}

	rangeData := data[r.Start():r.End()]
	indices := re.FindAllSubmatchIndex(rangeData, -1)

	if indices == nil {
		return
	}

	submatches := make([][]int, len(indices))
	for i, match := range indices {
		submatches[i] = make([]int, len(match)-2)
		copy(submatches[i], match[2:])
	}

	convertRegexMatchIndicesToRuneRanges(data, s.runeOffsetCache, r.Start(), indices)

	for i, match := range indices {
		replacement := s.buildSubstReplacementFromSubmatches(data, rangeData, submatches[i])
		offset = s.replace(match[0], match[1], offset, []byte(replacement))
	}
	newOffset = offset
	return
}

type groupStage struct {
	stages []stage
}

func newGroupStage(groupTree interface{}, dot int) (stg groupStage, err error) {
	grp, ok := groupTree.(group)
	if !ok {
		err = fmt.Errorf("Group stage was passed something that is not a group")
		return
	}

	var stages []stage
	stages, err = buildStagesFromTerms(grp.terms, dot)

	if err != nil {
		return
	}

	stg = groupStage{
		stages: stages,
	}
	return
}

func (s groupStage) Execute(data *[]byte, runeOffsetCache *runes.OffsetCache, ranges []Range) (result []Range, err error) {
	for i, stage := range s.stages {
		/*fmt.Printf("groupStage.Execute: executing on ranges [")
		for _, r := range ranges {
			fmt.Printf("[%d,%d] ", r.Start(), r.End())
		}
		fmt.Printf("]\n")
		*/

		newRanges, err2 := stage.Execute(data, runeOffsetCache, ranges)
		dbg("Interpreter.Execute: after executing stage %d (%T) ranges are: %s", i, stage, rangesToString(ranges))
		if err2 != nil {
			err = err2
		}

		//fmt.Printf("groupStage.Execute: stage %d resulted in %v\n", i, newRanges)
		result = append(result, newRanges...)
	}

	result = unionOfRanges(result)

	return
}

func (s commandStage) buildSubstReplacementFromSubmatches(data []byte, rangeData []byte, submatches []int) string {
	if len(submatches) == 0 {
		return s.cmd.args[1]
	}

	var replacement bytes.Buffer
	var num bytes.Buffer

	const (
		modeNormal = iota
		modeEscape
	)

	addFailedSubmatchTextToReplacement := func() {
		replacement.WriteRune('\\')
		replacement.Write(num.Bytes())
	}

	addSubmatchToReplacement := func(num int) {
		num -= 1
		startIndex := num * 2
		if startIndex < 0 || startIndex > len(submatches) {
			addFailedSubmatchTextToReplacement()
			return
		}

		s, e := submatches[startIndex], submatches[startIndex+1]
		if e > len(rangeData) {
			return
		}
		replacement.Write(rangeData[s:e])
	}

	mode := modeNormal

	for _, c := range s.cmd.args[1] {
		if mode == modeNormal {
			if c == '\\' {
				mode = modeEscape
				num.Reset()
			} else {
				replacement.WriteRune(c)
			}
		} else {
			if unicode.IsDigit(c) {
				num.WriteRune(c)
				continue
			}

			numVal, err := strconv.Atoi(num.String())
			if err == nil {
				addSubmatchToReplacement(numVal)
			} else {
				addFailedSubmatchTextToReplacement()
			}

			if c != '\\' {
				mode = modeNormal
				replacement.WriteRune(c)
			}

			num.Reset()
		}
	}

	if mode == modeEscape {
		numVal, err := strconv.Atoi(num.String())
		if err == nil {
			addSubmatchToReplacement(numVal)
		} else {
			addFailedSubmatchTextToReplacement()
		}
	}

	return replacement.String()
}

type noopCommandStage struct {
	handler Handler
}

func newNoopCommandStage(handler Handler) (stage noopCommandStage) {
	stage = noopCommandStage{
		handler: handler,
	}

	return
}

func (s noopCommandStage) Execute(data *[]byte, runeOffsetCache *runes.OffsetCache, ranges []Range) ([]Range, error) {
	for _, r := range ranges {
		s.handler.Noop(r)
	}
	return ranges, nil
}

func invertRanges(ranges []Range, start, end int) []Range {
	keep := make([]Range, 0, len(ranges)+1)

	var l int
	for i, r := range ranges {
		if i == 0 {
			if start != r.Start() {
				keep = append(keep, &irange{start, r.Start()})
			}
		} else {
			keep = append(keep, &irange{l, r.Start()})
		}
		l = r.End()
	}
	keep = append(keep, &irange{l, end})
	return keep
}

func expandRangesToStartOfNextRange(ranges []Range, start, end int) []Range {
	keep := make([]Range, 0, len(ranges)+1)

	for i, r := range ranges {
		if i < len(ranges)-1 {
			keep = append(keep, &irange{r.Start(), ranges[i+1].Start()})
		} else {
			keep = append(keep, &irange{r.Start(), end})
		}
	}
	return keep
}

func keepOnlyTheseRanges(data *[]byte, ranges []Range) {
	bytes := *data

	l := 0
	for _, k := range ranges {
		copy(bytes[l:], bytes[k.Start():k.End()])
		l += k.End() - k.Start()
	}
	*data = bytes[:l]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

type irange struct {
	start, end int
}

func (r irange) Start() int {
	return r.start
}

func (r irange) End() int {
	return r.end
}

type readDirection int

const (
	forwardDir readDirection = iota
	backwardDir
)

type directedRuneReader struct {
	dir   readDirection
	data  []byte
	index int
}

func CompileRegexpWithMultiline(expr string) (re *regexp.Regexp, err error) {
	expr = fmt.Sprintf("(?m)%s", expr)
	return regexp.Compile(expr)
}

func unionOfRanges(ranges []Range) (union []Range) {
	if len(ranges) == 0 {
		union = ranges
		return
	}

	sort.Slice(ranges, func(i, j int) bool {
		return ranges[i].Start() < ranges[j].Start()
	})

	extendLastToContain := func(r Range) {
		last := union[len(union)-1]

		ir, ok := last.(*irange)
		if ok {
			ir.end = r.End()
			return
		}
		ir = &irange{last.Start(), r.End()}
		union[len(union)-1] = ir
	}

	for _, r := range ranges {
		if len(union) == 0 {
			union = append(union, r)
			continue
		}

		last := union[len(union)-1]
		if r.Start() >= last.End() {
			union = append(union, r)
			continue
		}

		if r.End() <= last.End() {
			// r is completely contained in `last`
			continue
		}

		extendLastToContain(r)
	}

	return
}
