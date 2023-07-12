package loader_test

import (
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

	loader.Register("aTypeOfSource", func() loader.Builder[Source] { return &srcConfigA{} })
	loader.Register("sourceThatCanB", func() loader.Builder[Source] { return &srcConfigB{} })
	loader.Register("aTypeOfDest", func() loader.Builder[Destination] { return &dstConfigA{} })
	loader.Register("bAllThatUCanB", func() loader.Builder[Destination] { return &dstConfigB{} })

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
					{&srcConfigA{"localhost"}},
					{&srcConfigB{"gym"}},
				},
				Destinations: []loader.Loader[Destination]{
					{&dstConfigA{"localhost"}},
					{&dstConfigB{"output"}},
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
	Host string `json:"host"`
}

func (c *srcConfigA) Configure() (Source, error) {
	return &srcA{c.Host}, nil
}

type srcConfigB struct {
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
	Host string
}

func (c *dstConfigA) Configure() (Destination, error) {
	return &dstA{c.Host}, nil
}

type dstConfigB struct {
	Topic string
}

func (c *dstConfigB) Configure() (Destination, error) {
	return &dstB{c.Topic}, nil
}
