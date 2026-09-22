package note

import (
	"fmt"
	"strings"
)

type edit struct {
	op   byte
	line string
}

// Unified returns a line-oriented unified diff. Equal inputs return an empty string.
// Notes are capped at 64KB, so the LCS table stays small. Past 800 lines per side
// the function emits one replace hunk instead of allocating a giant table.
func Unified(from, to string) string {
	if from == to {
		return ""
	}
	a := strings.Split(from, "\n")
	b := strings.Split(to, "\n")
	var edits []edit
	if len(a) <= 800 && len(b) <= 800 {
		edits = diffLines(a, b)
	} else {
		edits = make([]edit, 0, len(a)+len(b))
		for _, line := range a {
			edits = append(edits, edit{op: '-', line: line})
		}
		for _, line := range b {
			edits = append(edits, edit{op: '+', line: line})
		}
	}
	var buf strings.Builder
	buf.WriteString("--- a\n+++ b\n")
	fmt.Fprintf(&buf, "@@ -1,%d +1,%d @@\n", len(a), len(b))
	for _, e := range edits {
		buf.WriteByte(e.op)
		buf.WriteString(e.line)
		buf.WriteByte('\n')
	}
	return buf.String()
}

func diffLines(a, b []string) []edit {
	n, m := len(a), len(b)
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}
	var out []edit
	i, j := 0, 0
	for i < n && j < m {
		if a[i] == b[j] {
			out = append(out, edit{op: ' ', line: a[i]})
			i++
			j++
			continue
		}
		if dp[i+1][j] >= dp[i][j+1] {
			out = append(out, edit{op: '-', line: a[i]})
			i++
			continue
		}
		out = append(out, edit{op: '+', line: b[j]})
		j++
	}
	for i < n {
		out = append(out, edit{op: '-', line: a[i]})
		i++
	}
	for j < m {
		out = append(out, edit{op: '+', line: b[j]})
		j++
	}
	return out
}
