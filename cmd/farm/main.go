package main

import (
	"cloud.google.com/go/compute/metadata"
	"context"
	"fmt"
	"github.com/estecker/farm/internal/airflow"
	"github.com/estecker/farm/internal/argo"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	_ "go.uber.org/automaxprocs"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
	"log/slog"
	"os"
	"sync"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "farm",
	Short: "Scrape metrics from Argo and Airflow and send them to Datadog",
	Long:  `Scrape metrics from Argo and Airflow and send them to Datadog.`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	Run: func(cmd *cobra.Command, args []string) {
		slog.Info("cmd/farm/main called")
	},
}

func init() {
	cobra.OnInitialize(initConfig)
	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	_ = viper.BindPFlag("argo", rootCmd.Flags().Lookup("argo"))
	_ = viper.BindPFlag("airflow", rootCmd.Flags().Lookup("airflow"))
	_ = viper.BindPFlag("airflow_host", rootCmd.Flags().Lookup("airflow_host"))
	_ = viper.BindPFlag("argo_namespace", rootCmd.Flags().Lookup("argo_namespace"))
	_ = viper.BindPFlag("bq_project_id", rootCmd.Flags().Lookup("bq_project_id"))
}

// Get some information for publishing, but never change
// getProjectID returns the GCE project ID if running in GCE
func getProjectID() (string, error) {
	if metadata.OnGCE() {
		c := metadata.NewClient(nil)
		return c.ProjectID()
	} else {
		slog.Info("Not in GCE")
		return "", nil
	}
}

// getServiceAccountEmail returns the GCE service account email if running in GCE
// the string "default" to use the instance's main account.
func getServiceAccountEmail() (string, error) {
	if metadata.OnGCE() {
		c := metadata.NewClient(nil)
		return c.Email("default")
	} else {
		fmt.Println("Not on GCE")
		return "", nil
	}
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	viper.SetEnvPrefix("farm")
	viper.AutomaticEnv() // read in environment variables that match
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	slog.Info("Starting FARM",
		"DD_SERVICE", os.Getenv("DD_SERVICE"),
		"tenant", os.Getenv("tenant"),
		"DD_ENV", os.Getenv("DD_ENV"))
	ctx := context.Background()

	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
	projectID, _ := getProjectID()
	saEmail, _ := getServiceAccountEmail()
	var topicProjectID string
	if !viper.IsSet("topic_project_id") {
		slog.Error("topic_project_id not set will use own project_id")
		topicProjectID = projectID
	} else {
		topicProjectID = viper.GetString("topic_project_id")
	}

	logger.Info("whoami",
		"projectID", projectID,
		"saEmail", saEmail)
	opts := []tracer.StartOption{tracer.WithLogStartup(false), tracer.WithRuntimeMetrics()}
	tracer.Start(opts...)
	defer tracer.Stop()
	var wg sync.WaitGroup
	if viper.GetBool("argo") {
		go argo.Exec(ctx, projectID, saEmail, viper.GetString("argo_namespace"), topicProjectID)
		wg.Add(1)
	}
	if viper.GetBool("airflow") {
		if viper.GetString("airflow_host") == "" {
			slog.Error("FARM: Airflow host not set")
			os.Exit(1)
		}
		go airflow.Exec(ctx, projectID, saEmail, viper.GetString("airflow_host"), topicProjectID)
		wg.Add(1)
	}
	wg.Wait()
}
