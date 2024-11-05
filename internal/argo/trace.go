package argo

import (
	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/ext"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
	"log/slog"
	"os"
)

// statusToCode maps the status of a Workflow to an HTTP status code
// Makes the DataDog UI look nice
func statusToCode(wf wfv1.Workflow) int {
	switch wf.Status.Phase {
	case "Succeeded":
		return 200
	default:
		return 500
	}
}

// Create a DataDog trace for an Argo workflow
func trace(wf wfv1.Workflow) {
	slog.Debug("trace",
		"phase", wf.Status.Phase,
		"name", wf.ObjectMeta.Name)
	name := normalizeName(wf)
	rootSpan := tracer.StartSpan("argo_workflow",
		tracer.StartTime(wf.ObjectMeta.CreationTimestamp.Time),
		tracer.ResourceName(name))
	rootSpan.SetTag(ext.HTTPCode, statusToCode(wf))
	rootSpan.SetTag(ext.HTTPMethod, "ARGO")
	rootSpan.SetTag("tenant", os.Getenv("tenant"))
	rootSpan.SetTag("url", wfUrl(wf))

	wfSpan := tracer.StartSpan(name,
		tracer.ServiceName(name),
		tracer.StartTime(wf.Status.StartedAt.Time),
		tracer.SpanType("argo_workflow"),
		tracer.ChildOf(rootSpan.Context()))
	wfSpan.SetTag("component", "argo")
	wfSpan.SetTag("generate_name", wf.ObjectMeta.GenerateName)
	wfSpan.SetTag("namespace", wf.ObjectMeta.Namespace)
	wfSpan.SetTag("completed", wf.ObjectMeta.Labels["workflows.argoproj.io/completed"])
	wfSpan.SetTag("phase", wf.Status.Phase)
	wfSpan.SetTag("name", wf.ObjectMeta.Name)
	wfSpan.SetTag("workflow", wf.ObjectMeta.UID)

	for _, node := range wf.Status.Nodes {
		nodeSpan := tracer.StartSpan(
			node.DisplayName,
			tracer.ResourceName(node.DisplayName),
			tracer.ChildOf(wfSpan.Context()),
			tracer.StartTime(node.StartedAt.Time),
			tracer.ResourceName(node.Name))
		nodeSpan.SetTag("parent", node.Name)
		nodeSpan.SetTag("children", node.Children)
		nodeSpan.SetTag("dagtask", node.IsDAGTask())
		nodeSpan.SetTag("span.kind", "consumer")
		nodeSpan.SetTag("component", name)
		nodeSpan.SetTag("started", node.StartedAt.Time)
		nodeSpan.SetTag("finished", node.FinishedAt.Time)
		nodeSpan.SetOperationName(string(node.Type))
		fo := []tracer.FinishOption{tracer.FinishTime(node.FinishedAt.Time), tracer.WithError(nil)}
		nodeSpan.Finish(fo...)
	}
	finishOptions := []tracer.FinishOption{tracer.FinishTime(wf.Status.FinishedAt.Time), tracer.WithError(nil)} //wf.Status.Phase
	wfSpan.Finish(finishOptions...)
	rootSpan.Finish(finishOptions...)
}
