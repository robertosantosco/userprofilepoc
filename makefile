include .env
export $(shell sed 's/=.*//' .env)

run-server:
	go run ./src/cmd/server/main.go

run-datagen:
	go run datagen_v2.go -clients=7000000 -bulk-size=500 -consumers=10 -months=12 -users-per-org=2
