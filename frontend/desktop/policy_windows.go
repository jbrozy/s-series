package main

import (
	"syscall"
	"unsafe"
)

const (
	keyRead      = 0x20019
	keyWow64_64  = 0x0100
	regSZ        = 1
	regExpandSZ  = 2
	regTypeDWORD = 4
)

// readPolicy reads the presets in this order:
//
//	HKLM/HKCU\SOFTWARE\Policies\S-Check  ServerURL, LockServer (Gruppenrichtlinie, Intune)
//	HKLM\SOFTWARE\S-Check                ServerURL (MSI-Eigenschaft SERVERURL)
//	HKCU\SOFTWARE\S-Check                ServerURL (Installer-Parameter /SERVER=)
func readPolicy() Policy {
	var p Policy
	for _, root := range []syscall.Handle{syscall.HKEY_LOCAL_MACHINE, syscall.HKEY_CURRENT_USER} {
		if s := regString(root, `SOFTWARE\Policies\S-Check`, "ServerURL"); s != "" {
			if n, err := normalizeServer(s); err == nil {
				p.Server = n
				p.Locked = regDWORD(root, `SOFTWARE\Policies\S-Check`, "LockServer") == 1
				return p
			}
		}
	}
	for _, root := range []syscall.Handle{syscall.HKEY_LOCAL_MACHINE, syscall.HKEY_CURRENT_USER} {
		if s := regString(root, `SOFTWARE\S-Check`, "ServerURL"); s != "" {
			if n, err := normalizeServer(s); err == nil {
				p.Server = n
				return p
			}
		}
	}
	return p
}

func openKey(root syscall.Handle, path string) (syscall.Handle, bool) {
	var h syscall.Handle
	p, _ := syscall.UTF16PtrFromString(path)
	if syscall.RegOpenKeyEx(root, p, 0, keyRead|keyWow64_64, &h) != nil {
		return 0, false
	}
	return h, true
}

func regString(root syscall.Handle, path, name string) string {
	h, ok := openKey(root, path)
	if !ok {
		return ""
	}
	defer syscall.RegCloseKey(h)
	n, _ := syscall.UTF16PtrFromString(name)
	var typ, size uint32
	if syscall.RegQueryValueEx(h, n, nil, &typ, nil, &size) != nil || (typ != regSZ && typ != regExpandSZ) || size < 2 {
		return ""
	}
	buf := make([]uint16, size/2+1)
	if syscall.RegQueryValueEx(h, n, nil, &typ, (*byte)(unsafe.Pointer(&buf[0])), &size) != nil {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

func regDWORD(root syscall.Handle, path, name string) uint32 {
	h, ok := openKey(root, path)
	if !ok {
		return 0
	}
	defer syscall.RegCloseKey(h)
	n, _ := syscall.UTF16PtrFromString(name)
	var typ, v uint32
	size := uint32(4)
	if syscall.RegQueryValueEx(h, n, nil, &typ, (*byte)(unsafe.Pointer(&v)), &size) != nil || typ != regTypeDWORD {
		return 0
	}
	return v
}
