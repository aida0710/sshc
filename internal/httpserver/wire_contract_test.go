package httpserver

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"sshc/internal/application"
	"sshc/internal/sftp"
	"sshc/internal/snippets"
	"sshc/internal/workspace"
)

// This test exercises the handwritten HTTP DTOs. Generated clients cannot
// catch a handler silently changing the server side of the wire boundary.
func TestHandwrittenHTTPWireTypesMatchOpenAPIRecursively(t *testing.T) {
	t.Parallel()

	document := readOpenAPIDocument(t)
	contracts := map[string]any{
		"Problem":                         problemPayload{},
		"HistoryList":                     historyList{},
		"TerminalBackground":              application.Background{},
		"TerminalBackgroundList":          backgroundListResponse{},
		"ChangedResponse":                 changedResponse{},
		"SFTPEntry":                       sftpEntry{},
		"SFTPListing":                     sftpListingResponse{},
		"SFTPTextFile":                    sftpTextFileResponse{},
		"SFTPSearchResult":                sftpSearchResponse{},
		"SFTPSaveTextRequest":             sftpSaveTextRequest{},
		"SFTPMkdirRequest":                sftpMkdirRequest{},
		"SFTPRenameRequest":               sftpRenameRequest{},
		"SFTPChmodRequest":                sftpChmodRequest{},
		"SFTPTransfer":                    sftpTransferResponse{},
		"SFTPTransferJob":                 sftpTransferJobResponse{},
		"SFTPTransferJobList":             sftpTransferJobListResponse{},
		"SFTPCreateTransferJobRequest":    sftpCreateTransferJobRequest{},
		"SFTPTransferJobActionRequest":    sftpTransferJobActionRequest{},
		"SFTPTransferSettingsRequest":     sftpTransferSettingsRequest{},
		"SFTPTransferQueueMoveRequest":    sftpTransferQueueMoveRequest{},
		"SFTPDownloadCheckpointRequest":   sftpDownloadCheckpointRequest{},
		"SFTPStartUploadRequest":          sftpStartUploadRequest{},
		"SFTPCompleteUploadRequest":       sftpCompleteUploadRequest{},
		"SFTPResumableUpload":             resumableUploadResponse{},
		"WorkspacePane":                   workspace.Pane{},
		"WorkspaceSplit":                  workspace.Split{},
		"WorkspaceNode":                   workspace.Node{},
		"WorkspaceDefinition":             workspaceDefinition{},
		"TerminalWorkspace":               workspace.Workspace{},
		"WorkspaceList":                   workspaceListResponse{},
		"WorkspaceReconnectPane":          workspace.PaneReconnect{},
		"WorkspaceRestorePlan":            workspace.RestorePlan{},
		"SnippetVariable":                 snippets.Variable{},
		"SnippetDraft":                    snippetDraft{},
		"Snippet":                         snippets.Snippet{},
		"StartupSnippet":                  snippets.Startup{},
		"StartupSnippetRequest":           startupSnippetRequest{},
		"SnippetLibrary":                  snippetLibrary{},
		"SnippetPreviewRequest":           snippets.PreviewRequest{},
		"SnippetExecutionTarget":          snippets.RequestedTarget{},
		"SnippetPreviewTarget":            snippets.TargetPreview{},
		"SnippetPreview":                  snippetPreviewResponse{},
		"SnippetExecuteRequest":           snippets.ExecuteRequest{},
		"SnippetTargetResult":             snippets.TargetResult{},
		"SnippetJob":                      snippets.Job{},
		"TerminalCommandTargetRequest":    terminalCommandTargetRequest{},
		"TerminalCommandPreviewRequest":   terminalCommandPreviewRequest{},
		"TerminalCommandDispatchRequest":  terminalCommandDispatchRequest{},
		"TerminalCommandPreviewTarget":    terminalCommandPreviewTarget{},
		"TerminalCommandPreview":          terminalCommandPreviewResponse{},
		"TerminalCommandResult":           terminalCommandResult{},
		"TerminalCommandDispatchResponse": terminalCommandDispatchResponse{},
	}

	names := make([]string, 0, len(contracts))
	for name := range contracts {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		name, value := name, contracts[name]
		t.Run(name, func(t *testing.T) {
			verifySchemaType(t, document, schemaReference(name), reflect.TypeOf(value), map[visit]bool{})
		})
	}
	assertEveryHandwrittenJSONStructIsClassified(t, contracts)
}

