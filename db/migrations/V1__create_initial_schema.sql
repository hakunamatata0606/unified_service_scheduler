CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE dealerships (
    id uuid PRIMARY KEY,
    name text NOT NULL,
    timezone text NOT NULL,
    created_at_utc timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE customers (
    id uuid PRIMARY KEY,
    name text NOT NULL,
    email text NOT NULL,
    created_at_utc timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE vehicles (
    id uuid PRIMARY KEY,
    customer_id uuid NOT NULL REFERENCES customers (id),
    vin varchar(17) NOT NULL UNIQUE,
    make text NOT NULL,
    model text NOT NULL,
    created_at_utc timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE service_types (
    id uuid PRIMARY KEY,
    name text NOT NULL,
    duration_minutes integer NOT NULL CHECK (duration_minutes > 0),
    required_skill text NOT NULL,
    created_at_utc timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE technicians (
    id uuid PRIMARY KEY,
    dealership_id uuid NOT NULL REFERENCES dealerships (id),
    name text NOT NULL,
    created_at_utc timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE technician_skills (
    technician_id uuid NOT NULL REFERENCES technicians (id) ON DELETE CASCADE,
    skill text NOT NULL,
    PRIMARY KEY (technician_id, skill)
);

CREATE TABLE service_bays (
    id uuid PRIMARY KEY,
    dealership_id uuid NOT NULL REFERENCES dealerships (id),
    name text NOT NULL,
    created_at_utc timestamptz NOT NULL DEFAULT now(),
    UNIQUE (dealership_id, name)
);

CREATE TABLE service_bay_capabilities (
    service_bay_id uuid NOT NULL REFERENCES service_bays (id) ON DELETE CASCADE,
    service_type_id uuid NOT NULL REFERENCES service_types (id) ON DELETE CASCADE,
    PRIMARY KEY (service_bay_id, service_type_id)
);

CREATE TABLE appointments (
    id uuid PRIMARY KEY,
    customer_id uuid NOT NULL REFERENCES customers (id),
    vehicle_id uuid NOT NULL REFERENCES vehicles (id),
    dealership_id uuid NOT NULL REFERENCES dealerships (id),
    technician_id uuid NOT NULL REFERENCES technicians (id),
    service_bay_id uuid NOT NULL REFERENCES service_bays (id),
    service_type_id uuid NOT NULL REFERENCES service_types (id),
    start_time_utc timestamptz NOT NULL,
    end_time_utc timestamptz NOT NULL,
    status text NOT NULL CHECK (status IN ('confirmed', 'cancelled', 'completed')),
    created_at_utc timestamptz NOT NULL DEFAULT now(),
    CHECK (end_time_utc > start_time_utc)
);

ALTER TABLE appointments
    ADD CONSTRAINT appointments_no_technician_overlap
    EXCLUDE USING gist (
        technician_id WITH =,
        tstzrange(start_time_utc, end_time_utc, '[)') WITH &&
    ) WHERE (status = 'confirmed');

ALTER TABLE appointments
    ADD CONSTRAINT appointments_no_service_bay_overlap
    EXCLUDE USING gist (
        service_bay_id WITH =,
        tstzrange(start_time_utc, end_time_utc, '[)') WITH &&
    ) WHERE (status = 'confirmed');

CREATE TABLE idempotency_requests (
    key text PRIMARY KEY,
    request_hash text NOT NULL,
    status text NOT NULL CHECK (status IN ('in_progress', 'completed')),
    appointment_id uuid REFERENCES appointments (id),
    response_status integer,
    response_body jsonb,
    created_at_utc timestamptz NOT NULL DEFAULT now(),
    expires_at_utc timestamptz NOT NULL,
    CHECK (
        status <> 'completed'
        OR (appointment_id IS NOT NULL AND response_status IS NOT NULL AND response_body IS NOT NULL)
    )
);

CREATE INDEX appointments_dealership_time_idx
    ON appointments (dealership_id, start_time_utc, end_time_utc);

CREATE INDEX appointments_technician_time_idx
    ON appointments (technician_id, start_time_utc, end_time_utc);

CREATE INDEX appointments_service_bay_time_idx
    ON appointments (service_bay_id, start_time_utc, end_time_utc);

CREATE INDEX vehicles_customer_idx ON vehicles (customer_id);

CREATE INDEX idempotency_requests_expiry_idx
    ON idempotency_requests (expires_at_utc);

