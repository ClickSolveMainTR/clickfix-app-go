-- Data-loss note: removes integration polling cursor history.
DROP TABLE integration_trigger_events,integration_event_cursors;
