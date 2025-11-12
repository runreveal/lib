package loader_test

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/runreveal/lib/loader"
	"github.com/stretchr/testify/assert"
)

type Source interface {
	Recv() (string, error)
}

type Destination interface {
	Send(string) error
}

type Config struct {
	Name         string                       `json:"name"`
	Sources      []loader.Loader[Source]      `json:"sources"`
	Destinations []loader.Loader[Destination] `json:"destinations"`
}

type unregistered struct {
}

type ConfigUnreg struct {
	Name string                      `json:"name"`
	Fire loader.Loader[unregistered] `json:"fire"`
}

func TestLoadConfigUnreg(t *testing.T) {
	var actual ConfigUnreg
	var testInput = []byte(`{"fire": {"type": "unregistered"}}`)
	err := loader.LoadConfig(testInput, &actual)
	assert.Error(t, err)
}

func TestLoadConfig(t *testing.T) {

	loader.Register("aTypeOfSource", func() loader.Builder[Source] { return &srcConfigA{Type: "aTypeOfSource"} })
	loader.Register("sourceThatCanB", func() loader.Builder[Source] { return &srcConfigB{Type: "sourceThatCanB"} })
	loader.Register("aTypeOfDest", func() loader.Builder[Destination] { return &dstConfigA{Type: "aTypeOfDest"} })
	loader.Register("bAllThatUCanB", func() loader.Builder[Destination] { return &dstConfigB{Type: "bAllThatUCanB"} })

	tests := []struct {
		name     string
		input    []byte
		expected Config
		err      bool
	}{
		{
			name: "happy",
			input: []byte(`{
				// Name from the environment
				"name": "$TEST_ENV",
				"sources": [
					{
						"type": "aTypeOfSource",
						"host": "localhost",
					},
					{
						"type": "sourceThatCanB",
						// Take topic from the environment
						"topic": "$TEST_TOPIC",
					},
				],
				"destinations": [
					{
						"type": "aTypeOfDest",
						"host": "localhost",
					},
					{
						"type": "bAllThatUCanB",
						"topic": "output",
					},
				],
			}`),
			expected: Config{
				Name: "jimmy",
				Sources: []loader.Loader[Source]{
					{&srcConfigA{Type: "aTypeOfSource", Host: "localhost"}},
					{&srcConfigB{Type: "sourceThatCanB", Topic: "gym"}},
				},
				Destinations: []loader.Loader[Destination]{
					{&dstConfigA{Type: "aTypeOfDest", Host: "localhost"}},
					{&dstConfigB{Type: "bAllThatUCanB", Topic: "output"}},
				},
			},
			err: false,
		},
		{
			name:     "malformed json",
			input:    []byte(`{"key: "$TEST_ENV"`),
			expected: Config{},
			err:      true,
		},
		{
			name:     "unregistered",
			input:    []byte(`{"sources": [{"type": "unregistered"}]}`),
			expected: Config{},
			err:      true,
		},
		{
			name:     "missing type",
			input:    []byte(`{"sources": [{"name": "hi"}]}`),
			expected: Config{},
			err:      true,
		},
	}

	os.Setenv("TEST_ENV", "jimmy")
	defer os.Unsetenv("TEST_ENV")

	os.Setenv("TEST_TOPIC", "gym")
	defer os.Unsetenv("TEST_TOPIC")

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var actual Config
			err := loader.LoadConfig(test.input, &actual)
			if !test.err {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
			if err == nil {
				assert.Equal(t, test.expected, actual, "expected and actual should be equal")
				fmt.Printf("%+v\n", actual)
				for _, srcCfg := range actual.Sources {
					src, _ := srcCfg.Configure()
					fmt.Printf("%+v, %T\n", src, src)
				}
				for _, dstCfg := range actual.Destinations {
					dst, _ := dstCfg.Configure()
					fmt.Printf("%+v, %T\n", dst, dst)
				}
			}
		})
	}
}

type srcA struct{ host string }

func (s *srcA) Recv() (string, error) { return s.host, nil }

type srcB struct{ topic string }

func (s *srcB) Recv() (string, error) { return s.topic, nil }

type srcConfigA struct {
	Type string `json:"type"`
	Host string `json:"host"`
}

func (c *srcConfigA) Configure() (Source, error) {
	return &srcA{c.Host}, nil
}

type srcConfigB struct {
	Type  string `json:"type"`
	Topic string `json:"topic"`
}

func (c *srcConfigB) Configure() (Source, error) {
	return &srcB{c.Topic}, nil
}

type dstA struct{ host string }

func (s *dstA) Send(string) error { return nil }

type dstB struct{ topic string }

func (s *dstB) Send(string) error { return nil }

type dstConfigA struct {
	Type string `json:"type"`
	Host string `json:"host"`
}

func (c *dstConfigA) Configure() (Destination, error) {
	return &dstA{c.Host}, nil
}

type dstConfigB struct {
	Type  string `json:"type"`
	Topic string `json:"topic"`
}

func (c *dstConfigB) Configure() (Destination, error) {
	return &dstB{c.Topic}, nil
}

func TestMarshalJSON(t *testing.T) {
	// Register test types if not already registered from other tests
	loader.Register("testSource", func() loader.Builder[Source] { return &srcConfigA{Type: "testSource"} })

	tests := []struct {
		name     string
		loader   loader.Loader[Source]
		expected string
	}{
		{
			name:     "marshal source config",
			loader:   loader.Loader[Source]{&srcConfigA{Type: "testSource", Host: "test-host"}},
			expected: `{"type":"testSource","host":"test-host"}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Marshal the loader
			data, err := json.Marshal(test.loader)
			assert.NoError(t, err)

			// Compare with expected JSON string
			assert.JSONEq(t, test.expected, string(data))

			// Try unmarshaling back to verify round-trip
			var newLoader loader.Loader[Source]
			err = json.Unmarshal(data, &newLoader)
			assert.NoError(t, err)

			// Verify the unmarshaled object produces the same JSON
			newData, err := json.Marshal(newLoader)
			assert.NoError(t, err)
			assert.JSONEq(t, test.expected, string(newData))
		})
	}
}
