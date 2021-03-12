CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE counter (
	id uuid PRIMARY KEY DEFAULT uuid_generate_v4 (),
	name text NOT NULL UNIQUE,
	value bigint NOT NULL
);
