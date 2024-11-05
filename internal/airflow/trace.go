package airflow

import (
	"context"
	"github.com/apache/airflow-client-go/airflow"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/ext"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
	"log/slog"
	"os"
	"time"
)

// statusToCode maps the status of a DAGRun to an HTTP status code
// Makes the DataDog UI look nice
func statusToCode(dr airflow.DAGRun) int {
	switch dr.GetState() {
	case airflow.DAGSTATE_SUCCESS:
		return 200
	case airflow.DAGSTATE_RUNNING, airflow.DAGSTATE_QUEUED:
		return 102
	default:
		return 500
	}
}

func trace(ctx context.Context, cli *airflow.APIClient, run airflow.DAGRun) {
	slog.Debug("trace",
		"type", "airflow:",
		"state", run.GetState(),
		"dagId", run.GetDagId(),
		"dagRunId", run.GetDagRunId())
	rootSpan := tracer.StartSpan("airflow.dagrun",
		tracer.StartTime(run.GetStartDate()),
		tracer.ResourceName(run.GetDagId()))
	rootSpan.SetTag(ext.HTTPMethod, "AIRFLOW")
	rootSpan.SetTag(ext.HTTPCode, statusToCode(run))
	rootSpan.SetTag("tenant", os.Getenv("tenant"))
	rootSpan.SetTag("url", "https://"+cli.GetConfig().Host+"/dags/"+run.GetDagId())

	dagRunSpan := tracer.StartSpan(run.GetDagRunId(),
		tracer.ServiceName(run.GetDagId()),
		tracer.StartTime(run.GetStartDate()),
		tracer.SpanType("airflow_dagrun"),
		tracer.ChildOf(rootSpan.Context()))
	dagRunSpan.SetTag("dag_run_id", run.GetDagRunId())
	dagRunSpan.SetTag("dag_id", run.GetDagId())
	dagRunSpan.SetTag("logical_date", run.GetLogicalDate())
	dagRunSpan.SetTag("start_date", run.GetStartDate())
	dagRunSpan.SetTag("end_date", run.GetEndDate())
	dagRunSpan.SetTag("data_interval_start", run.GetDataIntervalStart())
	dagRunSpan.SetTag("data_interval_end", run.GetDataIntervalEnd())
	dagRunSpan.SetTag("last_scheduling_decision", run.GetLastSchedulingDecision())
	dagRunSpan.SetTag("run_type", run.GetRunType())
	dagRunSpan.SetTag("state", run.GetState())
	dagRunSpan.SetTag("external_trigger", run.GetExternalTrigger())
	dagRunSpan.SetTag("conf", run.GetConf())
	dagRunSpan.SetTag("note", run.GetNote())
	//	dagRunSpan.SetTag("owners", "TODO")
	taskInstances, _, _ := cli.TaskInstanceApi.GetTaskInstances(ctx, run.GetDagId(), run.GetDagRunId()).Execute()
	for _, task := range taskInstances.GetTaskInstances() {
		st, _ := time.Parse(time.RFC3339, task.GetStartDate())
		et, _ := time.Parse(time.RFC3339, task.GetEndDate())
		taskSpan := tracer.StartSpan(
			task.GetTaskId(),
			tracer.ChildOf(dagRunSpan.Context()),
			tracer.StartTime(st),
			tracer.ResourceName(task.GetTaskId()))
		taskSpan.SetTag("dag_id", task.GetDagId())
		taskSpan.SetTag("dag_run_id", task.GetDagRunId())
		taskSpan.SetTag("execution_date", task.GetExecutionDate())
		taskSpan.SetTag("end_date", task.GetEndDate())
		taskSpan.SetTag("duration", task.GetDuration())
		taskSpan.SetTag("state", task.GetState())
		taskSpan.SetTag("try_number", task.GetTryNumber())
		taskSpan.SetTag("map_index", task.GetMapIndex())
		taskSpan.SetTag("max_tries", task.GetMaxTries())
		taskSpan.SetTag("hostname", task.GetHostname())
		taskSpan.SetTag("unixname", task.GetUnixname())
		taskSpan.SetTag("pool", task.GetPool())
		taskSpan.SetTag("pool_slots", task.GetPoolSlots())
		taskSpan.SetTag("queue", task.GetQueue())
		taskSpan.SetTag("priority_weight", task.GetPriorityWeight())
		taskSpan.SetTag("operator", task.GetOperator())
		taskSpan.SetTag("queued_when", task.GetQueuedWhen())
		taskSpan.SetTag("pid", task.GetPid())
		taskSpan.SetTag("executor_config", task.GetExecutorConfig())
		taskSpan.SetTag("sla_miss", task.GetSlaMiss())
		taskSpan.SetTag("rendered_fields", task.GetRenderedFields())
		taskSpan.SetTag("trigger", task.GetTrigger())
		taskSpan.SetTag("trigger_job", task.GetTriggererJob())
		taskSpan.SetTag("note", task.GetNote())
		taskSpan.SetOperationName("dagTask")
		taskSpanFO := []tracer.FinishOption{tracer.FinishTime(et), tracer.WithError(nil)}
		taskSpan.Finish(taskSpanFO...)
	}
	dagRunSpanFO := []tracer.FinishOption{tracer.FinishTime(run.GetEndDate()), tracer.WithError(nil)}
	dagRunSpan.Finish(dagRunSpanFO...)
	rootSpan.Finish(dagRunSpanFO...)
}
