-- name: CreateUser :one
INSERT INTO users (username, password_hash) VALUES (?, ?) RETURNING *;

-- name: GetUserByUsername :one
SELECT * FROM users WHERE username = ? LIMIT 1;

-- name: ListTasks :many
SELECT * FROM tasks WHERE is_deleted = 0 ORDER BY created_at DESC;

-- name: GetTask :one
SELECT * FROM tasks WHERE id = ? LIMIT 1;

-- name: CreateTask :one
INSERT INTO tasks (name, target_url, is_enabled, filename_template, custom_css, fps, crf) VALUES (?, ?, 0, ?, ?, ?, ?) RETURNING *;

-- name: DeleteTask :exec
UPDATE tasks SET is_deleted = 1, is_enabled = 0 WHERE id = ?;

-- name: EnableTask :exec
UPDATE tasks SET is_enabled = 1 WHERE id = ?;

-- name: DisableTask :exec
UPDATE tasks SET is_enabled = 0 WHERE id = ?;

-- name: ListEnabledTasks :many
SELECT * FROM tasks WHERE is_enabled = 1;

-- name: CreateRecording :one
INSERT INTO recordings (task_id, status, file_path, start_time) 
VALUES (?, ?, ?, CURRENT_TIMESTAMP) RETURNING *;

-- name: UpdateRecordingStatus :exec
UPDATE recordings SET status = ?, end_time = CURRENT_TIMESTAMP WHERE id = ?;

-- name: ListRecordings :many
SELECT r.*, t.name as task_name 
FROM recordings r 
JOIN tasks t ON r.task_id = t.id 
ORDER BY r.start_time DESC;




-- name: GetRecording :one
SELECT * FROM recordings WHERE id = ? LIMIT 1;

-- name: DeleteRecording :exec
DELETE FROM recordings WHERE id = ?;

-- name: UpdateUserPassword :exec
UPDATE users SET password_hash = ? WHERE username = ?;

-- name: UpdateTask :exec
UPDATE tasks 
SET name = ?, target_url = ?, filename_template = ?, custom_css = ?, fps = ?, crf = ?
WHERE id = ?;

-- name: CountUsers :one
SELECT COUNT(*) FROM users;
