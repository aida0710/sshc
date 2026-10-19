package application

// Notice は、UI が結果をでっち上げるのではなく示さなければならない何かを説明する。
type Notice struct {
	Code   string `json:"code"`
	Path   string `json:"path,omitempty"`
	Line   int    `json:"line,omitempty"`
	Detail string `json:"detail,omitempty"`
	// Related は、同じ注意に関わるもう一つの場所である。duplicate_alias では、同じ alias を
	// 先に宣言しているブロック（OpenSSH が先に読む方）を指す。
	Related NoticeLocation `json:"related,omitzero"`
}

// NoticeLocation は、注意が指す設定ファイルの行である。
type NoticeLocation struct {
	Path string `json:"path"`
	Line int    `json:"line"`
}

// Notice code は、UI が自前のコピーへ対応付ける安定した識別子である。
const (
	NoticeComplexExternalRule = "complex_external_rule"
	NoticeDuplicateAlias      = "duplicate_alias"
	NoticeProxyIgnored        = "proxy_ignored"
	NoticeWildcardShadow      = "wildcard_shadow"
	NoticeNegatedPattern      = "negated_pattern"
	NoticeUnnamedHostBlock    = "unnamed_host_block"
	NoticeMatchBlock          = "match_block"
	NoticeDangerousDirective  = "dangerous_directive"
	NoticeUnstructuredLine    = "unstructured_line"
	NoticeExternalFile        = "external_file"
	NoticeOrphanMetadata      = "orphan_metadata"
	NoticeGroupCycle          = "group_cycle"
	NoticeGroupMemberMissing  = "group_member_missing"
	NoticeExplainedValuesOnly = "explained_values_only"
	// Refusal は設定を解決できなかった理由である。
	NoticeMatchExecRefused    = "match_exec_refused"
	NoticeMatchFinalRefused   = "match_final_refused"
	NoticeCanonicaliseRefused = "canonicalise_refused"
	NoticeUnknownTokenRefused = "unknown_token_refused"
	// NoticeDestinationNotIncluded は、移動先のファイルにどの Include も届かないことを
	// 知らせる。そこへ移したブロックは OpenSSH に読まれなくなる。
	NoticeDestinationNotIncluded = "destination_not_included"
)

// appendNotice は、同一の通知が既に存在しない限り、通知を追加する。
func appendNotice(notices []Notice, notice Notice) []Notice {
	for _, existing := range notices {
		if existing == notice {
			return notices
		}
	}
	return append(notices, notice)
}
