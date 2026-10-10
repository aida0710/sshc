package sftp

import (
	"encoding/json"
	"errors"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Replacing the data does not change these ordinary file attributes. EFS,
// compression, sparse and cloud/reparse files need their own preservation APIs.
const windowsLocalTextOrdinaryAttributes = windows.FILE_ATTRIBUTE_ARCHIVE |
	windows.FILE_ATTRIBUTE_HIDDEN | windows.FILE_ATTRIBUTE_SYSTEM |
	windows.FILE_ATTRIBUTE_NORMAL | windows.FILE_ATTRIBUTE_TEMPORARY |
	windows.FILE_ATTRIBUTE_NOT_CONTENT_INDEXED

const windowsLocalTextSecurityInformation = windows.OWNER_SECURITY_INFORMATION |
	windows.GROUP_SECURITY_INFORMATION | windows.DACL_SECURITY_INFORMATION

// These SACL entries can be queried without enabling audit privileges. Labels
// and resource attributes can also be set through the staging handle; central
// access policies require ACCESS_SYSTEM_SECURITY and must instead be refused.
const windowsLocalTextAdditionalSecurityInformation = windows.LABEL_SECURITY_INFORMATION | windows.ATTRIBUTE_SECURITY_INFORMATION

const (
	windowsLocalTextMandatoryLabelACE    = 0x11
	windowsLocalTextResourceAttributeACE = 0x12
)

type windowsLocalTextMetadata struct {
	SecurityDescriptor string
	Attributes         uint32
	CreationTime       windows.Filetime
}

type windowsLocalTextBasicInformation struct {
	CreationTime   int64
	LastAccessTime int64
	LastWriteTime  int64
	ChangeTime     int64
	FileAttributes uint32
}

func setWindowsLocalTextAttributes(handle windows.Handle, attributes uint32) error {
	// Zero times leave timestamps alone. Keep creation time while allowing the
	// edited contents to have their new write time.
	native := windowsLocalTextBasicInformation{FileAttributes: attributes}
	return windows.SetFileInformationByHandle(handle, windows.FileBasicInfo,
		(*byte)(unsafe.Pointer(&native)), uint32(unsafe.Sizeof(native)))
}

func captureLocalTextMetadata(file *os.File) (localTextMetadata, error) {
	info, err := file.Stat()
	if err != nil {
		return localTextMetadata{}, err
	}
	if !info.Mode().IsRegular() {
		return localTextMetadata{}, ErrUnsupportedEntry
	}
	handle := windows.Handle(file.Fd())
	var native windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &native); err != nil {
		return localTextMetadata{}, err
	}
	descriptor, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT,
		windowsLocalTextSecurityInformation|windowsLocalTextAdditionalSecurityInformation|windows.SCOPE_SECURITY_INFORMATION)
	if err != nil {
		return localTextMetadata{}, err
	}
	security := descriptor.String()
	if security == "" {
		return localTextMetadata{}, ErrUnsupportedEntry
	}
	platform, err := json.Marshal(windowsLocalTextMetadata{
		SecurityDescriptor: security, Attributes: native.FileAttributes, CreationTime: native.CreationTime,
	})
	if err != nil {
		return localTextMetadata{}, err
	}
	streams, err := readWindowsLocalTextStreams(file)
	if err != nil {
		return localTextMetadata{}, err
	}
	metadata := localTextMetadata{mode: info.Mode(), extendedAttributes: streams, platformMetadata: platform}
	metadata.revision = localTextMetadataRevision(metadata)
	return metadata, nil
}

func applyLocalTextMetadata(_ *os.File, staged *os.File, snapshot localTextMetadata) error {
	var platform windowsLocalTextMetadata
	if err := json.Unmarshal(snapshot.platformMetadata, &platform); err != nil {
		return err
	}
	if platform.Attributes & ^uint32(windowsLocalTextOrdinaryAttributes) != 0 {
		return ErrUnsupportedEntry
	}
	if err := writeWindowsLocalTextStreams(staged, snapshot.extendedAttributes); err != nil {
		return err
	}
	if err := applyWindowsLocalTextSecurity(staged, platform.SecurityDescriptor); err != nil {
		return err
	}
	handle := windows.Handle(staged.Fd())
	if err := windows.SetFileTime(handle, &platform.CreationTime, nil, nil); err != nil {
		return err
	}
	return setWindowsLocalTextAttributes(handle, platform.Attributes)
}

func applyWindowsLocalTextSecurity(file *os.File, sddl string) error {
	descriptor, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return err
	}
	owner, _, err := descriptor.Owner()
	if err != nil {
		return err
	}
	group, _, err := descriptor.Group()
	if err != nil {
		return err
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		return err
	}
	control, _, err := descriptor.Control()
	if err != nil {
		return err
	}
	sacl, _, err := descriptor.SACL()
	if errors.Is(err, windows.ERROR_OBJECT_NOT_FOUND) {
		err = nil
	}
	if err != nil {
		return err
	}
	if control&windows.SE_SACL_PROTECTED != 0 {
		return ErrUnsupportedEntry
	}
	if err := validateWindowsLocalTextSACL(sacl); err != nil {
		return err
	}
	security := windows.SECURITY_INFORMATION(windowsLocalTextSecurityInformation | windowsLocalTextAdditionalSecurityInformation)
	if control&windows.SE_DACL_PROTECTED != 0 {
		security |= windows.PROTECTED_DACL_SECURITY_INFORMATION
	} else {
		security |= windows.UNPROTECTED_DACL_SECURITY_INFORMATION
	}
	return windows.SetSecurityInfo(windows.Handle(file.Fd()), windows.SE_FILE_OBJECT, security, owner, group, dacl, sacl)
}

func validateWindowsLocalTextSACL(sacl *windows.ACL) error {
	if sacl == nil {
		return nil
	}
	for index := uint32(0); index < uint32(sacl.AceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(sacl, index, &ace); err != nil {
			return err
		}
		switch ace.Header.AceType {
		case windowsLocalTextMandatoryLabelACE, windowsLocalTextResourceAttributeACE:
		default:
			return ErrUnsupportedEntry
		}
	}
	return nil
}
