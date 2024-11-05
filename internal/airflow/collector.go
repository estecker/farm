package airflow

import (
	"context"
	"encoding/json"
	"github.com/apache/airflow-client-go/airflow"
	"github.com/estecker/farm/internal/pubsub"
	"github.com/jellydator/ttlcache/v3"
	"golang.org/x/oauth2/google"
	"log/slog"
	"os"
	"regexp"
	"time"
)

// Get the Dags that I'm interested in
func getDags(ctx context.Context, cli *airflow.APIClient) (airflow.DAGCollection, error) {
	//tags := []string{"farm"} // Whitelist of DAG tags to monitor based on tags
	dags, r, err := cli.DAGApi.GetDags(ctx).OnlyActive(true).Execute()
	if err != nil {
		slog.Error("Error when calling `DAGApi.GetDags`", "error", err, "response", r)
	}
	//Now we need to filter out the dags we don't want to monitor
	var d []airflow.DAG
	var excludedDags = regexp.MustCompile(`airflow_monitoring`)
	for _, dag := range dags.GetDags() {
		if !excludedDags.MatchString(dag.GetDagId()) {
			d = append(d, dag)
		}
	}
	var dc airflow.DAGCollection
	dc.SetDags(d)
	return dc, err
}

// Get the DAG runs for the DAGs I'm interested in
func getDagRuns(ctx context.Context, cli *airflow.APIClient, dag airflow.DAG) (airflow.DAGRunCollection, error) {
	tenMinAgo := time.Now().Add(-10 * time.Minute)
	dagRuns, r, err := cli.DAGRunApi.GetDagRuns(ctx, dag.GetDagId()).Limit(10).StartDateGte(tenMinAgo).EndDateGte(tenMinAgo).Execute() //list DAG runs
	if err != nil {
		slog.Error("Error when calling `DAGRunApi.GetDagRuns`", "error", err, "response", r)
	}
	return dagRuns, err
}

// Create the event to be sent to pubsub
func main(ctx context.Context, cli *airflow.APIClient, projectID, saEmail string, run airflow.DAGRun, cache *ttlcache.Cache[string, airflow.DagState], topicProjectID string) {
	rID := run.GetDagRunId()
	rState := run.GetState()
	if !cache.Has(rID) || cache.Get(rID).Value() != rState {
		e := event{
			DagId:                  run.GetDagId(),
			DagRunId:               run.GetDagRunId(),
			LogicalDate:            run.GetLogicalDate().UnixMicro(), //I could not get strings to work
			StartDate:              run.GetStartDate().UnixMicro(),
			EndDate:                run.GetEndDate().UnixMicro(),
			DataIntervalStart:      run.GetDataIntervalStart().UnixMicro(),
			DataIntervalEnd:        run.GetDataIntervalEnd().UnixMicro(),
			LastSchedulingDecision: run.GetLastSchedulingDecision().UnixMicro(),
			RunType:                run.GetRunType(),
			State:                  string(rState),
			ExternalTrigger:        run.GetExternalTrigger(),
			Note:                   run.GetNote(),
		}
		msg, err := json.Marshal(e)
		if err != nil {
			slog.Error("Error marshalling Airflow event", "error", err)
		}
		attributes := map[string]string{
			"project_id":  projectID,
			"sa_email":    saEmail,
			"type":        "airflow",
			"tenant":      os.Getenv("tenant"),
			"environment": os.Getenv("DD_ENV"),
		}
		msgID, err := pubsub.Publish(msg, topicProjectID, "farm", attributes)
		if err == nil {
			cache.Set(rID, run.GetState(), 0)
			slog.Debug("pubsub.publish",
				"type", "airflow",
				"state", run.GetState(),
				"dagId", run.GetDagId(),
				"DagRunId", run.GetDagRunId(),
				"msgID", msgID)
		} else {
			slog.Error("Error publishing to pubsub", "error", err, "msgID", msgID)
		}
		if rState == airflow.DAGSTATE_SUCCESS || rState == airflow.DAGSTATE_FAILED {
			trace(ctx, cli, run)
		}
	}
}

// Main loop for collecting Airflow events, runs forever
func Exec(ctx context.Context, projectID string, saEmail string, host string, topicProjectID string) {
	cache := ttlcache.New[string, airflow.DagState](ttlcache.WithTTL[string, airflow.DagState](time.Hour))
	conf := airflow.NewConfiguration()
	conf.Host = host
	conf.Scheme = "https"
	client, err := google.DefaultClient(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		slog.Error("Error creating Google client", "error", err)
		os.Exit(1)
	}
	conf.HTTPClient = client
	cli := airflow.NewAPIClient(conf)
	for {
		dags, _ := getDags(ctx, cli)
		for _, dag := range dags.GetDags() { //doing it this way so not running into API rate limits
			runs, _ := getDagRuns(ctx, cli, dag)
			for _, dagRun := range runs.GetDagRuns() {
				main(ctx, cli, projectID, saEmail, dagRun, cache, topicProjectID)
			}
		}
		time.Sleep(311 * time.Second)
		cache.DeleteExpired()
	}
}
