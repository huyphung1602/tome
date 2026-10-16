//go:build !unix

package kitty

import "time"

// Probe reports no graphics support where /dev/tty isn't available.
func Probe(timeout time.Duration) Caps { return Caps{} }

// CellSize guesses a typical cell size.
func CellSize() (w, h int) { return 8, 16 }
