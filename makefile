include .env
export $(shell sed 's/=.*//' .env)

run-server:
	go run ./src/cmd/server/main.go

run-entities-edges-consumer:
	go run ./src/cmd/entities-edges-consumer/main.go

run-entity-properties-consumer:
	go run ./src/cmd/entity-properties-consumer/main.go

run-datagen-postgres:
	go run -tags datagen_postgres datagen_postgres.go -clients=7000000 -bulk-size=500 -consumers=10 -months=12 -users-per-org=2

run-datagen-kafka-entities-edges:
	go run -tags datagen_kafka_entities_edges datagen_kafka_entities_edges.go --count=-1 --batch-size=5000 --topic=flink.agg.user-profile.entities --brokers=localhost:9092 --group-id=userprofile-entities-edges --delay-ms=1000

run-datagen-entity-properties:
	go run -tags datagen_properties datagen_kafka_entity_properties.go --count=-1 --batch-size=1000 --topic=flink.agg.user-profile.entities.properties --brokers=localhost:9092 --group-id=userprofile-entity-properties --delay-ms=100 --conflict-rate=0.2
