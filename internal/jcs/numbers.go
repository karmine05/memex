// Copyright 2006-2019 WebPKI.org (http://webpki.org).
// Licensed under the Apache License, Version 2.0.
// Vendored from github.com/cyberphone/json-canonicalization (no Go module).
// Converts IEEE-754 doubles into the ES6 JSON number format used by RFC 8785.

package jcs

import (
	"errors"
	"math"
	"strconv"
	"strings"
)

const invalidPattern uint64 = 0x7ff0000000000000

func numberToJSON(ieeeF64 float64) (string, error) {
	ieeeU64 := math.Float64bits(ieeeF64)
	if (ieeeU64 & invalidPattern) == invalidPattern {
		return "null", errors.New("invalid JSON number: " + strconv.FormatUint(ieeeU64, 16))
	}
	if ieeeF64 == 0 {
		return "0", nil
	}
	sign := ""
	if ieeeF64 < 0 {
		ieeeF64 = -ieeeF64
		sign = "-"
	}
	format := byte('e')
	if ieeeF64 < 1e+21 && ieeeF64 >= 1e-6 {
		format = 'f'
	}
	es6Formatted := strconv.FormatFloat(ieeeF64, format, -1, 64)
	exponent := strings.IndexByte(es6Formatted, 'e')
	if exponent > 0 && es6Formatted[exponent+2] == '0' {
		es6Formatted = es6Formatted[:exponent+2] + es6Formatted[exponent+3:]
	}
	return sign + es6Formatted, nil
}
