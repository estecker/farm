package argo

import (
	"context"
	"encoding/json"
	"github.com/argoproj/argo-workflows/v3/cmd/argo/commands/client"
	workflowpkg "github.com/argoproj/argo-workflows/v3/pkg/apiclient/workflow"
	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	"github.com/argoproj/pkg/errors"
	argotime "github.com/argoproj/pkg/time"
	"github.com/estecker/farm/internal/pubsub"
	"github.com/jellydator/ttlcache/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"log/slog"
	"os"
	"strings"
	"time"
)

// Based on https://github.com/argoproj/argo-workflows/blob/13444e663387e3b5b331c278cd9e79fc88968d7e/pkg/apis/workflow/v1alpha1/workflow_types.go#L222C1-L238C2
var (
	activeInWindow = func(t time.Time) wfv1.WorkflowPredicate {
		return func(wf wfv1.Workflow) bool {
			return wf.ObjectMeta.CreationTimestamp.After(t) || wf.Status.FinishedAt.Time.After(t)
		}
	}
)

// Get workflows for both use cases, completed and not completed but recently changed
// Most logic from https://github.com/argoproj/argo-workflows/blob/6a39edf366319a40d37ccf406fe27dcee3d15705/cmd/argo/commands/list.go#L127
func listWorkflows(ctx context.Context, serviceClient workflowpkg.WorkflowServiceClient, nameSpace string) (wfv1.Workflows, error) {
	listOpts := &metav1.ListOptions{
		Limit: 0,
	}

	var workflows wfv1.Workflows
	for {
		wfList, err := serviceClient.ListWorkflows(ctx, &workflowpkg.WorkflowListRequest{
			Namespace:   nameSpace,
			ListOptions: listOpts,
		})
		if err != nil {
			return nil, err
		}
		workflows = append(workflows, wfList.Items...)
		if wfList.Continue == "" {
			break
		}
		listOpts.Continue = wfList.Continue
	}
	t, er := argotime.ParseSince("10m")
	errors.CheckError(er)
	workflows = workflows.Filter(activeInWindow(*t))
	return workflows, nil
}

// The "name" is not always the same, so we need to normalize it
func normalizeName(wf wfv1.Workflow) string {
	if n, ok := wf.ObjectMeta.Labels["workflows.argoproj.io/cron-workflow"]; ok {
		return n
	} else if n, ok := wf.ObjectMeta.Labels["workflows.argoproj.io/workflow-template"]; ok {
		return n
	} else if n := wf.ObjectMeta.GenerateName; n != "" {
		before, _ := strings.CutSuffix(n, "-")
		return before
	}
	return wf.ObjectMeta.Name
}

// Generate the URL for the workflow so we can click on a link and see the workflow in DD
func wfUrl(wf wfv1.Workflow) string {
	//"https://nz-argo-staging.cirrus.ai/workflows/network-staging/rm-cron-tagging-log-net-od-1704511800"
	config, err := rest.InClusterConfig()
	if err != nil {
		return "" //running farm locally?
	}
	clientset, _ := kubernetes.NewForConfig(config)
	ingress, err := clientset.NetworkingV1().Ingresses("argo").List(context.TODO(), metav1.ListOptions{FieldSelector: "metadata.name=argo-server"})
	if err != nil {
		return err.Error()
	}
	return "https://" + ingress.Items[0].ObjectMeta.Annotations["external-dns.alpha.kubernetes.io/hostname"] + "/workflows/" + wf.ObjectMeta.Namespace + "/" + wf.Name
}

// Main loop for collecting Argo events
func collect(projectID string, saEmail string, workflows wfv1.Workflows, cache *ttlcache.Cache[types.UID, wfv1.WorkflowPhase], topicProjectID string) {
	for _, wf := range workflows {
		_ = wf
		UID := wf.GetUID()
		if !cache.Has(UID) || cache.Get(UID).Value() != wf.Status.Phase {
			e := Event{
				Name:              wf.Name,
				NormalizedName:    normalizeName(wf),
				NameSpace:         wf.ObjectMeta.Namespace,
				Kind:              wf.GetObjectKind().GroupVersionKind().Kind,
				URL:               wfUrl(wf),
				Phase:             string(wf.Status.Phase),
				WorkflowTemplate:  wf.ObjectMeta.Labels["workflows.argoproj.io/workflow-template"],
				CreationTimestamp: wf.ObjectMeta.CreationTimestamp.UnixMicro(),
			}
			// if there are parameters, will send them as json in json
			// prevents sending the string "null" when there are no parameters
			if wf.ObjectMeta.Annotations != nil {
				annotations, _ := json.Marshal(wf.ObjectMeta.Annotations)
				e.Annotations = string(annotations) //json in json
			}
			if wf.ObjectMeta.Labels != nil {
				labels, _ := json.Marshal(wf.ObjectMeta.Labels)
				e.Labels = string(labels) //json in json
			}
			if wf.Spec.Arguments.Parameters != nil {
				params, _ := json.Marshal(wf.Spec.Arguments.Parameters) //Going to be json in json pubsub message
				e.Parameters = string(params)                           //json in json
			}
			// otherwise will send the zero value date, which is not null
			if !wf.Status.StartedAt.IsZero() {
				e.StartedAt = wf.Status.StartedAt.UnixMicro()
			}
			if !wf.Status.FinishedAt.IsZero() {
				e.FinishedAt = wf.Status.FinishedAt.UnixMicro()
			}
			msg, err := json.Marshal(e)
			if err != nil {
				slog.Error("Error marshalling Argo event.", "error", err)
			}
			attributes := map[string]string{
				"project_id":  projectID,
				"sa_email":    saEmail,
				"type":        "argo",
				"tenant":      os.Getenv("tenant"),
				"environment": os.Getenv("DD_ENV"),
			}
			msgID, err := pubsub.Publish(msg, topicProjectID, "farm", attributes)
			if err == nil {
				cache.Set(wf.UID, wf.Status.Phase, 0)
				slog.Debug("pubsub.publish",
					"type", "argo",
					"phase", wf.Status.Phase,
					"name", wf.ObjectMeta.Name,
					"msgID", msgID)
			} else {
				slog.Error("Argo: pubsub Error publishing to pubsub", "error", err, "msgID", msgID)
			}
			if wf.Status.Phase.Completed() {
				trace(wf)
			}
		}
	}
}

// Main loop for collecting Argo events, runs forever
func Exec(ctx context.Context, projectID string, saEmail string, nameSpace string, topicProjectID string) {
	cache := ttlcache.New[types.UID, wfv1.WorkflowPhase](ttlcache.WithTTL[types.UID, wfv1.WorkflowPhase](time.Hour))
	ctx, apiClient := client.NewAPIClient(ctx)
	serviceClient := apiClient.NewWorkflowServiceClient()
	for {
		createdSinceWf, _ := listWorkflows(ctx, serviceClient, nameSpace) //Something changed recently, might be completed too
		collect(projectID, saEmail, createdSinceWf, cache, topicProjectID)
		time.Sleep(191 * time.Second)
		cache.DeleteExpired()
	}
}
