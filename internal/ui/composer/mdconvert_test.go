package composer

import (
	"strings"
	"testing"

	"gioui.org/io/key"

	"github.com/some-du6e/clack/internal/paritytest"
)

// mdDescribe draws a Doc the way the expected values (below, and in
// testdata/parity/md-convert.json) were written down from Slack's own
// code (nTS+, Yu2X, 4NfZ run on quill-delta in node): one "kind|runs"
// per line, a run's formats as {b,i,u,s,c,a=URL}.
func mdDescribe(d *Doc) string {
	var b strings.Builder
	for li := range d.Lines {
		if li > 0 {
			b.WriteByte('\n')
		}
		l := d.Lines[li]
		switch l.Kind {
		case Bullet:
			b.WriteString("bullet" + itoa(l.Indent))
		case Ordered:
			b.WriteString("ordered" + itoa(l.Indent))
		case Quote:
			b.WriteString("quote")
		case CodeBlock:
			b.WriteString("code")
		case Header1, Header2, Header3:
			b.WriteString("h" + itoa(int(l.Kind-Header1)+1))
		default:
			b.WriteString("para")
		}
		b.WriteByte('|')
		if l.Kind == Divider {
			b.WriteString("<hr>")
		}
		ls, le := d.LineStart(li), d.LineEnd(li)
		for i := ls; i < le; {
			a := d.Attrs[i]
			if at := a.Atom; at != nil {
				if at.Kind == AtomEmoji {
					b.WriteString("<emoji " + at.Text + ">")
				} else {
					b.WriteString("<@" + at.Text + ">")
				}
				i++
				continue
			}
			j := i
			for j < le && d.Attrs[j].Atom == nil && d.Attrs[j] == a {
				j++
			}
			var tags []string
			for _, s := range [...]struct {
				st Style
				t  string
			}{{Bold, "b"}, {Italic, "i"}, {Underline, "u"}, {Strike, "s"}, {Code, "c"}} {
				if a.Style&s.st != 0 {
					tags = append(tags, s.t)
				}
			}
			if a.Link != "" {
				tags = append(tags, "a="+a.Link)
			}
			txt := string(d.Runes[i:j])
			if len(tags) > 0 {
				b.WriteString("{" + strings.Join(tags, ",") + "}" + txt + "{/}")
			} else {
				b.WriteString(txt)
			}
			i = j
		}
	}
	return b.String()
}

// mdIn is a case's input in testdata/parity/md-convert.json: typed
// markdown and the selection (anchor, caret) when Mod+Shift+F is pressed.
type mdIn struct {
	Text   string `json:"text"`
	Anchor int    `json:"anchor"`
	Caret  int    `json:"caret"`
}

// mdOut is what comes of it: the document as mdDescribe draws it and the
// selection.
type mdOut struct {
	Doc string `json:"doc"`
	Lo  int    `json:"lo"`
	Hi  int    `json:"hi"`
}

// Mod+Shift+F on typed markdown: the results and selections Slack's
// convertMarkdown gives for the same text (its modules run in node on
// quill-delta 5.1.0; a collapsed caret converts the whole message, a
// selection only itself), in testdata/parity/md-convert.json
// (docs/PARITY.md).
func TestConvertMarkdownLikeSlack(t *testing.T) {
	s := paritytest.Load[mdIn, mdOut](t, "testdata/parity/md-convert.json")
	s.Run(t, paritytest.Check[mdIn, mdOut]{Clack: func(t *testing.T, in mdIn) mdOut {
		d := NewDoc()
		d.Insert(0, in.Text, Attr{})
		nd, a, k, _ := mdConvert(d, in.Anchor, in.Caret)
		// a selection made backwards stays backwards (Quill's has no
		// direction, so this one is Clack's rule)
		if in.Anchor > in.Caret && a < k {
			t.Errorf("anchor %d caret %d, want the anchor after the caret", a, k)
		}
		if len(nd.Lines) != strings.Count(string(nd.Runes), "\n")+1 {
			t.Errorf("%d lines for %q", len(nd.Lines), string(nd.Runes))
		}
		return mdOut{Doc: mdDescribe(nd), Lo: min(a, k), Hi: max(a, k)}
	}})
}

