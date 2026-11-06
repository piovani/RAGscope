package retrieve

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Very common Portuguese words (and question words) that say nothing about the
// topic. Without them every question would match every chunk a little.
var stopwords = toSet(
	"a", "o", "as", "os", "um", "uma", "uns", "umas",
	"de", "do", "da", "dos", "das", "em", "no", "na", "nos", "nas",
	"e", "ou", "que", "se", "por", "para", "com", "sem", "ao", "aos",
	"ser", "sao", "foi", "como", "mais", "mas", "qual", "quais", "quem", "quando", "onde", "nao",
)

func toSet(words ...string) map[string]bool {
	set := make(map[string]bool, len(words))
	for _, w := range words {
		set[w] = true
	}
	return set
}

// tokenize lowercases the text, removes accents and splits it into words, so
// "Informação" and "informacao" are the same term. There is no stemming:
// "dado" and "dados" are different terms.
func tokenize(text string) []string {
	text = removeAccents(strings.ToLower(text))

	words := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})

	tokens := make([]string, 0, len(words))
	for _, w := range words {
		if !stopwords[w] {
			tokens = append(tokens, w)
		}
	}
	return tokens
}

func removeAccents(s string) string {
	// NFKD splits "ç" into "c" + a cedilla mark, and "º" into "o".
	var b strings.Builder
	for _, r := range norm.NFKD.String(s) {
		if !unicode.Is(unicode.Mn, r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
