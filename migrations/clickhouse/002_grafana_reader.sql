-- Grafana read-only ClickHouse user.
--
-- Grafana's ClickHouse datasource executes SQL exactly as written and does not
-- restrict it, so the datasource should never use the admin (`default`) user.
-- `grafana_reader` is granted SELECT only, which makes it read-only by grants
-- (it cannot INSERT/ALTER/DROP) while still allowing the plugin to set its own
-- per-query SETTINGS such as max_execution_time.
--
-- DEV DEFAULT PASSWORD: "agentplane". It matches docker-compose.yml and the
-- provisioned datasource. Change it for any non-development deployment and keep
-- the datasource's CLICKHOUSE_GRAFANA_PASSWORD in sync.

CREATE DATABASE IF NOT EXISTS agentplane;

CREATE USER IF NOT EXISTS grafana_reader
    IDENTIFIED WITH sha256_password BY 'agentplane';

GRANT SELECT ON agentplane.* TO grafana_reader;
