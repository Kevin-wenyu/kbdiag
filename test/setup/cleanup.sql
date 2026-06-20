-- 清理所有测试状态
DROP TABLE IF EXISTS kbdiag_test;
-- 终止所有 pg_sleep 后台进程
SELECT pg_terminate_backend(pid)
FROM sys_stat_activity
WHERE query LIKE '%pg_sleep%'
  AND pid <> sys_backend_pid();
