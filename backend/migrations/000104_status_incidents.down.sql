-- Data-loss note: deletes status pages, incident history, subscriptions and conversation links.
DROP TABLE status_incident_deliveries,status_subscriptions,status_incident_conversations,status_incident_updates,status_incident_components,status_incidents,status_components,status_pages;
