-- name: GetProcessInstanceRow :one
SELECT key, tenant_id, process_definition_key, bpmn_process_id, process_definition_version, state,
       has_incident, version, start_time, end_time
FROM process_instance
WHERE key = $1;

-- name: ListElementInstances :many
SELECT key, process_instance_key, flow_scope_key, element_id, bpmn_element_type, state, incident_key,
       start_time, end_time
FROM element_instance
WHERE process_instance_key = $1
ORDER BY key;

-- name: ListVariables :many
SELECT key, scope_key, name, value, updated_at
FROM variable
WHERE process_instance_key = $1
ORDER BY scope_key, name;

-- name: GetJobRef :one
SELECT j.process_instance_key, pi.process_definition_key
FROM job j
JOIN process_instance pi ON pi.key = j.process_instance_key
WHERE j.key = $1;

-- name: ListJobsByInstance :many
SELECT key, process_instance_key, element_instance_key, element_id, bpmn_process_id,
       process_definition_key, type, retries, custom_headers, state, created_at
FROM job
WHERE process_instance_key = $1
ORDER BY key;

-- name: ListJobs :many
-- Keyset pagination: terbaru dahulu (UUIDv7 terurut waktu). Job "open" = belum final.
SELECT key, process_instance_key, element_instance_key, element_id, bpmn_process_id,
       process_definition_key, type, retries, custom_headers, state, created_at,
       worker, deadline, activatable_at, error_message, error_code, activation_count
FROM job
WHERE (sqlc.arg(job_type)::text = '' OR type = sqlc.arg(job_type)::text)
  AND (NULLIF(sqlc.arg(process_instance_key)::text, '')::uuid IS NULL
       OR process_instance_key = NULLIF(sqlc.arg(process_instance_key)::text, '')::uuid)
  AND (NOT sqlc.arg(only_open)::bool OR state IN ('CREATED', 'ACTIVATED', 'FAILED', 'ERROR_THROWN'))
  AND (sqlc.arg(job_state)::text = '' OR state = sqlc.arg(job_state)::text)
  AND (NULLIF(sqlc.arg(after_key)::text, '')::uuid IS NULL
       OR key < NULLIF(sqlc.arg(after_key)::text, '')::uuid)
ORDER BY key DESC
LIMIT sqlc.arg(page_limit);

-- name: ListIncidents :many
SELECT key, element_instance_key, element_id, error_type, error_message, state, created_at
FROM incident
WHERE process_instance_key = $1
ORDER BY key;

-- name: GetIncidentRef :one
SELECT i.process_instance_key, pi.process_definition_key, i.state
FROM incident i
JOIN process_instance pi ON pi.key = i.process_instance_key
WHERE i.key = $1;

-- name: ListIncidentsPage :many
-- Keyset pagination: terbaru dahulu (UUIDv7 terurut waktu).
SELECT i.key, i.process_instance_key, i.element_instance_key, i.element_id, pi.bpmn_process_id,
       pi.process_definition_key, i.error_type, i.error_message, i.job_key, i.state, i.created_at,
       i.resolved_at
FROM incident i
JOIN process_instance pi ON pi.key = i.process_instance_key
WHERE (NULLIF(sqlc.arg(process_instance_key)::text, '')::uuid IS NULL
       OR i.process_instance_key = NULLIF(sqlc.arg(process_instance_key)::text, '')::uuid)
  AND (sqlc.arg(error_type)::text = '' OR i.error_type = sqlc.arg(error_type)::text)
  AND (NOT sqlc.arg(only_open)::bool OR i.state = 'CREATED')
  AND (NULLIF(sqlc.arg(after_key)::text, '')::uuid IS NULL
       OR i.key < NULLIF(sqlc.arg(after_key)::text, '')::uuid)
ORDER BY i.key DESC
LIMIT sqlc.arg(page_limit);

-- name: ListRecords :many
SELECT process_instance_key, sequence, record_key, value_type, intent, element_id, payload, created_at
FROM execution_record
WHERE process_instance_key = $1
ORDER BY sequence;

-- name: ListProcessInstances :many
-- Keyset pagination: terbaru dahulu (UUIDv7 terurut waktu).
SELECT key, tenant_id, process_definition_key, bpmn_process_id, process_definition_version, state,
       has_incident, version, start_time, end_time
FROM process_instance
WHERE tenant_id = sqlc.arg(tenant_id)
  AND (sqlc.arg(bpmn_process_id)::text = '' OR bpmn_process_id = sqlc.arg(bpmn_process_id)::text)
  AND (NULLIF(sqlc.arg(process_definition_key)::text, '')::uuid IS NULL
       OR process_definition_key = NULLIF(sqlc.arg(process_definition_key)::text, '')::uuid)
  AND (sqlc.arg(state)::text = '' OR state = sqlc.arg(state)::text)
  AND (NOT sqlc.arg(only_with_incident)::bool OR has_incident)
  AND (NULLIF(sqlc.arg(after_key)::text, '')::uuid IS NULL
       OR key < NULLIF(sqlc.arg(after_key)::text, '')::uuid)
ORDER BY key DESC
LIMIT sqlc.arg(page_limit);
