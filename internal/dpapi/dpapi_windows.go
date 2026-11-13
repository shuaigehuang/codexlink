//go:build windows

package dpapi

import (
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

const cryptProtectUIFobidden = 0x1

var (
	crypt32DLL             = syscall.NewLazyDLL("crypt32.dll")
	kernel32DLL            = syscall.NewLazyDLL("kernel32.dll")
	procCryptProtectData   = crypt32DLL.NewProc("CryptProtectData")
	procCryptUnprotectData = crypt32DLL.NewProc("CryptUnprotectData")
	procLocalFree          = kernel32DLL.NewProc("LocalFree")
)

type dataBlob struct {
	Size uint32
	Data *byte
}

func Protect(plain, entropy []byte) ([]byte, error) {
	if len(entropy) == 0 {
		return nil, errors.New("DPAPI entropy is empty")
	}
	return cryptData(plain, entropy, procCryptProtectData)
}

func Unprotect(cipher, entropy []byte) ([]byte, error) {
	if len(entropy) == 0 {
		return nil, errors.New("DPAPI entropy is empty")
	}
	return cryptData(cipher, entropy, procCryptUnprotectData)
}

func cryptData(input, entropy []byte, procedure *syscall.LazyProc) ([]byte, error) {
	if len(input) == 0 {
		return nil, errors.New("empty DPAPI data")
	}
	entropyBlob := dataBlob{
		Size: uint32(len(entropy)),
		Data: &entropy[0],
	}
	inputBlob := dataBlob{
		Size: uint32(len(input)),
		Data: &input[0],
	}
	var outputBlob dataBlob
	result, _, callErr := procedure.Call(
		uintptr(unsafe.Pointer(&inputBlob)),
		0,
		uintptr(unsafe.Pointer(&entropyBlob)),
		0,
		0,
		cryptProtectUIFobidden,
		uintptr(unsafe.Pointer(&outputBlob)),
	)
	runtime.KeepAlive(input)
	runtime.KeepAlive(entropy)
	if result == 0 {
		if callErr != nil && callErr != syscall.Errno(0) {
			return nil, callErr
		}
		return nil, errors.New("Windows DPAPI call failed")
	}
	if outputBlob.Data == nil || outputBlob.Size == 0 {
		return nil, errors.New("Windows DPAPI returned empty data")
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(outputBlob.Data)))
	output := append([]byte(nil), unsafe.Slice(outputBlob.Data, outputBlob.Size)...)
	if len(output) == 0 {
		return nil, fmt.Errorf("Windows DPAPI returned no bytes")
	}
	return output, nil
}
