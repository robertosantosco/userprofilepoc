package kafka

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/IBM/sarama"
)

type KafkaClient struct {
	consumer  sarama.ConsumerGroup
	producer  sarama.SyncProducer
	brokers   []string
	batchSize int
}

type Message struct {
	Key      string
	Value    []byte
	internal *sarama.ConsumerMessage
}

type Handler func(messages []Message) error

func NewKafkaClient(brokers string, groupID string, batchSize int) (*KafkaClient, error) {
	brokerList := strings.Split(brokers, ",")

	config := sarama.NewConfig()
	config.Version = sarama.V2_8_0_0

	// Consumer config - otimizado para performance com lotes maiores
	config.Consumer.Group.Rebalance.Strategy = sarama.NewBalanceStrategyRoundRobin()
	config.Consumer.Offsets.Initial = sarama.OffsetNewest
	config.Consumer.Group.Session.Timeout = 30 * time.Second // Increased for larger batches
	config.Consumer.Group.Heartbeat.Interval = 10 * time.Second
	config.Consumer.MaxProcessingTime = 60 * time.Second // Increased for larger batches
	config.Consumer.Fetch.Min = 2 * 1024 * 1024          // 2MB - larger minimum
	config.Consumer.Fetch.Default = 20 * 1024 * 1024     // 20MB - much larger default
	config.Consumer.MaxWaitTime = 100 * time.Millisecond // Reduced wait time
	config.ChannelBufferSize = batchSize * 2             // Buffer based on batch size

	// Producer config - otimizado para performance em lote
	config.Producer.RequiredAcks = sarama.WaitForLocal // Mais rápido que WaitForAll
	config.Producer.Retry.Max = 3
	config.Producer.Return.Successes = true
	config.Producer.Compression = sarama.CompressionSnappy
	config.Producer.Flush.Frequency = 50 * time.Millisecond // Flush mais frequente
	config.Producer.Flush.Messages = 50                     // Lotes menores para menor latência
	config.Producer.Flush.Bytes = 512 * 1024                // 512KB
	config.Producer.MaxMessageBytes = 1024 * 1024           // 1MB max por mensagem

	// Consumer Group
	consumer, err := sarama.NewConsumerGroup(brokerList, groupID, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create consumer group: %w", err)
	}

	// Producer
	producer, err := sarama.NewSyncProducer(brokerList, config)
	if err != nil {
		consumer.Close()
		return nil, fmt.Errorf("failed to create producer: %w", err)
	}

	log.Printf("Kafka client initialized with batch size: %d", batchSize)

	return &KafkaClient{
		consumer:  consumer,
		producer:  producer,
		brokers:   brokerList,
		batchSize: batchSize,
	}, nil
}

func (k *KafkaClient) Consumer(ctx context.Context, handler Handler, topic string) error {
	consumerHandler := &consumerGroupHandler{
		handler:   handler,
		batchSize: k.batchSize,
	}

	for {
		select {
		case <-ctx.Done():
			log.Println("Kafka consumer context cancelled")
			return nil
		default:
			if err := k.consumer.Consume(ctx, []string{topic}, consumerHandler); err != nil {
				log.Printf("Error consuming from topic %s: %v", topic, err)
				time.Sleep(5 * time.Second) // Retry delay
				continue
			}
		}
	}
}

