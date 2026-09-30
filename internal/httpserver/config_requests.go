package httpserver

import (
	"errors"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"sshc/internal/application"
)

// HTTP 境界におけるランタイム上限。生成された型は形を記述するが、
// こちらはサイズを制限する。ローカルな API であっても API には変わりないからだ。
const (
	// 設定の要求は、API 全体の上限（MaxRequestBodyCeiling）いっぱいまで受ける。
	maxRequestBody = MaxRequestBodyCeiling
	maxPathLength  = 512
	// maxHostBlockAliasLength は、ファイルにある Host ブロックを指す alias の上限。
	// validate.MaxAliasLength（アプリが接続・保存に使う alias）より広いのは、
	// 起動できない長い alias でも、ファイルにある以上は詳細の表示と生の編集は
	// できるからである。接続エディタの保存（PATCH /connections）はドメインと
	// Vault に合わせて validate.MaxAliasLength を使う。
	maxHostBlockAliasLength = 255
	maxFieldEdits           = 256
	maxFieldValues          = 64
	maxValueLength          = 1024
	maxRawLength            = 1 << 20
	// maxCommentLength は、1 個の Host ブロックに付くコメントを制限する。
	// ファイル全体よりはるかに小さいのは、コメントが 1 個の接続についての
	// 散文だからであり、この上限があるからこそ、ログ全体をうっかり設定に
	// 貼り付けてしまうことが防がれる。
	maxCommentLength = 4 << 10
	maxGroupCount    = 256
	maxIDLength      = 128
)

var (
	errInvalidBody  = errors.New("invalid_request_body")
	errInvalidPath  = errors.New("invalid_path")
	errInvalidAlias = errors.New("invalid_alias")
	errInvalidEdit  = errors.New("invalid_edit")
)

