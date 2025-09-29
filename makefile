include .env
export $(shell sed 's/=.*//' .env)

GINKGO_VERSION=v2.25.1
GINKGO_CMD=go run github.com/onsi/ginkgo/v2/ginkgo@$(GINKGO_VERSION)

test:
	$(GINKGO_CMD) --randomize-all --randomize-suites --fail-on-pending -v ./src/...

run-server:
	go run ./src/cmd/server/main.go

run-entities-edges-consumer:
	go run ./src/cmd/entities-edges-consumer/main.go

run-entity-properties-consumer:
	go run ./src/cmd/entity-properties-consumer/main.go

run-temporal-data-consumer:
	go run ./src/cmd/temporal-data-consumer/main.go

run-cdc-transformer:
	go run ./src/cmd/cdc-transformer/main.go

run-datagen-postgres:
	go run -tags datagen_postgres datagen_postgres.go -clients=10000 -bulk-size=500 -consumers=10 -months=12 -users-per-org=2

run-datagen-kafka-entities-edges:
	go run -tags datagen_kafka_entities_edges datagen_kafka_entities_edges.go --count=-1 --batch-size=5000 --topic=flink.agg.user-profile.entities --brokers=localhost:9092 --group-id=userprofile-entities-edges --delay-ms=1000

run-datagen-entity-properties:
	go run -tags datagen_properties datagen_kafka_entity_properties.go --count=-1 --batch-size=1000 --topic=flink.agg.user-profile.entities.properties --brokers=localhost:9092 --group-id=userprofile-entity-properties --delay-ms=100 --conflict-rate=0.2

run-datagen-temporal-data:
	go run -tags datagen_temporal datagen_kafka_temporal_data.go --count=-1 --batch-size=1000 --topic=flink.agg.user-profile.entities.temporal-properties --brokers=localhost:9092 --group-id=userprofile-temporal-data --delay-ms=100 --entities=100 --days-back=30 --granularities=day,month

setup:
	go get -u github.com/onsi/ginkgo/v2/ginkgo@$(GINKGO_VERSION)
	go get -u github.com/onsi/gomega
	go get -u github.com/brianvoe/gofakeit/v6
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
