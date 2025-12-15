![GitHub stars](https://img.shields.io/github/stars/croncommander/cc-agent?style=social)
![License](https://img.shields.io/github/license/croncommander/cc-agent)
[![cc-spec](https://img.shields.io/badge/spec-cc--spec-blue)](https://github.com/croncommander/cc-spec)

# cc-agent

> Part of the **CronCommander** project — a centralized control plane for cron jobs.

CronCommander Agent is a lightweight daemon that connects cron-based systems
to the CronCommander control plane.

It provides the foundation for centralized visibility and management of cron jobs
across servers, containers, and environments.

## What it does

- Runs as a long-lived agent on a host or container
- Observes cron job executions
- Reports execution metadata to the CronCommander server

The agent does **not** replace cron.
It enables centralization and control on top of existing cron setups.

## Design goals

- Lightweight and low overhead
- Explicit behavior, no hidden automation
- Safe to deploy in production environments
- Designed to support centralized inspection and control

## What it does not do

- It does not schedule jobs
- It does not modify cron configuration
- It does not make control decisions on its own

## Status

Early development.
Interfaces and behavior may evolve as centralized management features land.

## Project

Part of the **CronCommander** project — a centralized control plane for cron jobs.

Website: https://croncommander.com
