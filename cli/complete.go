package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// Completer is optionally implemented by command handlers to provide
// custom completions for positional arguments.
type Completer interface {
	Complete(ctx context.Context, args []string) []Completion
}

// Completion represents a single shell completion suggestion.
type Completion struct {
	Value       string
	Description string
}

// handleCompletion checks if args[0] is "completion" or "__complete" and
// handles them directly. Returns true if handled, false otherwise.
func (a *App) handleCompletion(args []string) (int, bool) {
	if len(args) == 0 {
		return 0, false
	}
	switch args[0] {
	case "completion":
		return a.handleCompletionScript(args[1:]), true
	case "__complete":
		a.handleCompleteRequest(args[1:])
		return 0, true
	}
	return 0, false
}

func (a *App) handleCompletionScript(args []string) int {
	if len(args) != 1 {
		fmt.Fprintf(
			a.output,
			"Usage: %s completion <bash|zsh|fish>\n",
			a.name,
		)
		return 1
	}
	// Completion scripts and __complete output go to stdout, not
	// a.output (which defaults to stderr). The shell reads stdout.
	switch args[0] {
	case "bash":
		writeBashCompletion(a.stdout, a.name)
	case "zsh":
		writeZshCompletion(a.stdout, a.name)
	case "fish":
		writeFishCompletion(a.stdout, a.name)
	default:
		fmt.Fprintf(
			a.output,
			"unsupported shell %q, expected bash, zsh, or fish\n",
			args[0],
		)
		return 1
	}
	return 0
}

func (a *App) handleCompleteRequest(args []string) {
	completions := a.computeCompletions(context.Background(), args)
	for _, c := range completions {
		if c.Description != "" {
			fmt.Fprintf(a.stdout, "%s\t%s\n", c.Value, c.Description)
		} else {
			fmt.Fprintf(a.stdout, "%s\n", c.Value)
		}
	}
}

func (a *App) computeCompletions(
	ctx context.Context, args []string,
) []Completion {
	// Walk the command tree to find the deepest matching node.
	children := a.children
	var currentNode Node
	consumed := 0

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			break
		}
		found := false
		for _, child := range children {
			if child.nodeName() == arg {
				currentNode = child
				consumed = i + 1
				switch n := child.(type) {
				case *commandNode:
					children = n.children
				case *groupNode:
					children = n.children
				}
				found = true
				break
			}
		}
		if !found {
			break
		}
	}

	remaining := args[consumed:]

	// Determine what the user is currently typing (the last token).
	var current string
	if len(remaining) > 0 {
		current = remaining[len(remaining)-1]
	}

	// If the current word starts with -, complete flags.
	if strings.HasPrefix(current, "-") {
		return a.completeFlags(currentNode, current)
	}

	// Otherwise complete subcommands, or positional args via Completer.
	var completions []Completion

	// Subcommand completions from the current level's children.
	for _, child := range children {
		name := child.nodeName()
		if strings.HasPrefix(name, current) {
			completions = append(completions, Completion{
				Value:       name,
				Description: child.nodeDesc(),
			})
		}
	}

	// If the current node is a command with a Completer handler,
	// include its completions for positional args.
	if cn, ok := currentNode.(*commandNode); ok {
		if comp, ok := cn.handler.(Completer); ok {
			// Pass remaining args (excluding the partial word being
			// completed) as context.
			var priorArgs []string
			for _, r := range remaining {
				if !strings.HasPrefix(r, "-") && r != current {
					priorArgs = append(priorArgs, r)
				}
			}
			custom := comp.Complete(ctx, priorArgs)
			for _, c := range custom {
				if strings.HasPrefix(c.Value, current) {
					completions = append(completions, c)
				}
			}
		}
	}

	return completions
}

func (a *App) completeFlags(
	node Node, current string,
) []Completion {
	var defs []*flagDef

	if cn, ok := node.(*commandNode); ok {
		fs, _, err := buildFlagSet(cn.handler)
		if err == nil {
			if a.globals != nil {
				if _, gerr := addGlobalsToFlagSet(fs, a.globals); gerr != nil {
					return nil
				}
			}
			defs = fs.defs
		}
	}

	// Always include --help.
	defs = append(defs, &flagDef{
		long:  "help",
		short: "h",
		usage: "show help",
	})

	// If globals are set but no specific command node, still offer
	// global flags.
	if node == nil && a.globals != nil {
		fs := newFlagSet()
		if _, err := addGlobalsToFlagSet(fs, a.globals); err == nil {
			defs = append(defs, fs.defs...)
		}
	}

	prefix := strings.TrimLeft(current, "-")
	var completions []Completion
	seen := map[string]bool{}
	for _, d := range defs {
		flag := "--" + d.long
		if seen[flag] {
			continue
		}
		seen[flag] = true
		if strings.HasPrefix(d.long, prefix) {
			completions = append(completions, Completion{
				Value:       flag,
				Description: d.usage,
			})
		}
	}
	return completions
}

// writeBashCompletion outputs a bash completion script for the given app.
func writeBashCompletion(w io.Writer, appName string) {
	fmt.Fprintf(w, `_%[1]s_completions() {
    local cur="${COMP_WORDS[COMP_CWORD]}"
    local IFS=$'\n'
    local args=("${COMP_WORDS[@]:1:$COMP_CWORD}")
    local completions
    completions=$(%[1]s __complete "${args[@]}" 2>/dev/null)
    COMPREPLY=()
    while IFS=$'\t' read -r val desc; do
        COMPREPLY+=("$val")
    done <<< "$completions"
}
complete -F _%[1]s_completions %[1]s
`, appName)
}

// writeZshCompletion outputs a zsh completion script for the given app.
func writeZshCompletion(w io.Writer, appName string) {
	fmt.Fprintf(w, `#compdef %[1]s

_%[1]s() {
    local -a completions
    local IFS=$'\n'
    local args=("${words[@]:1:$CURRENT-1}")
    completions=($(${words[1]} __complete "${args[@]}" 2>/dev/null))
    local -a descs
    for line in "${completions[@]}"; do
        if [[ "$line" == *$'\t'* ]]; then
            local val="${line%%%%$'\t'*}"
            local desc="${line#*$'\t'}"
            descs+=("${val}:${desc}")
        else
            descs+=("${line}")
        fi
    done
    _describe 'completions' descs
}

compdef _%[1]s %[1]s
`, appName)
}

// writeFishCompletion outputs a fish completion script for the given app.
func writeFishCompletion(w io.Writer, appName string) {
	fmt.Fprintf(w, `complete -c %[1]s -f -a '(%[1]s __complete (commandline -cop) 2>/dev/null | string replace -r "\\t.*" "")'
`, appName)
}