// Text that already has formats, mentions, emoji or block formats, the
// same cases run through Slack's code.
func TestConvertMarkdownKeepsWhatIsThere(t *testing.T) {
	karim := &Atom{Kind: AtomUser, ID: "U1", Text: "@karim"}
	smile := &Atom{Kind: AtomEmoji, ID: "smile", Text: ":smile:"}
	tada := &Atom{Kind: AtomEmoji, ID: "tada", Text: ":tada:"}
	type run struct {
		s    string
		a    Attr
		atom *Atom
	}
	build := func(lines []Line, runs ...run) *Doc {
		d := NewDoc()
		for _, r := range runs {
			if r.atom != nil {
				d.InsertAtom(d.Len(), r.atom)
				continue
			}
			d.Insert(d.Len(), r.s, r.a)
		}
		for i, l := range lines {
			d.Lines[i] = l
		}
		return d
	}
	cases := []struct {
		name string
		d    *Doc
		want string
	}{
		{"existing bold kept", build(nil, run{s: "pre "}, run{s: "bold", a: Attr{Style: Bold}}, run{s: " *new*"}), "para|pre {b}bold{/} {b}new{/}"},
		{"existing code masked", build(nil, run{s: "x "}, run{s: "*a*", a: Attr{Style: Code}}, run{s: " y"}), "para|x {c}*a*{/} y"},
		{"existing link masked", build(nil, run{s: "go "}, run{s: "*l*", a: Attr{Link: "https://l.com"}}, run{s: " *b*"}), "para|go {a=https://l.com}*l*{/} {b}b{/}"},
		{"mention in bold", build(nil, run{s: "*hi "}, run{atom: karim}, run{s: "*"}), "para|{b}hi {/}<@@karim>"},
		{"emoji in fence", build(nil, run{s: "```a "}, run{atom: smile}, run{s: " b```"}), "code|a :smile: b"},
		{"mention in fence", build(nil, run{s: "```hi "}, run{atom: karim}, run{s: "```"}), "code|hi @karim"},
		{"emoji in code", build(nil, run{s: "`x "}, run{atom: tada}, run{s: " y`"}), "para|{c}x :tada: y{/}"},
		{"already a list", build([]Line{{Kind: Bullet}}, run{s: "*b*\n- x"}), "bullet0|{b}b{/}\nbullet0|x"},
		{"already quote", build([]Line{{Kind: Quote}}, run{s: "> a"}), "quote|> a"},
		{"in code block", build([]Line{{Kind: CodeBlock}}, run{s: "*b*"}), "code|*b*"},
		{"header line", build([]Line{{Kind: Header1}}, run{s: "*b* - x"}), "h1|{b}b{/} - x"},
		{"hr", build([]Line{{Kind: Divider}}, run{s: "\n*b*"}), "para|<hr>\npara|{b}b{/}"},
	}
	// one subtest per case: tools/parity.py counts them (docs/PARITY.md)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			nd, _, _, _ := mdConvert(c.d, c.d.Len(), c.d.Len())
			if got := mdDescribe(nd); got != c.want {
				t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
			}
		})
	}
}

// The binding: Mod+Shift+F converts as one undo step; markup mode has no
// such binding; Mod+F alone isn't it.
func TestConvertMarkdownKey(t *testing.T) {
	e := NewEditor()
	d := NewDoc()
	d.Insert(0, "make *this* bold", Attr{}) // as pasted: typing would format it live
	e.SetDoc(d)
	edPress(e, "F", key.ModShortcut)
	if e.Doc().Text() != "make *this* bold" {
		t.Fatal("Mod+F converted")
	}
	edPress(e, "F", key.ModShortcut|key.ModShift)
	if got := mdDescribe(e.Doc()); got != "para|make {b}this{/} bold" {
		t.Fatalf("after Mod+Shift+F %q", got)
	}
	if _, c := e.Selection(); c != 14 {
		t.Errorf("caret %d, want 14 (still at the end)", c)
	}
	e.Undo()
	if e.Doc().Text() != "make *this* bold" {
		t.Fatalf("undo gave %q", e.Doc().Text())
	}
	e.plainText = true
	edPress(e, "F", key.ModShortcut|key.ModShift)
	if e.Doc().Text() != "make *this* bold" {
		t.Error("converted in markup mode")
	}
}
