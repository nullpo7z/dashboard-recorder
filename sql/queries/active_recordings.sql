-- name: ListActiveRecordings :many
SELECT 
    r.id,
    r.task_id,
    r.status,
    r.start_time,
    r.file_path,
    t.name as task_name
FROM recordings r
JOIN tasks t ON r.task_id = t.id
WHERE r.status = 'RECORDING'
ORDER BY r.start_time DESC;
