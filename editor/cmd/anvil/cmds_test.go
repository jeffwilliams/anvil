package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCommandExecutorSplitCommandOnSemicolons(t *testing.T) {

	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty",
			input:    "",
			expected: []string{},
		},
		{
			name:     "semi",
			input:    ";",
			expected: []string{},
		},
		{
			name:     "dbl-semi",
			input:    ";;",
			expected: []string{},
		},
		{
			name:     "semi2",
			input:    ";Dump;",
			expected: []string{"Dump"},
		},
		{
			name:     "os cmd",
			input:    "ls",
			expected: []string{"ls"},
		},
		{
			name:     "Newcol",
			input:    "Newcol",
			expected: []string{"Newcol"},
		},
		{
			name:     "  Newcol",
			input:    "  Newcol",
			expected: []string{"Newcol"},
		},
		{
			name:     "Newcol;Dump",
			input:    "Newcol;Dump",
			expected: []string{"Newcol", "Dump"},
		},
		{
			name:     "many",
			input:    "MyAlias;Dump;|sort;>wc;<date;!x/a/",
			expected: []string{"MyAlias", "Dump", "|sort", ">wc", "<date", "!x/a/"},
		},
		{
			name:     "many with args",
			input:    "MyAlias 1; Dump 2; |sort 3; >wc 4; <date 5; !x/a aa/",
			expected: []string{"MyAlias 1", "Dump 2", "|sort 3", ">wc 4", "<date 5", "!x/a aa/"},
		},
		{
			name:     "long os cmd",
			input:    "ls -la; echo yes; wc -la | grep",
			expected: []string{"ls -la; echo yes; wc -la | grep"},
		},
		{
			name:     "os, non-os, os",
			input:    "ls -la; Load; ls",
			expected: []string{"ls -la", "Load", "ls"},
		},
		{
			name:     "non-os, os, non-os",
			input:    "Load; ls -la; Load",
			expected: []string{"Load", "ls -la", "Load"},
		},
		{
			name:     "os, non-os, os embedded semicolons",
			input:    "ls -la; echo hi; Load; ls; echo hi",
			expected: []string{"ls -la; echo hi", "Load", "ls; echo hi"},
		},
		{
			name:     "expr and sleep",
			input:    `!x/\s\.[^\s]+/;sleep 1;Tint yellow; !0,0`,
			expected: []string{`!x/\s\.[^\s]+/`, "sleep 1", "Tint yellow", "!0,0"},
		},
		{
			name:     "expr with semi",
			input:    `!x/;/;Tint yellow`,
			expected: []string{`!x/;/`, "Tint yellow"},
		},
		{
			name:     "echo with semi",
			input:    `echo ';'`,
			expected: []string{`echo ';'`},
		},
		{
			name:     "complex scp",
			input:    `bash -c "scp debug-dhcp.sh debug-dhcp-pause.sh jefwill3-sj:/tmp"; On jefwill3-sj:/tmp chmod +x debug-dhcp.sh; On jefwill3-sj:/tmp chmod +x debug-dhcp-pause.sh;`,
			expected: []string{`bash -c "scp debug-dhcp.sh debug-dhcp-pause.sh jefwill3-sj:/tmp"`, "On jefwill3-sj:/tmp chmod +x debug-dhcp.sh", "On jefwill3-sj:/tmp chmod +x debug-dhcp-pause.sh"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			executor := NewCommandExecutor(nil)
			settings.Alias["MyAlias"] = "blah"

			actual := executor.splitCommandOnSemicolons(tc.input)

			assert.Equal(t, tc.expected, actual)
		})
	}
}

func TestCommandExecutorSplitStringOnSemicolons(t *testing.T) {

	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty",
			input:    "",
			expected: []string{""},
		},
		{
			name:     "semi",
			input:    ";",
			expected: []string{"", ""},
		},
		{
			name:     "dbl-semi",
			input:    ";;",
			expected: []string{"", "", ""},
		},
		{
			name:     "semi2",
			input:    ";Dump;",
			expected: []string{"", "Dump", ""},
		},
		{
			name:     "os cmd",
			input:    "ls",
			expected: []string{"ls"},
		},
		{
			name:     "Newcol",
			input:    "Newcol",
			expected: []string{"Newcol"},
		},
		{
			name:     "Newcol;Dump",
			input:    "Newcol;Dump",
			expected: []string{"Newcol", "Dump"},
		},
		{
			name:     "single quote",
			input:    "echo ';'",
			expected: []string{"echo ';'"},
		},
		{
			name:     "single quote 2",
			input:    "echo ';'; Load",
			expected: []string{"echo ';'", " Load"},
		},
		{
			name:     "double quote",
			input:    `echo ";"`,
			expected: []string{`echo ";"`},
		},
		{
			name:     "double quote escaped",
			input:    `echo "\";"`,
			expected: []string{`echo "\";"`},
		},
		{
			name:     "double quote not escaped",
			input:    `echo "\\";blah`,
			expected: []string{`echo "\\"`, "blah"},
		},
		{
			name:     "double quote escaped extra bslash",
			input:    `echo "\\\";"`,
			expected: []string{`echo "\\\";"`},
		},
		{
			name:     "double quote not escaped extra bslash",
			input:    `echo "\\\\";blah`,
			expected: []string{`echo "\\\\"`, "blah"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			executor := NewCommandExecutor(nil)
			actual := executor.splitStringOnSemicolons(tc.input)
			assert.Equal(t, tc.expected, actual)
		})
	}
}
