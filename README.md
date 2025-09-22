Configure o DNS local para os nos do REDIS

sudo tee -a /etc/hosts << EOF
127.0.0.1 redis-node-1
127.0.0.1 redis-node-2
127.0.0.1 redis-node-3
EOF



- mudar key para type no temporal
- testar mandar uma parte só do json no update para ver se atualzia e n perde


----------------------------------------------------------------------------
# 1. Parar completamente as réplicas
docker-compose stop postgres-replica-1 postgres-replica-2
docker rm -f user-profile-replica-1-db user-profile-replica-2-db

# 2. Verificar e remover volumes
docker volume ls | grep replica
docker volume rm userprofilepoc_postgres_replica_1_data userprofilepoc_postgres_replica_2_data

# 3. Inicie as novas replicas
docker-compose up -d postgres-replica-1 postgres-replica-2

# 4. Verifique nos logs se esta sincronizando os WAL
docker logs -f user-profile-replica-1-db


------------------------------------------------------------------------------
limpa todos os dados do Redis:
docker exec -it userprofilepoc-redis-node-1-1 redis-cli -c -p 7001
FLUSHALL


-------------------------------------------------------------------------------
Remover o connector debezium:
curl -X DELETE http://localhost:8083/connectors/user-profile-postgres-connector

Consultar status do connector debezium:
curl http://localhost:8083/connectors/user-profile-postgres-connector/status