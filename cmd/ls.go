package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/puppetlabs/wash/api"
	"github.com/puppetlabs/wash/api/client"
	"github.com/puppetlabs/wash/config"
)

func lsCommand() *cobra.Command {
	lsCmd := &cobra.Command{
		Use:   "ls [file]",
		Short: "Lists the resources at the indicated path.",
		Args:  cobra.MaximumNArgs(1),
	}

	lsCmd.RunE = toRunE(lsMain)

	return lsCmd
}

func formatListEntries(ls []api.ListEntry) string {
	headers := []columnHeader{
		{"size", "NAME"},
		{"ctime", "CREATED"},
		{"verbs", "ACTIONS"},
	}
	table := make([][]string, len(ls))
	for i, entry := range ls {
		var ctimeStr string
		if entry.Attributes.Ctime.IsZero() {
			ctimeStr = "<unknown>"
		} else {
			ctimeStr = entry.Attributes.Ctime.Format(time.RFC822)
		}

		actions := entry.Actions
		sort.Strings(actions)
		verbs := strings.Join(actions, ", ")

		name := entry.Name
		isDir := actions[sort.SearchStrings(actions, "list")] == "list"
		if isDir {
			name += "/"
		}

		table[i] = []string{name, ctimeStr, verbs}
	}
	return formatTabularListing(headers, table)
}

func lsMain(cmd *cobra.Command, args []string) exitCode {
	var path string
	if len(args) > 0 {
		path = args[0]
	} else {
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return exitCode{1}
		}

		path = cwd
	}

	apiPath, err := client.APIKeyFromPath(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitCode{1}
	}

	conn := client.ForUNIXSocket(config.Socket)

	ls, err := conn.List(apiPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitCode{1}
	}

	// TODO: Handle individual ListEntry errors
	fmt.Print(formatListEntries(ls))
	return exitCode{0}
}
