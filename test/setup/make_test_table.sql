-- 创建测试用表（幂等）
CREATE TABLE IF NOT EXISTS kbdiag_test (
  id SERIAL PRIMARY KEY,
  data TEXT,
  created_at TIMESTAMP DEFAULT NOW()
);
-- 插入少量数据确保表存在统计信息
INSERT INTO kbdiag_test (data)
SELECT 'test_row_' || generate_series(1, 100)
ON CONFLICT DO NOTHING;
ANALYZE kbdiag_test;