// problemPayload は OpenAPI の Problem スキーマの通信形式である。
// location と安定した code を運ぶが、ファイルの中身は決して運ばない。
type problemPayload struct {
	Code            string                       `json:"code"`
	Message         string                       `json:"message"`
	Detail          string                       `json:"detail,omitempty"`
	Path            string                       `json:"path,omitempty"`
	Line            int                          `json:"line,omitempty"`
	Column          int                          `json:"column,omitempty"`
	Diagnostics     []application.DiagnosticView `json:"diagnostics,omitempty"`
	Conflict        *application.ConflictReport  `json:"conflict,omitempty"`
	CurrentVersion  *int                         `json:"currentVersion,omitempty"`
	RequiredVersion *int                         `json:"requiredVersion,omitempty"`
	// Blockers は group 操作が拒否した理由を示す。これらはコロンの後に
	// detail を伴う安定した code であり、鍵の relocation が使うのと同じ形である。
	Blockers []string `json:"blockers,omitempty"`
	// Field・Reason・Limit は、入力のどの項目を、どの理由で受け取れなかったかである。
	// Field は保存形式の JSON のパス、Reason は決まった語で、画面と CLI が翻訳する。
	// 経路を用意できなかったときは、Reason だけにその理由の語が入る。
	Field  string `json:"field,omitempty"`
	Reason string `json:"reason,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	// Directive は、設定ファイル（OpenVPN の .ovpn）の中で断った指示の名前である。行番号は
	// Line に入る。どちらも Go が決めた語と数だけで、設定ファイルの中身は載せない。
	Directive string `json:"directive,omitempty"`
}

// declaredGroup は、拒否された directory 操作が対象としていた group を示す。
// 拒否が他の理由による場合は空文字列となる。
func declaredGroup(err error) string {
	var declared *application.GroupDeclaredError
	if errors.As(err, &declared) {
		return declared.Group
	}
	return ""
}

func problemWith(c *echo.Context, status int, payload problemPayload) error {
	if payload.Message == "" {
		payload.Message = "request rejected"
	}
	c.Response().Header().Set(echo.HeaderContentType, "application/problem+json")
	return c.JSON(status, payload)
}

// validatePathParameter は、traversal も制御文字もない、単一ルートの
// 相対 path のみを受け付ける。正式なチェックはワークスペースが行うが、これは
// 明らかに悪意ある入力をアプリケーション層から締め出すためのものである。
func validatePathParameter(value string) error {
	if value == "" || len(value) > maxPathLength {
		return errInvalidPath
	}
	if strings.HasPrefix(value, "/") || strings.ContainsAny(value, "\x00\n\r") {
		return errInvalidPath
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return errInvalidPath
		}
	}
	return nil
}

// validateHostBlockAlias は、ファイルにある Host ブロックを指す alias を受け付ける。
// maxHostBlockAliasLength までの、制御文字と空白を含まない値なら通す。
// validate.Alias より広い理由は maxHostBlockAliasLength に書いてある。
func validateHostBlockAlias(value string) error {
	if value == "" || len(value) > maxHostBlockAliasLength {
		return errInvalidAlias
	}
	for _, character := range value {
		if character <= ' ' || character == 0x7f {
			return errInvalidAlias
		}
	}
	return nil
}

func validateIdentifier(value string) error {
	if value == "" || len(value) > maxIDLength {
		return errInvalidEdit
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		isAllowed := character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' ||
			character == '-' || character == '.'
		if !isAllowed {
			return errInvalidEdit
		}
	}
	return nil
}

// validateEditRequest は、リクエストがアプリケーション層に届く前に
// kind ごとの要件を強制する。
func validateEditRequest(request application.EditRequest) error {
	if len(request.Raw) > maxRawLength || len(request.Base) > maxRawLength ||
		len(request.DestinationBase) > maxRawLength || len(request.Comment) > maxCommentLength {
		return errInvalidEdit
	}
	switch request.Kind {
	case application.EditHostFields, application.EditBlockRaw, application.EditFileRaw,
		application.EditRename, application.EditDuplicate, application.EditMove, application.EditComment,
		application.EditFileRename, application.EditFileDelete,
		application.EditDirectoryCreate, application.EditDirectoryDelete, application.EditMetadata:
		if err := validatePathParameter(request.Path); err != nil {
			return err
		}
	case application.EditGroups:
	default:
		return errInvalidEdit
	}

	switch request.Kind {
	case application.EditHostFields:
		if err := validateHostBlockAlias(request.Alias); err != nil {
			return err
		}
		if len(request.Fields) == 0 || len(request.Fields) > maxFieldEdits {
			return errInvalidEdit
		}
		for _, edit := range request.Fields {
			if err := validateFieldEdit(edit); err != nil {
				return err
			}
		}
	case application.EditBlockRaw:
		if err := validateHostBlockAlias(request.Alias); err != nil {
			return err
		}
		if request.Raw == "" {
			return errInvalidEdit
		}
	case application.EditComment:
		if err := validateHostBlockAlias(request.Alias); err != nil {
			return err
		}
		// 空のコメントはコメントを削除する手段なので、最小長というものはない。
		// carriage return は renderer によって正規化される。行を早期に
		// 終わらせてしまう他の文字はここで拒否される。テキスト内の newline だけが
		// 書き込まれるコメント行数を決めるものであり、迷い込んだ制御文字が
		// それを勝手に作り出してはならないからだ。
		if strings.ContainsAny(request.Comment, "\x00\v\f\u0085\u2028\u2029") {
			return errInvalidEdit
		}
	case application.EditFileRaw:
		// 空のファイルは、最後のブロックを削除した結果として正当にあり得る。
		// 既存のファイルを誤った空書き込みから守るのは、長さのチェックではなく
		// base digest の事前条件である。
	case application.EditRename, application.EditDuplicate:
		if err := validateHostBlockAlias(request.Alias); err != nil {
			return err
		}
		if err := application.ValidateAlias(request.NewAlias); err != nil {
			return errInvalidAlias
		}
	case application.EditMove:
		if err := validateHostBlockAlias(request.Alias); err != nil {
			return err
		}
		// move は移動先を二通りのいずれかで指定する。path をサービスが導出する
		// group か、path そのものかである。両方を指定することはそこで拒否される。
		// 両者が食い違い得るし、このアプリケーションはどちらかを選んだりしないからだ。
		if request.DestinationGroup != "" {
			if err := application.ValidateGroupName(request.DestinationGroup); err != nil {
				return err
			}
			break
		}
		if err := validatePathParameter(request.DestinationPath); err != nil {
			return err
		}
	case application.EditFileRename:
		if err := validatePathParameter(request.DestinationPath); err != nil {
			return err
		}
	case application.EditFileDelete:
		// base がすべての事前条件である。delete は新しいバイトを一切
		// 伴わないので、ここで他に検証すべきことは何もない。
	case application.EditMetadata:
		return validateHostMetadataEdit(request)
	case application.EditGroups:
		if len(request.Groups) > maxGroupCount || len(request.GroupsBase) > maxGroupCount {
			return errInvalidEdit
		}
		// 送られたグループの設定だけで形を確かめる。ほかの節はディスクの値を使うので、
		// ここでは見ない。
		if err := application.ValidateMetadata(application.Metadata{Groups: request.Groups}); err != nil {
			return err
		}
	}
	return nil
}

// validateHostMetadataEdit は、接続 1 件の metadata の変更を確かめる。base と新しい
// entry のどちらも、Path と Alias が指す接続のものでなければならない。新しい entry
// だけは、orphan を付け直すために別の接続を指してよい。
func validateHostMetadataEdit(request application.EditRequest) error {
	if err := validateHostBlockAlias(request.Alias); err != nil {
		return err
	}
	identity := application.HostIdentity{Path: request.Path, Alias: request.Alias}
	if base := request.HostMetadataBase; base != nil && base.Identity != identity {
		return errInvalidEdit
	}
	if request.HostMetadata == nil {
		return nil
	}
	if err := validatePathParameter(request.HostMetadata.Identity.Path); err != nil {
		return err
	}
	if err := validateHostBlockAlias(request.HostMetadata.Identity.Alias); err != nil {
		return err
	}
	return application.ValidateMetadata(application.Metadata{Hosts: []application.HostMetadata{*request.HostMetadata}})
}

func validateFieldEdit(edit application.FieldEdit) error {
	switch edit.Action {
	case application.ActionSet, application.ActionRemove:
		if edit.Line <= 0 {
			return errInvalidEdit
		}
	case application.ActionAdd:
		if edit.Keyword == "" {
			return errInvalidEdit
		}
	default:
		return errInvalidEdit
	}
	if len(edit.Keyword) > 64 || len(edit.Values) > maxFieldValues {
		return errInvalidEdit
	}
	for _, value := range edit.Values {
		if len(value) > maxValueLength {
			return errInvalidEdit
		}
	}
	return nil
}

// serviceProblem は、アプリケーションエラーを HTTP の problem レスポンスに
// 対応付ける。この対応付けにファイルの中身が含まれることは決してなく、
// 既定は汎用の 500 なので、予期しないエラーがメッセージを漏らすことはない。
func serviceProblem(c *echo.Context, err error) error {
	var syntaxError *application.SyntaxError
	var graphError *application.GraphError
	var conflictError *application.ConflictError
	var groupBlocked *application.GroupBlockedError
	switch {
	case errors.As(err, &groupBlocked):
		// 何も書き込まれていない。blockers は transaction を組み立てる前に
		// 計算され、ユーザーが必要とするのは素の 409 ではなくそれである。
		return problemWith(c, http.StatusConflict, problemPayload{
			Code:     "group_blocked",
			Blockers: groupBlocked.Blockers,
		})
	case errors.As(err, &syntaxError):
		return problemWith(c, http.StatusUnprocessableEntity, problemPayload{
			Code:   "config_syntax_error",
			Path:   syntaxError.Path,
			Line:   syntaxError.Line,
			Column: syntaxError.Column,
			Detail: syntaxError.Detail,
		})
	case errors.As(err, &graphError):
		return problemWith(c, http.StatusUnprocessableEntity, problemPayload{
			Code:        "config_graph_error",
			Diagnostics: graphError.Diagnostics,
		})
	case errors.As(err, &conflictError):
		report := conflictError.Report
		return problemWith(c, http.StatusConflict, problemPayload{
			Code:     "config_conflict",
			Path:     report.Path,
			Conflict: &report,
		})
	case declaredGroup(err) != "":
		// 名前が付いているのは、単に拒否するだけでなく、インターフェースが
		// その操作のある画面へユーザーを送れるようにするためである。
		return problemWith(c, http.StatusConflict, problemPayload{
			Code: "group_is_declared", Detail: declaredGroup(err),
		})
	case errors.Is(err, application.ErrHostNotFound), errors.Is(err, application.ErrUnknownTransaction),
		errors.Is(err, application.ErrFileNotFound):
		return problemWith(c, http.StatusNotFound, problemPayload{Code: "not_found"})
	case errors.Is(err, application.ErrMetadataChanged):
		// 画面の写しは古い。読み直してから変更し直してもらう。
		return problemWith(c, http.StatusConflict, problemPayload{Code: "metadata_changed"})
	case errors.Is(err, application.ErrCannotTouchEntryFile):
		return problemWith(c, http.StatusConflict, problemPayload{Code: "entry_file_protected"})
	case errors.Is(err, application.ErrDestinationExists):
		return problemWith(c, http.StatusConflict, problemPayload{Code: "destination_exists"})
	case errors.Is(err, application.ErrSamePath):
		return problemWith(c, http.StatusBadRequest, problemPayload{Code: "invalid_request"})
	case errors.Is(err, application.ErrGroupNotDeclared):
		return problemWith(c, http.StatusUnprocessableEntity, problemPayload{Code: "group_not_declared"})
	case errors.Is(err, application.ErrRegionDamaged):
		// 内部の欠陥ではない。ファイルには、このアプリケーションが書く 2 種類の
		// マーカーのどちらかがあり、自分の行がどこで終わるかを推測したりしない。
		return problemWith(c, http.StatusConflict, problemPayload{Code: "region_damaged"})
	case errors.Is(err, application.ErrGroupExists):
		return problemWith(c, http.StatusConflict, problemPayload{Code: "group_exists"})
	case errors.Is(err, application.ErrDirectoryNotEmpty):
		return problemWith(c, http.StatusConflict, problemPayload{Code: "directory_not_empty"})
	case errors.Is(err, application.ErrNotADirectory):
		return problemWith(c, http.StatusBadRequest, problemPayload{Code: "not_a_directory"})
	case errors.Is(err, application.ErrExternalPath), errors.Is(err, application.ErrOutsideWorkspace),
		errors.Is(err, application.ErrSymlinkPath), errors.Is(err, application.ErrNotRegularFile),
		// 存在しないディレクトリを指す path や、ディレクトリでない要素を含む path は、
		// リクエストについての事実であり、内部の欠陥ではない。
		// この 2 つがなければ、呼び出し側が渡す "~/x/y" のような path は 500 を返していた。
		errors.Is(err, application.ErrMissingDirectory), errors.Is(err, application.ErrNotDirectory),
		errors.Is(err, application.ErrNotEditable):
		return problemWith(c, http.StatusForbidden, problemPayload{Code: "path_not_editable"})
	case errors.Is(err, application.ErrUnknownEditKind), errors.Is(err, application.ErrUnknownRecoveryAction),
		errors.Is(err, application.ErrMetadataSecret), errors.Is(err, application.ErrMetadataPath),
		errors.Is(err, application.ErrMetadataGroup), errors.Is(err, application.ErrMetadataVersion),
		errors.Is(err, application.ErrMetadataTerminal), errors.Is(err, application.ErrMetadataEncoding),
		errors.Is(err, application.ErrEngineSettings), errors.Is(err, application.ErrMetadataDuplicateHost),
		errors.Is(err, application.ErrSameFileMove), errors.Is(err, application.ErrAmbiguousDestination),
		errors.Is(err, application.ErrInvalidGroupName), errors.Is(err, application.ErrGroupSelfNesting),
		errors.Is(err, application.ErrKeyRelocateUnchanged),
		errors.Is(err, errInvalidBody), errors.Is(err, errInvalidPath),
		errors.Is(err, errInvalidAlias), errors.Is(err, errInvalidEdit):
		return problemWith(c, http.StatusBadRequest, problemPayload{Code: "invalid_request"})
	case errors.Is(err, application.ErrUnquotableValue), errors.Is(err, application.ErrStructuralKeyword),
		errors.Is(err, application.ErrInvalidKeyword), errors.Is(err, application.ErrEmptyKeyword),
		errors.Is(err, application.ErrInvalidAlias), errors.Is(err, application.ErrRawBlockHeader),
		errors.Is(err, application.ErrRawBlockStructure), errors.Is(err, application.ErrEditLineOutsideBlock),
		errors.Is(err, application.ErrEditLineNotDirective), errors.Is(err, application.ErrDuplicateEditLine),
		errors.Is(err, application.ErrUnknownEditAction):
		return problemWith(c, http.StatusUnprocessableEntity, problemPayload{Code: "invalid_edit"})
	case errors.Is(err, application.ErrKeyFilesChanged), application.IsExternalChange(err):
		// 設定ファイル以外（Vault、鍵ファイル、metadata.json）が、読んだあとに
		// 変わっていた。何も書いていないので、読み直してやり直せばよい。
		return problemWith(c, http.StatusConflict, problemPayload{Code: "external_change"})
	case errors.Is(err, application.ErrAliasAlreadyDeclared),
		errors.Is(err, application.ErrDuplicateDestinationAlias):
		// invalid_edit ではなく専用の code である。リクエストの形式に問題はなく、
		// ユーザーに伝えるべきは編集が不正だったことではなく、
		// 名前がすでに使われているということだからだ。
		return problemWith(c, http.StatusConflict, problemPayload{Code: "alias_already_declared"})
	default:
		if refusal, ok := boundaryRefusalFor(err); ok {
			return writeProblemReply(c, refusal)
		}
		logUnexpectedFailure(c, "internal_error", err)
		return problemWith(c, http.StatusInternalServerError, problemPayload{Code: "internal_error"})
	}
}
