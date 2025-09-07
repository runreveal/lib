package loader

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReplaceEnvInJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		envVars  map[string]string
		expected string
	}{
		{
			name:     "simple replacement",
			input:    `{"name": "$TEST_VAR"}`,
			envVars:  map[string]string{"TEST_VAR": "hello"},
			expected: `{"name": "hello"}`,
		},
		{
			name:     "multiple replacements",
			input:    `{"name": "$NAME", "host": "$HOST"}`,
			envVars:  map[string]string{"NAME": "test", "HOST": "localhost"},
			expected: `{"name": "test", "host": "localhost"}`,
		},
		{
			name:     "quotes in env value",
			input:    `{"message": "$QUOTED_VAR"}`,
			envVars:  map[string]string{"QUOTED_VAR": `say "hello"`},
			expected: `{"message": "say \"hello\""}`,
		},
		{
			name:     "backslashes in env value",
			input:    `{"path": "$PATH_VAR"}`,
			envVars:  map[string]string{"PATH_VAR": `C:\Program Files\App`},
			expected: `{"path": "C:\\Program Files\\App"}`,
		},
		{
			name:     "quotes and backslashes together",
			input:    `{"command": "$CMD_VAR"}`,
			envVars:  map[string]string{"CMD_VAR": `echo "C:\test"`},
			expected: `{"command": "echo \"C:\\test\""}`,
		},
		{
			name:     "no matching env var",
			input:    `{"name": "$NONEXISTENT"}`,
			envVars:  map[string]string{},
			expected: `{"name": ""}`,
		},
		{
			name:     "mixed env and regular strings",
			input:    `{"env": "$TEST", "regular": "value"}`,
			envVars:  map[string]string{"TEST": "replaced"},
			expected: `{"env": "replaced", "regular": "value"}`,
		},
		{
			name:     "env var in nested structure",
			input:    `{"config": {"host": "$HOST", "port": 8080}}`,
			envVars:  map[string]string{"HOST": "api.example.com"},
			expected: `{"config": {"host": "api.example.com", "port": 8080}}`,
		},
		{
			name:     "env var in array",
			input:    `{"servers": ["$SERVER1", "$SERVER2"]}`,
			envVars:  map[string]string{"SERVER1": "host1", "SERVER2": "host2"},
			expected: `{"servers": ["host1", "host2"]}`,
		},
		{
			name:     "complex escaping case",
			input:    `{"script": "$COMPLEX_SCRIPT"}`,
			envVars:  map[string]string{"COMPLEX_SCRIPT": `echo "Hello \"World\"" && echo 'C:\Program Files\test'`},
			expected: `{"script": "echo \"Hello \\\"World\\\"\" \u0026\u0026 echo 'C:\\Program Files\\test'"}`,
		},
		{
			name:     "newlines and tabs in env value",
			input:    `{"multiline": "$MULTILINE_VAR"}`,
			envVars:  map[string]string{"MULTILINE_VAR": "line1\nline2\tindented"},
			expected: `{"multiline": "line1\nline2\tindented"}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Set up environment variables
			for key, value := range test.envVars {
				os.Setenv(key, value)
				defer os.Unsetenv(key)
			}

			result := replaceEnvInJSON([]byte(test.input))
			assert.Equal(t, test.expected, string(result))
		})
	}
}

func TestEscapeForJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no escaping needed",
			input:    "simple string",
			expected: "simple string",
		},
		{
			name:     "escape quotes",
			input:    `say "hello"`,
			expected: `say \"hello\"`,
		},
		{
			name:     "escape backslashes",
			input:    `C:\Program Files`,
			expected: `C:\\Program Files`,
		},
		{
			name:     "escape both quotes and backslashes",
			input:    `echo "C:\test"`,
			expected: `echo \"C:\\test\"`,
		},
		{
			name:     "multiple backslashes",
			input:    `path\\to\\file`,
			expected: `path\\\\to\\\\file`,
		},
		{
			name:     "newlines and tabs",
			input:    "line1\nline2\tindented",
			expected: "line1\\nline2\\tindented",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := escapeForJSON(test.input)
			assert.Equal(t, test.expected, result)
		})
	}
}

