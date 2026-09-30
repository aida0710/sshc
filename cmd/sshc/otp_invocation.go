package main

type otpAction uint8

const (
	otpInvalid otpAction = iota
	otpList
	otpShow
	otpAdd
	otpEdit
	otpRemove
)

type otpInvocation struct {
	Action otpAction
	Name   string
	JSON   bool
	Yes    bool
}

func parseOTPInvocation(args []string) (invocation, error) {
	if helpRequested(args) {
		return helpInvocation(canonicalCLICommand(cliCommandOtp)), nil
	}
	if len(args) > 1 && validOTPAction(args[0]) && isHelpFlag(args[1]) {
		return helpInvocation(canonicalCLICommand(cliCommandOtp) + " " + args[0]), nil
	}
	if len(args) == 0 {
		return invalidInvocation("otp requires list, a saved name, add, edit, or remove")
	}
	if args[0] == "list" {
		asJSON, err := parseOptionalJSON("otp list", args[1:])
		if err != nil {
			return invalidInvocation(err.Error())
		}
		return invocation{Kind: invocationOTP, OTP: &otpInvocation{Action: otpList, JSON: asJSON}}, nil
	}
	if args[0] == "add" || args[0] == "edit" {
		if len(args) != 2 || args[1] == "" {
			return invalidInvocation("otp " + args[0] + " requires exactly one name")
		}
		action := otpAdd
		if args[0] == "edit" {
			action = otpEdit
		}
		return invocation{Kind: invocationOTP, OTP: &otpInvocation{Action: action, Name: args[1]}}, nil
	}
	if args[0] == "remove" {
		if len(args) < 2 || args[1] == "" {
			return invalidInvocation("otp remove requires one name and optionally --yes")
		}
		yes, err := parseOptionalYes("otp remove", args[2:])
		if err != nil {
			return invalidInvocation(err.Error())
		}
		return invocation{Kind: invocationOTP, OTP: &otpInvocation{Action: otpRemove, Name: args[1], Yes: yes}}, nil
	}
	command, nameIndex := "otp", 0
	if args[0] == "show" {
		if len(args) < 2 {
			return invalidInvocation("otp show requires one name")
		}
		command, nameIndex = "otp show", 1
	}
	if args[nameIndex] == "" {
		return invalidInvocation("otp requires a non-empty saved name")
	}
	asJSON, err := parseOptionalJSON(command, args[nameIndex+1:])
	if err != nil {
		return invalidInvocation(err.Error())
	}
	return invocation{Kind: invocationOTP, OTP: &otpInvocation{
		Action: otpShow, Name: args[nameIndex], JSON: asJSON,
	}}, nil
}
