package httpserver

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"sshc/internal/api"
	"sshc/internal/api/contracttest"
	"sshc/internal/application"
	"sshc/internal/sftp"
	"sshc/internal/snippets"
	"sshc/internal/vpn"
	"sshc/internal/workspace"
)

// This test exercises the handwritten HTTP DTOs. Generated clients cannot
// catch a handler silently changing the server side of the wire boundary.
func TestHandwrittenHTTPWireTypesMatchOpenAPIRecursively(t *testing.T) {
	t.Parallel()

	contract := contracttest.Contract{Document: contracttest.ReadDocument(t, filepath.Join("..", "..")), Enums: wireEnumValues}
	contracts := map[string]any{
		"Problem":                         problemPayload{},
		"VPNOverview":                     VPNOverview{},
		"VPNProfileStatus":                VPNProfileStatus{},
		"VPNTunnel":                       VPNTunnel{},
		"VPNLogs":                         VPNLogs{},
		"VPNProfileRequest":               VPNProfileRequest{},
		"VPNSecrets":                      vpn.SecretsDocument{},
		"VPNBindingRequest":               VPNBindingRequest{},
		"VPNRenameRequest":                VPNRenameRequest{},
		"HistoryList":                     historyList{},
		"TerminalBackground":              application.Background{},
		"TerminalBackgroundList":          backgroundListResponse{},
		"ChangedResponse":                 changedResponse{},
		"SFTPListing":                     SFTPListing{},
		"SFTPTextFile":                    sftpTextFileResponse{},
		"SFTPSearchResult":                sftpSearchResponse{},
		"SFTPContentMatch":                sftpContentMatchResponse{},
		"SFTPSearchOmission":              sftpSearchOmissionResponse{},
		"SFTPSaveTextRequest":             sftpSaveTextRequest{},
		"SFTPRenameRequest":               sftpRenameRequest{},
		"SFTPChmodRequest":                sftpChmodRequest{},
		"SFTPTransfer":                    SFTPTransfer{},
		"SFTPDownloadPartProgress":        SFTPDownloadPartProgress{},
		"SFTPTransferJob":                 SFTPTransferJob{},
		"SFTPTransferJobList":             SFTPTransferJobList{},
		"SFTPCreateTransferJobRequest":    sftpCreateTransferJobRequest{},
		"SFTPTransferJobActionRequest":    sftpTransferJobActionRequest{},
		"SFTPTransferSettingsRequest":     sftpTransferSettingsRequest{},
		"SFTPTransferQueueMoveRequest":    sftpTransferQueueMoveRequest{},
		"SFTPDownloadCheckpointRequest":   sftpDownloadCheckpointRequest{},
		"SFTPStartUploadRequest":          sftpStartUploadRequest{},
		"SFTPCompleteUploadRequest":       sftpCompleteUploadRequest{},
		"SFTPResumableUpload":             SFTPResumableUpload{},
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
		"StartupSnippet":                  snippets.StartupAssignment{},
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
			contract.Verify(t, name, value)
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
		"exitMessage", "replayMessage", "resizeMessage", "vaultChangeRequest", "vaultPassphraseRequest",
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

// backendNames は、engine の backends の表にある方式の名前である。OpenAPI の enum と
// 表を直接比べ、方式を足したときにどちらかだけを変えると落ちるようにする。
func backendNames() []string {
	var names []string
	for _, backend := range vpn.Backends() {
		names = append(names, string(backend))
	}
	return names
}

var wireEnumValues = map[reflect.Type][]string{
	reflect.TypeOf(vpn.BackendName("")): backendNames(),
	reflect.TypeOf(VPNUnavailable("")): {
		string(VPNDockerMissing), string(VPNDockerNotRunning),
	},
	reflect.TypeOf(vpn.StartPhase("")): {
		string(vpn.PhaseImage), string(vpn.PhaseContainer),
		string(vpn.PhaseTunnel), string(vpn.PhaseApproval),
	},
	reflect.TypeOf(sftp.LinkTargetType("")): {
		string(sftp.LinkTargetFile), string(sftp.LinkTargetDirectory), string(sftp.LinkTargetOther),
	},
	reflect.TypeOf(sftp.EntryType("")): {
		string(sftp.EntryFile), string(sftp.EntryDirectory), string(sftp.EntrySymlink), string(sftp.EntryOther),
	},
	// The generated SFTPEntry is embedded in hand-written listings; its enum is
	// the generated one.
	reflect.TypeOf(api.SFTPEntryType("")): {
		string(api.File), string(api.Directory), string(api.Symlink), string(api.Other),
	},
	reflect.TypeOf(api.SFTPEntryTargetType("")): {
		string(api.SFTPEntryTargetTypeFile), string(api.SFTPEntryTargetTypeDirectory), string(api.SFTPEntryTargetTypeOther),
	},
	reflect.TypeOf(sftp.TransferDirection("")): {
		string(sftp.TransferUpload), string(sftp.TransferDownload), string(sftp.TransferRemote),
	},
	reflect.TypeOf(sftp.RemoteTransferOperation("")): {
		"", string(sftp.RemoteCopy), string(sftp.RemoteMove), string(sftp.RemoteDelete), string(sftp.RemoteGet), string(sftp.RemotePut),
	},
	reflect.TypeOf(sftp.DirectoryDifferenceStatus("")): {
		string(sftp.DirectorySame), string(sftp.DirectoryDifferent), string(sftp.DirectoryLeftOnly),
		string(sftp.DirectoryRightOnly), string(sftp.DirectoryTypeMismatch),
		string(sftp.DirectoryUnverified),
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
