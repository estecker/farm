package pubsub

import (
	"cloud.google.com/go/pubsub"
	"context"
	"log/slog"
)

// Generic function to publish a message to a topic
func Publish(msg []byte, topicProjectID string, topicID string, attributes map[string]string) (string, error) {
	ctx := context.Background()
	client, err := pubsub.NewClient(ctx, topicProjectID)
	if err != nil {
		slog.Error("Failed to create client", "error", err)
	}
	defer func(client *pubsub.Client) {
		err := client.Close()
		if err != nil {
			slog.Error("Failed to close client", "error", err)
		}
	}(client)

	t := client.Topic(topicID)

	res := t.Publish(ctx, &pubsub.Message{
		Data:       msg,
		Attributes: attributes,
	})
	// Block until the result is returned and a server-generated
	// ID is returned for the published message.
	id, err := res.Get(ctx)
	return id, err
}
