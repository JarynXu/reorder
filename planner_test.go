package reorder

import (
	"errors"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func TestRewrite(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  Config
		src  string
		want string
	}{
		{
			name: "constructor before type",
			cfg:  DefaultConfig(),
			src: `package sample

func NewThing() *Thing { return &Thing{} }

type Thing struct{}

func (Thing) Public() {}
`,
			want: `package sample

type Thing struct{}

func NewThing() *Thing { return &Thing{} }

func (Thing) Public() {}
`,
		},
		{
			name: "constructor after methods",
			cfg:  DefaultConfig(),
			src: `package sample

type Thing struct{}

func (Thing) Public() {}

func NewThing() *Thing { return &Thing{} }
`,
			want: `package sample

type Thing struct{}

func NewThing() *Thing { return &Thing{} }

func (Thing) Public() {}
`,
		},
		{
			name: "exported methods before unexported with unrelated function",
			cfg:  DefaultConfig(),
			src: `package sample

type Thing struct{}

func (Thing) private() {}

func helper() {}

func (Thing) Public() {}
`,
			want: `package sample

type Thing struct{}

func helper() {}

func (Thing) Public() {}

func (Thing) private() {}
`,
		},
		{
			name: "alphabetical constructors and method groups",
			cfg: Config{
				Constructor:  true,
				StructMethod: true,
				Alphabetical: true,
			},
			src: `package sample

type Thing struct{}

func NewZed() *Thing { return &Thing{} }
func NewAlpha() *Thing { return &Thing{} }
func (Thing) Zebra() {}
func (Thing) Alpha() {}
func (Thing) zebra() {}
func (Thing) alpha() {}
`,
			want: `package sample

type Thing struct{}

func NewAlpha() *Thing { return &Thing{} }
func NewZed() *Thing { return &Thing{} }
func (Thing) Alpha() {}
func (Thing) Zebra() {}
func (Thing) alpha() {}
func (Thing) zebra() {}
`,
		},
		{
			name: "function check excludes init",
			cfg: Config{
				Function: true,
			},
			src: `package sample

func private() {}

func init() {}

func Public() {}
`,
			want: `package sample

func init() {}

func Public() {}

func private() {}
`,
		},
		{
			name: "constructor and function rules compose",
			cfg: Config{
				Constructor: true,
				Function:    true,
			},
			src: `package sample

func private() {}

type Thing struct{}

func NewThing() *Thing { return &Thing{} }
`,
			want: `package sample

type Thing struct{}

func NewThing() *Thing { return &Thing{} }

func private() {}
`,
		},
		{
			name: "methods for type not declared in file are ignored",
			cfg:  DefaultConfig(),
			src: `package sample

func (External) private() {}
func (External) Public() {}
`,
			want: `package sample

func (External) private() {}
func (External) Public() {}
`,
		},
		{
			name: "constructor compatibility excludes bare New",
			cfg:  DefaultConfig(),
			src: `package sample

func New() *Thing { return &Thing{} }

type Thing struct{}
`,
			want: `package sample

func New() *Thing { return &Thing{} }

type Thing struct{}
`,
		},
		{
			name: "grouped type declaration is an anchor",
			cfg:  DefaultConfig(),
			src: `package sample

func MustThing() *Thing { return &Thing{} }

type (
	Other int
	Thing struct{}
)
`,
			want: `package sample

type (
	Other int
	Thing struct{}
)

func MustThing() *Thing { return &Thing{} }
`,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertRewrite(t, tt.src, tt.want, tt.cfg)
		})
	}
}

func TestIssue32BodyCommentsArePreserved(t *testing.T) {
	t.Parallel()

	src := `package sample

type Thing struct {
	Name string
}

func (t Thing) GetName() string {
	// method body comment must survive
	return t.Name
}

func NewThing() *Thing {
	// constructor body comment must survive
	return &Thing{Name: "John"}
}
`
	want := `package sample

type Thing struct {
	Name string
}

func NewThing() *Thing {
	// constructor body comment must survive
	return &Thing{Name: "John"}
}

func (t Thing) GetName() string {
	// method body comment must survive
	return t.Name
}
`

	assertRewrite(t, src, want, DefaultConfig())
}

func TestDocDirectiveAndTrailingCommentMoveWithFunction(t *testing.T) {
	t.Parallel()

	cfg := Config{Function: true}
	src := `package sample

//go:noinline
func private() { // keep trailing
	// keep body
}

func Public() {}
`
	want := `package sample

func Public() {}

//go:noinline
func private() { // keep trailing
	// keep body
}
`

	assertRewrite(t, src, want, cfg)
}

func TestPackageVarAndInitRelativeOrderArePreserved(t *testing.T) {
	t.Parallel()

	cfg := Config{Function: true}
	src := `package sample

func private() {}

var first = mark("first")

func init() { mark("init-one") }

var second = mark("second")

func init() { mark("init-two") }

func Public() {}

func mark(s string) string { return s }
`

	got, changed, err := Rewrite("sample.go", []byte(src), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected rewrite")
	}
	text := string(got)

	assertBefore(t, text, `var first`, `func init() { mark("init-one") }`)
	assertBefore(t, text, `func init() { mark("init-one") }`, `var second`)
	assertBefore(t, text, `var second`, `func init() { mark("init-two") }`)
	assertBefore(t, text, `func init() { mark("init-two") }`, `func private()`)
	assertBefore(t, text, `func Public()`, `func private()`)
}

