-- 在后台运行一个 30 秒的慢查询，供 sql/sessions/wait 命令验证
-- 调用方式：通过 psql -b 在后台执行，或单独 session
SELECT pg_sleep(30);
