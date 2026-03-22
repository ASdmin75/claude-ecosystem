package outputcheck

import "testing"

func TestCheckStepOutput(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		wantErr bool
	}{
		{
			name:    "empty output",
			output:  "",
			wantErr: false,
		},
		{
			name:    "normal successful output",
			output:  "Файл сохранён: data/report.xlsx\nВсего записей: 9",
			wantErr: false,
		},
		{
			name:    "normal output with permission word in context",
			output:  "Установлен permission_mode: dontAsk для задачи",
			wantErr: false,
		},
		{
			name:    "RU permission request",
			output:  "Нужно получить доступ к файлу xlsx. Запрашиваю permission для чтения данных.",
			wantErr: true,
		},
		{
			name:    "RU give me permission",
			output:  "Дайте мне permission на использование Bash для чтения xlsx файла",
			wantErr: true,
		},
		{
			name:    "RU access denied",
			output:  "Доступ запрещен к инструменту mcp__excel__read",
			wantErr: true,
		},
		{
			name:    "RU no access",
			output:  "У меня нет доступа к файловой системе",
			wantErr: true,
		},
		{
			name:    "EN permission denied",
			output:  "Permission denied when trying to read the file",
			wantErr: true,
		},
		{
			name:    "EN need permission",
			output:  "I need permission to use the Bash tool to read the xlsx file",
			wantErr: true,
		},
		{
			name:    "EN tool not available",
			output:  "The mcp__excel__read_spreadsheet tool is not available in this context",
			wantErr: true,
		},
		{
			name:    "EN not in allowed tools",
			output:  "mcp__filesystem__read_file is not in allowed_tools for this task",
			wantErr: true,
		},
		{
			name:    "RU tool unavailable",
			output:  "Инструмент недоступен в текущей конфигурации",
			wantErr: true,
		},
		{
			name:    "case insensitive match",
			output:  "ЗАПРАШИВАЮ PERMISSION для выполнения операции",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason := CheckStepOutput(tt.output)
			if tt.wantErr && reason == "" {
				t.Errorf("expected failure detection but got empty reason")
			}
			if !tt.wantErr && reason != "" {
				t.Errorf("expected no failure but got reason: %s", reason)
			}
		})
	}
}
