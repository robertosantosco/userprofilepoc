package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"userprofilepoc/src/adapters/kafka/consumers"
	"userprofilepoc/src/helper/env"
	"userprofilepoc/src/infra/debezium"
	"userprofilepoc/src/infra/kafka"
	"userprofilepoc/src/services/events"

	"go.uber.org/fx"
)

func main() {
	log.SetOutput(os.Stdout)
	log.Println("Starting CDC Transformer with Uber Fx...")

	app := fx.New(
		// Providers
		fx.Provide(
			newLogger,
			newKafkaClient,
			newCDCClient,
			newCDCTransformer,
			newDomainEventPublisher,
			newCDCConsumer,
		),

		// Invocations
		fx.Invoke(startConsumer),
	)

	// Start the application
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := app.Start(ctx); err != nil {
		log.Fatalf("Failed to start CDC transformer application: %v", err)
	}

	// Wait for interrupt signal to gracefully shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	log.Println("Shutting down CDC transformer...")

	// Stop the application
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer stopCancel()

	if err := app.Stop(stopCtx); err != nil {
		log.Printf("Failed to stop application gracefully: %v", err)
	}

	log.Println("CDC transformer shutdown complete")
}

func newLogger() *slog.Logger {
	logLevel := env.GetString("LOG_LEVEL", "info")
	var level slog.Level

	switch logLevel {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	logger := slog.New(h)
	slog.SetDefault(logger)
	return logger
}

func newKafkaClient() (*kafka.KafkaClient, error) {
	brokers := env.MustGetString("KAFKA_BROKERS")
	groupID := env.MustGetString("KAFKA_CDC_CONSUMER_GROUP_ID")
	batchSize := env.MustGetInt("KAFKA_BATCH_SIZE")

	return kafka.NewKafkaClient(brokers, groupID, batchSize)
}

func newCDCClient(logger *slog.Logger, kafkaClient *kafka.KafkaClient) *debezium.CDCClient {
	topic := env.MustGetString("KAFKA_CDC_TOPIC")
	tables := env.MustGetString("KAFKA_CDC_TABLES")

	return debezium.NewCDCClient(logger, topic, kafkaClient, strings.Split(tables, ","))
}

func newCDCTransformer(logger *slog.Logger) *events.CDCTransformer {
	return events.NewCDCTransformer(logger)
}

func newDomainEventPublisher(
	logger *slog.Logger,
	kafkaClient *kafka.KafkaClient,
) *events.DomainEventPublisher {
	topic := env.MustGetString("KAFKA_DOMAIN_EVENTS_TOPIC")
	return events.NewDomainEventPublisher(logger, kafkaClient, topic)
}

func newCDCConsumer(
	logger *slog.Logger,
	cdcClient *debezium.CDCClient,
	transformer *events.CDCTransformer,
	eventPublisher *events.DomainEventPublisher,
) *consumers.CDCConsumer {
	return consumers.NewCDCConsumer(logger, cdcClient, transformer, eventPublisher)
}

func startConsumer(
	lc fx.Lifecycle,
	logger *slog.Logger,
	cdcConsumer *consumers.CDCConsumer,
) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			logger.Info("Starting CDC transformer")

			// Start consumer in background
			go func() {
				if err := cdcConsumer.Start(ctx); err != nil {
					logger.Error("CDC consumer failed", "error", err)
				}
			}()

			return nil
		},
		OnStop: func(ctx context.Context) error {
			logger.Info("Shutting down CDC consumer...")
			if err := cdcConsumer.Close(); err != nil {
				logger.Error("Failed to close CDC consumer", "error", err)
				return err
			}
			logger.Info("CDC consumer shut down gracefully")
			return nil
		},
	})
}
