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
			name:     "real world config with deeply nested env var",
			input:    `{
				"pprof": "localhost:6060",
				"sources": {
					"journald": {
						"maxLineLenKB": 200,
						"type": "journald"
					}
				},
				"destinations": {
					"s3_batch": {
						"type": "s3b",
						"batchSize": 10000,
						"flushFrequency": "30s",
						"s3": {
							"type": "s3",
							"region": "us-east-2",
							"bucket": "$REVEALD_BUCKET"
						}
					}
				}
			}`,
			envVars:  map[string]string{"REVEALD_BUCKET": "reveald-log-upload-staging"},
			expected: `{
				"pprof": "localhost:6060",
				"sources": {
					"journald": {
						"maxLineLenKB": 200,
						"type": "journald"
					}
				},
				"destinations": {
					"s3_batch": {
						"type": "s3b",
						"batchSize": 10000,
						"flushFrequency": "30s",
						"s3": {
							"type": "s3",
							"region": "us-east-2",
							"bucket": "reveald-log-upload-staging"
						}
					}
				}
			}`,
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

func TestLoadConfigWithNestedEnvironmentVars(t *testing.T) {
	// Test the actual LoadConfig function with a realistic nested config
	type S3Config struct {
		Type   string `json:"type"`
		Region string `json:"region"`
		Bucket string `json:"bucket"`
	}
	
	type S3BatchConfig struct {
		Type           string   `json:"type"`
		BatchSize      int      `json:"batchSize"`
		FlushFrequency string   `json:"flushFrequency"`
		S3             S3Config `json:"s3"`
	}
	
	type JournaldConfig struct {
		MaxLineLenKB int    `json:"maxLineLenKB"`
		Type         string `json:"type"`
	}
	
	type RealWorldConfig struct {
		Pprof        string                    `json:"pprof"`
		Sources      map[string]JournaldConfig `json:"sources"`
		Destinations map[string]S3BatchConfig  `json:"destinations"`
	}

	// Set up the environment variable
	os.Setenv("REVEALD_BUCKET", "reveald-log-upload-staging")
	defer os.Unsetenv("REVEALD_BUCKET")

	configJSON := []byte(`{
		"pprof": "localhost:6060",
		"sources": {
			"journald": {
				"maxLineLenKB": 200,
				"type": "journald"
			}
		},
		"destinations": {
			"s3_batch": {
				"type": "s3b",
				"batchSize": 10000,
				"flushFrequency": "30s",
				"s3": {
					"type": "s3",
					"region": "us-east-2",
					"bucket": "$REVEALD_BUCKET"
				}
			}
		}
	}`)

	var config RealWorldConfig
	err := LoadConfig(configJSON, &config)
	assert.NoError(t, err)

	// Verify the structure was parsed correctly
	assert.Equal(t, "localhost:6060", config.Pprof)
	assert.Equal(t, "journald", config.Sources["journald"].Type)
	assert.Equal(t, 200, config.Sources["journald"].MaxLineLenKB)
	
	// Verify the nested environment variable was replaced correctly
	assert.Equal(t, "s3b", config.Destinations["s3_batch"].Type)
	assert.Equal(t, 10000, config.Destinations["s3_batch"].BatchSize)
	assert.Equal(t, "30s", config.Destinations["s3_batch"].FlushFrequency)
	assert.Equal(t, "s3", config.Destinations["s3_batch"].S3.Type)
	assert.Equal(t, "us-east-2", config.Destinations["s3_batch"].S3.Region)
	assert.Equal(t, "reveald-log-upload-staging", config.Destinations["s3_batch"].S3.Bucket)
}

