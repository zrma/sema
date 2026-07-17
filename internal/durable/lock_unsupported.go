//go:build !darwin && !linux

package durable

import (
	"fmt"
	"os"
)

func lockFile(_ *os.File) error {
	return fmt.Errorf("durable runtime is supported only on darwin and linux")
}

func unlockFile(_ *os.File) error {
	return nil
}
