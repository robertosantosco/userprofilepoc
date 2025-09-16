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
