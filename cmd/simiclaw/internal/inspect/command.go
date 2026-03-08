package inspect

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/client"
	"github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/common"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func NewCommand(streams common.IOStreams, globals *common.RuntimeFlagValues) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect",
		Short: "查询服务运行状态与数据",
	}
	cmd.AddCommand(newHealthCommand(streams, globals))
	cmd.AddCommand(newSessionsCommand(streams, globals))
	cmd.AddCommand(newEventsCommand(streams, globals))
	cmd.AddCommand(newRunsCommand(streams, globals))
	cmd.AddCommand(newTraceCommand(streams, globals))
	return cmd
}

func newHealthCommand(streams common.IOStreams, globals *common.RuntimeFlagValues) *cobra.Command {
	return &cobra.Command{
		Use:   "health",
		Short: "查询 healthz 与 readyz",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			opts, cli, err := newClient(globals, streams)
			if err != nil {
				return common.WrapExit(2, err)
			}
			report, err := cli.Health(ctx)
			if err != nil {
				return err
			}
			return render(streams.Out, opts.Output, report, func(w io.Writer) {
				tw := newTabWriter(w)
				fmt.Fprintln(tw, "endpoint\tstatus")
				fmt.Fprintf(tw, "healthz\t%v\n", report.Health["status"])
				fmt.Fprintf(tw, "readyz\t%v\n", report.Ready["status"])
				_ = tw.Flush()
			})
		},
	}
}

func newSessionsCommand(streams common.IOStreams, globals *common.RuntimeFlagValues) *cobra.Command {
	var (
		limit          int
		cursor         string
		sessionKey     string
		conversationID string
	)
	cmd := &cobra.Command{
		Use:   "sessions",
		Short: "列出会话",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
			defer cancel()
			opts, cli, err := newClient(globals, streams)
			if err != nil {
				return common.WrapExit(2, err)
			}
			page, err := cli.ListSessions(ctx, sessionKey, conversationID, cursor, limit)
			if err != nil {
				return err
			}
			return render(streams.Out, opts.Output, page, func(w io.Writer) {
				tw := newTabWriter(w)
				fmt.Fprintln(tw, "conversation_id\tsession_key\tmessage_count\tlast_model\tlast_activity_at")
				for _, item := range page.Items {
					fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\n", item.ConversationID, item.SessionKey, item.MessageCount, item.LastModel, item.LastActivityAt.Format(time.RFC3339))
				}
				_ = tw.Flush()
				if page.NextCursor != "" {
					fmt.Fprintf(w, "\nnext_cursor: %s\n", page.NextCursor)
				}
			})
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 20, "返回条数")
	cmd.Flags().StringVar(&cursor, "cursor", "", "分页游标")
	cmd.Flags().StringVar(&sessionKey, "session-key", "", "按 session_key 过滤")
	cmd.Flags().StringVar(&conversationID, "conversation", "", "按 conversation_id 过滤")
	return cmd
}

func newEventsCommand(streams common.IOStreams, globals *common.RuntimeFlagValues) *cobra.Command {
	var (
		limit      int
		cursor     string
		sessionKey string
		status     string
	)
	cmd := &cobra.Command{
		Use:   "events",
		Short: "列出事件",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
			defer cancel()
			opts, cli, err := newClient(globals, streams)
			if err != nil {
				return common.WrapExit(2, err)
			}
			page, err := cli.ListEvents(ctx, sessionKey, model.EventStatus(strings.TrimSpace(status)), cursor, limit)
			if err != nil {
				return err
			}
			return render(streams.Out, opts.Output, page, func(w io.Writer) {
				tw := newTabWriter(w)
				fmt.Fprintln(tw, "event_id\tstatus\trun_id\tsession_key\tupdated_at")
				for _, item := range page.Items {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", item.EventID, item.Status, item.RunID, item.SessionKey, item.UpdatedAt.Format(time.RFC3339))
				}
				_ = tw.Flush()
				if page.NextCursor != "" {
					fmt.Fprintf(w, "\nnext_cursor: %s\n", page.NextCursor)
				}
			})
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 20, "返回条数")
	cmd.Flags().StringVar(&cursor, "cursor", "", "分页游标")
	cmd.Flags().StringVar(&sessionKey, "session-key", "", "按 session_key 过滤")
	cmd.Flags().StringVar(&status, "status", "", "按状态过滤")
	return cmd
}

