package nginx

import "strings"

// ---------------------------------------------------------------------------
// Line diff (LCS). Config files are small; inputs above the cap fall back to
// "replace everything" rather than using quadratic memory.
// ---------------------------------------------------------------------------

type DiffLine struct {
	Op   string `json:"op"` // " ", "+", "-"
	Old  int    `json:"old,omitempty"`
	New  int    `json:"new,omitempty"`
	Text string `json:"text"`
}

type DiffResult struct {
	Lines     []DiffLine `json:"lines"`
	Additions int        `json:"additions"`
	Deletions int        `json:"deletions"`
}

const diffMaxLines = 4000

func splitLines(s string) []string {
	s = strings.TrimSuffix(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func LineDiff(oldText, newText string) DiffResult {
	a, b := splitLines(oldText), splitLines(newText)
	var res DiffResult
	add := func(l DiffLine) {
		res.Lines = append(res.Lines, l)
		switch l.Op {
		case "+":
			res.Additions++
		case "-":
			res.Deletions++
		}
	}
	if len(a) > diffMaxLines || len(b) > diffMaxLines {
		for i, l := range a {
			add(DiffLine{Op: "-", Old: i + 1, Text: l})
		}
		for i, l := range b {
			add(DiffLine{Op: "+", New: i + 1, Text: l})
		}
		return res
	}
	// lcs[i][j] = LCS length of a[i:], b[j:]
	lcs := make([][]int32, len(a)+1)
	for i := range lcs {
		lcs[i] = make([]int32, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			add(DiffLine{Op: " ", Old: i + 1, New: j + 1, Text: a[i]})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			add(DiffLine{Op: "-", Old: i + 1, Text: a[i]})
			i++
		default:
			add(DiffLine{Op: "+", New: j + 1, Text: b[j]})
			j++
		}
	}
	for ; i < len(a); i++ {
		add(DiffLine{Op: "-", Old: i + 1, Text: a[i]})
	}
	for ; j < len(b); j++ {
		add(DiffLine{Op: "+", New: j + 1, Text: b[j]})
	}
	return res
}