func (k *KafkaClient) Producer(messages []Message, topic string) error {
	if len(messages) == 0 {
		return nil
	}

	batchSize := len(messages)
	log.Printf("Sending batch of %d messages to topic %s", batchSize, topic)

	// Prepare all messages
	kafkaMessages := make([]*sarama.ProducerMessage, batchSize)
	for i, msg := range messages {
		kafkaMessages[i] = &sarama.ProducerMessage{
			Topic: topic,
			Key:   sarama.StringEncoder(msg.Key),
			Value: sarama.ByteEncoder(msg.Value),
		}
	}

	// Send all messages asynchronously and collect results
	type result struct {
		partition int32
		offset    int64
		err       error
		index     int
	}

	resultChan := make(chan result, batchSize)

	// Send all messages concurrently
	for i, kafkaMsg := range kafkaMessages {
		go func(idx int, msg *sarama.ProducerMessage) {
			partition, offset, err := k.producer.SendMessage(msg)
			resultChan <- result{
				partition: partition,
				offset:    offset,
				err:       err,
				index:     idx,
			}
		}(i, kafkaMsg)
	}

	// Collect all results
	var errors []error
	successCount := 0

	for i := 0; i < batchSize; i++ {
		res := <-resultChan
		if res.err != nil {
			errors = append(errors, fmt.Errorf("message %d failed: %w", res.index, res.err))
		} else {
			successCount++
		}
	}

	// Log consolidated results
	if len(errors) > 0 {
		log.Printf("Batch completed with errors: %d/%d failed", len(errors), batchSize)
		for _, err := range errors {
			log.Printf("  - %v", err)
		}
		return fmt.Errorf("batch send failed: %d/%d messages failed", len(errors), batchSize)
	}

	log.Printf("✅ Batch sent successfully: %d messages to topic %s", successCount, topic)
	return nil
}

func (k *KafkaClient) Close() error {
	var errs []error

	if err := k.consumer.Close(); err != nil {
		errs = append(errs, fmt.Errorf("failed to close consumer: %w", err))
	}

	if err := k.producer.Close(); err != nil {
		errs = append(errs, fmt.Errorf("failed to close producer: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing kafka client: %v", errs)
	}

	return nil
}

// consumerGroupHandler implementa sarama.ConsumerGroupHandler
type consumerGroupHandler struct {
	handler   Handler
	batchSize int
}

func (h *consumerGroupHandler) Setup(session sarama.ConsumerGroupSession) error {
	log.Printf("Kafka consumer group session setup - batch size: %d", h.batchSize)
	return nil
}

func (h *consumerGroupHandler) Cleanup(sarama.ConsumerGroupSession) error {
	log.Println("Kafka consumer group session cleanup")
	return nil
}

func (h *consumerGroupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	batchSize := h.batchSize
	batchTimeout := 2 * time.Second // Reduced timeout for faster processing

	// Log apenas para primeira partição (evitar spam)
	log.Printf("Starting consumer for partition %d (batch: %d, timeout: %v)",
		claim.Partition(), batchSize, batchTimeout)

	messages := make([]Message, 0, batchSize)
	timer := time.NewTimer(batchTimeout)
	defer timer.Stop()

	for {
		select {
		case message := <-claim.Messages():
			if message == nil {
				// Channel closed, process remaining messages
				if len(messages) > 0 {
					h.processBatch(session, messages)
				}
				return nil
			}

			messages = append(messages, Message{
				Key:      string(message.Key),
				Value:    message.Value,
				internal: message,
			})

			// Process batch when full
			if len(messages) >= batchSize {
				h.processBatch(session, messages)
				messages = messages[:0] // Reset slice
				timer.Reset(batchTimeout)
			}

		case <-timer.C:
			// Process batch on timeout
			if len(messages) > 0 {
				h.processBatch(session, messages)
				messages = messages[:0] // Reset slice
			}
			timer.Reset(batchTimeout)

		case <-session.Context().Done():
			// Session cancelled, process remaining messages
			if len(messages) > 0 {
				h.processBatch(session, messages)
			}
			return nil
		}
	}
}

func (h *consumerGroupHandler) processBatch(session sarama.ConsumerGroupSession, messages []Message) {
	if len(messages) == 0 {
		return
	}

	log.Printf("Processing batch of %d messages", len(messages))

	err := h.handler(messages)
	if err != nil {
		log.Printf("Handler error for batch: %v", err)
		// Don't mark messages - they will be retried
		return
	}

	// Mark all messages in batch as processed
	for _, msg := range messages {
		if msg.internal != nil {
			session.MarkMessage(msg.internal, "")
		}
	}

	log.Printf("Successfully processed batch of %d messages", len(messages))
}
