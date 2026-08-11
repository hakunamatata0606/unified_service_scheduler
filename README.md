# Unified Service Scheduler

## System Design Document

Scenario A - Keyloop Technical Assessment

This design follows the HelloInterview Delivery Framework: requirements, core entities, API, data flow, high-level design, and deep dives.

## 1. Requirements

### Functional requirements

The system should allow a customer or dealership employee to:

1. Request a service appointment for a specific customer, vehicle, dealership, service type, and desired start time.
2. Check that a qualified technician is available for the entire service duration.
3. Check that a compatible service bay is available for the entire service duration.
4. Confirm and persist an appointment associating the customer, vehicle, technician, and service bay.
5. Retrieve a confirmed appointment.

### Out of scope for the MVP

- Authentication and authorization
- Payments
- Customer notifications
- Cancellation and rescheduling
- Holiday and complex working-hour rules
- Calendar integrations
- Customer-facing frontend

The backend will be implemented fully. Swagger UI and cURL examples will act as the client layer.

### Non-functional requirements

- **Consistency:** The system must not confirm overlapping appointments for the same technician or service bay.
- **Durability:** Confirmed appointments must survive application restarts.
- **Performance:** Appointment creation should normally complete within 500 ms for a single dealership.
- **Maintainability:** Booking rules should be separate from HTTP and database code.
- **Observability:** Failed bookings and resource conflicts should be diagnosable from structured logs and metrics.

### Capacity estimation

Capacity estimation does not materially change the MVP architecture. A dealership may have tens of technicians and bays and hundreds of appointments per day. A stateless API backed by a single relational database is sufficient for the assessment. The API can be scaled horizontally later.

## 2. Core entities

- **Customer:** Person who owns or brings in the vehicle.
- **Vehicle:** Vehicle receiving the service.
- **Dealership:** Location where the appointment takes place.
- **ServiceType:** Service requested, including its duration and required skill.
- **Technician:** Employee who performs the service.
- **ServiceBay:** Physical workshop area where the vehicle is serviced.
- **Appointment:** Confirmed reservation of a technician and service bay for a time interval.

A service bay is a constrained physical workspace, such as an oil-change bay, diagnostic bay, or vehicle-lift bay.

## 3. API design

REST is used because the system exposes resource-oriented operations and does not require real-time communication.

### Create an appointment

```http
POST /api/v1/appointments
Idempotency-Key: 8fb93d6e-7802-4e79-a99d-b6eac86f8548
```

Request:

```json
{
  "customerId": "customer-1",
  "vehicleId": "vehicle-1",
  "dealershipId": "dealer-1",
  "serviceTypeId": "oil-service",
  "requestedStart": "2026-08-12T09:00:00+07:00"
}
```

The client provides the requested start time and a unique idempotency key for the logical booking attempt. The server calculates the end time from the service type duration. Retrying the same payload with the same key returns the original response without creating another appointment.

Successful response:

```http
HTTP 201 Created
```

```json
{
  "appointmentId": "appointment-123",
  "status": "Confirmed",
  "customerId": "customer-1",
  "vehicleId": "vehicle-1",
  "dealershipId": "dealer-1",
  "technicianId": "technician-2",
  "serviceBayId": "bay-3",
  "serviceTypeId": "oil-service",
  "startTime": "2026-08-12T02:00:00Z",
  "endTime": "2026-08-12T03:30:00Z"
}
```

### Get an appointment

```http
GET /api/v1/appointments/{appointmentId}
```

### List dealership appointments

```http
GET /api/v1/appointments?dealershipId=dealer-1&from=2026-08-12&to=2026-08-13
```

### Check availability

```http
GET /api/v1/availability?dealershipId=dealer-1&serviceTypeId=oil-service&start=2026-08-12T09:00:00%2B07:00
```

Example response:

```json
{
  "available": true,
  "durationMinutes": 90,
  "technicians": [
    {
      "id": "technician-2",
      "name": "Alex Smith"
    }
  ],
  "serviceBays": [
    {
      "id": "bay-3",
      "name": "Bay 3"
    }
  ]
}
```

### Error responses

| Status | Meaning |
|---|---|
| `400 Bad Request` | Invalid input |
| `404 Not Found` | Unknown customer, vehicle, dealership, or service |
| `409 Conflict` | No resource combination is available, or an idempotency key is reused with a different payload |
| `500 Internal Server Error` | Unexpected failure |

## 4. Data model

PostgreSQL is the production database choice. SQLite can be used locally to reduce setup time.

