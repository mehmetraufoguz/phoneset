//go:build !windows

package camera

import (
	"fmt"
	"os"
)

// ElevatedMain is the part of `phoneset camera` that runs with administrator rights on Windows; no other system has it.
func ElevatedMain(args []string) int {
	fmt.Fprintln(os.Stderr, "phoneset camera _elevated is for Windows only")
	return 1
}
