ALTER TABLE tasks
    MODIFY idempotency_key VARCHAR(128)
        CHARACTER SET utf8mb4
        COLLATE utf8mb4_0900_ai_ci
        NULL;
