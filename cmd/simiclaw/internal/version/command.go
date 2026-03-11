package version

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/similarityyoung/simiclaw/internal/ui/messages"
)

const AppVersion = "v1.0"

func NewCommand(out io.Writer) *cobra.Command {
	if out == nil {
		out = os.Stdout
	}
	return &cobra.Command{
		Use:   "version",
		Short: messages.Command.VersionShort,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(out, AppVersion)
			return err
		},
	}
}
