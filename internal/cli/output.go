package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
)

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func newTabWriter() *tabwriter.Writer {
	return tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
}

func printfTable(tw *tabwriter.Writer, format string, a ...any) {
	_, _ = fmt.Fprintf(tw, format, a...)
}

func flushTable(tw *tabwriter.Writer) {
	_ = tw.Flush()
}
