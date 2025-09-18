# 1. Use a imagem oficial do PostgreSQL 16 como base.
# A versão 16 é a mais atual no RDS da AWS.
FROM postgres:16

# 2. Instale as dependências necessárias.
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        postgresql-server-dev-16 \
        build-essential \
        git \
        make && \
    rm -rf /var/lib/apt/lists/*

# 3. Configure o Git para ignorar a verificação SSL.
RUN git config --global http.sslVerify false

# 4. Clone, compile e instale o pg_partman.
RUN git clone https://github.com/pgpartman/pg_partman.git /tmp/pg_partman && \
    cd /tmp/pg_partman && \
    make install && \
    rm -rf /tmp/pg_partman

    # 5. Configurar PostgreSQL para replicação
# Criar arquivo pg_hba.conf customizado
RUN echo "# PostgreSQL Client Authentication Configuration File" > /tmp/pg_hba.conf && \
    echo "# TYPE  DATABASE        USER            ADDRESS                 METHOD" >> /tmp/pg_hba.conf && \
    echo "" >> /tmp/pg_hba.conf && \
    echo "# Local connections" >> /tmp/pg_hba.conf && \
    echo "local   all             all                                     trust" >> /tmp/pg_hba.conf && \
    echo "host    all             all             127.0.0.1/32            md5" >> /tmp/pg_hba.conf && \
    echo "host    all             all             ::1/128                 md5" >> /tmp/pg_hba.conf && \
    echo "" >> /tmp/pg_hba.conf && \
    echo "# Docker network connections" >> /tmp/pg_hba.conf && \
    echo "host    all             all             172.0.0.0/8             md5" >> /tmp/pg_hba.conf && \
    echo "host    all             all             0.0.0.0/0               md5" >> /tmp/pg_hba.conf && \
    echo "" >> /tmp/pg_hba.conf && \
    echo "# Replication connections" >> /tmp/pg_hba.conf && \
    echo "host    replication     replicator      172.0.0.0/8             md5" >> /tmp/pg_hba.conf && \
    echo "host    replication     replicator      0.0.0.0/0               md5" >> /tmp/pg_hba.conf

# 6. Script de inicialização para replicação
RUN echo "#!/bin/bash" > /docker-entrypoint-initdb.d/01-setup-replication.sh && \
    echo "set -e" >> /docker-entrypoint-initdb.d/01-setup-replication.sh && \
    echo "# Criar usuário de replicação" >> /docker-entrypoint-initdb.d/01-setup-replication.sh && \
    echo "psql -v ON_ERROR_STOP=1 --username \"\$POSTGRES_USER\" --dbname \"\$POSTGRES_DB\" <<-EOSQL" >> /docker-entrypoint-initdb.d/01-setup-replication.sh && \
    echo "    CREATE USER replicator WITH REPLICATION ENCRYPTED PASSWORD 'replicator_password';" >> /docker-entrypoint-initdb.d/01-setup-replication.sh && \
    echo "    GRANT CONNECT ON DATABASE \$POSTGRES_DB TO replicator;" >> /docker-entrypoint-initdb.d/01-setup-replication.sh && \
    echo "EOSQL" >> /docker-entrypoint-initdb.d/01-setup-replication.sh && \
    echo "" >> /docker-entrypoint-initdb.d/01-setup-replication.sh && \
    echo "# Copiar pg_hba.conf customizado" >> /docker-entrypoint-initdb.d/01-setup-replication.sh && \
    echo "cp /tmp/pg_hba.conf /var/lib/postgresql/data/pg_hba.conf" >> /docker-entrypoint-initdb.d/01-setup-replication.sh && \
    echo "chown postgres:postgres /var/lib/postgresql/data/pg_hba.conf" >> /docker-entrypoint-initdb.d/01-setup-replication.sh && \
    chmod +x /docker-entrypoint-initdb.d/01-setup-replication.sh

# 7. Configurações padrão do PostgreSQL para replicação
ENV POSTGRES_INITDB_ARGS="--data-checksums"

RUN mkdir -p /var/lib/postgresql/archive && chown -R postgres:postgres /var/lib/postgresql/archive
