package launcher

import (
	"debug/macho"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"syscall"
)

// EnsureSupportedHost is deliberately pure; callers gate startup before any IO.
func EnsureSupportedHost(goos, goarch string) error {
	if goos != "darwin" || goarch != "arm64" {
		return errors.New("this launcher requires native macOS Apple Silicon arm64; Intel/Rosetta and other operating systems are unsupported")
	}
	return nil
}
func dataContractOpenRegular(path string) (*os.File, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	f := os.NewFile(uintptr(fd), path)
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() {
		f.Close()
		return nil, fmt.Errorf("not a regular file: %s", path)
	}
	return f, nil
}
func dataContractThinCPU(f *os.File, offset, size uint64) (uint32, error) {
	var header [32]byte
	if size < 28 {
		return 0, errors.New("truncated Mach-O header")
	}
	if _, err := f.ReadAt(header[:4], int64(offset)); err != nil {
		return 0, err
	}
	var order binary.ByteOrder
	headerSize := uint64(32)
	switch string(header[:4]) {
	case "\xcf\xfa\xed\xfe":
		order = binary.LittleEndian
	case "\xfe\xed\xfa\xcf":
		order = binary.BigEndian
	case "\xce\xfa\xed\xfe":
		order = binary.LittleEndian
		headerSize = 28
	case "\xfe\xed\xfa\xce":
		order = binary.BigEndian
		headerSize = 28
	default:
		return 0, errors.New("invalid Mach-O magic")
	}
	if size < headerSize {
		return 0, errors.New("truncated Mach-O header")
	}
	if _, err := f.ReadAt(header[:headerSize], int64(offset)); err != nil {
		return 0, err
	}
	cpu := order.Uint32(header[4:])
	kind := order.Uint32(header[12:])
	commands := order.Uint32(header[16:])
	commandSize := uint64(order.Uint32(header[20:]))
	if kind != 2 || commandSize > size-headerSize || uint64(commands) > commandSize/8 || (cpu == 0x0100000c && headerSize != 32) {
		return 0, errors.New("invalid Mach-O executable header")
	}
	// Bound all parser reads to this slice, including load-command referenced tables.
	if _, err := macho.NewFile(io.NewSectionReader(f, int64(offset), int64(size))); err != nil {
		return 0, err
	}
	return cpu, nil
}

// IsARM64Executable validates Mach-O headers and load commands, not signatures, dylibs or game compatibility.
// External universal executables are allowed; NW installation receipts are separate.
func IsARM64Executable(path string) bool {
	f, err := dataContractOpenRegular(path)
	if err != nil {
		return false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.Mode().Perm()&0111 == 0 || syscall.Access(path, 1) != nil {
		return false
	}
	var header [8]byte
	if _, err = f.ReadAt(header[:], 0); err != nil {
		return false
	}
	var order binary.ByteOrder
	entrySize := uint64(20)
	switch string(header[:4]) {
	case "\xca\xfe\xba\xbe":
		order = binary.BigEndian
	case "\xbe\xba\xfe\xca":
		order = binary.LittleEndian
	case "\xca\xfe\xba\xbf":
		order = binary.BigEndian
		entrySize = 32
	case "\xbf\xba\xfe\xca":
		order = binary.LittleEndian
		entrySize = 32
	default:
		cpu, e := dataContractThinCPU(f, 0, uint64(info.Size()))
		return e == nil && cpu == 0x0100000c
	}
	count := uint64(order.Uint32(header[4:]))
	tableEnd := 8 + count*entrySize
	total := uint64(info.Size())
	if count == 0 || tableEnd > total {
		return false
	}
	type span struct{ start, end uint64 }
	ranges := make([]span, 0)
	arches := make(map[[2]uint32]bool)
	found := false
	for i := uint64(0); i < count; i++ {
		var entry [32]byte
		if _, err = f.ReadAt(entry[:entrySize], int64(8+i*entrySize)); err != nil {
			return false
		}
		cpu := order.Uint32(entry[:])
		arch := [2]uint32{cpu, order.Uint32(entry[4:])}
		if arches[arch] {
			return false
		}
		arches[arch] = true
		offset := uint64(order.Uint32(entry[8:]))
		size := uint64(order.Uint32(entry[12:]))
		if entrySize == 32 {
			offset = order.Uint64(entry[8:])
			size = order.Uint64(entry[16:])
		}
		if offset < tableEnd || size < 28 || size > total || offset > total-size {
			return false
		}
		actual, e := dataContractThinCPU(f, offset, size)
		if e != nil || actual != cpu {
			return false
		}
		ranges = append(ranges, span{offset, offset + size})
		found = found || cpu == 0x0100000c
	}
	sort.Slice(ranges, func(i, j int) bool { return ranges[i].start < ranges[j].start })
	for i := 1; i < len(ranges); i++ {
		if ranges[i].start < ranges[i-1].end {
			return false
		}
	}
	return found
}
func RequireARM64(path string) error {
	if !IsARM64Executable(path) {
		return fmt.Errorf("unsupported runtime: executable Mach-O with arm64 support required: %s; existing files were not removed", path)
	}
	return nil
}

// RequireNWArm allows pre-receipt native ARM installations for discovery/launch.
// Installers must additionally require their own successful-install receipt.
func RequireNWArm(directory string) (string, error) {
	directory = filepath.Clean(directory)
	version := filepath.Base(directory)
	if !regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`).MatchString(version) {
		return "", errors.New("select a valid arm64 NW.js version (vX.Y.Z)")
	}
	binaryPath := filepath.Join(directory, "nwjs.app", "Contents", "MacOS", "nwjs")
	for _, p := range []string{directory, filepath.Join(directory, "nwjs.app"), filepath.Join(directory, "nwjs.app", "Contents"), filepath.Dir(binaryPath)} {
		info, err := os.Lstat(p)
		if err != nil {
			return "", err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("unsafe NW.js directory: %s", p)
		}
	}
	if _, err := os.Lstat(filepath.Join(directory, "run-with-rosetta")); err == nil {
		return "", errors.New("legacy Rosetta NW.js is unsupported; files preserved")
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	receipt := filepath.Join(directory, ".nwjs-install.json")
	if info, err := os.Lstat(receipt); err == nil {
		if !info.Mode().IsRegular() || info.Size() > 4096 {
			return "", errors.New("invalid NW.js receipt; files preserved")
		}
		f, err := dataContractOpenRegular(receipt)
		if err != nil {
			return "", err
		}
		b, err := io.ReadAll(io.LimitReader(f, 4097))
		f.Close()
		if err != nil {
			return "", err
		}
		if len(b) > 4096 {
			return "", errors.New("oversized NW.js receipt")
		}
		obj, err := dataContractObject(b)
		if err != nil {
			return "", err
		}
		if obj["version"] != version || obj["arch"] != "arm64" {
			return "", errors.New("legacy or invalid NW.js receipt; arm64 required")
		}
		if r, ok := obj["rosetta"]; ok {
			rosetta, valid := r.(bool)
			if !valid || rosetta {
				return "", errors.New("invalid Rosetta receipt")
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := RequireARM64(binaryPath); err != nil {
		return "", err
	}
	return binaryPath, nil
}
