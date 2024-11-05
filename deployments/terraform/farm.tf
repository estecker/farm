
### BQ Section
resource "google_bigquery_dataset" "farm" {
  dataset_id                      = "farm"
  location                        = "EU"
  default_table_expiration_ms     = 365 * 86400000 # 365 days
  default_partition_expiration_ms = 365 * 86400000 # 365 days
}
data "google_project" "project" {
}

resource "google_project_iam_member" "editor" {
  project = data.google_project.project.project_id
  role    = "roles/bigquery.dataEditor"
  member  = "serviceAccount:service-${data.google_project.project.number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}
resource "google_bigquery_table" "airflow" {
  deletion_protection = false
  table_id            = "airflow"
  dataset_id          = google_bigquery_dataset.farm.dataset_id
  schema              = file("farm-airflow-schema.json")
  time_partitioning {
    type  = "DAY"
    field = "publish_time"
  }
}
resource "google_bigquery_table" "argo" {
  deletion_protection = false
  table_id            = "argo"
  dataset_id          = google_bigquery_dataset.farm.dataset_id
  schema              = file("farm-argo-schema.json")
  time_partitioning {
    type  = "DAY"
    field = "publish_time"
  }
}

### PubSub Section
resource "google_pubsub_topic" "farm" {
  name                       = "farm"
  message_retention_duration = "604800s" # 7 days
}


resource "google_pubsub_subscription" "airflow" {
  name                       = "farm-airflow-bigquery"
  topic                      = google_pubsub_topic.farm.name
  message_retention_duration = "604800s" # 7 days
  expiration_policy {
    ttl = "" # never expires
  }
  bigquery_config {
    table               = "${google_bigquery_table.airflow.project}.${google_bigquery_table.airflow.dataset_id}.${google_bigquery_table.airflow.table_id}"
    use_table_schema    = true
    write_metadata      = true
    drop_unknown_fields = true
  }
  filter = "attributes.type = \"airflow\""
}


resource "google_pubsub_subscription" "argo" {
  name                       = "farm-argo-bigquery"
  topic                      = google_pubsub_topic.farm.name
  message_retention_duration = "604800s" # 7 days
  expiration_policy {
    ttl = "" #never expires
  }
  bigquery_config {
    table               = "${google_bigquery_table.argo.project}.${google_bigquery_table.argo.dataset_id}.${google_bigquery_table.argo.table_id}"
    use_table_schema    = true
    write_metadata      = true
    drop_unknown_fields = true
  }
  filter = "attributes.type = \"argo\""
}

