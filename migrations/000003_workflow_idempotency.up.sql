ALTER TABLE tasks
    DROP INDEX uk_tasks_idempotency_key,
    ADD UNIQUE KEY uk_tasks_workflow_idempotency (workflow, idempotency_key);
