package main

import (
	"context"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/alecthomas/chroma"
	"github.com/alecthomas/chroma/lexers"
	"github.com/jeffwilliams/anvil/editor/internal/intvl"
	"github.com/jeffwilliams/syn"
	synlexers "github.com/jeffwilliams/syn/lexers"
)

type SyntaxInterval struct {
	start, end int
	color      Color
}

func NewSyntaxInterval(start, end int, color Color) *SyntaxInterval {
	return &SyntaxInterval{start, end, color}
}

func (s SyntaxInterval) Start() int {
	return s.start
}

func (s SyntaxInterval) End() int {
	return s.end
}

func (s SyntaxInterval) Color() Color {
	return s.color
}

func NewSyntaxHighlighter(style SyntaxStyle) Highlighter {
	/*
		return &chromaHighlighter{
			style: style,
		}*/
	return &synHighlighter{
		style: style,
	}
}

type Highlighter interface {
	Highlight(text string, ctx context.Context) (seq []intvl.Interval, err error)
	SetFilename(filename string)
	SetLanguage(language string)
	SetStyle(style SyntaxStyle)
}

type AnalyzingHighlighter interface {
	SetAnalyse(analyse bool)
}

type StatableHighlighter interface {
	Next() (intvl.Interval, error)
	State() interface{}
	SetState(state interface{})
}

type chromaHighlighter struct {
	style    SyntaxStyle
	filename string
	language string
	analyse  bool
}

var ErrTimeout timeoutError

type timeoutError struct{}

func (e timeoutError) Error() string {
	return "Highlighting timed out"
}

var ErrCancel cancelError

type cancelError struct{}

func (e cancelError) Error() string {
	return "Highlighting canceled"
}

func (s chromaHighlighter) Highlight(text string, ctx context.Context) (seq []intvl.Interval, err error) {

	deadline, deadlineDefined := ctx.Deadline()

	lexer := s.lexer(text)

	if lexer == nil {
		// Return empty sequence
		return
	}

	lexer = chroma.Coalesce(lexer)

	iter, err := lexer.Tokenise(nil, text)
	if err != nil {
		return
	}

	runeIndex := 0

LOOP:
	for {
		tok := iter()

		if tok == chroma.EOF {
			break
		}

		if deadlineDefined && time.Now().After(deadline) {
			err = ErrTimeout
			log(LogCatgSyntax, "chromaHighlighter.Highlight: exiting due to deadline\n")
			break LOOP
		}

		select {
		case <-ctx.Done():
			err = ErrCancel
			log(LogCatgSyntax, "chromaHighlighter.Highlight: exiting due to cancellation\n")
			break LOOP
		default:
		}

		var color *Color

		//log(LogCatgSyntax,"SyntaxHighlighter.Highlight: token category %s, subcat %s, '%s'\n", tok.Type.Category(), tok.Type.SubCategory(), tok)

		switch tok.Type.Category() {
		case chroma.Keyword:
			color = &s.style.KeywordColor
		case chroma.Name:
			color = &s.style.NameColor
		case chroma.Literal:
			switch tok.Type.SubCategory() {
			case chroma.LiteralString:
				color = &s.style.StringColor
			case chroma.LiteralNumber:
				color = &s.style.NumberColor
			}
		case chroma.Operator:
			color = &s.style.OperatorColor
		case chroma.Comment:
			color = &s.style.CommentColor
			if tok.Type.SubCategory() == chroma.CommentPreproc {
				color = &s.style.PreprocessorColor
			}
		case chroma.Generic:
			switch tok.Type {
			case chroma.GenericHeading:
				color = &s.style.HeadingColor
			case chroma.GenericSubheading:
				color = &s.style.SubheadingColor
			case chroma.GenericInserted:
				color = &s.style.InsertedColor
			case chroma.GenericDeleted:
				color = &s.style.DeletedColor
			}
		}

		tokLen := utf8.RuneCountInString(tok.Value)
		if color == nil {
			// Just normal text
			runeIndex += tokLen
			continue
		}

		si := &SyntaxInterval{
			start: runeIndex,
			end:   runeIndex + tokLen,
			color: *color,
		}
		seq = append(seq, si)

		runeIndex += tokLen
	}
	return
}

func (s chromaHighlighter) lexer(text string) chroma.Lexer {
	if s.language == "" && s.filename == "" {
		if !s.analyse {
			return nil
		}
		return lexers.Analyse(text)
	}

	var lexer chroma.Lexer
	if s.language != "" {
		lexer = lexers.Get(s.language)
	}

	if lexer == nil && s.filename != "" {
		lexer = lexers.Match(s.filename)
	}

	return lexer
}

func (s *chromaHighlighter) SetFilename(filename string) {
	s.filename = filename
}
func (s *chromaHighlighter) SetLanguage(language string) {
	s.language = language
}

func (s *chromaHighlighter) SetAnalyse(analyse bool) {
	s.analyse = analyse
}

func (s *chromaHighlighter) SetStyle(style SyntaxStyle) {
	s.style = style
}

type synHighlighter struct {
	style    SyntaxStyle
	filename string
	language string
}

