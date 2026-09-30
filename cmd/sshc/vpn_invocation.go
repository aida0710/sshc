package main

import "net"

type vpnAction uint8

const (
	vpnInvalid vpnAction = iota
	vpnList
	vpnAdd
	vpnEdit
	vpnRemove
	vpnUp
	vpnDown
	vpnBind
	vpnUnbind
	vpnRename
	vpnLogsAction
	vpnProxy
)

type vpnInvocation struct {
	Action vpnAction
	// Name は、VPNプロファイルの名前である。unbind では空になる。
	Name string
	// Alias は、紐付けを変える接続である。bind と unbind だけが使う。
	Alias string
	// Rename は、新しいプロファイル名である。rename だけが使う。
	Rename string
	// Target は、繋ごうとしている相手（`host:port`）である。proxy だけが使う。
	Target string
	JSON   bool
	Yes    bool
}

// parseVPNInvocation は、VPN経路の操作を決める。
//
// 秘密を引数で受け取らない。追加は対話で no-echo に読む。
func parseVPNInvocation(args []string) (invocation, error) {
	if helpRequested(args) {
		return helpInvocation(canonicalCLICommand(cliCommandVpn)), nil
	}
	if len(args) > 1 && validCLIAction("vpn", args[0]) && isHelpFlag(args[1]) {
		return helpInvocation(canonicalCLICommand(cliCommandVpn) + " " + args[0]), nil
	}
	if len(args) == 0 {
		return invocation{Kind: invocationVPN, VPN: &vpnInvocation{Action: vpnList}}, nil
	}
	if len(args) == 1 && args[0] == "--json" {
		return invocation{Kind: invocationVPN, VPN: &vpnInvocation{Action: vpnList, JSON: true}}, nil
	}
	switch args[0] {
	case "add", "edit":
		if len(args) != 2 || args[1] == "" {
			return invalidInvocation("vpn " + args[0] + " requires exactly one profile name")
		}
		action := vpnAdd
		if args[0] == "edit" {
			action = vpnEdit
		}
		return invocation{Kind: invocationVPN, VPN: &vpnInvocation{Action: action, Name: args[1]}}, nil
	case "remove":
		if len(args) < 2 || args[1] == "" {
			return invalidInvocation("vpn remove requires one profile name and optionally --yes")
		}
		yes, err := parseOptionalYes("vpn remove", args[2:])
		if err != nil {
			return invalidInvocation(err.Error())
		}
		return invocation{Kind: invocationVPN, VPN: &vpnInvocation{Action: vpnRemove, Name: args[1], Yes: yes}}, nil
	case "up":
		return parseVPNProfileAction(args, vpnUp)
	case "down":
		return parseVPNProfileAction(args, vpnDown)
	case "logs":
		return parseVPNProfileAction(args, vpnLogsAction)
	case "rename":
		if len(args) < 3 || args[1] == "" || args[2] == "" {
			return invalidInvocation("vpn rename requires the current name, the new name, and optionally --json")
		}
		asJSON, err := parseOptionalJSON("vpn rename", args[3:])
		if err != nil {
			return invalidInvocation(err.Error())
		}
		return invocation{Kind: invocationVPN, VPN: &vpnInvocation{
			Action: vpnRename, Name: args[1], Rename: args[2], JSON: asJSON,
		}}, nil
	case "proxy":
		// ProxyCommand から %h と %p を付けて呼ばれる。接続先はプロファイルに
		// 無いので、省略できない。
		if len(args) != 4 || args[1] == "" || args[2] == "" || args[3] == "" {
			return invalidInvocation("vpn proxy requires one profile name, the host, and the port")
		}
		return invocation{Kind: invocationVPN, VPN: &vpnInvocation{
			Action: vpnProxy, Name: args[1], Target: net.JoinHostPort(args[2], args[3]),
		}}, nil
	case "bind":
		if len(args) < 3 || args[1] == "" || args[2] == "" {
			return invalidInvocation("vpn bind requires an alias and a profile name")
		}
		asJSON, err := parseOptionalJSON("vpn bind", args[3:])
		if err != nil {
			return invalidInvocation(err.Error())
		}
		return invocation{Kind: invocationVPN, VPN: &vpnInvocation{
			Action: vpnBind, Alias: args[1], Name: args[2], JSON: asJSON,
		}}, nil
	case "unbind":
		if len(args) < 2 || args[1] == "" {
			return invalidInvocation("vpn unbind requires one alias and optionally --json")
		}
		asJSON, err := parseOptionalJSON("vpn unbind", args[2:])
		if err != nil {
			return invalidInvocation(err.Error())
		}
		return invocation{Kind: invocationVPN, VPN: &vpnInvocation{Action: vpnUnbind, Alias: args[1], JSON: asJSON}}, nil
	}
	return invalidInvocation(missingActionMessage("vpn"))
}

// parseVPNProfileAction は、プロファイル名 1 つと任意の --json を取る操作（up、down、logs）を読む。
func parseVPNProfileAction(args []string, action vpnAction) (invocation, error) {
	command := "vpn " + args[0]
	if len(args) < 2 || args[1] == "" {
		return invalidInvocation(command + " requires one profile name and optionally --json")
	}
	asJSON, err := parseOptionalJSON(command, args[2:])
	if err != nil {
		return invalidInvocation(err.Error())
	}
	return invocation{Kind: invocationVPN, VPN: &vpnInvocation{Action: action, Name: args[1], JSON: asJSON}}, nil
}
