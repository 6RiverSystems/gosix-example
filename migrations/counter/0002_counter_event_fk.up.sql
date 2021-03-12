ALTER TABLE counter
	ADD COLUMN last_update uuid NOT NULL REFERENCES counter_events;
