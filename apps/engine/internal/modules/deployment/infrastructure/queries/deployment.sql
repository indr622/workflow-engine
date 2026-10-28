-- name: AdvisoryLockProcessID :exec
-- Kunci per bpmnProcessId selama transaksi agar penomoran versi serial.
SELECT pg_advisory_xact_lock(hashtext(sqlc.arg(tenant_id)::text || ':' || sqlc.arg(bpmn_process_id)::text));

-- name: InsertDeployment :exec
INSERT INTO deployment (key, tenant_id) VALUES ($1, $2);

-- name: InsertResource :exec
INSERT INTO deployment_resource (id, deployment_key, resource_name, content, checksum)
VALUES ($1, $2, $3, $4, $5);

-- name: InsertProcessDefinition :exec
INSERT INTO process_definition
    (key, tenant_id, bpmn_process_id, version, version_tag, name, resource_id, checksum, deployment_key)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: GetLatestProcessDefinition :one
SELECT pd.key, pd.tenant_id, pd.bpmn_process_id, pd.version, pd.version_tag, pd.name,
       pd.checksum, pd.deployment_key, pd.created_at, r.resource_name
FROM process_definition pd
JOIN deployment_resource r ON r.id = pd.resource_id
WHERE pd.tenant_id = $1 AND pd.bpmn_process_id = $2
ORDER BY pd.version DESC
LIMIT 1;

-- name: GetProcessDefinitionByKey :one
SELECT pd.key, pd.tenant_id, pd.bpmn_process_id, pd.version, pd.version_tag, pd.name,
       pd.checksum, pd.deployment_key, pd.created_at, r.resource_name, r.content
FROM process_definition pd
JOIN deployment_resource r ON r.id = pd.resource_id
WHERE pd.key = $1;

-- name: GetLatestProcessDefinitionWithContent :one
SELECT pd.key, pd.tenant_id, pd.bpmn_process_id, pd.version, pd.version_tag, pd.name,
       pd.checksum, pd.deployment_key, pd.created_at, r.resource_name, r.content
FROM process_definition pd
JOIN deployment_resource r ON r.id = pd.resource_id
WHERE pd.tenant_id = $1 AND pd.bpmn_process_id = $2
ORDER BY pd.version DESC
LIMIT 1;

-- name: ListProcessDefinitions :many
-- Keyset pagination: urut bpmn_process_id naik, version turun.
SELECT pd.key, pd.tenant_id, pd.bpmn_process_id, pd.version, pd.version_tag, pd.name,
       pd.checksum, pd.deployment_key, pd.created_at, r.resource_name
FROM process_definition pd
JOIN deployment_resource r ON r.id = pd.resource_id
WHERE pd.tenant_id = sqlc.arg(tenant_id)
  AND (sqlc.arg(bpmn_process_id)::text = '' OR pd.bpmn_process_id = sqlc.arg(bpmn_process_id)::text)
  AND (sqlc.arg(after_process_id)::text = ''
       OR pd.bpmn_process_id > sqlc.arg(after_process_id)::text
       OR (pd.bpmn_process_id = sqlc.arg(after_process_id)::text AND pd.version < sqlc.arg(after_version)::int))
ORDER BY pd.bpmn_process_id ASC, pd.version DESC
LIMIT sqlc.arg(page_limit);

-- name: ListLatestProcessDefinitions :many
SELECT * FROM (
    SELECT DISTINCT ON (pd.bpmn_process_id)
           pd.key, pd.tenant_id, pd.bpmn_process_id, pd.version, pd.version_tag, pd.name,
           pd.checksum, pd.deployment_key, pd.created_at, r.resource_name
    FROM process_definition pd
    JOIN deployment_resource r ON r.id = pd.resource_id
    WHERE pd.tenant_id = sqlc.arg(tenant_id)
      AND (sqlc.arg(bpmn_process_id)::text = '' OR pd.bpmn_process_id = sqlc.arg(bpmn_process_id)::text)
      AND pd.bpmn_process_id > sqlc.arg(after_process_id)::text
    ORDER BY pd.bpmn_process_id ASC, pd.version DESC
) latest
ORDER BY latest.bpmn_process_id ASC
LIMIT sqlc.arg(page_limit);
