ALTER TABLE tasks
    MODIFY idempotency_key VARBINARY(128) NULL;
