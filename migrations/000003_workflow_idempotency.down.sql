ALTER TABLE tasks
    DROP INDEX uk_tasks_workflow_idempotency,
    ADD UNIQUE KEY uk_tasks_idempotency_key (idempotency_key);
