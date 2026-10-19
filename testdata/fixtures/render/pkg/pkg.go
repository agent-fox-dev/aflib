// Package render is a fixture for the exported-API renderer: exported and
// unexported constants, variables, functions and types, with doc, field and
// line comments of every kind.
package render

import "errors"

// Version is the fixture's version.
const Version = "1" // the version

const hidden = 2

// Colors are grouped; blue is not exported.
const (
	// Red comes first.
	Red = iota // the first color
	Green
	blue
)

var (
	// ErrBad is a sentinel error.
	ErrBad    = errors.New("bad")
	errHidden = errors.New("hidden")
)

// DefaultMode is the mode used when none is given; its type makes go/doc
// attach it to Mode.
var DefaultMode Mode = ModeA

// Names is an alias, which only the "=" tells from a defined type.
type Names = []string

// List is a generic alias.
type List[T any] = []T

// Widget is the main type.
type Widget struct {
	// Name is exported.
	Name string // the name

	size int /* unexported */

	/* standalone block comment */
	Tags []string
}

// Mode selects how a Widget behaves.
type Mode int

// The modes.
const (
	ModeA Mode = iota // the default
	ModeB
)

// New returns a Widget; go/doc attaches it to Widget.
func New(name string) *Widget { return &Widget{Name: name} }

// Size returns the size.
func (w *Widget) Size() int { return w.size }

func (w *Widget) grow() { w.size++ }

// Reader is an interface with an unexported method.
type Reader interface {
	// Read returns the next line.
	Read() (string, error) // may fail

	internal()
}

type secret struct{ n int }

// Helper is a package-level function.
func Helper(n int) string { return "" }

func helper() {}
