// Copyright 2006-2019 WebPKI.org (http://webpki.org).
// Licensed under the Apache License, Version 2.0.
// Vendored from github.com/cyberphone/json-canonicalization (no Go module).
// RFC 8785 JSON Canonicalization Scheme.

package jcs

import (
	"container/list"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf16"
)

type nameValue struct {
	name    string
	sortKey []uint16
	value   string
}

var asciiEscapes = []byte{'\\', '"', 'b', 'f', 'n', 'r', 't'}
var binaryEscapes = []byte{'\\', '"', '\b', '\f', '\n', '\r', '\t'}
var literals = []string{"true", "false", "null"}

// Transform returns the JCS form of a JSON object or array.
func Transform(jsonData []byte) ([]byte, error) {
	jsonDataLength := len(jsonData)
	index := 0
	var globalError error

	var parseElement func() string
	var parseSimpleType func() string
	var parseQuotedString func() string
	var parseObject func() string
	var parseArray func() string

	checkError := func(e error) {
		if globalError == nil {
			globalError = e
		}
	}
	setError := func(msg string) { checkError(errors.New(msg)) }

	isWhiteSpace := func(c byte) bool {
		return c == 0x20 || c == 0x0a || c == 0x0d || c == 0x09
	}
	nextChar := func() byte {
		if index < jsonDataLength {
			c := jsonData[index]
			if c > 0x7f {
				setError("unexpected non-ASCII character")
			}
			index++
			return c
		}
		setError("unexpected EOF")
		return '"'
	}
	scan := func() byte {
		for {
			c := nextChar()
			if isWhiteSpace(c) {
				continue
			}
			return c
		}
	}
	scanFor := func(expected byte) {
		c := scan()
		if c != expected {
			setError("expected '" + string(expected) + "' but got '" + string(c) + "'")
		}
	}
	getUEscape := func() rune {
		start := index
		nextChar()
		nextChar()
		nextChar()
		nextChar()
		if globalError != nil {
			return 0
		}
		u16, err := strconv.ParseUint(string(jsonData[start:index]), 16, 64)
		checkError(err)
		return rune(u16)
	}
	testNext := func() byte {
		save := index
		c := scan()
		index = save
		return c
	}
	decorateString := func(raw string) string {
		var quoted strings.Builder
		quoted.WriteByte('"')
	CoreLoop:
		for _, c := range []byte(raw) {
			for i, esc := range binaryEscapes {
				if esc == c {
					quoted.WriteByte('\\')
					quoted.WriteByte(asciiEscapes[i])
					continue CoreLoop
				}
			}
			if c < 0x20 {
				quoted.WriteString(fmt.Sprintf("\\u%04x", c))
			} else {
				quoted.WriteByte(c)
			}
		}
		quoted.WriteByte('"')
		return quoted.String()
	}
	parseQuotedString = func() string {
		var raw strings.Builder
	CoreLoop:
		for globalError == nil {
			var c byte
			if index < jsonDataLength {
				c = jsonData[index]
				index++
			} else {
				nextChar()
				break
			}
			if c == '"' {
				break
			}
			if c < ' ' {
				setError("unterminated string literal")
			} else if c == '\\' {
				c = nextChar()
				if c == 'u' {
					first := getUEscape()
					if utf16.IsSurrogate(first) {
						if nextChar() != '\\' || nextChar() != 'u' {
							setError("missing surrogate")
						} else {
							raw.WriteRune(utf16.DecodeRune(first, getUEscape()))
						}
					} else {
						raw.WriteRune(first)
					}
				} else if c == '/' {
					raw.WriteByte('/')
				} else {
					for i, esc := range asciiEscapes {
						if esc == c {
							raw.WriteByte(binaryEscapes[i])
							continue CoreLoop
						}
					}
					setError("unexpected escape: \\" + string(c))
				}
			} else {
				raw.WriteByte(c)
			}
		}
		return raw.String()
	}
	parseSimpleType = func() string {
		var token strings.Builder
		index--
		for globalError == nil {
			c := testNext()
			if c == ',' || c == ']' || c == '}' {
				break
			}
			c = nextChar()
			if isWhiteSpace(c) {
				break
			}
			token.WriteByte(c)
		}
		if token.Len() == 0 {
			setError("missing argument")
		}
		value := token.String()
		for _, literal := range literals {
			if literal == value {
				return literal
			}
		}
		ieee, err := strconv.ParseFloat(value, 64)
		checkError(err)
		value, err = numberToJSON(ieee)
		checkError(err)
		return value
	}
	parseElement = func() string {
		switch scan() {
		case '{':
			return parseObject()
		case '"':
			return decorateString(parseQuotedString())
		case '[':
			return parseArray()
		default:
			return parseSimpleType()
		}
	}
	parseArray = func() string {
		var arrayData strings.Builder
		arrayData.WriteByte('[')
		next := false
		for globalError == nil && testNext() != ']' {
			if next {
				scanFor(',')
				arrayData.WriteByte(',')
			} else {
				next = true
			}
			arrayData.WriteString(parseElement())
		}
		scan()
		arrayData.WriteByte(']')
		return arrayData.String()
	}
	precedes := func(sortKey []uint16, e *list.Element) bool {
		old := e.Value.(nameValue).sortKey
		minLength := len(old)
		if minLength > len(sortKey) {
			minLength = len(sortKey)
		}
		for q := 0; q < minLength; q++ {
			diff := int(sortKey[q]) - int(old[q])
			if diff < 0 {
				return true
			} else if diff > 0 {
				return false
			}
		}
		if len(sortKey) < len(old) {
			return true
		}
		if len(sortKey) == len(old) {
			setError("duplicate key: " + e.Value.(nameValue).name)
		}
		return false
	}
	parseObject = func() string {
		nameValueList := list.New()
		next := false
	CoreLoop:
		for globalError == nil && testNext() != '}' {
			if next {
				scanFor(',')
			}
			next = true
			scanFor('"')
			rawUTF8 := parseQuotedString()
			if globalError != nil {
				break
			}
			sortKey := utf16.Encode([]rune(rawUTF8))
			scanFor(':')
			nv := nameValue{rawUTF8, sortKey, parseElement()}
			for e := nameValueList.Front(); e != nil; e = e.Next() {
				if precedes(sortKey, e) {
					nameValueList.InsertBefore(nv, e)
					continue CoreLoop
				}
			}
			nameValueList.PushBack(nv)
		}
		scan()
		var objectData strings.Builder
		objectData.WriteByte('{')
		next = false
		for e := nameValueList.Front(); e != nil; e = e.Next() {
			if next {
				objectData.WriteByte(',')
			}
			next = true
			nv := e.Value.(nameValue)
			objectData.WriteString(decorateString(nv.name))
			objectData.WriteByte(':')
			objectData.WriteString(nv.value)
		}
		objectData.WriteByte('}')
		return objectData.String()
	}

	var transformed string
	if testNext() == '[' {
		scan()
		transformed = parseArray()
	} else {
		scanFor('{')
		transformed = parseObject()
	}
	for index < jsonDataLength {
		if !isWhiteSpace(jsonData[index]) {
			setError("improperly terminated JSON")
			break
		}
		index++
	}
	return []byte(transformed), globalError
}
