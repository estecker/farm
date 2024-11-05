package argo

// An Argo event to publish to PubSub
type Event struct {
	Name              string `json:"name,omitempty"`
	NormalizedName    string `json:"normalized_name,omitempty"`
	NameSpace         string `json:"namespace,omitempty"`
	Kind              string `json:"kind,omitempty"`
	URL               string `json:"url,omitempty"`
	Phase             string `json:"phase,omitempty"`
	WorkflowTemplate  string `json:"workflow_template,omitempty"`  //metadata.labels."workflows.argoproj.io/workflow-template"
	Labels            string `json:"labels,omitempty"`             //JSON metadata.labels
	Annotations       string `json:"annotations,omitempty"`        //JSON metadata.annotations
	CreationTimestamp int64  `json:"creation_timestamp,omitempty"` //metadata.creationTimestamp
	Parameters        string `json:"parameters,omitempty"`         //JSON spec.arguments.parameters
	StartedAt         int64  `json:"started_at,omitempty"`         //status.startedAt
	FinishedAt        int64  `json:"finished_at,omitempty"`        //status.finishedAt
}
