package main

type Initialize struct {
	ProcessId    int                `json:"processId"`
	RootUri      string             `json:"rootUri"`
	Capabilities ClientCapabilities `json:"capabilities"`
}

type ClientCapabilities struct {
	General GeneralClientCapabilities `json:"general"`
	// OffsetEncoding is a clangd-specific extension to LSP. See https://clangd.llvm.org/extensions#utf-8-offsets
	OffsetEncoding []string `json:"offsetEncoding"`
}

type GeneralClientCapabilities struct {
	// PositionEncodings values can be 'utf-8', 'utf-16', or 'utf-32'
	PositionEncodings []string `json:"positionEncodings"`
}

type InitializeResult struct {
	ServerCapabilities ServerCapabilities
	ServerInfo         *ServerInfo
}

type ServerCapabilities struct {
	PositionEncoding string `json:"positionEncoding"`
}

type ServerInfo struct {
	Name    string
	Version string
}

type DidOpenTextDocument struct {
	TextDocument TextDocumentItem `json:"textDocument"`
}

type TextDocumentItem struct {
	Uri        string `json:"uri"`
	LanguageId string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

type TextDocumentPosition struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

type TextDocumentIdentifier struct {
	Uri string `json:"uri"`
}

type DidSaveTextDocument struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

// Position represents a line and character position in a file. The line and character
// positions are 0-based.
type Position struct {
	Line      uint `json:"line"`
	Character uint `json:"character"`
}

type Location struct {
	Uri   string `json:"uri"`
	Range Range  `json:"range"`
}

type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type PublishDiagnosticParams struct {
	Uri         string       `json:"uri"`
	Version     int          `json:"version"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

type Diagnostic struct {
	Range    Range  `json:"range"`
	Severity int    `json:"severity"`
	Code     string `json:"code"`
	Source   string `json:"source"`
	Message  string `json:"message"`
}

/*
type HoverRequest struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}
*/

type HoverResponse struct {
	Contents string `json:"contents"`
}
