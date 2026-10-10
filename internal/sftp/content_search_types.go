package sftp

type SearchMode string

const (
	SearchName    SearchMode = "name"
	SearchContent SearchMode = "content"
)

type SearchOptions struct {
	Alias string
	Path  string
	Query string
	Mode  SearchMode
}

type ContentMatch struct {
	Entry   Entry
	Line    int
	Snippet string
}

type SearchOmission struct {
	Reason string
	Count  int
}