```text
customers
- id PK
- name
- email

vehicles
- id PK
- customer_id FK
- vin
- make
- model

dealerships
- id PK
- name
- timezone

service_types
- id PK
- name
- duration_minutes
- required_skill

technicians
- id PK
- dealership_id FK
- name

technician_skills
- technician_id FK
- skill
- PRIMARY KEY (technician_id, skill)

service_bays
- id PK
- dealership_id FK
- name

service_bay_capabilities
- service_bay_id FK
- service_type_id FK
- PRIMARY KEY (service_bay_id, service_type_id)

appointments
- id PK
- customer_id FK
- vehicle_id FK
- dealership_id FK
- technician_id FK
- service_bay_id FK
- service_type_id FK
- start_time_utc
- end_time_utc
- status
- created_at_utc

idempotency_requests
- key PK
- request_hash
- status
- appointment_id FK
- response_status
- response_body
- created_at_utc
```

### Important indexes

```text
appointments(dealership_id, start_time_utc, end_time_utc)
appointments(technician_id, start_time_utc)
appointments(service_bay_id, start_time_utc)
vehicles(customer_id)
```

All timestamps are stored in UTC. The dealership timezone is used at the API boundary to convert local input into UTC.

## 5. Data flow

1. The client submits a booking request with an `Idempotency-Key` header.
2. The API validates the request and calculates a hash of its payload.
3. If the key already exists with the same payload hash, the system returns the previously stored response.
4. If the key exists with a different payload hash, the system returns `409 Conflict`.
5. For a new key, the system loads the vehicle, dealership, and service type.
6. The system calculates the end time:

   ```text
   endTime = startTime + serviceType.duration
   ```

7. The system finds technicians who belong to the dealership, have the required skill, and have no overlapping appointment.
8. The system finds service bays that belong to the dealership, support the service type, and have no overlapping appointment.
9. If no valid technician/bay combination exists, the system returns `409 Conflict`.
10. Otherwise, the system creates the appointment and completes the idempotency record inside the same database transaction.
11. The transaction is committed.
12. The confirmed appointment is returned to the client.

## 6. High-level design

```mermaid
flowchart LR
    Client["Swagger UI or cURL"]
    API["Appointment REST API"]
    Booking["Booking Service"]
    Availability["Availability Service"]
    Repositories["Repositories"]
    Database[("PostgreSQL or SQLite")]
    Observability["Logs and metrics"]

    Client --> API
    API --> Booking
    Booking --> Availability
    Booking --> Repositories
    Availability --> Repositories
    Repositories --> Database
    API --> Observability
    Booking --> Observability
    Availability --> Observability
```

### Appointment API

Responsible for HTTP routing, request validation, response formatting, status codes, and Swagger documentation. It should not contain the booking algorithm.

### Booking Service

Responsible for orchestrating the booking process, calculating the appointment duration, starting the transaction, and persisting the appointment.

### Availability Service

Responsible for technician qualification, service-bay compatibility, time-overlap checks, and returning valid resource combinations.

### Repositories

Responsible for database access. Suggested repositories are:

- `AppointmentRepository`
- `TechnicianRepository`
- `ServiceBayRepository`
- `ServiceTypeRepository`

### Relational database

Responsible for durable storage, relationships, indexes, and transactional consistency.

## 7. Availability algorithm

Two appointments overlap when:

```text
requestedStart < existingEnd
AND requestedEnd > existingStart
```

Example:

```text
Existing:  09:00 -------- 10:30
Requested:          10:00 -------- 11:30
```

These appointments overlap.

The algorithm is:

```text
1. Find qualified technicians at the dealership.
2. Find compatible service bays at the dealership.
3. Remove technicians with overlapping appointments.
4. Remove service bays with overlapping appointments.
5. Return the first valid technician/bay pair.
6. If no pair exists, return no availability.
```

The client does not choose the technician or service bay in the MVP. The scheduler assigns them so that qualification and dealership rules cannot be bypassed.

## 8. Deep dive: preventing double booking

A naive implementation can fail when two requests check the same resource at the same time:

```text
Request A checks Bay 1: available
Request B checks Bay 1: available
Request A creates appointment
Request B creates appointment
```

Both requests could incorrectly succeed.

### Transactional solution

Availability checking and appointment creation must happen inside one database transaction:

```text
BEGIN TRANSACTION

Lock or reserve candidate technician and service bay

Check overlapping appointments again

If a conflict exists:
    ROLLBACK
    return 409 Conflict

Insert appointment

COMMIT
```

Use a strong transaction isolation level or PostgreSQL row/advisory locks. A production PostgreSQL implementation could also use exclusion constraints on time ranges to guarantee that a technician or service bay cannot have overlapping appointments.

### Preventing duplicate submissions with an idempotency key

A client may send the same booking request more than once because a user double-clicks the submit button or retries after a network timeout. The create endpoint accepts a client-generated idempotency key:

```http
POST /api/v1/appointments
Idempotency-Key: 8fb93d6e-7802-4e79-a99d-b6eac86f8548
```

The server stores each key with a hash of the request payload and the resulting appointment:

```text
idempotency_requests
- key PK
- request_hash
- status              // InProgress or Completed
- appointment_id FK
- response_status
- response_body
- created_at_utc
```

The booking flow is:

