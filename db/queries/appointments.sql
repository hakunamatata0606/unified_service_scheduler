-- name: CreateAppointment :one
INSERT INTO appointments (
    id,
    customer_id,
    vehicle_id,
    dealership_id,
    technician_id,
    service_bay_id,
    service_type_id,
    start_time_utc,
    end_time_utc,
    status
) VALUES (
    sqlc.arg(id),
    sqlc.arg(customer_id),
    sqlc.arg(vehicle_id),
    sqlc.arg(dealership_id),
    sqlc.arg(technician_id),
    sqlc.arg(service_bay_id),
    sqlc.arg(service_type_id),
    sqlc.arg(start_time_utc),
    sqlc.arg(end_time_utc),
    'confirmed'
)
RETURNING *;

-- name: GetAppointment :one
SELECT *
FROM appointments
WHERE id = sqlc.arg(id)
LIMIT 1;

-- name: ListAppointmentsByDealership :many
SELECT *
FROM appointments
WHERE dealership_id = sqlc.arg(dealership_id)
  AND start_time_utc < sqlc.arg(range_end)
  AND end_time_utc > sqlc.arg(range_start)
ORDER BY start_time_utc, id;

-- name: FindAvailableTechnicians :many
SELECT t.*
FROM technicians AS t
JOIN service_types AS st
  ON st.id = sqlc.arg(service_type_id)
JOIN technician_skills AS ts
  ON ts.technician_id = t.id
 AND ts.skill = st.required_skill
WHERE t.dealership_id = sqlc.arg(dealership_id)
  AND NOT EXISTS (
      SELECT 1
      FROM appointments AS a
      WHERE a.technician_id = t.id
        AND a.status = 'confirmed'
        AND a.start_time_utc < sqlc.arg(requested_end)
        AND a.end_time_utc > sqlc.arg(requested_start)
  )
ORDER BY t.id;

-- name: FindAvailableServiceBays :many
SELECT sb.*
FROM service_bays AS sb
JOIN service_bay_capabilities AS sbc
  ON sbc.service_bay_id = sb.id
 AND sbc.service_type_id = sqlc.arg(service_type_id)
WHERE sb.dealership_id = sqlc.arg(dealership_id)
  AND NOT EXISTS (
      SELECT 1
      FROM appointments AS a
      WHERE a.service_bay_id = sb.id
        AND a.status = 'confirmed'
        AND a.start_time_utc < sqlc.arg(requested_end)
        AND a.end_time_utc > sqlc.arg(requested_start)
  )
ORDER BY sb.id;

