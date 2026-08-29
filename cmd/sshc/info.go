package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"sshc/internal/app"
	"sshc/internal/sshclient"
	"sshc/internal/validate"
)

const infoSchemaVersion = 1

type infoDestination struct {
	HostName string `json:"hostName"`
	User     string `json:"user"`
	Port     string `json:"port"`
}

type infoHop struct {
	Alias                  string          `json:"alias"`
	Destination            infoDestination `json:"destination"`
	IdentityFiles          []string        `json:"identityFiles"`
	IdentitiesOnly         bool            `json:"identitiesOnly"`
	ProxyCommandConfigured bool            `json:"proxyCommandConfigured"`
}

type infoNotice struct {
	Keyword string `json:"keyword"`
	Detail  string `json:"detail"`
}

type infoDocument struct {
	SchemaVersion              int             `json:"schemaVersion"`
	Alias                      string          `json:"alias"`
	Destination                infoDestination `json:"destination"`
	IdentityFiles              []string        `json:"identityFiles"`
	IdentitiesOnly             bool            `json:"identitiesOnly"`
	ProxyJump                  []infoHop       `json:"proxyJump"`
	ProxyCommandConfigured     bool            `json:"proxyCommandConfigured"`
	Encoding                   string          `json:"encoding"`
	AuthenticationMethods      []string        `json:"authenticationMethods"`
	RequestTTY                 string          `json:"requestTTY"`
	StrictHostKeyChecking      string          `json:"strictHostKeyChecking"`
	ConnectTimeoutSeconds      int64           `json:"connectTimeoutSeconds"`
	ServerAliveIntervalSeconds int64           `json:"serverAliveIntervalSeconds"`
	ServerAliveCountMax        int             `json:"serverAliveCountMax"`
	AgentForward               bool            `json:"agentForward"`
	Notices                    []infoNotice    `json:"notices"`
}

func runInfo(alias, home string, asJSON bool, stdout, stderr io.Writer) int {
	if err := validate.Alias(alias); err != nil {
		fmt.Fprintf(stderr, "sshc: %q is not an alias this can describe\n", alias)
		return 2
	}
	connection, err := app.NewCLIConnection(home, nil, nil)
	if err != nil {
		fmt.Fprintln(stderr, "sshc: could not read the SSH configuration")
		return 1
	}
	target, err := connection.Resolve(alias)
	if err != nil {
		// Resolver errors can contain user-authored ProxyCommand or SetEnv text.
		// Those values are intentionally outside the info allowlist.
		fmt.Fprintf(stderr, "sshc: could not resolve %q as an SSH target\n", alias)
		return 1
	}
	document := describeInfoTarget(target)
	if asJSON {
		if err := json.NewEncoder(stdout).Encode(document); err != nil {
			fmt.Fprintln(stderr, "sshc: could not encode the resolved SSH target")
			return 1
		}
		return 0
	}
	writeInfo(stdout, document)
	return 0
}

func describeInfoTarget(target sshclient.Target) infoDocument {
	document := infoDocument{
		SchemaVersion:              infoSchemaVersion,
		Alias:                      target.Alias,
		Destination:                describeInfoDestination(target),
		IdentityFiles:              append([]string{}, target.Identities...),
		IdentitiesOnly:             target.IdentitiesOnly,
		ProxyJump:                  make([]infoHop, 0),
		ProxyCommandConfigured:     target.ProxyCommand != "",
		Encoding:                   string(target.Encoding),
		AuthenticationMethods:      append([]string{}, target.Methods.Order()...),
		RequestTTY:                 target.RequestTTY,
		StrictHostKeyChecking:      target.Strict,
		ConnectTimeoutSeconds:      int64(target.Timeout.Seconds()),
		ServerAliveIntervalSeconds: int64(target.KeepAlive.Seconds()),
		ServerAliveCountMax:        target.KeepAliveMax,
		AgentForward:               target.AgentForward,
		Notices:                    make([]infoNotice, 0, len(target.Notices)),
	}
	for _, jump := range target.Jump {
		document.ProxyJump = appendInfoJump(document.ProxyJump, jump)
	}
	for _, notice := range target.Notices {
		document.Notices = append(document.Notices, infoNotice{Keyword: notice.Keyword, Detail: notice.Detail})
	}
	return document
}

func appendInfoJump(destination []infoHop, target sshclient.Target) []infoHop {
	for _, nested := range target.Jump {
		destination = appendInfoJump(destination, nested)
	}
	return append(destination, infoHop{
		Alias:                  target.Alias,
		Destination:            describeInfoDestination(target),
		IdentityFiles:          append([]string{}, target.Identities...),
		IdentitiesOnly:         target.IdentitiesOnly,
		ProxyCommandConfigured: target.ProxyCommand != "",
	})
}

func describeInfoDestination(target sshclient.Target) infoDestination {
	return infoDestination{HostName: target.HostName, User: target.User, Port: target.Port}
}

func writeInfo(out io.Writer, document infoDocument) {
	rows := [][2]string{
		{"alias", document.Alias},
		{"host", document.Destination.HostName},
		{"user", document.Destination.User},
		{"port", document.Destination.Port},
		{"encoding", document.Encoding},
		{"identities only", fmt.Sprintf("%t", document.IdentitiesOnly)},
		{"proxy command", configuredWord(document.ProxyCommandConfigured)},
		{"authentication", strings.Join(document.AuthenticationMethods, ", ")},
		{"request tty", defaultWord(document.RequestTTY)},
		{"strict host key", defaultWord(document.StrictHostKeyChecking)},
		{"connect timeout", fmt.Sprintf("%ds", document.ConnectTimeoutSeconds)},
		{"server alive", fmt.Sprintf("%ds × %d", document.ServerAliveIntervalSeconds, document.ServerAliveCountMax)},
		{"agent forwarding", fmt.Sprintf("%t", document.AgentForward)},
	}
	for _, identity := range document.IdentityFiles {
		rows = append(rows, [2]string{"identity", identity})
	}
	for _, jump := range document.ProxyJump {
		rows = append(rows, [2]string{"proxy jump", fmt.Sprintf("%s (%s@%s:%s)",
			jump.Alias, jump.Destination.User, jump.Destination.HostName, jump.Destination.Port)})
	}
	for _, notice := range document.Notices {
		rows = append(rows, [2]string{"notice " + notice.Keyword, notice.Detail})
	}
	width := 0
	for _, row := range rows {
		if len(row[0]) > width {
			width = len(row[0])
		}
	}
	for _, row := range rows {
		fmt.Fprintf(out, "%-*s  %s\n", width, row[0], row[1])
	}
}

func configuredWord(configured bool) string {
	if configured {
		return "configured (value hidden)"
	}
	return "not configured"
}

func defaultWord(value string) string {
	if value == "" {
		return "default"
	}
	return value
}