func TestUnsafeStandaloneCommentRefusesCrossingMove(t *testing.T) {
	t.Parallel()

	src := `package sample

type Thing struct{}

func (Thing) private() {}

// Helpers are intentionally separated from methods.

func helper() {}

func (Thing) Public() {}
`

	_, _, err := Rewrite("sample.go", []byte(src), DefaultConfig())
	if !errors.Is(err, ErrUnsafeTrivia) {
		t.Fatalf("expected ErrUnsafeTrivia, got %v", err)
	}
}

func TestAlreadyCompliantIsByteIdentical(t *testing.T) {
	t.Parallel()

	src := "package sample\r\n\r\ntype Thing struct{}\r\n\r\nfunc NewThing() *Thing {\r\n\t// keep CRLF and body comment\r\n\treturn &Thing{}\r\n}\r\n\r\nfunc (Thing) Public() {}\r\nfunc (Thing) private() {}\r\n"
	got, changed, err := Rewrite("sample.go", []byte(src), DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("compliant source unexpectedly changed")
	}
	if string(got) != src {
		t.Fatal("compliant source was not byte-identical")
	}
}

func TestRewritePreservesCRLFWhileMoving(t *testing.T) {
	t.Parallel()

	src := "package sample\r\n\r\ntype Thing struct{}\r\n\r\nfunc (Thing) Public() {}\r\n\r\nfunc NewThing() *Thing {\r\n\t// body\r\n\treturn &Thing{}\r\n}\r\n"
	want := "package sample\r\n\r\ntype Thing struct{}\r\n\r\nfunc NewThing() *Thing {\r\n\t// body\r\n\treturn &Thing{}\r\n}\r\n\r\nfunc (Thing) Public() {}\r\n"
	assertRewrite(t, src, want, DefaultConfig())
}

func TestAlphabeticalOnlyHasEffectWhenOwningRuleEnabled(t *testing.T) {
	t.Parallel()

	src := `package sample

type Thing struct{}

func (Thing) Zebra() {}
func (Thing) Alpha() {}
`
	cfg := Config{Alphabetical: true}
	assertRewrite(t, src, src, cfg)
}

func TestGenericReceiverMatchesCurrentFuncorderSemanticsAndIsIgnored(t *testing.T) {
	t.Parallel()

	src := `package sample

type Box[T any] struct{}

func (Box[T]) private() {}
func (Box[T]) Public() {}
`
	assertRewrite(t, src, src, DefaultConfig())
}

func assertRewrite(t *testing.T, src, want string, cfg Config) {
	t.Helper()

	got, changed, err := Rewrite("sample.go", []byte(src), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("unexpected rewrite\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
	if changed != (src != want) {
		t.Fatalf("changed=%v, want %v", changed, src != want)
	}

	if _, err := parser.ParseFile(token.NewFileSet(), "sample.go", got, parser.ParseComments); err != nil {
		t.Fatalf("rewritten source is not valid Go: %v", err)
	}

	gotAgain, changedAgain, err := Rewrite("sample.go", got, cfg)
	if err != nil {
		t.Fatalf("second rewrite failed: %v", err)
	}
	if changedAgain {
		t.Fatalf("rewrite is not idempotent\n%s", gotAgain)
	}
	if string(gotAgain) != string(got) {
		t.Fatal("second rewrite changed bytes")
	}
}

func assertBefore(t *testing.T, text, before, after string) {
	t.Helper()
	beforeIndex := strings.Index(text, before)
	afterIndex := strings.Index(text, after)
	if beforeIndex < 0 || afterIndex < 0 {
		t.Fatalf("missing markers %q or %q", before, after)
	}
	if beforeIndex >= afterIndex {
		t.Fatalf("expected %q before %q\n%s", before, after, text)
	}
}

func TestFunctionRuleTreatsMainAsUnexported(t *testing.T) {
	t.Parallel()

	cfg := Config{Function: true}
	src := `package main

func main() {}

func Public() {}
`
	want := `package main

func Public() {}

func main() {}
`
	assertRewrite(t, src, want, cfg)
}

func TestBuildConstraintHeaderIsUntouched(t *testing.T) {
	t.Parallel()

	cfg := Config{Function: true}
	src := `//go:build linux
// +build linux

package sample

func private() {}

func Public() {}
`
	want := `//go:build linux
// +build linux

package sample

func Public() {}

func private() {}
`
	assertRewrite(t, src, want, cfg)
}

func TestRewritePreservesMissingFinalNewline(t *testing.T) {
	t.Parallel()

	cfg := Config{Function: true}
	src := "package sample\n\nfunc private() {}\n\nfunc Public() {}"
	want := "package sample\n\nfunc Public() {}\n\nfunc private() {}"
	assertRewrite(t, src, want, cfg)
}

func TestUncrossedStandaloneCommentDoesNotBlockIndependentMove(t *testing.T) {
	t.Parallel()

	src := `package sample

type One struct{}

func (One) private() {}
func (One) Public() {}

// Section boundary deliberately remains in place.
type Two struct{}

func (Two) Public() {}
func (Two) private() {}
`
	want := `package sample

type One struct{}

func (One) Public() {}
func (One) private() {}

// Section boundary deliberately remains in place.
type Two struct{}

func (Two) Public() {}
func (Two) private() {}
`
	assertRewrite(t, src, want, DefaultConfig())
}

func TestBlockDocAndBodyCommentsMoveVerbatim(t *testing.T) {
	t.Parallel()

	src := `package sample

type Thing struct{}

func (Thing) Public() {}

/* NewThing constructs a Thing. */
func NewThing() *Thing {
	/* body comment */
	return &Thing{}
}
`
	want := `package sample

type Thing struct{}

/* NewThing constructs a Thing. */
func NewThing() *Thing {
	/* body comment */
	return &Thing{}
}

func (Thing) Public() {}
`
	assertRewrite(t, src, want, DefaultConfig())
}
