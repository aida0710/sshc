// Package snippets stores reusable command templates and runs an explicitly
// previewed template against one or more SSH aliases.
//
// Command templates are configuration, not shell-escaping helpers. Variable
// values are substituted exactly as entered so the preview is the authority for
// what will run. Secret values are never persisted and are redacted from every
// public preview and result.
package snippets
