# cc-agent

CronCommander Agent is a lightweight daemon that observes cron job execution and reports metadata to CronCommander.

It is designed to run alongside existing cron setups with minimal overhead and minimal configuration.

## What it does

- Runs as a long-lived agent on a host or container
- Collects execution metadata from cron jobs
- Reports status, timing, and failures to the CronCommander server

The agent does **not** replace cron.
It makes cron observable.

## Design goals

- Lightweight and low overhead
- Explicit behavior, no hidden magic
- Easy to deploy on servers and containers
- Safe to run in production environments

## What it does not do

- It does not schedule jobs
- It does not modify existing cron configuration
- It does not execute jobs itself

## Status

Early development.
Interfaces and behavior may change.

## Project

Part of the **CronCommander** project.

Website: https://croncommander.com