1. Calculate a stable hash of the request payload and begin a database transaction.
2. Attempt to insert the idempotency key. A unique constraint ensures that only one request can claim it.
3. If the key is new, execute the availability check and create the appointment.
4. Store the appointment ID and successful response against the key, then commit both records atomically.
5. If the same key and payload are received again, return the original status code and response without creating another appointment.
6. If the same key is reused with a different payload, return `409 Conflict` because the key cannot represent two operations.
7. A concurrent request using the same key waits for the first transaction to finish and then replays its response. A lock timeout may instead produce a retryable response.

If booking fails before the transaction commits, both the appointment and idempotency record are rolled back so the client can safely retry.

Idempotency keys prevent duplicate submissions of the same logical request. They do not prevent two different requests with different keys from reserving the same technician or service bay. Database transactions, locks, and overlap constraints are still required to prevent resource double booking.

Completed idempotency records can be retained for 24 hours and then removed by a cleanup job.

## 9. Deep dive: scalability

The first version is a stateless API backed by one relational database:

```text
Load Balancer
      |
Multiple API instances
      |
PostgreSQL primary database
```

Future improvements include:

- Read replicas for appointment queries
- Redis caching for service types and resource metadata
- Partitioning appointments by dealership or date
- Separating availability calculation if it becomes expensive
- A message queue for notifications

These improvements are intentionally deferred from the MVP.

## 10. Deep dive: reliability

- Use database transactions for booking.
- Return `409 Conflict` when availability changes during the request.
- Use request IDs for troubleshooting.
- Do not confirm an appointment before the database transaction commits.
- Add health endpoints:

  ```http
  GET /health
  GET /ready
  ```

- Use database migrations so the schema can be recreated consistently.

## 11. Deep dive: observability

### Structured events

```text
AppointmentBookingStarted
AvailabilityCheckCompleted
AppointmentConfirmed
AppointmentBookingRejected
AppointmentBookingFailed
```

### Useful log fields

```text
requestId
dealershipId
serviceTypeId
appointmentId
durationMs
failureReason
```

### Metrics

- Booking success count
- Booking conflict count
- Booking failure count
- Booking latency
- Availability-check latency
- Appointments by dealership

Avoid logging unnecessary customer personal data.


## 12. Testing strategy

Prioritize business-rule tests:

1. Books when both resources are available.
2. Rejects when the technician is already booked.
3. Rejects when the service bay is already booked.
4. Detects partial and full time overlaps.
5. Rejects an unqualified technician.
6. Rejects a resource from another dealership.
7. Calculates the correct end time from service duration.
8. Persists customer, vehicle, technician, and service-bay associations.
9. Handles two simultaneous booking attempts without double booking.
10. Returns meaningful validation errors.
11. Replays the original appointment response when the same idempotency key and payload are submitted again.
12. Rejects reuse of an idempotency key with a different payload.

## 13. MVP success criteria

The MVP is complete when this scenario works:

1. Seed a dealership with two technicians, two service bays, one vehicle, and one service type.
2. Send a booking request.
3. Confirm an appointment with a qualified technician and compatible service bay.
4. Send another overlapping request.
5. Reject it when no technician/bay combination remains.
6. Prove with automated tests that overlapping resources cannot be double-booked.
7. Retry the first request with the same idempotency key and verify that the same appointment is returned without creating a duplicate record.

## Backend development

The backend scaffold uses Go, Gin, PostgreSQL, pgx, Flyway, and sqlc.

### Prerequisites

- Go 1.26 or newer
- PostgreSQL
- sqlc 1.31.1
- Flyway CLI

### Generate database code

sqlc reads the upgrade schema in `db/migrations/V1__create_initial_schema.sql` and the queries in `db/queries`:

```bash
sqlc generate
```

Generated Go code is written to `internal/database/sqlc` and committed so a normal build does not require sqlc.

### Apply database migrations

Set the Flyway connection environment variables, then migrate:

```bash
export FLYWAY_URL='jdbc:postgresql://localhost:5432/unified_service_scheduler'
export FLYWAY_USER='postgres'
export FLYWAY_PASSWORD='postgres'
flyway migrate
```

The upgrade migration is `db/migrations/V1__create_initial_schema.sql`. Its matching downgrade is `db/migrations/U1__create_initial_schema.sql`:

```bash
flyway undo
```

Flyway's `undo` command and `U` migrations require Flyway Teams. With Flyway Community, use forward-only corrective migrations in shared environments; the `U1` script can still be executed manually to reset a disposable local database.

### Run the API

```bash
export DATABASE_URL='postgres://postgres:postgres@localhost:5432/unified_service_scheduler?sslmode=disable'
go run .
```

The initial scaffold exposes `GET /health`. Appointment endpoints are registered and return `501 Not Implemented` until their handlers are implemented.

### Build and test

```bash
go test ./...
go build .
```

GitHub Actions runs sqlc generation verification, formatting, vet, race-enabled tests, and the Linux build on every push and pull request.


