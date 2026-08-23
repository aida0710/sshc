package nativebuild

import (
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"fmt"
)

// VerifyBinaryArchitecture はバイナリヘッダーの OS と architecture を検証する。
// 実行検証ではないため、クロスコンパイルした成果物にも使用できる。
func VerifyBinaryArchitecture(path, goos, goarch string) error {
	switch goos {
	case "windows":
		return verifyPE(path, goarch)
	case "darwin":
		return verifyMachO(path, goarch)
	case "linux", "android":
		return verifyELF(path, goarch)
	default:
		return fmt.Errorf("no way to read the architecture of a %s binary", goos)
	}
}

func verifyPE(path, goarch string) error {
	file, err := pe.Open(path)
	if err != nil {
		return fmt.Errorf("read %s as a Windows binary: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	want, known := map[string]uint16{
		"amd64": pe.IMAGE_FILE_MACHINE_AMD64,
		"arm64": pe.IMAGE_FILE_MACHINE_ARM64,
		"386":   pe.IMAGE_FILE_MACHINE_I386,
	}[goarch]
	if !known {
		return fmt.Errorf("no Windows machine type is known for %s", goarch)
	}
	if file.Machine != want {
		return fmt.Errorf("%s is machine 0x%04x, want 0x%04x for %s", path, file.Machine, want, goarch)
	}
	return nil
}

func verifyMachO(path, goarch string) error {
	file, err := macho.Open(path)
	if err != nil {
		return fmt.Errorf("read %s as a macOS binary: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	want, known := map[string]macho.Cpu{
		"amd64": macho.CpuAmd64,
		"arm64": macho.CpuArm64,
	}[goarch]
	if !known {
		return fmt.Errorf("no macOS cpu type is known for %s", goarch)
	}
	if file.Cpu != want {
		return fmt.Errorf("%s is cpu %s, want %s for %s", path, file.Cpu, want, goarch)
	}
	return nil
}

func verifyELF(path, goarch string) error {
	file, err := elf.Open(path)
	if err != nil {
		return fmt.Errorf("read %s as an ELF binary: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	want, known := map[string]elf.Machine{
		"amd64": elf.EM_X86_64,
		"arm64": elf.EM_AARCH64,
	}[goarch]
	if !known {
		return fmt.Errorf("no ELF machine is known for %s", goarch)
	}
	if file.Machine != want {
		return fmt.Errorf("%s is machine %s, want %s for %s", path, file.Machine, want, goarch)
	}
	return nil
}
