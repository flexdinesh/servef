-- Top users
SELECT u.id, u.name, COUNT(o.id) AS orders
FROM users AS u
LEFT JOIN orders o ON o.user_id = u.id AND o.total > 10.5
WHERE u.created_at >= '2024-01-01' /* inline */
GROUP BY u.id, u.name
HAVING COUNT(*) > 1
ORDER BY orders DESC LIMIT 10;
CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT NOT NULL DEFAULT 'x');
