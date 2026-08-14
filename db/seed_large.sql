-- Large deterministic dataset for local development.
-- Apply after V1__create_initial_schema.sql. Safe to run repeatedly.
BEGIN;

-- 25 dealerships
INSERT INTO dealerships (id, name, timezone)
SELECT md5('large-dealership-' || n)::uuid,
       'Demo Dealership ' || lpad(n::text, 2, '0'),
       CASE WHEN n % 2 = 0 THEN 'Asia/Ho_Chi_Minh' ELSE 'America/New_York' END
FROM generate_series(1, 25) AS n
ON CONFLICT (id) DO NOTHING;

-- 1,000 customers
INSERT INTO customers (id, name, email)
SELECT md5('large-customer-' || n)::uuid,
       'Customer ' || lpad(n::text, 4, '0'),
       'customer' || n || '@example.com'
FROM generate_series(1, 1000) AS n
ON CONFLICT (id) DO NOTHING;

-- 20 service types
INSERT INTO service_types (id, name, duration_minutes, required_skill)
SELECT md5('large-service-' || n)::uuid,
       CASE n % 5
         WHEN 0 THEN 'Full service ' || n
         WHEN 1 THEN 'Oil service ' || n
         WHEN 2 THEN 'Brake service ' || n
         WHEN 3 THEN 'Tire service ' || n
         ELSE 'Inspection ' || n
       END,
       30 + ((n % 5) * 30),
       CASE n % 5
         WHEN 2 THEN 'brakes'
         WHEN 3 THEN 'tires'
         ELSE 'general'
       END
FROM generate_series(1, 20) AS n
ON CONFLICT (id) DO NOTHING;

-- 2,000 vehicles, two per customer
INSERT INTO vehicles (id, customer_id, vin, make, model)
SELECT md5('large-vehicle-' || n)::uuid,
       md5('large-customer-' || (((n - 1) % 1000) + 1))::uuid,
       'LARGE' || lpad(n::text, 12, '0'),
       CASE n % 5 WHEN 0 THEN 'Toyota' WHEN 1 THEN 'Honda' WHEN 2 THEN 'Ford' WHEN 3 THEN 'Mazda' ELSE 'Hyundai' END,
       CASE n % 4 WHEN 0 THEN 'Sedan' WHEN 1 THEN 'SUV' WHEN 2 THEN 'Truck' ELSE 'Hatchback' END
FROM generate_series(1, 2000) AS n
ON CONFLICT (id) DO NOTHING;

-- 500 technicians, 20 per dealership
INSERT INTO technicians (id, dealership_id, name)
SELECT md5('large-technician-' || n)::uuid,
       md5('large-dealership-' || (((n - 1) % 25) + 1))::uuid,
       'Technician ' || lpad(n::text, 4, '0')
FROM generate_series(1, 500) AS n
ON CONFLICT (id) DO NOTHING;

INSERT INTO technician_skills (technician_id, skill)
SELECT md5('large-technician-' || n)::uuid, skill
FROM generate_series(1, 500) AS n
CROSS JOIN LATERAL unnest(
  CASE n % 3
    WHEN 0 THEN ARRAY['general', 'brakes', 'tires']::text[]
    WHEN 1 THEN ARRAY['general', 'brakes']::text[]
    ELSE ARRAY['general', 'tires']::text[]
  END
) AS skill
ON CONFLICT DO NOTHING;

-- 250 service bays, ten per dealership
INSERT INTO service_bays (id, dealership_id, name)
SELECT md5('large-bay-' || n)::uuid,
       md5('large-dealership-' || (((n - 1) % 25) + 1))::uuid,
       'Bay ' || (((n - 1) / 25) + 1)
FROM generate_series(1, 250) AS n
ON CONFLICT (id) DO NOTHING;

-- Give every bay several capabilities.
INSERT INTO service_bay_capabilities (service_bay_id, service_type_id)
SELECT md5('large-bay-' || bay)::uuid,
       md5('large-service-' || service_type)::uuid
FROM generate_series(1, 250) AS bay
CROSS JOIN generate_series(1, 20) AS service_type
WHERE service_type % 2 = bay % 2
ON CONFLICT DO NOTHING;

-- 5,000 completed appointments. Completed rows do not occupy current slots,
-- so this remains safe with the overlap exclusion constraints.
INSERT INTO appointments (
  id, customer_id, vehicle_id, dealership_id, technician_id, service_bay_id,
  service_type_id, start_time_utc, end_time_utc, status
)
SELECT md5('large-appointment-' || n)::uuid,
       md5('large-customer-' || (((n - 1) % 1000) + 1))::uuid,
       md5('large-vehicle-' || (((n - 1) % 2000) + 1))::uuid,
       md5('large-dealership-' || (((n - 1) % 25) + 1))::uuid,
       md5('large-technician-' || (((n - 1) % 500) + 1))::uuid,
       md5('large-bay-' || (((n - 1) % 250) + 1))::uuid,
       md5('large-service-' || (((n - 1) % 20) + 1))::uuid,
       (timestamp with time zone '2025-01-01 08:00:00+00' + ((n - 1) / 500) * interval '1 day' + ((n - 1) % 10) * interval '1 hour'),
       (timestamp with time zone '2025-01-01 08:00:00+00' + ((n - 1) / 500) * interval '1 day' + ((n - 1) % 10) * interval '1 hour' + interval '1 hour'),
       'completed'
FROM generate_series(1, 5000) AS n
ON CONFLICT (id) DO NOTHING;

INSERT INTO idempotency_requests (
  key, request_hash, status, appointment_id, response_status, response_body, expires_at_utc
)
SELECT 'large-idempotency-' || n,
       md5('large-appointment-' || n),
       'completed',
       md5('large-appointment-' || n)::uuid,
       201,
       jsonb_build_object('appointmentId', md5('large-appointment-' || n)::uuid, 'status', 'completed'),
       now() + interval '24 hours'
FROM generate_series(1, 5000) AS n
ON CONFLICT (key) DO NOTHING;

COMMIT;
