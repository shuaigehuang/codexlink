//go:build !windows

package magpie

import "errors"

func protectBytes([]byte) ([]byte, error) {
	return nil, errors.New("CodexLink credential protection requires Windows DPAPI")
}

func unprotectBytes([]byte) ([]byte, error) {
	return nil, errors.New("CodexLink credential protection requires Windows DPAPI")
}
