//go:build windows

package magpie

import (
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

const (
	cryptProtectUIFobidden = 0x1
)

var (
	crypt32DLL              = syscall.NewLazyDLL("crypt32.dll")
	kernel32DLL             = syscall.NewLazyDLL("kernel32.dll")
	procCryptProtectData    = crypt32DLL.NewProc("CryptProtectData")
	procCryptUnprotectData  = crypt32DLL.NewProc("CryptUnprotectData")
	procLocalFree           = kernel32DLL.NewProc("LocalFree")
	codexLinkCredentialSalt = []byte("CodexLink.Magpie.Admin.v1")
)

type dataBlob struct {
	Size uint32
	Data *byte
}

func protectBytes(plain []byte) ([]byte, error) {
	return cryptData(plain, procCryptProtectData)
}

func unprotectBytes(cipher []byte) ([]byte, error) {
	return cryptData(cipher, procCryptUnprotectData)
}

func cryptData(input []byte, procedure *syscall.LazyProc) ([]byte, error) {
	if len(input) == 0 {
		return nil, errors.New("empty credential data")
	}
	entropy := dataBlob{
		Size: uint32(len(codexLinkCredentialSalt)),
		Data: &codexLinkCredentialSalt[0],
	}
	in := dataBlob{
		Size: uint32(len(input)),
		Data: &input[0],
	}
	var out dataBlob
	result, _, callErr := procedure.Call(
		uintptr(unsafe.Pointer(&in)),
		0,
		uintptr(unsafe.Pointer(&entropy)),
		0,
		0,
		cryptProtectUIFobidden,
		uintptr(unsafe.Pointer(&out)),
	)
	runtime.KeepAlive(input)
	runtime.KeepAlive(codexLinkCredentialSalt)
	if result == 0 {
		if callErr != nil && callErr != syscall.Errno(0) {
			return nil, callErr
		}
		return nil, errors.New("Windows DPAPI call failed")
	}
	if out.Data == nil || out.Size == 0 {
		return nil, errors.New("Windows DPAPI returned empty data")
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(out.Data)))
	encrypted := append([]byte(nil), unsafe.Slice(out.Data, out.Size)...)
	if len(encrypted) == 0 {
		return nil, fmt.Errorf("Windows DPAPI returned no bytes")
	}
	return encrypted, nil
}
