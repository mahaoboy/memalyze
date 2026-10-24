package sqlite

// 迁移按版本递增，每个版本一个语句块；已存在的库从 user_version 继续往上跑。
//
// 约定：**只追加，不改历史版本**。改了 v1 的语句会让老库和新库分叉。
var migrations = []struct {
	version int
	stmts   []string
}{
	{1, schemaV1},
	{2, schemaV2},
}

// latestVersion 是最新 schema 版本，必须等于 migrations 里最大的 version。
func latestVersion() int {
	return migrations[len(migrations)-1].version
}

// schemaV1 是 v1 的完整 DDL，按顺序执行（迁移在一个事务里跑完）。
//
// 两个关键点（都来自实测，见 04-技术路线/数据模型 与 检索验证-S8）：
//  1. memories 额外存一列 fts_text（CJK 二元组文本），FTS5 用 external content
//     指向这一列 —— 这样触发器可以直接引用 new.fts_text/old.fts_text，
//     不需要在 SQL 里做中文分词；
//  2. 去重是「部分唯一索引」：只约束 active 状态，软删除后同内容可以重新写入。
var schemaV1 = []string{
	`CREATE TABLE IF NOT EXISTS memories (
		id             TEXT PRIMARY KEY,
		tenant_id      TEXT NOT NULL DEFAULT 'default',
		scope          TEXT NOT NULL DEFAULT 'default',
		content        TEXT NOT NULL,
		fts_text       TEXT NOT NULL,
		content_hash   TEXT NOT NULL,
		kind           TEXT NOT NULL DEFAULT 'note',
		source         TEXT,
		metadata       TEXT,
		status         TEXT NOT NULL DEFAULT 'active',
		created_at     INTEGER NOT NULL,
		updated_at     INTEGER NOT NULL,
		expires_at     INTEGER,
		last_access_at INTEGER,
		access_count   INTEGER NOT NULL DEFAULT 0
	)`,

	`CREATE UNIQUE INDEX IF NOT EXISTS ux_memories_dedup
		ON memories(tenant_id, scope, content_hash) WHERE status = 'active'`,
	`CREATE INDEX IF NOT EXISTS ix_memories_scope
		ON memories(tenant_id, scope, status)`,
	`CREATE INDEX IF NOT EXISTS ix_memories_expires
		ON memories(expires_at) WHERE expires_at IS NOT NULL`,

	`CREATE TABLE IF NOT EXISTS embeddings (
		memory_id  TEXT NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
		model      TEXT NOT NULL,
		dim        INTEGER NOT NULL,
		vector     BLOB NOT NULL,
		created_at INTEGER NOT NULL,
		PRIMARY KEY (memory_id, model)
	)`,

	// 外部内容表的列名必须与 memories 中的列名一致（fts_text）
	`CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts
		USING fts5(fts_text, content='memories', content_rowid='rowid')`,

	`CREATE TRIGGER IF NOT EXISTS memories_ai AFTER INSERT ON memories BEGIN
		INSERT INTO memories_fts(rowid, fts_text) VALUES (new.rowid, new.fts_text);
	END`,
	`CREATE TRIGGER IF NOT EXISTS memories_ad AFTER DELETE ON memories BEGIN
		INSERT INTO memories_fts(memories_fts, rowid, fts_text) VALUES ('delete', old.rowid, old.fts_text);
	END`,
	`CREATE TRIGGER IF NOT EXISTS memories_au AFTER UPDATE OF fts_text ON memories BEGIN
		INSERT INTO memories_fts(memories_fts, rowid, fts_text) VALUES ('delete', old.rowid, old.fts_text);
		INSERT INTO memories_fts(rowid, fts_text) VALUES (new.rowid, new.fts_text);
	END`,
}

// schemaV2 增加审计表（S15）。
//
// 设计要点：
//   - 独立表、不进 FTS、不参与检索：审计是"记录"，不是"记忆"
//   - 索引只建在时间与租户上，够导出用，不做过早优化
//   - UPDATE 触发器让 append-only 成为数据库层约束，而不是口头约定
var schemaV2 = []string{
	`CREATE TABLE IF NOT EXISTS audit_log (
		id           TEXT PRIMARY KEY,
		ts           INTEGER NOT NULL,
		actor        TEXT,
		tenant_id    TEXT NOT NULL,
		scope        TEXT,
		action       TEXT NOT NULL,
		memory_id    TEXT,
		query_hash   TEXT,
		query_len    INTEGER NOT NULL DEFAULT 0,
		result_count INTEGER NOT NULL DEFAULT 0,
		degraded     INTEGER NOT NULL DEFAULT 0,
		duplicate    INTEGER NOT NULL DEFAULT 0,
		redacted     INTEGER NOT NULL DEFAULT 0,
		dry_run      INTEGER NOT NULL DEFAULT 0,
		detail       TEXT
	)`,
	`CREATE INDEX IF NOT EXISTS ix_audit_ts ON audit_log(ts)`,
	`CREATE INDEX IF NOT EXISTS ix_audit_tenant_ts ON audit_log(tenant_id, ts)`,
	`CREATE INDEX IF NOT EXISTS ix_audit_memory ON audit_log(memory_id) WHERE memory_id IS NOT NULL`,
	`CREATE TRIGGER IF NOT EXISTS audit_no_update BEFORE UPDATE ON audit_log BEGIN
		SELECT RAISE(ABORT, 'audit_log is append-only');
	END`,
}
