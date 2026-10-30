-- Data-loss note: removes on-call configuration, alerts, monitors and check history.
DROP TABLE uptime_results,uptime_monitors,oncall_alerts,escalation_policies,oncall_rotas;
