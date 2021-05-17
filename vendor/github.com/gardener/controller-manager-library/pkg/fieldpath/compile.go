/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package fieldpath

import (
	"fmt"
	"strconv"
	"sync"
	"unicode"
	"unicode/utf8"
)

var paths = map[string]Node{}
var lock sync.Mutex

func FieldPath(path string) (Node, error) {
	lock.Lock()
	defer lock.Unlock()

	old := paths[path]
	if old != nil {
		return old, nil
	}
	old, err := Compile(path)
	if err != nil {
		paths[path] = old
	}
	return old, err
}

func MustFieldPath(path string) Node {
	node, err := FieldPath(path)
	if err != nil {
		panic(err)
	}
	return node
}

////////////////////////////////////////////////////////////////////////////////
// Scanner
////////////////////////////////////////////////////////////////////////////////

const EOI rune = 0

type scanner struct {
	bytes   []byte
	index   int
	pos     int
	current rune
}

func NewScanner(path string) *scanner {
	return &scanner{bytes: []byte(path)}
}

func (this *scanner) Current() rune {
	return this.current
}

func (this *scanner) Position() int {
	return this.pos
}

func (this *scanner) Next() rune {
	if this.index >= len(this.bytes) {
		this.current = EOI
		return EOI
	}
	r, size := utf8.DecodeRune(this.bytes[this.index:])
	if r == utf8.RuneError {
		panic("invalid utf8 string")
	}
	this.index += size
	this.current = r
	this.pos++
	return r
}

////////////////////////////////////////////////////////////////////////////////
// Compiler
////////////////////////////////////////////////////////////////////////////////

func Compile(path string) (Node, error) {
	s := NewScanner(path)

	s.Next()
	n, err := parseSequence(nil, s)
	if err != nil {
		return nil, err
	}
	if n == nil {
		return unexpected(s, "expecting '.' or '['")
	}
	if s.Current() != EOI {
		return nil, fmt.Errorf("unexpected trailing input at position %d", s.Position())
	}
	return n, err
}

func parseSequence(ctx Node, s *scanner) (Node, error) {
	var last Node
	var next Node
	var err error

	for {
		n := s.Current()
		switch n {
		case '.':
			next, err = parseField(ctx, s, last)
		case '[':
			next, err = parseEntry(ctx, s, last)
		default:
			return last, nil
		}

		if err != nil {
			return nil, err
		}
		last = next
	}
	return last, nil
}

func parseField(ctx Node, s *scanner, last Node) (Node, error) {
	s.Next()

	name, err := parseIdentifier(s, "field name")
	if err != nil {
		return nil, err
	}
	return NewFieldNode(ctx, name, last), nil
}

func parseEntry(ctx Node, s *scanner, last Node) (Node, error) {
	index := ""

	for unicode.IsDigit(s.Next()) {
		index = index + string(s.Current())
	}
	if index == "" {
		if s.Current() == ':' {
			for unicode.IsDigit(s.Next()) {
				index = index + string(s.Current())
			}
			if s.Current() != ']' {
				return unexpected(s, "expected ']'")
			}
			if index != "" {
				v, _ := strconv.ParseInt(index, 10, 32)
				return parseProjection(ctx, s, NewSlice(ctx, 0, int(v), last))
			} else {
				return parseProjection(ctx, s, NewSlice(ctx, 0, -1, last))
			}
		}

		if s.Current() != ']' {
			return parseSelect(ctx, s, last)
		} else {
			return parseProjection(ctx, s, NewSlice(ctx, -1, -1, last))
		}
	}
	if s.Current() != ']' {
		if s.Current() == ':' {
			end := ""
			for unicode.IsDigit(s.Next()) {
				end = end + string(s.Current())
			}
			if s.Current() != ']' {
				return unexpected(s, "expected ']'")
			}
			start, _ := strconv.ParseInt(index, 10, 32)
			if end != "" {
				v, _ := strconv.ParseInt(end, 10, 32)
				if start > v {
					return nil, fmt.Errorf("start index (%d) larger than end index (%d)", start, v)
				}
				return parseProjection(ctx, s, NewSlice(ctx, int(start), int(v), last))
			} else {
				return parseProjection(ctx, s, NewSlice(ctx, int(start), -1, last))
			}
		}
		return unexpected(s, "expected ']'")
	}
	s.Next()
	v, _ := strconv.ParseInt(index, 10, 32)
	return NewEntry(ctx, int(v), last), nil
}

func parseSelect(ctx Node, s *scanner, last Node) (Node, error) {
	n, err := parseSequence(last, s)
	if err != nil {
		return nil, err
	}
	if n == nil {
		return unexpected(s, "index or path")
	}

	if s.Current() != '=' {
		return unexpected(s, "expected '='")
	}
	s.Next()
	v, err := parseValue(s)
	if err != nil {
		return nil, err
	}
	if s.Current() != ']' {
		return unexpected(s, "expected ']'")
	}
	s.Next()
	return NewSelection(ctx, n, v, last), nil
}

func parseProjection(ctx Node, s *scanner, last Node) (Node, error) {
	if s.Current() != ']' {
		return unexpected(s, "expected ']'")
	}
	if s.Next() == EOI {
		return last, nil
	}
	n, err := parseSequence(last, s)
	if err != nil {
		return nil, err
	}
	if n == nil {
		return unexpected(s, "index or path")
	}
	return NewProjection(ctx, n, last), nil
}

func parseValue(s *scanner) (interface{}, error) {
	name := ""
	pos := s.Position()
	if s.Current() == '"' {
		for s.Next() != '"' {
			if s.Current() == EOI {
				return nil, fmt.Errorf("unexpected end of input (missing '\"') at position %d ", pos)
			}
			name = name + string(s.Current())
		}
		s.Next()
		return name, nil
	} else {
		for unicode.IsDigit(s.Current()) {
			name = name + string(s.Current())
			s.Next()
		}
		if name == "" {
			return nil, fmt.Errorf("integer expected at position %d (found %q)", s.Position(), s.Current())
		}
		v, _ := strconv.ParseInt(name, 10, 32)
		return int(v), nil
	}
}

func parseIdentifier(s *scanner, msg string) (string, error) {
	if !IsIdentifierStart(s.Current()) {
		return "", fmt.Errorf("expected %s at position %d", msg, s.Position())
	}
	name := string(s.Current())
	for IsIdentifierPart(s.Next()) {
		name = name + string(s.Current())
	}
	if name == "_" {
		return "", fmt.Errorf("'_' is no valid field name at position %d", s.Position()-1)
	}
	return name, nil
}

func unexpected(s *scanner, msg string) (Node, error) {
	if s.Current() == EOI {
		return nil, fmt.Errorf("unexpected end of path")
	} else {
		if msg == "" {
			return nil, fmt.Errorf("unexpected character at position %d", s.Position())
		}
		return nil, fmt.Errorf("%s at position %d", msg, s.Position())
	}
}
