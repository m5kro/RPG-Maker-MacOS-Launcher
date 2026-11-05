package launcher

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

const dataContractARM uint32 = 0x0100000c
const dataContractIntel uint32 = 0x01000007

func dataContractThin(cpu uint32, order binary.ByteOrder) []byte {
	b := make([]byte, 32)
	for i, v := range []uint32{0xfeedfacf, cpu, 0, 2, 0, 0, 0, 0} {
		order.PutUint32(b[i*4:], v)
	}
	return b
}
func dataContractFat(order binary.ByteOrder, wide bool, cpus ...uint32) []byte {
	slices := make([][]byte, len(cpus))
	for i, cpu := range cpus {
		slices[i] = dataContractThin(cpu, binary.LittleEndian)
	}
	return dataContractFatSlices(order, wide, slices...)
}
func dataContractFatSlices(order binary.ByteOrder, wide bool, slices ...[]byte) []byte {
	size := 20
	magic := uint32(0xcafebabe)
	if wide {
		size = 32
		magic = 0xcafebabf
	}
	offset := 8 + size*len(slices)
	b := make([]byte, offset)
	order.PutUint32(b, magic)
	order.PutUint32(b[4:], uint32(len(slices)))
	for i, slice := range slices {
		var thinOrder binary.ByteOrder = binary.LittleEndian
		if slice[0] == 0xfe {
			thinOrder = binary.BigEndian
		}
		e := b[8+i*size:]
		order.PutUint32(e, thinOrder.Uint32(slice[4:]))
		order.PutUint32(e[4:], thinOrder.Uint32(slice[8:]))
		if wide {
			order.PutUint64(e[8:], uint64(offset))
			order.PutUint64(e[16:], uint64(len(slice)))
		} else {
			order.PutUint32(e[8:], uint32(offset))
			order.PutUint32(e[12:], uint32(len(slice)))
		}
		b = append(b, slice...)
		offset += len(slice)
	}
	return b
}
func dataContractExecutable(t *testing.T, p string, b []byte) {
	t.Helper()
	dataContractPut(t, p, b)
	if e := os.Chmod(p, 0700); e != nil {
		t.Fatal(e)
	}
}
func TestDataContractARMThinAndUniversal(t *testing.T) {
	p := filepath.Join(t.TempDir(), "runtime")
	for _, o := range []binary.ByteOrder{binary.BigEndian, binary.LittleEndian} {
		for _, b := range [][]byte{dataContractThin(dataContractARM, o), dataContractFat(o, false, dataContractIntel, dataContractARM), dataContractFat(o, true, dataContractIntel, dataContractARM)} {
			dataContractExecutable(t, p, b)
			if !IsARM64Executable(p) {
				t.Fatal("rejected valid ARM")
			}
			if e := RequireARM64(p); e != nil {
				t.Fatal(e)
			}
		}
	}
}
func TestDataContractARMInvalidFiles(t *testing.T) {
	p := filepath.Join(t.TempDir(), "runtime")
	if IsARM64Executable(p) {
		t.Fatal("missing")
	}
	for _, b := range [][]byte{dataContractThin(dataContractIntel, binary.LittleEndian), dataContractFat(binary.BigEndian, false, dataContractIntel), []byte("MZ Windows executable")} {
		dataContractExecutable(t, p, b)
		if IsARM64Executable(p) {
			t.Fatal("invalid accepted")
		}
	}
	dataContractExecutable(t, p, dataContractThin(dataContractARM, binary.LittleEndian))
	os.Chmod(p, 0600)
	if IsARM64Executable(p) {
		t.Fatal("non executable")
	}
	os.Chmod(p, 0700)
	link := p + "-link"
	os.Symlink(p, link)
	if IsARM64Executable(link) || IsARM64Executable(filepath.Dir(p)) {
		t.Fatal("link/directory")
	}
	fifo := p + "-fifo"
	if e := syscall.Mkfifo(fifo, 0600); e != nil {
		t.Fatal(e)
	}
	if IsARM64Executable(fifo) {
		t.Fatal("FIFO")
	}
}
func TestDataContractARMMalformed(t *testing.T) {
	thin := dataContractThin(dataContractARM, binary.LittleEndian)
	fat := dataContractFat(binary.BigEndian, false, dataContractIntel, dataContractARM)
	bad := [][]byte{nil, []byte("fake"), thin[:31], thin[:4], fat[:47], fat[:len(fat)-1], {0xca, 0xfe, 0xba, 0xbe, 0, 0, 0, 0}, {0xca, 0xfe, 0xba, 0xbe, 255, 255, 255, 255}}
	for _, pair := range [][2]uint32{{0, 32}, {8, 32}, {28, 0}, {28, 31}, {29, 32}, {0xffffffff, 32}, {28, 0xffffffff}} {
		b := dataContractFat(binary.BigEndian, false, dataContractARM)
		binary.BigEndian.PutUint32(b[16:], pair[0])
		binary.BigEndian.PutUint32(b[20:], pair[1])
		bad = append(bad, b)
	}
	for _, pair := range [][2]uint64{{0, 32}, {40, 0}, {40, 31}, {41, 32}, {^uint64(0), 32}, {40, ^uint64(0)}} {
		b := dataContractFat(binary.BigEndian, true, dataContractARM)
		binary.BigEndian.PutUint64(b[16:], pair[0])
		binary.BigEndian.PutUint64(b[24:], pair[1])
		bad = append(bad, b)
	}
	overlap := append([]byte(nil), fat...)
	binary.BigEndian.PutUint32(overlap[36:], 48)
	bad = append(bad, overlap)
	mismatch := dataContractFat(binary.BigEndian, false, dataContractARM)
	copy(mismatch[28:], dataContractThin(dataContractIntel, binary.LittleEndian))
	bad = append(bad, mismatch)
	for _, pair := range [][2]uint32{{12, 6}, {16, 1}, {20, 32}} {
		b := append([]byte(nil), thin...)
		binary.LittleEndian.PutUint32(b[pair[0]:], pair[1])
		bad = append(bad, b)
	}
	p := filepath.Join(t.TempDir(), "runtime")
	for i, b := range bad {
		dataContractExecutable(t, p, b)
		if IsARM64Executable(p) {
			t.Fatalf("accepted malformed %d", i)
		}
		if e := RequireARM64(p); e == nil {
			t.Fatal("require accepted malformed")
		}
	}
}
func TestDataContractARMValidatorUnification(t *testing.T) {
	check := func(t *testing.T, b []byte, valid bool) {
		t.Helper()
		p := filepath.Join(t.TempDir(), "nwjs.app", "Contents", "MacOS", "nwjs")
		dataContractExecutable(t, p, b)
		for name, validate := range map[string]func(string) error{"RequireARM64": RequireARM64, "installer": validateMachO} {
			t.Run(name, func(t *testing.T) {
				if err := validate(p); (err == nil) != valid {
					t.Errorf("valid=%v, error=%v", valid, err)
				}
			})
		}
		if err := validateBundle(context.Background(), filepath.Join(p, "../../..")); (err == nil) != valid {
			t.Errorf("installer bundle: valid=%v, error=%v", valid, err)
		}
		if !bytes.Equal(dataContractRead(t, p), b) {
			t.Fatal("validation changed executable")
		}
	}
	for _, order := range []binary.ByteOrder{binary.BigEndian, binary.LittleEndian} {
		t.Run(order.String(), func(t *testing.T) {
			thin := dataContractThin(dataContractARM, order)
			// LC_SEGMENT_64 with no sections is structurally valid, not an OS launchability fixture.
			segment := append(bytes.Clone(thin), make([]byte, 72)...)
			order.PutUint32(segment[16:], 1)
			order.PutUint32(segment[20:], 72)
			order.PutUint32(segment[32:], 0x19)
			order.PutUint32(segment[36:], 72)
			zeroSize := append(bytes.Clone(thin), make([]byte, 8)...)
			order.PutUint32(zeroSize[16:], 1)
			order.PutUint32(zeroSize[20:], 8)
			order.PutUint32(zeroSize[32:], 1) // Exact review probe: LC_SEGMENT with cmdsize=0.
			shortSegment := bytes.Clone(zeroSize)
			order.PutUint32(shortSegment[36:], 8)
			longCommand := bytes.Clone(zeroSize)
			order.PutUint32(longCommand[36:], 16)
			hugeCommands := bytes.Clone(thin)
			order.PutUint32(hugeCommands[16:], ^uint32(0))
			order.PutUint32(hugeCommands[20:], ^uint32(0))
			hugeSize := bytes.Clone(thin)
			order.PutUint32(hugeSize[20:], ^uint32(0))
			for _, tc := range []struct {
				name  string
				data  []byte
				valid bool
			}{
				{"header-only", thin, true},
				{"segment", segment, true},
				{"zero-command-size", zeroSize, false},
				{"short-segment", shortSegment, false},
				{"command-outside-sizeofcmds", longCommand, false},
				{"huge-ncmds-sizeofcmds", hugeCommands, false},
				{"huge-sizeofcmds", hugeSize, false},
			} {
				t.Run("thin/"+tc.name, func(t *testing.T) { check(t, tc.data, tc.valid) })
				for _, wide := range []bool{false, true} {
					t.Run(fmt.Sprintf("fat64=%v/%s", wide, tc.name), func(t *testing.T) {
						check(t, dataContractFatSlices(order, wide, dataContractThin(dataContractIntel, order), tc.data), tc.valid)
					})
				}
			}
			for _, wide := range []bool{false, true} {
				t.Run(fmt.Sprintf("fat64=%v", wide), func(t *testing.T) {
					t.Run("declared-cpu-mismatch", func(t *testing.T) {
						b := dataContractFatSlices(order, wide, dataContractThin(dataContractIntel, order))
						order.PutUint32(b[8:], dataContractARM)
						check(t, b, false)
					})
					t.Run("duplicate-architecture", func(t *testing.T) { check(t, dataContractFatSlices(order, wide, thin, thin), false) })
					t.Run("malformed-non-arm-slice", func(t *testing.T) {
						b := bytes.Clone(shortSegment)
						order.PutUint32(b[4:], dataContractIntel)
						check(t, dataContractFatSlices(order, wide, thin, b), false)
					})
					t.Run("32-bit-intel-slice", func(t *testing.T) {
						b := dataContractThin(7, order)[:28]
						order.PutUint32(b, 0xfeedface)
						check(t, dataContractFatSlices(order, wide, b, thin), true)
					})
					t.Run("symtab-outside-slice", func(t *testing.T) {
						b := append(bytes.Clone(thin), make([]byte, 24)...)
						order.PutUint32(b[16:], 1)
						order.PutUint32(b[20:], 24)
						order.PutUint32(b[32:], 2) // LC_SYMTAB string bytes exist only in the next slice.
						order.PutUint32(b[36:], 24)
						order.PutUint32(b[48:], uint32(len(b)))
						order.PutUint32(b[52:], 1)
						check(t, dataContractFatSlices(order, wide, b, dataContractThin(dataContractIntel, order)), false)
					})
				})
			}
		})
	}
}

