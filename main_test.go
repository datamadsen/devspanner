package main

import (
	"strings"
	"testing"
)

func TestCollapseCR(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"no carriage return", "hello world", "hello world"},
		{"simple overwrite", "abc\rX", "Xbc"},
		{"progress bar", "10%\r 50%\r100%", "100%"},
		{"shorter overwrite leaves tail", "abcdef\rXY", "XYcdef"},
		{"trailing spaces trimmed", "done       \rok", "okne"},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := collapseCR(tc.in); got != tc.want {
				t.Errorf("collapseCR(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestReadTail(t *testing.T) {
	t.Run("fewer lines than limit returns all", func(t *testing.T) {
		got := readTail("a\nb\nc", 10)
		if got != "a\nb\nc" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("more lines truncates to last N", func(t *testing.T) {
		got := readTail("1\n2\n3\n4\n5", 2)
		if got != "4\n5" {
			t.Errorf("got %q, want \"4\\n5\"", got)
		}
	})

	t.Run("trailing newline ignored", func(t *testing.T) {
		got := readTail("a\nb\n", 10)
		if got != "a\nb" {
			t.Errorf("got %q, want \"a\\nb\"", got)
		}
	})

	t.Run("applies collapseCR per line", func(t *testing.T) {
		got := readTail("first\nprog\r100%", 10)
		if !strings.Contains(got, "100%") || strings.Contains(got, "\r") {
			t.Errorf("got %q, expected CR collapsed", got)
		}
	})
}