func assertEveryHandwrittenJSONStructIsClassified(t *testing.T, contracts map[string]any) {
	t.Helper()
	classified := map[string]bool{}
	for _, value := range contracts {
		typeID := reflect.TypeOf(value)
		for typeID.Kind() == reflect.Pointer {
			typeID = typeID.Elem()
		}
		if typeID.PkgPath() == "sshc/internal/httpserver" {
			classified[typeID.Name()] = true
		}
	}
	// These are real wire types on deliberately non-OpenAPI CLI or WebSocket
	// protocols. Listing them explicitly keeps those boundaries visible while
	// ensuring a newly added JSON struct cannot silently bypass this test.
	for _, name := range []string{
		"CLIStatus", "connectRequest", "connectResponse", "openResponse",
		"exitMessage", "resizeMessage", "vaultChangeRequest", "vaultPassphraseRequest",
	} {
		classified[name] = true
	}

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range parsed.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.TYPE {
				continue
			}
			for _, specification := range general.Specs {
				named := specification.(*ast.TypeSpec)
				structure, ok := named.Type.(*ast.StructType)
				if !ok || !structHasJSONTag(structure) || classified[named.Name.Name] {
					continue
				}
				t.Errorf("handwritten JSON struct %s is neither checked against OpenAPI nor classified as a non-OpenAPI protocol", named.Name.Name)
			}
		}
	}
}

func structHasJSONTag(structure *ast.StructType) bool {
	for _, field := range structure.Fields.List {
		if field.Tag == nil {
			continue
		}
		tag, err := strconv.Unquote(field.Tag.Value)
		if err == nil && reflect.StructTag(tag).Get("json") != "" {
			return true
		}
	}
	return false
}

type openAPIDocument struct {
	Components struct {
		Schemas map[string]map[string]any `yaml:"schemas"`
	} `yaml:"components"`
}

type visit struct {
	schema string
	typeID reflect.Type
}

type jsonField struct {
	typeID   reflect.Type
	required bool
}

func readOpenAPIDocument(t *testing.T) openAPIDocument {
	t.Helper()
	contents, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var document openAPIDocument
	if err := yaml.Unmarshal(contents, &document); err != nil {
		t.Fatal(err)
	}
	return document
}

func schemaReference(name string) map[string]any {
	return map[string]any{"$ref": "#/components/schemas/" + name}
}

func verifySchemaType(t *testing.T, document openAPIDocument, schema map[string]any, typeID reflect.Type, visited map[visit]bool) {
	t.Helper()
	for typeID.Kind() == reflect.Pointer {
		typeID = typeID.Elem()
	}
	identity := visit{schema: schemaIdentity(schema), typeID: typeID}
	if identity.schema != "" && visited[identity] {
		return
	}
	if identity.schema != "" {
		visited[identity] = true
	}

	resolved := resolveSchema(t, document, schema)
	schemaType, _ := resolved["type"].(string)
	if schemaType == "" && resolved["properties"] != nil {
		schemaType = "object"
	}
	switch schemaType {
	case "object":
		verifyObject(t, document, resolved, typeID, visited)
	case "array":
		if typeID.Kind() != reflect.Slice && typeID.Kind() != reflect.Array {
			t.Fatalf("OpenAPI array is represented by %v", typeID)
		}
		items, ok := resolved["items"].(map[string]any)
		if !ok {
			t.Fatal("array schema has no items")
		}
		verifySchemaType(t, document, items, typeID.Elem(), visited)
	case "string":
		if typeID != reflect.TypeOf(time.Time{}) && typeID.Kind() != reflect.String {
			t.Fatalf("OpenAPI string is represented by %v", typeID)
		}
		verifyEnum(t, resolved, typeID)
	case "integer":
		if typeID.Kind() < reflect.Int || typeID.Kind() > reflect.Uint64 {
			t.Fatalf("OpenAPI integer is represented by %v", typeID)
		}
		if format, _ := resolved["format"].(string); format == "int64" && typeID.Kind() != reflect.Int64 {
			t.Fatalf("OpenAPI int64 is represented by %v", typeID)
		}
	case "number":
		if typeID.Kind() != reflect.Float32 && typeID.Kind() != reflect.Float64 {
			t.Fatalf("OpenAPI number is represented by %v", typeID)
		}
		if format, _ := resolved["format"].(string); format == "double" && typeID.Kind() != reflect.Float64 {
			t.Fatalf("OpenAPI double is represented by %v", typeID)
		}
	case "boolean":
		if typeID.Kind() != reflect.Bool {
			t.Fatalf("OpenAPI boolean is represented by %v", typeID)
		}
	default:
		t.Fatalf("unsupported or missing OpenAPI type %q in %#v", schemaType, resolved)
	}
}

