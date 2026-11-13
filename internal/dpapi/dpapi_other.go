//go:build !windows

package dpapi

import "errors"

func Protect([]byte, []byte) ([]byte, error) {
	return nil, errors.New("DPAPI is only available on Windows")
}

func Unprotect([]byte, []byte) ([]byte, error) {
	return nil, errors.New("DPAPI is only available on Windows")
}
