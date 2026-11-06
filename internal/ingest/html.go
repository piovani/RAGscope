package ingest

import (
	"io"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	"golang.org/x/net/html/charset"
)

// Official law pages show revoked text struck through. When the page is copied
// as plain text the strike is lost and old and current wording get mixed. Here
// we read the HTML itself, so that text can be dropped.
var skipped = map[atom.Atom]bool{
	atom.Strike: true, atom.S: true, atom.Del: true, // struck-through text
	atom.Script: true, atom.Style: true, atom.Head: true, // not part of the law
}

// Elements that start a new line in the page.
var lineBreaks = map[atom.Atom]bool{
	atom.P: true, atom.Div: true, atom.Br: true, atom.Li: true, atom.Tr: true,
	atom.H1: true, atom.H2: true, atom.H3: true, atom.H4: true, atom.H5: true, atom.H6: true,
	atom.Blockquote: true, atom.Center: true, atom.Table: true,
}

// HTMLToText turns a saved law page into plain text, one paragraph per line
// and without the struck-through (revoked) parts. The page encoding is
// detected, because older government pages are not UTF-8.
func HTMLToText(page io.Reader) (string, error) {
	utf8Page, err := charset.NewReader(page, "")
	if err != nil {
		return "", err
	}
	doc, err := html.Parse(utf8Page)
	if err != nil {
		return "", err
	}

	var raw strings.Builder
	collectText(doc, &raw)

	var paragraphs []string
	for _, line := range strings.Split(raw.String(), "\n") {
		// Fields also turns non-breaking spaces and repeated spaces into one space.
		if text := strings.Join(strings.Fields(line), " "); text != "" {
			paragraphs = append(paragraphs, text)
		}
	}
	return strings.Join(paragraphs, "\n\n") + "\n", nil
}

func collectText(n *html.Node, out *strings.Builder) {
	if n.Type == html.ElementNode {
		if skipped[n.DataAtom] || isStruckByStyle(n) {
			return
		}
		if lineBreaks[n.DataAtom] {
			out.WriteString("\n")
			defer out.WriteString("\n")
		}
	}
	if n.Type == html.TextNode {
		// In HTML a line break inside the text is just a space. Only the tags
		// above start a new line, and "\n" is how we mark that.
		out.WriteString(strings.NewReplacer("\r", " ", "\n", " ").Replace(n.Data))
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		collectText(child, out)
	}
}

// Some pages strike text with CSS instead of a <strike> tag.
func isStruckByStyle(n *html.Node) bool {
	for _, attr := range n.Attr {
		if attr.Key == "style" && strings.Contains(attr.Val, "line-through") {
			return true
		}
	}
	return false
}
