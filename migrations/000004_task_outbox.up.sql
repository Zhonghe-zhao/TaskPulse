CREATE TABLE task_outbox (
    id            VARCHAR(128) NOT NULL,
    task_id       VARCHAR(64)  NOT NULL,
    workflow      VARCHAR(64)  NOT NULL,
    event_type    VARCHAR(64)  NOT NULL,
    payload_json  JSON         NOT NULL,
    status        VARCHAR(32)  NOT NULL DEFAULT 'pending',
    attempts      INT UNSIGNED NOT NULL DEFAULT 0,
    available_at  DATETIME(6)  NOT NULL,
    published_at  DATETIME(6)  NULL,
    last_error    TEXT         NULL,
    created_at    DATETIME(6)  NOT NULL,
    updated_at    DATETIME(6)  NOT NULL,

    PRIMARY KEY (id),
    KEY idx_task_outbox_pending (status, available_at, created_at, id),
    KEY idx_task_outbox_task (task_id),
    CONSTRAINT fk_task_outbox_task
        FOREIGN KEY (task_id) REFERENCES tasks(id)
        ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
