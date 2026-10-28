-- name: LockProcessInstance :one
-- Single writer per instance: dikunci di awal setiap command (ADR-0012).
SELECT key FROM process_instance WHERE key = $1 FOR UPDATE;

-- name: GetProcessInstanceRow :one
SELECT key, tenant_id, process_definition_key, bpmn_process_id, process_definition_version, state,
       has_incident, version, start_time, end_time
FROM process_instance
WHERE key = $1;

-- name: InsertProcessInstance :exec
INSERT INTO process_instance
    (key, process_definition_key, bpmn_process_id, process_definition_version, state, start_time)
VALUES ($1, $2, $3, $4, 'ACTIVE', $5);

-- name: FinishProcessInstance :exec
UPDATE process_instance SET state = $2, end_time = $3 WHERE key = $1;

-- name: TouchProcessInstance :exec
-- Dipanggil sekali per command setelah proyeksi: menaikkan version dan menghitung has_incident.
UPDATE process_instance
SET version = version + 1,
    has_incident = EXISTS (SELECT 1 FROM incident i WHERE i.process_instance_key = $1 AND i.state = 'CREATED')
WHERE key = $1;

-- name: InsertElementInstance :exec
INSERT INTO element_instance
    (key, process_instance_key, flow_scope_key, element_id, bpmn_element_type, state, start_time)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: UpdateElementInstanceState :exec
UPDATE element_instance SET state = $2, end_time = $3 WHERE key = $1;

-- name: SetElementInstanceIncident :exec
UPDATE element_instance SET incident_key = $2 WHERE key = $1;

-- name: ListRuntimeElementInstances :many
SELECT key, process_instance_key, flow_scope_key, element_id, bpmn_element_type, state, incident_key
FROM element_instance
WHERE process_instance_key = $1 AND state NOT IN ('COMPLETED', 'TERMINATED')
ORDER BY key;

-- name: ListElementInstances :many
SELECT key, process_instance_key, flow_scope_key, element_id, bpmn_element_type, state, incident_key,
       start_time, end_time
FROM element_instance
WHERE process_instance_key = $1
ORDER BY key;

-- name: ListTakenFlows :many
SELECT flow_scope_key, gateway_id, sequence_flow_id, count
FROM taken_sequence_flow
WHERE process_instance_key = $1
ORDER BY flow_scope_key, gateway_id, sequence_flow_id;

-- name: DeleteTakenFlows :exec
DELETE FROM taken_sequence_flow WHERE process_instance_key = $1;

-- name: InsertTakenFlow :exec
INSERT INTO taken_sequence_flow (process_instance_key, flow_scope_key, gateway_id, sequence_flow_id, count)
VALUES ($1, $2, $3, $4, $5);

-- name: ListVariables :many
SELECT key, scope_key, name, value, updated_at
FROM variable
WHERE process_instance_key = $1
ORDER BY scope_key, name;

-- name: InsertVariable :exec
INSERT INTO variable (key, process_instance_key, scope_key, name, value, updated_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: UpdateVariable :exec
UPDATE variable SET value = $2, updated_at = $3 WHERE key = $1;

-- name: InsertJob :exec
INSERT INTO job
    (key, process_instance_key, element_instance_key, element_id, bpmn_process_id,
     process_definition_key, process_definition_version, type, retries, custom_headers, state,
     created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'CREATED', $11, $11);

-- name: UpdateJobState :exec
UPDATE job SET state = $2, updated_at = $3 WHERE key = $1;

-- name: GetJobInstanceKey :one
SELECT process_instance_key FROM job WHERE key = $1;

-- name: ListOpenJobs :many
SELECT key, element_instance_key, element_id, type, retries, custom_headers, state
FROM job
WHERE process_instance_key = $1 AND state = 'CREATED'
ORDER BY key;

-- name: ListJobsByInstance :many
SELECT key, process_instance_key, element_instance_key, element_id, bpmn_process_id,
       process_definition_key, type, retries, custom_headers, state, created_at
FROM job
WHERE process_instance_key = $1
ORDER BY key;

-- name: ListJobs :many
-- Keyset pagination: terbaru dahulu (UUIDv7 terurut waktu).
SELECT key, process_instance_key, element_instance_key, element_id, bpmn_process_id,
       process_definition_key, type, retries, custom_headers, state, created_at
FROM job
WHERE (sqlc.arg(job_type)::text = '' OR type = sqlc.arg(job_type)::text)
  AND (NULLIF(sqlc.arg(process_instance_key)::text, '')::uuid IS NULL
       OR process_instance_key = NULLIF(sqlc.arg(process_instance_key)::text, '')::uuid)
  AND (NOT sqlc.arg(only_open)::bool OR state = 'CREATED')
  AND (NULLIF(sqlc.arg(after_key)::text, '')::uuid IS NULL
       OR key < NULLIF(sqlc.arg(after_key)::text, '')::uuid)
ORDER BY key DESC
LIMIT sqlc.arg(page_limit);

-- name: InsertIncident :exec
INSERT INTO incident
    (key, process_instance_key, element_instance_key, element_id, error_type, error_message, state, created_at)
VALUES ($1, $2, $3, $4, $5, $6, 'CREATED', $7);

-- name: ResolveIncident :exec
UPDATE incident SET state = 'RESOLVED', resolved_at = $2 WHERE key = $1;

-- name: ListOpenIncidents :many
SELECT key, element_instance_key, element_id, error_type, error_message
FROM incident
WHERE process_instance_key = $1 AND state = 'CREATED'
ORDER BY key;

-- name: ListIncidents :many
SELECT key, element_instance_key, element_id, error_type, error_message, state, created_at
FROM incident
WHERE process_instance_key = $1
ORDER BY key;

-- name: MaxRecordSequence :one
SELECT COALESCE(MAX(sequence), 0)::int AS max_sequence FROM execution_record WHERE process_instance_key = $1;

-- name: InsertRecords :copyfrom
INSERT INTO execution_record
    (process_instance_key, sequence, record_key, value_type, intent, element_id, payload, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

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
