"""Header-only Mach-O fixtures; never execute these files."""
import struct

ARM64 = 0x0100000C
X64 = 0x01000007


def thin(cpu=ARM64, endian="<"):
    return struct.pack(endian + "8I", 0xFEEDFACF, cpu, 0, 2, 0, 0, 0, 0)


def fat(cpus=(X64, ARM64), endian=">", wide=False):
    entry_size = 32 if wide else 20
    offset = 8 + entry_size * len(cpus)
    entries, bodies = [], []
    for cpu in cpus:
        body = thin(cpu)
        values = (cpu, 0, offset, len(body), 0)
        if wide:
            values += (0,)
        entries.append(struct.pack(endian + ("IIQQII" if wide else "5I"), *values))
        bodies.append(body)
        offset += len(body)
    return struct.pack(endian + "II", 0xCAFEBABF if wide else 0xCAFEBABE, len(cpus)) + b"".join(entries + bodies)


def executable(path, data=None):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_bytes(thin() if data is None else data)
    path.chmod(0o700)
    return path
