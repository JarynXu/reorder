// Package reorder computes safe, deterministic source edits that make Go
// declarations satisfy funcorder-compatible ordering rules.
package reorder

import (
	"errors"
	"fmt"
)

// ErrUnsafeTrivia is returned when a required declaration move would cross
// an unattached, non-whitespace source fragment. Reorder deliberately refuses
// such edits rather than guessing how a standalone comment or directive should
// move.
var ErrUnsafeTrivia = errors.New("reorder: unsafe unattached source trivia")

// Config mirrors funcorder's ordering switches.
type Config struct {
	Constructor  bool
	StructMethod bool
	Alphabetical bool
	Function     bool
}

// DefaultConfig returns funcorder's default rule configuration.
func DefaultConfig() Config {
	return Config{
		Constructor:  true,
		StructMethod: true,
		Alphabetical: false,
		Function:     false,
	}
}

// Edit is a byte-offset source edit. Start is inclusive and End is exclusive.
type Edit struct {
	Start   int
	End     int
	NewText []byte
}

// Plan describes the single minimal contiguous edit needed for a file.
// An unchanged file has a nil Edit.
type Plan struct {
	Edit *Edit
}

// Changed reports whether applying the plan changes the file.
func (p Plan) Changed() bool {
	return p.Edit != nil
}

// Apply applies the plan to src and returns a new byte slice.
func (p Plan) Apply(src []byte) []byte {
	if p.Edit == nil {
		return append([]byte(nil), src...)
	}

	out := make([]byte, 0, len(src)-(p.Edit.End-p.Edit.Start)+len(p.Edit.NewText))
	out = append(out, src[:p.Edit.Start]...)
	out = append(out, p.Edit.NewText...)
	out = append(out, src[p.Edit.End:]...)
	return out
}

// UnsafeTriviaError identifies the source boundary that prevented an
// automatic edit.
type UnsafeTriviaError struct {
	Filename string
	Offset   int
}

func (e *UnsafeTriviaError) Error() string {
	if e.Filename == "" {
		return fmt.Sprintf("%v at byte offset %d", ErrUnsafeTrivia, e.Offset)
	}
	return fmt.Sprintf("%s: %v at byte offset %d", e.Filename, ErrUnsafeTrivia, e.Offset)
}

// Unwrap makes UnsafeTriviaError compatible with errors.Is.
func (e *UnsafeTriviaError) Unwrap() error {
	return ErrUnsafeTrivia
}

// Rewrite computes and applies a reorder plan.
func Rewrite(filename string, src []byte, cfg Config) ([]byte, bool, error) {
	plan, err := PlanFile(filename, src, cfg)
	if err != nil {
		return nil, false, err
	}
	return plan.Apply(src), plan.Changed(), nil
}
