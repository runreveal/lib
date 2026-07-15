package cli

import (
	"fmt"
	"io"
	"strings"
)

func printAppHelp(w io.Writer, appName, desc string, children []Node, version, defconCmd string) {
	if desc != "" {
		fmt.Fprintf(w, "%s - %s\n\n", appName, desc)
	} else {
		fmt.Fprintf(w, "%s\n\n", appName)
	}

	fmt.Fprintf(w, "Usage:\n")
	if len(children) > 0 {
		fmt.Fprintf(w, "  %s <command> [flags]\n\n", appName)
	} else {
		fmt.Fprintf(w, "  %s [flags]\n\n", appName)
	}

	if len(children) > 0 {
		fmt.Fprintf(w, "Commands:\n")
		maxLen := maxNodeNameLen(children)
		for _, child := range children {
			fmt.Fprintf(w, "  %-*s  %s\n", maxLen, child.nodeName(), child.nodeDesc())
		}
		fmt.Fprintln(w)
	}

	fmt.Fprintf(w, "Flags:\n")
	fmt.Fprintf(w, "  -h, --help      show help\n")
	if version != "" {
		fmt.Fprintf(w, "      --version   show version\n")
	}

	if len(children) > 0 {
		fmt.Fprintf(w, "\nUse \"%s <command> --help\" for more information.\n", appName)
	}
	if defconCmd != "" {
		fmt.Fprintf(w, "Use \"%s %s\" to print the default configuration.\n", appName, defconCmd)
	}
}

// printAliases prints an "Aliases:" line when a node has alternate names.
func printAliases(w io.Writer, aliases []string) {
	if len(aliases) > 0 {
		fmt.Fprintf(w, "Aliases: %s\n\n", strings.Join(aliases, ", "))
	}
}

func printLongText(w io.Writer, long string) {
	for _, line := range strings.Split(long, "\n") {
		if line == "" {
			fmt.Fprintln(w)
		} else {
			fmt.Fprintf(w, "  %s\n", line)
		}
	}
	fmt.Fprintln(w)
}

func printGroupHelp(w io.Writer, appName, path, desc, long string, aliases []string, children []Node) {
	if desc != "" {
		fmt.Fprintf(w, "%s %s - %s\n\n", appName, path, desc)
	} else {
		fmt.Fprintf(w, "%s %s\n\n", appName, path)
	}

	printAliases(w, aliases)

	if long != "" {
		printLongText(w, long)
	}

	fmt.Fprintf(w, "Usage:\n")
	fmt.Fprintf(w, "  %s %s <command> [flags]\n\n", appName, path)

	if len(children) > 0 {
		fmt.Fprintf(w, "Commands:\n")
		maxLen := maxNodeNameLen(children)
		for _, child := range children {
			fmt.Fprintf(w, "  %-*s  %s\n", maxLen, child.nodeName(), child.nodeDesc())
		}
		fmt.Fprintln(w)
	}

	fmt.Fprintf(w, "Flags:\n")
	fmt.Fprintf(w, "  -h, --help   show help\n")
	fmt.Fprintf(w, "\nUse \"%s %s <command> --help\" for more information.\n", appName, path)
}

func printCommandHelp(
	w io.Writer, appName, path, desc, long string, aliases []string, handler Runnable, children []Node, globals any,
) {
	if desc != "" {
		fmt.Fprintf(w, "%s %s - %s\n\n", appName, path, desc)
	} else {
		fmt.Fprintf(w, "%s %s\n\n", appName, path)
	}

	printAliases(w, aliases)

	if long != "" {
		printLongText(w, long)
	}

	fmt.Fprintf(w, "Usage:\n")
	if len(children) > 0 {
		fmt.Fprintf(w, "  %s %s [command] [flags]\n\n", appName, path)
	} else {
		fmt.Fprintf(w, "  %s %s [flags]\n\n", appName, path)
	}

	if len(children) > 0 {
		fmt.Fprintf(w, "Commands:\n")
		maxLen := maxNodeNameLen(children)
		for _, child := range children {
			fmt.Fprintf(w, "  %-*s  %s\n", maxLen, child.nodeName(), child.nodeDesc())
		}
		fmt.Fprintln(w)
	}

	// Print flags from handler + globals
	fs, _, err := buildFlagSet(handler)
	if err == nil {
		if globals != nil {
			if _, gerr := addGlobalsToFlagSet(fs, globals); gerr != nil {
				fmt.Fprintf(w, "  (error loading global flags: %s)\n", gerr)
			}
		}
		if len(fs.defs) > 0 {
			fmt.Fprintf(w, "Flags:\n")
			printFlagDefs(w, fs.defs)
			fmt.Fprintln(w)
		} else {
			fmt.Fprintf(w, "Flags:\n")
		}
	} else {
		fmt.Fprintf(w, "Flags:\n")
	}
	fmt.Fprintf(w, "  -h, --help   show help\n")

	// Append extra help from handler and/or globals if they implement HelpExtra.
	if he, ok := handler.(HelpExtra); ok {
		fmt.Fprint(w, he.ExtraHelp())
	}
	if globals != nil {
		if he, ok := globals.(HelpExtra); ok {
			fmt.Fprint(w, he.ExtraHelp())
		}
	}
}

func printFlagDefs(w io.Writer, defs []*flagDef) {
	// Calculate column widths
	maxFlag := 0
	for _, d := range defs {
		s := flagString(d)
		if len(s) > maxFlag {
			maxFlag = len(s)
		}
	}

	for _, d := range defs {
		fs := flagString(d)
		padding := strings.Repeat(" ", maxFlag-len(fs))
		if d.defVal != "" {
			fmt.Fprintf(w, "  %s%s   %s (default: %s)\n", fs, padding, d.usage, d.defVal)
		} else {
			fmt.Fprintf(w, "  %s%s   %s\n", fs, padding, d.usage)
		}
	}
}

func flagString(d *flagDef) string {
	var sb strings.Builder
	if d.short != "" {
		sb.WriteString("-")
		sb.WriteString(d.short)
		sb.WriteString(", ")
	} else {
		sb.WriteString("    ")
	}
	sb.WriteString("--")
	sb.WriteString(d.long)
	if d.typeName != "" {
		sb.WriteString(" ")
		sb.WriteString(d.typeName)
	}
	return sb.String()
}

func maxNodeNameLen(nodes []Node) int {
	max := 0
	for _, n := range nodes {
		if l := len(n.nodeName()); l > max {
			max = l
		}
	}
	return max
}
