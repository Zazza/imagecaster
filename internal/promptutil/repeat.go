package promptutil

import (
	"strings"
	"unicode"
)

func isSentenceEnd(r rune) bool {
	return r == '.' || r == '!' || r == '?' || r == '…'
}

func splitSentences(line string) []string {
	rs := []rune(line)
	var out []string
	start := 0
	for i := 0; i < len(rs); i++ {
		if !isSentenceEnd(rs[i]) {
			continue
		}
		j := i
		for j+1 < len(rs) && isSentenceEnd(rs[j+1]) {
			j++
		}
		if j+1 < len(rs) && !unicode.IsSpace(rs[j+1]) {
			i = j
			continue
		}
		for j+1 < len(rs) && unicode.IsSpace(rs[j+1]) {
			j++
		}
		out = append(out, string(rs[start:j+1]))
		start = j + 1
		i = j
	}
	if start < len(rs) {
		out = append(out, string(rs[start:]))
	}
	return out
}

const minRepeatClause = 12

func repeatKey(s string) string {
	s = strings.ToLower(s)
	s = strings.TrimFunc(s, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsPunct(r) })
	return strings.Join(strings.Fields(s), " ")
}

func DedupeSentences(s string) string {
	if strings.TrimSpace(s) == "" {
		return s
	}
	seen := map[string]bool{}
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for i, line := range lines {
		last := i == len(lines)-1
		kept := dedupeLine(line, seen, last)
		if strings.TrimSpace(kept) == "" && strings.TrimSpace(line) != "" {
			continue
		}
		out = append(out, kept)
	}
	return strings.TrimRight(strings.Join(out, "\n"), " \n")
}

func dedupeLine(line string, seen map[string]bool, lastLine bool) string {
	parts := splitSentences(line)
	var b strings.Builder
	for i, p := range parts {
		sentence := dedupeClauses(p)
		key := repeatKey(sentence)
		if key == "" {
			b.WriteString(p)
			continue
		}
		if seen[key] {
			continue
		}
		trimmed := []rune(strings.TrimSpace(p))
		unfinished := lastLine && i == len(parts)-1 && !isSentenceEnd(trimmed[len(trimmed)-1])
		if unfinished && prefixOfSeen(key, seen) {
			continue
		}
		seen[key] = true
		b.WriteString(sentence)
	}
	return b.String()
}

func prefixOfSeen(key string, seen map[string]bool) bool {
	for k := range seen {
		if strings.HasPrefix(k, key) {
			return true
		}
	}
	return false
}

func dedupeClauses(sentence string) string {
	clauses := strings.Split(sentence, ", ")
	if len(clauses) < 3 {
		return sentence
	}
	seen := map[string]bool{}
	kept := make([]string, 0, len(clauses))
	for _, c := range clauses {
		key := repeatKey(c)
		if len([]rune(key)) >= minRepeatClause {
			if seen[key] {
				continue
			}
			seen[key] = true
		}
		kept = append(kept, c)
	}
	out := strings.Join(kept, ", ")
	if trimmed := strings.TrimRight(sentence, " "); strings.HasSuffix(trimmed, ".") && !strings.HasSuffix(strings.TrimRight(out, " "), ".") {
		out = strings.TrimRight(out, " ,") + "."
	}
	return out
}
