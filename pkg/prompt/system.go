package prompt

import systemprompt "github.com/similarityyoung/simiclaw/internal/systemprompt"

type SystemTextSet = systemprompt.SystemTextSet

var SystemText = systemprompt.SystemText

func Render(raw string, replacements map[string]string) string {
	return systemprompt.Render(raw, replacements)
}
