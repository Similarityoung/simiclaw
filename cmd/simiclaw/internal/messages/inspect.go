package messages

import "fmt"

type InspectText struct {
	HealthHeader   string
	SessionsHeader string
	EventsHeader   string
	RunsHeader     string
	NextCursor     string
	ToolExecutions string
	ErrorLine      string
}

var Inspect = InspectText{
	HealthHeader:   "endpoint\tstatus",
	SessionsHeader: "conversation_id\tsession_key\tmessage_count\tlast_model\tlast_activity_at",
	EventsHeader:   "event_id\tstatus\trun_id\tsession_key\tupdated_at",
	RunsHeader:     "run_id\tstatus\trun_mode\tevent_id\tstarted_at",
	NextCursor:     "\nnext_cursor: %s\n",
	ToolExecutions: "\nTool executions: %d\n",
	ErrorLine:      "Error: %s: %s\n",
}

func (i InspectText) NextCursorLine(cursor string) string {
	return fmt.Sprintf(i.NextCursor, cursor)
}

func (i InspectText) ToolExecutionsLine(count int) string {
	return fmt.Sprintf(i.ToolExecutions, count)
}

func (i InspectText) Error(code, message string) string {
	return fmt.Sprintf(i.ErrorLine, code, message)
}
