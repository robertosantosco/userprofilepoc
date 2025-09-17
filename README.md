Configure o DNS local para os nos do REDIS

sudo tee -a /etc/hosts << EOF
127.0.0.1 redis-node-1
127.0.0.1 redis-node-2
127.0.0.1 redis-node-3
EOF