func verifyObject(t *testing.T, document openAPIDocument, schema map[string]any, typeID reflect.Type, visited map[visit]bool) {
	t.Helper()
	if additional, exists := schema["additionalProperties"]; exists && additional != false {
		if typeID.Kind() != reflect.Map || typeID.Key().Kind() != reflect.String {
			t.Fatalf("OpenAPI string map is represented by %v", typeID)
		}
		valueSchema, ok := additional.(map[string]any)
		if !ok {
			t.Fatalf("unsupported additionalProperties %#v", additional)
		}
		verifySchemaType(t, document, valueSchema, typeID.Elem(), visited)
		return
	}
	if typeID.Kind() != reflect.Struct {
		t.Fatalf("OpenAPI object is represented by %v", typeID)
	}
	properties, _ := schema["properties"].(map[string]any)
	required := stringSet(schema["required"])
	fields := collectJSONFields(t, typeID)
	for name, property := range properties {
		field, ok := fields[name]
		if !ok {
			t.Errorf("OpenAPI property %q has no Go JSON field", name)
			continue
		}
		if field.required != required[name] {
			t.Errorf("property %q required=%t but Go omitempty implies required=%t", name, required[name], field.required)
		}
		propertySchema, ok := property.(map[string]any)
		if !ok {
			t.Errorf("property %q has invalid schema %#v", name, property)
			continue
		}
		verifySchemaType(t, document, propertySchema, field.typeID, visited)
	}
	for name := range fields {
		if _, ok := properties[name]; !ok {
			t.Errorf("Go JSON field %q is absent from OpenAPI", name)
		}
	}
}

func collectJSONFields(t *testing.T, typeID reflect.Type) map[string]jsonField {
	t.Helper()
	fields := make(map[string]jsonField)
	for index := 0; index < typeID.NumField(); index++ {
		field := typeID.Field(index)
		tag := field.Tag.Get("json")
		name, options, _ := strings.Cut(tag, ",")
		if name == "-" {
			continue
		}
		if field.Anonymous && name == "" {
			embedded := field.Type
			for embedded.Kind() == reflect.Pointer {
				embedded = embedded.Elem()
			}
			if embedded.Kind() != reflect.Struct {
				t.Fatalf("unsupported anonymous JSON field %v", field.Type)
			}
			for embeddedName, embeddedField := range collectJSONFields(t, embedded) {
				if _, duplicate := fields[embeddedName]; duplicate {
					t.Fatalf("duplicate JSON field %q", embeddedName)
				}
				fields[embeddedName] = embeddedField
			}
			continue
		}
		if name == "" {
			name = field.Name
		}
		if _, duplicate := fields[name]; duplicate {
			t.Fatalf("duplicate JSON field %q", name)
		}
		fields[name] = jsonField{typeID: field.Type, required: !hasJSONOption(options, "omitempty")}
	}
	return fields
}

func hasJSONOption(options, wanted string) bool {
	for _, option := range strings.Split(options, ",") {
		if option == wanted {
			return true
		}
	}
	return false
}

func resolveSchema(t *testing.T, document openAPIDocument, schema map[string]any) map[string]any {
	t.Helper()
	resolved := make(map[string]any)
	if reference, ok := schema["$ref"].(string); ok {
		const prefix = "#/components/schemas/"
		name := strings.TrimPrefix(reference, prefix)
		if name == reference {
			t.Fatalf("unsupported schema reference %q", reference)
		}
		definition, exists := document.Components.Schemas[name]
		if !exists {
			t.Fatalf("unknown schema reference %q", reference)
		}
		mergeSchema(t, document, resolved, definition)
	}
	mergeSchema(t, document, resolved, schema)
	return resolved
}

func mergeSchema(t *testing.T, document openAPIDocument, destination, source map[string]any) {
	t.Helper()
	if allOf, ok := source["allOf"].([]any); ok {
		for _, part := range allOf {
			partSchema, ok := part.(map[string]any)
			if !ok {
				t.Fatalf("invalid allOf part %#v", part)
			}
			mergeResolvedSchema(destination, resolveSchema(t, document, partSchema))
		}
	}
	copy := make(map[string]any)
	for key, value := range source {
		if key != "$ref" && key != "allOf" {
			copy[key] = value
		}
	}
	mergeResolvedSchema(destination, copy)
}

func mergeResolvedSchema(destination, source map[string]any) {
	for key, value := range source {
		switch key {
		case "properties":
			properties, _ := destination[key].(map[string]any)
			if properties == nil {
				properties = make(map[string]any)
				destination[key] = properties
			}
			for name, property := range value.(map[string]any) {
				properties[name] = property
			}
		case "required":
			required := append([]any{}, anySlice(destination[key])...)
			required = append(required, anySlice(value)...)
			destination[key] = required
		default:
			destination[key] = value
		}
	}
}

