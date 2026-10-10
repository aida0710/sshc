package sftp

import (
	"errors"
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

func openLocalTextStagingFile(parent *os.Root, name string) (localTextStaging, error) {
	directory, err := parent.Open(".")
	if err != nil {
		return localTextStaging{}, err
	}
	defer directory.Close()
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return localTextStaging{}, err
	}
	// Protect the sibling from inherited Readers/Users ACEs before any edited
	// bytes are written. Metadata is applied only after the complete write.
	descriptor, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;" + user.User.Sid.String() + ")(A;;FA;;;SY)")
	if err != nil {
		return localTextStaging{}, err
	}
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return localTextStaging{}, err
	}
	attributes := windows.OBJECT_ATTRIBUTES{
		RootDirectory: windows.Handle(directory.Fd()), ObjectName: objectName,
		Attributes: windows.OBJ_CASE_INSENSITIVE, SecurityDescriptor: descriptor,
	}
	attributes.Length = uint32(unsafe.Sizeof(attributes))
	var handle windows.Handle
	var status windows.IO_STATUS_BLOCK
	access := uint32(windows.FILE_GENERIC_READ | windows.FILE_GENERIC_WRITE | windows.WRITE_DAC | windows.WRITE_OWNER | windows.DELETE)
	err = windows.NtCreateFile(&handle, access, &attributes, &status, nil, windows.FILE_ATTRIBUTE_NORMAL,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_DELETE, windows.FILE_CREATE,
		windows.FILE_NON_DIRECTORY_FILE|windows.FILE_OPEN_REPARSE_POINT|windows.FILE_SYNCHRONOUS_IO_NONALERT, 0, 0)
	runtime.KeepAlive(descriptor)
	runtime.KeepAlive(directory)
	if err != nil {
		return localTextStaging{}, localWindowsFileError(err)
	}
	var publicationHandle windows.Handle
	process := windows.CurrentProcess()
	if err := windows.DuplicateHandle(process, handle, process, &publicationHandle, 0, false, windows.DUPLICATE_SAME_ACCESS); err != nil {
		cleanupError := markWindowsLocalTextStagingForDeletion(handle)
		windows.CloseHandle(handle)
		return localTextStaging{}, errors.Join(err, cleanupError)
	}
	// Keep the same file object through the common close-before-publish step.
	// Its share mode continues to refuse data writes; a renamed sibling still
	// publishes the same file object instead of a replacement at its old name.
	return localTextStaging{
		file: os.NewFile(uintptr(handle), name), name: name,
		publicationFile: os.NewFile(uintptr(publicationHandle), name),
	}, nil
}
