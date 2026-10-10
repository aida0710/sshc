package sftp

// Stable reason keys let clients explain partial results in their own language.
const (
	omissionSymlink     = "symlink"
	omissionUnsupported = "unsupported"
	omissionBinary      = "binary"
	omissionFileSize    = "file_size"
	omissionUnreadable  = "unreadable"
	omissionChanged     = "changed"
	omissionByteLimit   = "byte_limit"
	omissionResultLimit = "result_limit"
	omissionEntryLimit  = "entry_limit"
	omissionDepthLimit  = "depth_limit"
)
