include .env
export $(shell sed 's/=.*//' .env)

run-server:
	go run ./src/cmd/server/main.go

run-datagen:
	go run datagen_v2.go -clients=-1 -bulk-size=100 -consumers=8