func anySlice(value any) []any {
	items, _ := value.([]any)
	return items
}

func stringSet(value any) map[string]bool {
	result := make(map[string]bool)
	for _, item := range anySlice(value) {
		if text, ok := item.(string); ok {
			result[text] = true
		}
	}
	return result
}

func schemaIdentity(schema map[string]any) string {
	if reference, ok := schema["$ref"].(string); ok {
		return reference
	}
	return ""
}

func verifyEnum(t *testing.T, schema map[string]any, typeID reflect.Type) {
	t.Helper()
	items := anySlice(schema["enum"])
	if len(items) == 0 {
		return
	}
	actual, ok := wireEnumValues[typeID]
	if !ok {
		t.Fatalf("OpenAPI enum is represented by unregistered Go type %v", typeID)
	}
	expected := make([]string, 0, len(items))
	for _, item := range items {
		expected = append(expected, fmt.Sprint(item))
	}
	sort.Strings(expected)
	actual = append([]string(nil), actual...)
	sort.Strings(actual)
	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("enum differs: OpenAPI=%v Go=%v", expected, actual)
	}
}

var wireEnumValues = map[reflect.Type][]string{
	reflect.TypeOf(sftp.EntryType("")): {
		string(sftp.EntryFile), string(sftp.EntryDirectory), string(sftp.EntrySymlink), string(sftp.EntryOther),
	},
	reflect.TypeOf(sftp.TransferDirection("")): {
		string(sftp.TransferUpload), string(sftp.TransferDownload), string(sftp.TransferRemote),
	},
	reflect.TypeOf(sftp.RemoteTransferOperation("")): {
		"", string(sftp.RemoteCopy), string(sftp.RemoteMove),
	},
	reflect.TypeOf(sftp.DirectoryDifferenceStatus("")): {
		string(sftp.DirectorySame), string(sftp.DirectoryDifferent), string(sftp.DirectoryLeftOnly),
		string(sftp.DirectoryRightOnly), string(sftp.DirectoryTypeMismatch),
	},
	reflect.TypeOf(sftp.TransferKind("")): {
		string(sftp.TransferFile), string(sftp.TransferFolder),
	},
	reflect.TypeOf(sftp.TransferJobStatus("")): {
		string(sftp.TransferQueued), string(sftp.TransferRunning), string(sftp.TransferPaused), string(sftp.TransferReattach),
		string(sftp.TransferNeedsOverwrite), string(sftp.TransferCompleted), string(sftp.TransferFailed), string(sftp.TransferCancelled),
	},
	reflect.TypeOf(sftp.TransferQueueMove("")): {
		string(sftp.TransferMoveUp), string(sftp.TransferMoveDown),
		string(sftp.TransferMoveTop), string(sftp.TransferMoveBottom),
	},
	reflect.TypeOf(sftp.TransferJobAction("")): {
		string(sftp.TransferStartAction), string(sftp.TransferPauseAction), string(sftp.TransferResumeAction),
		string(sftp.TransferRetryAction), string(sftp.TransferCancelAction), string(sftp.TransferProgressAction),
		string(sftp.TransferCompleteAction), string(sftp.TransferFailAction), string(sftp.TransferNeedsOverwriteAction),
	},
	reflect.TypeOf(sftp.TransferControlAction("")): {
		string(sftp.TransferPauseControl), string(sftp.TransferResumeControl),
		string(sftp.TransferRetryControl), string(sftp.TransferCancelControl), string(sftp.TransferRemoveControl),
	},
	reflect.TypeOf(sftpMkdirType("")):            {string(sftpMkdirDirectory)},
	reflect.TypeOf(workspace.Direction("")):      {string(workspace.Horizontal), string(workspace.Vertical)},
	reflect.TypeOf(workspace.PaneKind("")):       {string(workspace.PaneSSH), string(workspace.PaneShell)},
	reflect.TypeOf(workspace.ReconnectState("")): {string(workspace.ReconnectRequired)},
	reflect.TypeOf(snippets.VariableType("")): {
		string(snippets.VariableString), string(snippets.VariableInteger), string(snippets.VariableBoolean), string(snippets.VariableSecret),
	},
	reflect.TypeOf(snippets.TargetStatus("")): {
		string(snippets.TargetQueued), string(snippets.TargetRunning), string(snippets.TargetSucceeded),
		string(snippets.TargetFailed), string(snippets.TargetCancelled),
	},
	reflect.TypeOf(snippets.JobStatus("")): {
		string(snippets.JobRunning), string(snippets.JobCompleted), string(snippets.JobCancelled),
	},
	reflect.TypeOf(terminalCommandStatus("")): {
		string(terminalCommandDelivered), string(terminalCommandFailed),
	},
}
