package cli

import "fmt"

// ArgsFunc validates positional arguments.
type ArgsFunc func(args []string) error

// NoArgs returns an error if any positional args are present.
func NoArgs(args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("unexpected arguments: %v", args)
	}
	return nil
}

// ExactArgs returns an ArgsFunc that requires exactly n positional args.
func ExactArgs(n int) ArgsFunc {
	return func(args []string) error {
		if len(args) != n {
			return fmt.Errorf("expected exactly %d argument(s), got %d", n, len(args))
		}
		return nil
	}
}

// MinArgs returns an ArgsFunc that requires at least n positional args.
func MinArgs(n int) ArgsFunc {
	return func(args []string) error {
		if len(args) < n {
			return fmt.Errorf("expected at least %d argument(s), got %d", n, len(args))
		}
		return nil
	}
}
