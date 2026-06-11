FROM debian:12-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        cron \
        curl \
        default-mysql-client \
        gzip \
        procps \
        util-linux \
    && rm -rf /var/lib/apt/lists/*

COPY profiles/ /opt/demo/profiles/
COPY bin/demo-task /opt/demo/bin/demo-task
COPY entrypoint.sh /usr/local/bin/demo-agent-entrypoint

RUN chmod 0755 /opt/demo/bin/demo-task /usr/local/bin/demo-agent-entrypoint \
    && mkdir -p /var/backups/croncommander-demo /var/log/croncommander-demo

ENTRYPOINT ["/usr/local/bin/demo-agent-entrypoint"]
