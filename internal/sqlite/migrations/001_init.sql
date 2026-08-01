CREATE TABLE progress (
    module_slug  TEXT NOT NULL,
    step_slug    TEXT NOT NULL,
    completed_at TEXT NOT NULL,
    PRIMARY KEY (module_slug, step_slug)
);

CREATE TABLE notes (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    module_slug TEXT NOT NULL,
    step_slug   TEXT NOT NULL,
    body_md     TEXT NOT NULL,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE TABLE submissions (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    module_slug TEXT NOT NULL,
    step_slug   TEXT NOT NULL,
    kind        TEXT NOT NULL CHECK (kind IN ('lab', 'question')),
    content     TEXT NOT NULL,
    test_output TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL CHECK (status IN ('pending', 'running', 'complete', 'failed')),
    created_at  TEXT NOT NULL
);

CREATE TABLE evaluations (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    submission_id  INTEGER NOT NULL REFERENCES submissions (id),
    model          TEXT NOT NULL,
    rubric_version TEXT NOT NULL,
    verdict_json   TEXT NOT NULL,
    created_at     TEXT NOT NULL
);
