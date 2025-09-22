package debezium

import (
	"context"
	"fmt"
	"log/slog"
	"userprofilepoc/src/infra/kafka"
)

// CDCEventHandler is the function signature for handling CDC events
type CDCEventHandler func(ctx context.Context, event *CDCEvent) error

// CDCBatchEventHandler is the function signature for handling batches of CDC events
type CDCBatchEventHandler func(ctx context.Context, events []*CDCEvent) error

// CDCClient implements CDC event consumption using Kafka
type CDCClient struct {
	logger      *slog.Logger
	kafkaClient *kafka.KafkaClient
	serializer  *CDCSerializer
	topic       string
}

// NewCDCClient creates a new CDC client
func NewCDCClient(logger *slog.Logger, topic string, kafkaClient *kafka.KafkaClient, tables []string) *CDCClient {
	serializer := &CDCSerializer{
		IncludeTables: tables,
	}

	return &CDCClient{
		logger:      logger,
		kafkaClient: kafkaClient,
		serializer:  serializer,
		topic:       topic,
	}
}

// ConsumeCDCEvents starts consuming CDC events and calls handler for each valid event
func (c *CDCClient) ConsumeCDCEvents(ctx context.Context, handler CDCEventHandler) error {
	c.logger.Info("Starting CDC event consumption", "topic", c.topic)

	kafkaHandler := func(messages []kafka.Message) error {
		return c.processCDCMessages(ctx, messages, handler)
	}

	return c.kafkaClient.Consumer(ctx, kafkaHandler, c.topic)
}

// ConsumeCDCEventsBatch starts consuming CDC events and calls handler for batches of valid events
func (c *CDCClient) ConsumeCDCEventsBatch(ctx context.Context, handler CDCBatchEventHandler) error {
	c.logger.Info("Starting CDC batch event consumption", "topic", c.topic)

	kafkaHandler := func(messages []kafka.Message) error {
		return c.processCDCMessagesBatch(ctx, messages, handler)
	}

	return c.kafkaClient.Consumer(ctx, kafkaHandler, c.topic)
}

// processCDCMessages processes a batch of Kafka messages as CDC events
func (c *CDCClient) processCDCMessages(ctx context.Context, messages []kafka.Message, handler CDCEventHandler) error {
	if len(messages) == 0 {
		return nil
	}

	c.logger.Debug("Processing CDC messages batch", "count", len(messages))

	processedCount := 0
	skippedCount := 0
	errorCount := 0

	for _, msg := range messages {
		// Parse CDC event
		cdcEvent, err := c.serializer.ParseCDCEvent(msg.Value)
		if err != nil {
			c.logger.Error("Failed to parse CDC message",
				"error", err,
				"key", msg.Key,
				"value_length", len(msg.Value))
			errorCount++
			continue
		}

		// Check if event should be processed
		if !c.serializer.ShouldProcessEvent(cdcEvent) {
			c.logger.Debug("Skipping CDC event",
				"table", cdcEvent.Source.Table,
				"operation", cdcEvent.Operation,
				"snapshot", cdcEvent.Source.Snapshot)
			skippedCount++
			continue
		}

		// Handle CDC event
		if err := handler(ctx, cdcEvent); err != nil {
			c.logger.Error("CDC event handler failed",
				"error", err,
				"table", cdcEvent.Source.Table,
				"operation", cdcEvent.Operation,
				"ts_ms", cdcEvent.TsMs)
			errorCount++
			continue
		}

		processedCount++
		c.logger.Debug("Successfully processed CDC event",
			"table", cdcEvent.Source.Table,
			"operation", cdcEvent.Operation,
			"ts_ms", cdcEvent.TsMs)
	}

	c.logger.Info("Completed CDC messages batch processing",
		"total", len(messages),
		"processed", processedCount,
		"skipped", skippedCount,
		"errors", errorCount)

	// Return error if all messages failed
	if errorCount > 0 && processedCount == 0 {
		return fmt.Errorf("failed to process any CDC messages in batch")
	}

	return nil
}

// processCDCMessagesBatch processes a batch of Kafka messages and calls handler with all valid CDC events at once
func (c *CDCClient) processCDCMessagesBatch(ctx context.Context, messages []kafka.Message, handler CDCBatchEventHandler) error {
	if len(messages) == 0 {
		return nil
	}

	c.logger.Debug("Processing CDC messages batch", "count", len(messages))

	var validEvents []*CDCEvent
	skippedCount := 0
	errorCount := 0

	for _, msg := range messages {
		// Parse CDC event
		cdcEvent, err := c.serializer.ParseCDCEvent(msg.Value)
		if err != nil {
			c.logger.Error("Failed to parse CDC message",
				"error", err,
				"key", msg.Key,
				"value_length", len(msg.Value))
			errorCount++
			continue
		}

		// Check if event should be processed
		if !c.serializer.ShouldProcessEvent(cdcEvent) {
			c.logger.Debug("Skipping CDC event",
				"table", cdcEvent.Source.Table,
				"operation", cdcEvent.Operation,
				"snapshot", cdcEvent.Source.Snapshot)
			skippedCount++
			continue
		}

		validEvents = append(validEvents, cdcEvent)
	}

	// Handle all valid events as a batch
	if len(validEvents) > 0 {
		if err := handler(ctx, validEvents); err != nil {
			c.logger.Error("CDC batch event handler failed",
				"error", err,
				"valid_events", len(validEvents))
			return fmt.Errorf("failed to handle CDC events batch: %w", err)
		}

		c.logger.Debug("Successfully processed CDC events batch",
			"processed", len(validEvents))
	}

	c.logger.Info("Completed CDC messages batch processing",
		"total", len(messages),
		"processed", len(validEvents),
		"skipped", skippedCount,
		"errors", errorCount)

	// Return error if all messages failed
	if errorCount > 0 && len(validEvents) == 0 {
		return fmt.Errorf("failed to process any CDC messages in batch")
	}

	return nil
}

// Close closes the CDC client
func (c *CDCClient) Close() error {
	c.logger.Info("Closing CDC client")
	return c.kafkaClient.Close()
}
