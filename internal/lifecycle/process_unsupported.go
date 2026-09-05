//go:build !linux && !darwin

package lifecycle

import (
	"fmt"
	"runtime"
)

func processState(int) (State, error) {
	return State{}, fmt.Errorf("managed process identity is unsupported on %s", runtime.GOOS)
}
