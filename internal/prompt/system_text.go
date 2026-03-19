package prompt

import (
	"embed"
	"fmt"
	"strings"
)

const (
	identityRuntimeSystemFile  = "system/identity_runtime.md"
	toolContractSystemFile     = "system/tool_contract.md"
	memoryPolicySystemFile     = "system/memory_policy.md"
	heartbeatRuntimeSystemFile = "system/heartbeat_runtime.md"
)

//go:embed system/*.md
var systemFS embed.FS

var systemText = mustLoadSystemText()

type systemTextSet struct {
	IdentityRuntime string
	ToolContract    string
	MemoryPolicy    string
	HeartbeatPolicy string
}

func mustLoadSystemText() systemTextSet {
	return systemTextSet{
		IdentityRuntime: mustReadSystemText(identityRuntimeSystemFile),
		ToolContract:    mustReadSystemText(toolContractSystemFile),
		MemoryPolicy:    mustReadSystemText(memoryPolicySystemFile),
		HeartbeatPolicy: mustReadSystemText(heartbeatRuntimeSystemFile),
	}
}

func mustReadSystemText(path string) string {
	data, err := systemFS.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("prompt: read embedded system file %s: %v", path, err))
	}
	return strings.TrimSpace(string(data))
}

func renderSystemTemplate(raw string, replacements map[string]string) string {
	for key, value := range replacements {
		raw = strings.ReplaceAll(raw, "{{"+key+"}}", value)
	}
	return strings.TrimSpace(raw)
}
