package version

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

const AppVersion = "v0.4"

func Run() {
	_, _ = fmt.Fprintln(os.Stdout, AppVersion)
}

func NewCommand(out io.Writer) *cobra.Command {
	if out == nil {
		out = os.Stdout
	}
	return &cobra.Command{
		Use:   "version",
		Short: "输出版本号",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(out, AppVersion)
			return err
		},
	}
}