func newRunsCommand(streams common.IOStreams, globals *common.RuntimeFlagValues) *cobra.Command {
	var (
		limit      int
		cursor     string
		sessionKey string
	)
	cmd := &cobra.Command{
		Use:   "runs",
		Short: "列出运行记录",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
			defer cancel()
			opts, cli, err := newClient(globals, streams)
			if err != nil {
				return common.WrapExit(2, err)
			}
			page, err := cli.ListRuns(ctx, sessionKey, cursor, limit)
			if err != nil {
				return err
			}
			return render(streams.Out, opts.Output, page, func(w io.Writer) {
				tw := newTabWriter(w)
				fmt.Fprintln(tw, "run_id\tstatus\trun_mode\tevent_id\tstarted_at")
				for _, item := range page.Items {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", item.RunID, item.Status, item.RunMode, item.EventID, item.StartedAt.Format(time.RFC3339))
				}
				_ = tw.Flush()
				if page.NextCursor != "" {
					fmt.Fprintf(w, "\nnext_cursor: %s\n", page.NextCursor)
				}
			})
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 20, "返回条数")
	cmd.Flags().StringVar(&cursor, "cursor", "", "分页游标")
	cmd.Flags().StringVar(&sessionKey, "session-key", "", "按 session_key 过滤")
	return cmd
}

func newTraceCommand(streams common.IOStreams, globals *common.RuntimeFlagValues) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "trace <run-id>",
		Short: "查看单个 run trace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
			defer cancel()
			opts, cli, err := newClient(globals, streams)
			if err != nil {
				return common.WrapExit(2, err)
			}
			trace, err := cli.GetRunTrace(ctx, args[0])
			if err != nil {
				return err
			}
			return render(streams.Out, opts.Output, trace, func(w io.Writer) {
				tw := newTabWriter(w)
				fmt.Fprintf(tw, "run_id\t%s\n", trace.RunID)
				fmt.Fprintf(tw, "status\t%s\n", trace.Status)
				fmt.Fprintf(tw, "run_mode\t%s\n", trace.RunMode)
				fmt.Fprintf(tw, "event_id\t%s\n", trace.EventID)
				fmt.Fprintf(tw, "session_key\t%s\n", trace.SessionKey)
				fmt.Fprintf(tw, "model\t%s\n", trace.Model)
				fmt.Fprintf(tw, "provider\t%s\n", trace.Provider)
				fmt.Fprintf(tw, "started_at\t%s\n", trace.StartedAt.Format(time.RFC3339))
				fmt.Fprintf(tw, "finished_at\t%s\n", trace.FinishedAt.Format(time.RFC3339))
				fmt.Fprintf(tw, "output_text\t%s\n", trace.OutputText)
				_ = tw.Flush()
				if len(trace.ToolExecutions) > 0 {
					fmt.Fprintf(w, "\n工具调用: %d\n", len(trace.ToolExecutions))
				}
				if trace.Error != nil {
					fmt.Fprintf(w, "错误: %s: %s\n", trace.Error.Code, trace.Error.Message)
				}
			})
		},
	}
	return cmd
}

func newClient(globals *common.RuntimeFlagValues, streams common.IOStreams) (common.RuntimeOptions, *client.Client, error) {
	opts, err := common.ResolveRuntimeOptions(*globals, streams.Out)
	if err != nil {
		return common.RuntimeOptions{}, nil, err
	}
	return opts, client.New(opts.BaseURL, opts.APIKey, opts.Timeout), nil
}

func render(w io.Writer, output string, v any, table func(io.Writer)) error {
	if output == "json" {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	}
	table(w)
	return nil
}

func newTabWriter(w io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
}
