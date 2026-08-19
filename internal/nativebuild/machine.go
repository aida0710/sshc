package nativebuild

import (
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"fmt"
)

// VerifyBinaryArchitecture は、焼けた実行ファイルが本当にその行き先のものかを
// 見る。
//
// **名前は中身を保証しない。** `verify-artifact-name` が確かめているのは綴り
// だけであり、`sshc-linux-arm64` という名前の amd64 バイナリは、その検査を
// 通り抜ける。束に一つの実体を使い回せば、Linux の AppImage に macOS の
// バイナリが入る——**ビルドは通り、配ってから初めて壊れる。**
//
// この検査は、どのホストでも走る。arm64 の Windows を持っていなくても、
// arm64 の束に arm64 の CLI が入っていることは、ここで言える。走ることまでは
// 言えないが、間違ったものが入っていないことは言える。
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
