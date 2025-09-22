package consumers

import (
	"context"
	"fmt"
	"log/slog"
	"userprofilepoc/src/infra/debezium"
	"userprofilepoc/src/services/events"
)

type CDCConsumer struct {
	logger         *slog.Logger
	cdcClient      *debezium.CDCClient
	transformer    *events.CDCTransformer
	eventPublisher *events.DomainEventPublisher
}

func NewCDCConsumer(
	logger *slog.Logger,
	cdcClient *debezium.CDCClient,
	transformer *events.CDCTransformer,
	eventPublisher *events.DomainEventPublisher,
) *CDCConsumer {
	return &CDCConsumer{
		logger:         logger,
		cdcClient:      cdcClient,
		transformer:    transformer,
		eventPublisher: eventPublisher,
	}
}

func (c *CDCConsumer) Start(ctx context.Context) error {
	c.logger.Info("Starting CDC consumer")

	// Use batch handler to process multiple CDC events at once
	batchHandler := func(ctx context.Context, cdcEvents []*debezium.CDCEvent) error {
		return c.handleCDCEventsBatch(ctx, cdcEvents)
	}

	return c.cdcClient.ConsumeCDCEventsBatch(ctx, batchHandler)
}

// handleCDCEventsBatch processes multiple CDC events and publishes domain events in a single batch
func (c *CDCConsumer) handleCDCEventsBatch(ctx context.Context, cdcEvents []*debezium.CDCEvent) error {
	if len(cdcEvents) == 0 {
		return nil
	}

	c.logger.Debug("Processing CDC events batch", "count", len(cdcEvents))

	var allDomainEvents []events.DomainEventWithMetadata
	processedCount := 0
	errorCount := 0

	// Transform all CDC events to domain events
	for _, cdcEvent := range cdcEvents {
		domainEventsWithMetadata, err := c.transformer.TransformCDCEvent(ctx, cdcEvent)
		if err != nil {
			c.logger.Error("Failed to transform CDC event",
				"error", err,
				"table", cdcEvent.Source.Table,
				"operation", cdcEvent.Operation)
			errorCount++
			continue
		}

		// Accumulate domain events
		allDomainEvents = append(allDomainEvents, domainEventsWithMetadata...)
		processedCount++
	}

	// Publish all domain events in a single batch
	if len(allDomainEvents) > 0 {
		if err := c.eventPublisher.PublishDomainEvents(ctx, allDomainEvents); err != nil {
			c.logger.Error("Failed to publish domain events batch",
				"error", err,
				"events_count", len(allDomainEvents))
			return fmt.Errorf("failed to publish domain events batch: %w", err)
		}

		c.logger.Info("Successfully published domain events batch",
			"cdc_events_processed", processedCount,
			"domain_events_published", len(allDomainEvents))
	}

	if errorCount > 0 {
		c.logger.Warn("Some CDC events failed to transform",
			"failed", errorCount,
			"successful", processedCount)
	}

	return nil
}

// Close gracefully shuts down the CDC consumer
func (c *CDCConsumer) Close() error {
	c.logger.Info("Closing CDC consumer")
	return c.cdcClient.Close()
}
