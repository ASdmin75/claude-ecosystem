// Package outputcheck detects "soft failures" in Claude CLI output — cases
// where the process exits 0 but the text indicates the task was not completed
// (e.g., permission requests, unavailable tools).
package outputcheck

import "strings"

// failurePatterns are case-insensitive substrings that indicate Claude could
// not actually perform the requested work despite a successful exit code.
var failurePatterns = []struct {
	pattern string
	reason  string
}{
	// Permission / access issues (RU)
	{"запрашиваю permission", "Claude requested permission instead of executing the task"},
	{"дайте мне permission", "Claude requested permission instead of executing the task"},
	{"доступ запрещен", "access denied"},
	{"нет доступа", "access denied"},
	{"не имею доступа", "access denied"},
	{"требуется разрешение", "permission required"},

	// Permission / access issues (EN)
	{"permission denied", "permission denied"},
	{"need permission", "Claude requested permission instead of executing the task"},
	{"requesting permission", "Claude requested permission instead of executing the task"},
	{"i don't have permission", "Claude reported lack of permission"},
	{"i do not have permission", "Claude reported lack of permission"},

	// Tool not available (RU)
	{"инструмент недоступен", "tool not available"},
	{"не могу использовать инструмент", "tool not available"},

	// Tool not available (EN)
	{"tool is not available", "tool not available"},
	{"not in allowed_tools", "tool not in allowed_tools"},
	{"is not included in the allowed", "tool not in allowed_tools"},
	{"tool was not found", "tool not available"},

	// Claude asks for input instead of completing (RU)
	{"предоставь эти данные", "Claude asked for input instead of completing the task"},
	{"предоставьте эти данные", "Claude asked for input instead of completing the task"},
	{"мне нужны данные", "Claude asked for input instead of completing the task"},

	// Claude asks for input instead of completing (EN)
	{"please provide these", "Claude asked for input instead of completing the task"},
	{"i need you to provide", "Claude asked for input instead of completing the task"},
}

// CheckStepOutput scans Claude's output for patterns that indicate the task
// could not be completed despite exit code 0. Returns a human-readable reason
// string if a failure is detected, or "" if the output looks normal.
func CheckStepOutput(output string) string {
	if output == "" {
		return ""
	}
	lower := strings.ToLower(output)
	for _, fp := range failurePatterns {
		if strings.Contains(lower, fp.pattern) {
			return fp.reason
		}
	}
	return ""
}
