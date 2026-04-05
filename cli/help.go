package cli

import (
	"fmt"
	"io"
	"strings"
)

func printAppHelp(w io.Writer, appName, desc string, children []Node, version string) {
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
}

func printGroupHelp(w io.Writer, appName, path, desc string, children []Node) {
	if desc != "" {
		fmt.Fprintf(w, "%s %s - %s\n\n", appName, path, desc)
	} else {
		fmt.Fprintf(w, "%s %s\n\n", appName, path)
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

func printCommandHelp(w io.Writer, appName, path, desc string, handler Runnable, children []Node) {
	if desc != "" {
		fmt.Fprintf(w, "%s %s - %s\n\n", appName, path, desc)
	} else {
		fmt.Fprintf(w, "%s %s\n\n", appName, path)
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

	// Print flags from handler
	fs, _, err := buildFlagSet(handler)
	if err == nil && len(fs.defs) > 0 {
		fmt.Fprintf(w, "Flags:\n")
		printFlagDefs(w, fs.defs)
		fmt.Fprintln(w)
	} else {
		fmt.Fprintf(w, "Flags:\n")
	}
	fmt.Fprintf(w, "  -h, --help   show help\n")
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