func (s synHighlighter) Highlight(text string, ctx context.Context) (seq []intvl.Interval, err error) {
	deadline, deadlineDefined := ctx.Deadline()

	started := time.Now()
	log(LogCatgSyntax, "synHighlighter.Highlight: called\n")
	lexer := s.lexer(text)

	if lexer == nil {
		log(LogCatgSyntax, "synHighlighter.Highlight: no lexer found\n")
		// Return empty sequence
		return
	}

	runes := []rune(text)

	iter := lexer.Tokenise(runes)

LOOP:
	for {
		var tok syn.Token
		tok, err = iter.Next()
		if err != nil {
			return
		}

		if tok.Type == syn.EOFType {
			break
		}

		if deadlineDefined && time.Now().After(deadline) {
			log(LogCatgSyntax, "synHighlighter.Highlight: exiting due to deadline\n")
			err = ErrTimeout
			break LOOP
		}

		select {
		case <-ctx.Done():
			log(LogCatgSyntax, "synHighlighter.Highlight: exiting due to cancellation\n")
			err = ErrCancel
			break LOOP
		default:
		}

		var color *Color

		//log(LogCatgSyntax, "SyntaxHighlighter.Highlight: token category %s, subcat %s, '%s'\n", tok.Type.Category(), tok.Type.SubCategory(), tok)

		switch tok.Type.Category() {
		case syn.Keyword:
			color = &s.style.KeywordColor
		case syn.Name:
			if tok.Type == syn.NameTag {
				// Specifically for HTML use the Keyword color so tags are highlighted with the default syntax color scheme
				color = &s.style.KeywordColor
			} else {
				color = &s.style.NameColor
			}
		case syn.Literal:
			switch tok.Type.SubCategory() {
			case syn.LiteralString:
				color = &s.style.StringColor
			case syn.LiteralNumber:
				color = &s.style.NumberColor
			}
		case syn.Operator:
			color = &s.style.OperatorColor
		case syn.Comment:
			color = &s.style.CommentColor
			if tok.Type.SubCategory() == syn.CommentPreproc {
				color = &s.style.PreprocessorColor
			}
		case syn.Generic:
			switch tok.Type {
			case syn.GenericHeading:
				color = &s.style.HeadingColor
			case syn.GenericSubheading:
				color = &s.style.SubheadingColor
			case syn.GenericInserted:
				color = &s.style.InsertedColor
			case syn.GenericDeleted:
				color = &s.style.DeletedColor
			}
		}

		if color == nil {
			// Just normal text
			continue
		}

		si := &SyntaxInterval{
			start: tok.Start,
			end:   tok.End,
			color: *color,
		}
		seq = append(seq, si)
	}

	log(LogCatgSyntax, "synHighlighter.Highlight: done (took %s)\n", time.Now().Sub(started))
	return
}

func (s synHighlighter) lexer(text string) *syn.Lexer {
	if s.language == "" && s.filename == "" {
		return nil
	}

	var lexer *syn.Lexer
	if s.language != "" {
		lexer = synlexers.Get(s.language)
	}

	if lexer == nil && s.filename != "" {
		lexer = synlexers.Match(s.filename)
	}

	return lexer
}

func (s *synHighlighter) SetFilename(filename string) {
	s.filename = filename
}

func (s *synHighlighter) SetLanguage(language string) {
	s.language = language
}

func (s *synHighlighter) SetStyle(style SyntaxStyle) {
	s.style = style
}

type AsyncHighlighter struct {
	timeout time.Duration
	done    func(seq []intvl.Interval, err error)
	cancel  func()
	h       Highlighter
}

func NewAsyncHighlighter(h Highlighter, timeout time.Duration, done func(seq []intvl.Interval, err error)) *AsyncHighlighter {
	return &AsyncHighlighter{
		timeout: timeout,
		h:       h,
		done:    done,
	}
}

// Highlight tries to highlight the text, but if it takes longer than blockingLimit then
// it Highlight returns with the error ErrTimeout and continues in the background. If
// it is continued in the background, and when it's finished the function `done` is called
// with the result. Done is called from a separate goroutine
func (ah *AsyncHighlighter) Highlight(text string) (seq []intvl.Interval, err error) {

	// stop any background job by closing c
	ah.Cancel()

	// try and highlight
	ctx := context.Background()
	ctx, cancel := context.WithDeadline(ctx, time.Now().Add(ah.timeout))
	defer cancel()

	seq, err = ah.h.Highlight(text, ctx)
	if err != nil && err == ErrTimeout {
		log(LogCatgSyntax, "AsyncHighlighter.Highlight: starting background highlighter\n")
		// if too long, send to background goroutine
		ctx := context.Background()
		ctx, ah.cancel = context.WithCancel(ctx)
		go ah.highlightInBackground(text, ctx)
		return
	}

	return
}

func (ah *AsyncHighlighter) Cancel() {
	if ah.cancel != nil {
		log(LogCatgSyntax, "AsyncHighlighter.Highlight: cancelling background highlighter\n")
		ah.cancel()
		ah.cancel = nil
	}
}

func (ah AsyncHighlighter) highlightInBackground(text string, ctx context.Context) (seq []intvl.Interval, err error) {
	seq, err = ah.h.Highlight(text, ctx)
	if err != nil {
		return
	}
	ah.done(seq, err)
	return
}

/*func init() {
	syn.DebugLogger = log.New(os.Stdout, "", 0)
}*/

func printSyntaxLexerParseErrors() {
	if len(synlexers.GlobalLexerLoadErrors) > 0 {
		fmt.Printf("Failed to load some of the lexers used for syntax highlighting, and so they won't be available:\n")
		for _, e := range synlexers.GlobalLexerLoadErrors {
			fmt.Printf("  * %v\n", e)
		}
	}
}
