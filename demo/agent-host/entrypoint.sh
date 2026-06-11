#!/bin/sh
set -eu

profile="${DEMO_PROFILE:-}"
case "$profile" in
    wordpress|mail|erp|backup|edge|finance) ;;
    *)
        echo "[demo-agent] Unsupported DEMO_PROFILE: $profile" >&2
        exit 1
        ;;
esac

install -m 0644 "/opt/demo/profiles/${profile}.cron" /etc/cron.d/demo-workloads
touch /var/log/croncommander-demo/tasks.log
cron

echo "[demo-agent] Loaded ${profile} workload cron entries"
exec bash /opt/build/entrypoint.sh
