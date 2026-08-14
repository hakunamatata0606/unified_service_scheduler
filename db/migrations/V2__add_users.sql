CREATE TABLE users (
    id uuid PRIMARY KEY,
    customer_id uuid NOT NULL UNIQUE REFERENCES customers (id),
    email text NOT NULL UNIQUE,
    password_salt text NOT NULL,
    password_hash text NOT NULL,
    created_at_utc timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX users_customer_idx ON users (customer_id);
