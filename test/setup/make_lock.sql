-- 持有一个表锁，不提交，供 locks 命令验证
-- 调用方式：在独立 session 中 BEGIN 后执行，保持 session 不关闭
BEGIN;
CREATE TABLE IF NOT EXISTS kbdiag_test (
  id SERIAL PRIMARY KEY, data TEXT, created_at TIMESTAMP DEFAULT NOW()
);
LOCK TABLE kbdiag_test IN EXCLUSIVE MODE;
-- 调用方负责在测试结束后 ROLLBACK 或断开 session
