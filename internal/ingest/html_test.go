package ingest

import (
	"strings"
	"testing"

	"golang.org/x/text/encoding/charmap"
)

const lawPage = `<html><head><title>Lei</title><style>p { color: red }</style></head><body>
<p>Art. 5º Para os fins desta Lei, considera-se:</p>
<p>VIII - encarregado: <strike>pessoa natural, indicada pelo controlador</strike>;</p>
<p><s>VIII - encarregado: texto antigo inteiro.</s></p>
<p>VIII - encarregado: pessoa indicada pelo
   controlador e operador (Redação dada pela Lei nº 13.853, de 2019)</p>
<p><span style="text-decoration: line-through">Art. 55-A. Texto revogado.</span></p>
<p>Art. 55-A.&nbsp;&nbsp;Fica criada a ANPD.<br>§ 1º Texto vigente.</p>
</body></html>`

func TestHTMLToTextDropsStruckTextAndKeepsParagraphs(t *testing.T) {
	got, err := HTMLToText(strings.NewReader(lawPage))
	if err != nil {
		t.Fatal(err)
	}

	for _, banned := range []string{"pessoa natural, indicada", "texto antigo", "Texto revogado", "color: red", "Lei\n"} {
		if strings.Contains(got, banned) {
			t.Errorf("output should not contain %q:\n%s", banned, got)
		}
	}

	want := []string{
		"Art. 5º Para os fins desta Lei, considera-se:",
		"VIII - encarregado: ;", // only the struck words were removed from this paragraph
		"VIII - encarregado: pessoa indicada pelo controlador e operador (Redação dada pela Lei nº 13.853, de 2019)",
		"Art. 55-A. Fica criada a ANPD.",
		"§ 1º Texto vigente.", // <br> starts a new line
	}
	for _, line := range want {
		if !strings.Contains(got, line+"\n") {
			t.Errorf("missing line %q in:\n%s", line, got)
		}
	}
}

func TestHTMLToTextDetectsOldEncodings(t *testing.T) {
	// Government pages are often windows-1252, declared only in a <meta> tag.
	page := `<html><head><meta http-equiv="Content-Type" content="text/html; charset=windows-1252"></head>` +
		`<body><p>Art. 1º Proteção de dados é um direito.</p></body></html>`
	encoded, err := charmap.Windows1252.NewEncoder().String(page)
	if err != nil {
		t.Fatal(err)
	}

	got, err := HTMLToText(strings.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Art. 1º Proteção de dados é um direito.") {
		t.Errorf("accents were not decoded: %q", got)
	}
}
