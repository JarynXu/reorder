package reorder

import "testing"

func FuzzRewriteIdempotent(f *testing.F) {
	seeds := []string{
		"package p\n",
		"package p\n\ntype T struct{}\nfunc (T) b() {}\nfunc (T) A() {}\n",
		"package p\n\ntype T struct{}\nfunc (T) A() {}\nfunc NewT() *T { // keep\n return &T{}\n}\n",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, source string) {
		cfg := DefaultConfig()
		first, _, err := Rewrite("fuzz.go", []byte(source), cfg)
		if err != nil {
			return
		}
		second, changed, err := Rewrite("fuzz.go", first, cfg)
		if err != nil {
			t.Fatalf("second rewrite failed after successful first rewrite: %v", err)
		}
		if changed {
			t.Fatal("rewrite is not idempotent")
		}
		if string(first) != string(second) {
			t.Fatal("second rewrite changed bytes")
		}
	})
}
