CREATE TABLE drafts (
    module_slug TEXT NOT NULL,
    step_slug   TEXT NOT NULL,
    files_json  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    PRIMARY KEY (module_slug, step_slug)
);

CREATE TABLE submissions_new (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    module_slug TEXT NOT NULL,
    step_slug   TEXT NOT NULL,
    kind        TEXT NOT NULL CHECK (kind IN ('lab', 'question', 'exercise')),
    content     TEXT NOT NULL,
    test_output TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL CHECK (status IN ('pending', 'running', 'complete', 'failed')),
    passed      INTEGER,
    created_at  TEXT NOT NULL
);
INSERT INTO submissions_new (id, module_slug, step_slug, kind, content, test_output, status, created_at)
    SELECT id, module_slug, step_slug, kind, content, test_output, status, created_at FROM submissions;
DROP TABLE submissions;
ALTER TABLE submissions_new RENAME TO submissions;
