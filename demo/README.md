# Discovery Demo Workloads

The workspace Docker Compose stack builds `agent-host/Dockerfile` for six
business-shaped hosts:

- WordPress application and MySQL backups
- Mail gateway maintenance
- ERP invoicing and inventory
- Backup retention and restore verification
- Edge cache and certificate operations
- Finance settlement and reconciliation

Each host installs a profile in `/etc/cron.d/demo-workloads`. The agent runs in
system mode, discovers those unmanaged entries, and reports them to the Jobs
review queue. CronCommander-managed entries remain in
`/etc/cron.d/croncommander` and are excluded from discovery.

From the workspace root:

```bash
cd cc-agent
make local-linux-amd64
cd ..
docker compose up -d --build
```
