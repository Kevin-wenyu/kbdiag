-- 先确保测试表存在
CREATE TABLE IF NOT EXISTS kbdiag_test (
  id SERIAL PRIMARY KEY, data TEXT, created_at TIMESTAMP DEFAULT NOW()
);
INSERT INTO kbdiag_test (data)
SELECT 'bloat_row_' || generate_series(1, 10000);
-- 删除大量行制造 dead tuples（bloat）
DELETE FROM kbdiag_test WHERE id % 2 = 0;
-- 注意：不 VACUUM，让 dead tuples 留着
