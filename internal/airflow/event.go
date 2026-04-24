package airflow

// An Airflow event to publish to PubSub
type event struct {
	DagId                  string `json:"dag_id,omitempty"`
	DagRunId               string `json:"dag_run_id,omitempty"`
	LogicalDate            int64  `json:"logical_date,omitempty"`
	StartDate              int64  `json:"start_date,omitempty"`
	EndDate                int64  `json:"end_date,omitempty"`
	DataIntervalStart      int64  `json:"data_interval_start,omitempty"`
	DataIntervalEnd        int64  `json:"data_interval_end,omitempty"`
	LastSchedulingDecision int64  `json:"last_scheduling_decision,omitempty"`
	RunType                string `json:"run_type,omitempty"`
	State                  string `json:"state,omitempty"`
	ExternalTrigger        bool   `json:"external_trigger,omitempty"` //BOOLEAN
	Note                   string `json:"note,omitempty"`
}
