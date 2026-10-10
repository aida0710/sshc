package sftp

import "os"

func removeLocalTextStagingFile(parent *os.Root, staging localTextStaging) {
	parent.Remove(staging.name)
}