func TestDataContractHostGateIsPure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, name := range []string{"log.txt", "settings.json"} {
		dataContractPut(t, filepath.Join(DefaultRoot(home), name), []byte("keep "+name))
	}
	for _, pair := range [][2]string{{"linux", "arm64"}, {"darwin", "amd64"}, {"windows", "arm64"}, {"darwin", "x86_64"}} {
		if e := EnsureSupportedHost(pair[0], pair[1]); e == nil {
			t.Fatal(pair)
		}
	}
	if e := EnsureSupportedHost("darwin", "arm64"); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"log.txt", "settings.json"} {
		if string(dataContractRead(t, filepath.Join(DefaultRoot(home), name))) != "keep "+name {
			t.Fatal("host gate changed HOME", name)
		}
	}
}
func TestDataContractNWLegacyAndReceipt(t *testing.T) {
	root := filepath.Join(t.TempDir(), "v1.2.3")
	p := filepath.Join(root, "nwjs.app/Contents/MacOS/nwjs")
	dataContractExecutable(t, p, dataContractThin(dataContractARM, binary.LittleEndian))
	if got, e := RequireNWArm(root); e != nil || got != p {
		t.Fatal(got, e)
	}
	receipt := filepath.Join(root, ".nwjs-install.json")
	for _, raw := range []string{`{"version":"v1.2.3","arch":"arm64","rosetta":false}`, `{"version":"v1.2.3","arch":"arm64"}`} {
		dataContractPut(t, receipt, []byte(raw))
		if _, e := RequireNWArm(root); e != nil {
			t.Fatal(e)
		}
	}
	for _, raw := range []string{`{bad`, `[]`, `null`, `{"version":"v1.2.3","arch":"x64"}`, `{"version":"v1.2.3","arch":"arm64","rosetta":true}`, `{"version":"v1.2.3","arch":"arm64","rosetta":null}`, `{"version":"v9.9.9","arch":"arm64"}`} {
		dataContractPut(t, receipt, []byte(raw))
		if _, e := RequireNWArm(root); e == nil {
			t.Fatal(raw)
		}
		if string(dataContractRead(t, receipt)) != raw {
			t.Fatal("receipt changed")
		}
	}
	os.Remove(receipt)
	dataContractPut(t, filepath.Join(root, "run-with-rosetta"), nil)
	if _, e := RequireNWArm(root); e == nil {
		t.Fatal("Rosetta marker")
	}
	os.Remove(filepath.Join(root, "run-with-rosetta"))
	os.Symlink(p, receipt)
	if _, e := RequireNWArm(root); e == nil {
		t.Fatal("receipt symlink")
	}
	os.Remove(receipt)
	dataContractPut(t, receipt, make([]byte, 4097))
	if _, e := RequireNWArm(root); e == nil {
		t.Fatal("large receipt")
	}
	os.Remove(receipt)
	link := filepath.Join(t.TempDir(), "v1.2.3")
	os.Symlink(root, link)
	if _, e := RequireNWArm(link); e == nil {
		t.Fatal("version symlink")
	}
	if _, e := RequireNWArm(filepath.Join(root, "..")); e == nil {
		t.Fatal("invalid version")
	}
}
