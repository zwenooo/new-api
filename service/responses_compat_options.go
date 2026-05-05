package service

import (
	"fmt"
	"strings"
)

const (
	cxCompatResponsesCodexCLIUAContainsOpt   = "cx_compat.responses.codex_cli_rs_ua_contains"
	cxCompatResponsesOverrideInstructionsOpt = "cx_compat.responses.override_instructions"
	cxCompatResponsesBodyPatchJSONOpt        = "cx_compat.responses.body_patch_json"

	cxCompatResponsesDirectPassUADefault       = "codex_vscode,codex_exec,Codex Desktop,codex_cli_rs"
	cxCompatResponsesDirectPassUALegacyDefault = "codex_cli_rs/"
)

func GetCxCompatResponsesInstructionsSettings() (codexCLIUAContains string, overrideInstructions bool, err error) {
	codexCLIUAContains = strings.TrimSpace(readOption(cxCompatResponsesCodexCLIUAContainsOpt))
	if codexCLIUAContains == "" || strings.EqualFold(codexCLIUAContains, cxCompatResponsesDirectPassUALegacyDefault) {
		codexCLIUAContains = cxCompatResponsesDirectPassUADefault
	}
	overrideInstructions = false

	overrideStr := strings.TrimSpace(readOption(cxCompatResponsesOverrideInstructionsOpt))
	if overrideStr == "" {
		return codexCLIUAContains, overrideInstructions, nil
	}
	switch strings.ToLower(overrideStr) {
	case "true":
		overrideInstructions = true
	case "false":
		overrideInstructions = false
	default:
		return "", false, fmt.Errorf("%s must be true/false", cxCompatResponsesOverrideInstructionsOpt)
	}
	return codexCLIUAContains, overrideInstructions, nil
}
