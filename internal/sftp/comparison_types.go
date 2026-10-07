package sftp

type ComparisonMode string

const (
	ComparisonMetadata ComparisonMode = "metadata"
	ComparisonContent  ComparisonMode = "content"
	// Bound traffic from both sides while permitting individual large files
	// to be hashed without loading them into memory.
	maxContentComparisonBytes = 256 << 20
)

type ComparisonLocation struct {
	Alias string
	Path  string
}

type CompareOptions struct {
	Left  ComparisonLocation
	Right ComparisonLocation
	Mode  ComparisonMode
}
